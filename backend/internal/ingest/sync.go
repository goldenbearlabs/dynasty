package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/sportsday"
)

// ErrBusy is returned when a sync for the same competition is already running.
var ErrBusy = errors.New("sync already running")

// Syncer writes what a Source returns into the database.
type Syncer struct {
	// OnMove, if set, is called when a roster sync finds a player in a
	// different competition than before: a college player reaching the NBA.
	OnMove func(ctx context.Context, playerID pgtype.UUID, from, to string)

	q       *db.Queries
	log     *slog.Logger
	running sync.Map // "competition/job" -> struct{}
}

func NewSyncer(q *db.Queries, log *slog.Logger) *Syncer {
	return &Syncer{q: q, log: log}
}

// SyncRosters refreshes one competition's teams and players. It is safe to
// run again at any time.
func (s *Syncer) SyncRosters(ctx context.Context, competition string, src Source) error {
	return s.run(ctx, competition, "rosters", func(startedAt pgtype.Timestamptz) (int, error) {
		return s.syncRosters(ctx, competition, src, startedAt)
	})
}

// SyncProspects adds and refreshes one competition's prospects. A prospect
// who has since arrived is left alone: the roster sync owns him from then on.
func (s *Syncer) SyncProspects(ctx context.Context, competition string, src ProspectSource) error {
	return s.run(ctx, competition, "prospects", func(pgtype.Timestamptz) (int, error) {
		prospects, err := src.Prospects(ctx)
		if err != nil {
			return 0, err
		}
		for i, p := range prospects {
			if err := s.upsertProspect(ctx, competition, p); err != nil {
				return i, fmt.Errorf("prospect %s: %w", p.FullName, err)
			}
		}
		return len(prospects), nil
	})
}

// SyncGames refreshes one competition's schedule for a span of sports days,
// then fetches box scores for the games that need them: live ones, newly
// finished ones, and each finished game once more a day later for stat
// corrections. It reports the number of stat lines stored.
func (s *Syncer) SyncGames(ctx context.Context, competition string, src GameSource, from, to time.Time) error {
	return s.run(ctx, competition, "games", func(pgtype.Timestamptz) (int, error) {
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			games, err := src.Games(ctx, day)
			if err != nil {
				return 0, fmt.Errorf("games on %s: %w", day.Format(time.DateOnly), err)
			}
			for _, game := range games {
				if err := s.upsertGame(ctx, competition, game); err != nil {
					return 0, err
				}
			}
		}

		pending, err := s.q.ListGamesNeedingStats(ctx, competition)
		if err != nil {
			return 0, err
		}
		lines := 0
		for _, game := range pending {
			n, err := s.syncBoxScore(ctx, src, game)
			lines += n
			if err != nil {
				if ctx.Err() != nil {
					return lines, ctx.Err()
				}
				// One bad box score should not stop the rest; it is retried next run.
				s.log.Warn("box score failed", "competition", competition, "game", game.ProviderID, "err", err)
			}
		}
		return lines, nil
	})
}

func (s *Syncer) upsertGame(ctx context.Context, competition string, game Game) error {
	team := func(providerID string) pgtype.UUID {
		id, _ := s.q.FindProTeam(ctx, db.FindProTeamParams{Competition: competition, ProviderID: providerID})
		return id // left empty for a team we do not know, such as an all-star side
	}
	home, away := team(game.HomeTeam), team(game.AwayTeam)
	if !home.Valid && !away.Valid {
		return nil // a game between teams outside the competition is not ours to follow
	}
	return s.q.UpsertGame(ctx, db.UpsertGameParams{
		Competition: competition,
		ProviderID:  game.ProviderID,
		Day:         sportsday.Date(sportsday.Of(game.StartsAt)),
		StartsAt:    pgtype.Timestamptz{Time: game.StartsAt, Valid: true},
		Status:      game.Status,
		HomeTeamID:  home,
		AwayTeamID:  away,
		HomeScore:   int32(game.HomeScore),
		AwayScore:   int32(game.AwayScore),
		Detail:      game.Detail,
	})
}

// LiveUpdate is what one poll of the games in play changed.
type LiveUpdate struct {
	Scoreboard bool          // a score, a state or a status moved
	Games      []pgtype.UUID // games whose box score was refreshed
}

// PollLive refreshes the games in play for one competition: their scores
// and state from the scoreboard, and a box score for each one that is live
// or has just ended. It makes no requests when nothing is in play, and it
// is not recorded in ingest_runs: it runs every few seconds on a game day.
func (s *Syncer) PollLive(ctx context.Context, competition string, src GameSource) (LiveUpdate, error) {
	if _, busy := s.running.LoadOrStore(competition+"/live", struct{}{}); busy {
		return LiveUpdate{}, nil // the last poll is still going
	}
	defer s.running.Delete(competition + "/live")

	before, err := s.q.ListGamesInPlay(ctx, competition)
	if err != nil || len(before) == 0 {
		return LiveUpdate{}, err
	}

	// One scoreboard request per calendar date in play: almost always one.
	fetched := map[time.Time]bool{}
	for _, game := range before {
		day := sportsday.Calendar(game.StartsAt.Time)
		if fetched[day] {
			continue
		}
		fetched[day] = true
		games, err := src.Games(ctx, day)
		if err != nil {
			return LiveUpdate{}, err
		}
		for _, g := range games {
			if err := s.upsertGame(ctx, competition, g); err != nil {
				return LiveUpdate{}, err
			}
		}
	}

	var update LiveUpdate
	for _, was := range before {
		now, err := s.q.GetGame(ctx, was.ID)
		if err != nil {
			return update, err
		}
		if now.Status != was.Status || now.HomeScore != was.HomeScore || now.AwayScore != was.AwayScore || now.Detail != was.Detail {
			update.Scoreboard = true
		}
		// A live game, or one that ended since the last poll, gets its box score.
		if now.Status == GameLive || (now.Status == GameFinal && was.Status != GameFinal) {
			if _, err := s.syncBoxScore(ctx, src, now); err != nil {
				if ctx.Err() != nil {
					return update, ctx.Err()
				}
				s.log.Warn("live box score failed", "competition", competition, "game", now.ProviderID, "err", err)
				continue
			}
			update.Games = append(update.Games, now.ID)
		}
	}
	return update, nil
}

func (s *Syncer) syncBoxScore(ctx context.Context, src GameSource, game db.Game) (int, error) {
	lines, err := src.BoxScore(ctx, Game{ProviderID: game.ProviderID})
	if err != nil {
		return 0, err
	}
	stored := 0
	for _, line := range lines {
		playerID, err := s.q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: line.Provider, ProviderID: line.ProviderID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // someone the roster feed has never listed
		}
		if err != nil {
			return stored, err
		}
		stats, _ := json.Marshal(line.Stats)
		if err := s.q.UpsertStatLine(ctx, db.UpsertStatLineParams{GameID: game.ID, PlayerID: playerID, Stats: stats}); err != nil {
			return stored, err
		}
		stored++
	}
	return stored, s.q.MarkGameStatsSynced(ctx, game.ID)
}

// SyncSeasons stores every player's season totals for a competition: always
// the latest season and the one before it, which is still being corrected
// early in a new season, and any of the backfill seasons before those that
// are not stored yet. So the first run fetches years of history and later
// runs only keep it current. It reports the number of season lines stored.
func (s *Syncer) SyncSeasons(ctx context.Context, competition string, src SeasonSource, backfill int) error {
	return s.run(ctx, competition, "seasons", func(startedAt pgtype.Timestamptz) (int, error) {
		stored, err := s.q.ListSeasonYears(ctx, competition)
		if err != nil {
			return 0, err
		}
		latest := src.LatestSeason(time.Now())
		lines := 0
		for year := latest; year > latest-backfill; year-- {
			if year < latest-1 && slices.Contains(stored, int32(year)) {
				continue // history that is already here does not change
			}
			n, err := s.syncSeason(ctx, competition, src, year, startedAt)
			lines += n
			if err != nil {
				return lines, fmt.Errorf("season %d: %w", year, err)
			}
		}
		return lines, nil
	})
}

func (s *Syncer) syncSeason(ctx context.Context, competition string, src SeasonSource, year int, startedAt pgtype.Timestamptz) (int, error) {
	lines, err := src.SeasonStats(ctx, year)
	if err != nil {
		return 0, err
	}
	stored := 0
	for _, line := range lines {
		playerID, err := s.q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: line.Provider, ProviderID: line.ProviderID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // someone who is not in our player pool
		}
		if err != nil {
			return stored, err
		}
		if err := s.StoreSeason(ctx, playerID, competition, line.Season); err != nil {
			return stored, err
		}
		stored++
	}
	if len(lines) == 0 {
		return 0, nil // a season not played yet: leave whatever is stored alone
	}
	return stored, s.q.DeleteStalePlayerSeasons(ctx, db.DeleteStalePlayerSeasonsParams{
		Competition: competition, Year: int32(year), SyncedBefore: startedAt,
	})
}

// StoreSeason writes one season line for a player.
func (s *Syncer) StoreSeason(ctx context.Context, playerID pgtype.UUID, competition string, season Season) error {
	stats, _ := json.Marshal(season.Stats)
	return s.q.UpsertPlayerSeason(ctx, db.UpsertPlayerSeasonParams{
		PlayerID: playerID, Competition: competition, Year: int32(season.Year), Label: season.Label,
		Team: season.Team, League: season.League, Games: int32(season.Games), Stats: stats,
	})
}

// run records one job in ingest_runs and refuses to overlap with itself.
func (s *Syncer) run(ctx context.Context, competition, job string, work func(startedAt pgtype.Timestamptz) (int, error)) error {
	if _, busy := s.running.LoadOrStore(competition+"/"+job, struct{}{}); busy {
		return ErrBusy
	}
	defer s.running.Delete(competition + "/" + job)

	run, err := s.q.StartIngestRun(ctx, db.StartIngestRunParams{Competition: competition, Job: job})
	if err != nil {
		return err
	}
	rows, err := work(run.StartedAt)

	finish := db.FinishIngestRunParams{ID: run.ID, RowsUpserted: int32(rows)}
	if err != nil {
		finish.Error = err.Error()
	}
	// The run is recorded even when ctx was cancelled mid-sync.
	if ferr := s.q.FinishIngestRun(context.WithoutCancel(ctx), finish); ferr != nil {
		s.log.Error("finish ingest run", "err", ferr)
	}
	s.log.Info("sync finished", "competition", competition, "job", job, "rows", rows, "err", err)
	return err
}

func (s *Syncer) syncRosters(ctx context.Context, competition string, src Source, startedAt pgtype.Timestamptz) (int, error) {
	teams, err := src.Teams(ctx)
	if err != nil {
		return 0, fmt.Errorf("teams: %w", err)
	}

	rows, failed := 0, 0
	for _, team := range teams {
		n, err := s.syncTeam(ctx, competition, src, team)
		rows += n
		if err != nil {
			if ctx.Err() != nil {
				return rows, ctx.Err()
			}
			failed++
			s.log.Warn("team roster failed", "competition", competition, "team", team.Abbrev, "err", err)
		}
	}
	if failed > 0 {
		// Without every roster we cannot tell who has really left.
		return rows, fmt.Errorf("%d of %d team rosters failed", failed, len(teams))
	}

	// A narrowed competition first removes the players it no longer covers,
	// while their teams still show where they are; whoever is left and was
	// not seen has gone, and is kept as inactive.
	listed := make([]string, len(teams))
	for i, t := range teams {
		listed[i] = t.ProviderID
	}
	var pool Pool
	if pooled, ok := src.(PoolSource); ok {
		pool = pooled.Pool()
	}
	if len(pool.Positions) > 0 || pool.ListedTeamsOnly {
		removed, err := s.q.DeletePlayersOutsidePool(ctx, db.DeletePlayersOutsidePoolParams{
			Competition: competition, Positions: pool.Positions, ListedTeamsOnly: pool.ListedTeamsOnly, TeamIds: listed,
		})
		if err != nil {
			return rows, err
		}
		if removed > 0 {
			s.log.Info("removed players outside the pool", "competition", competition, "players", removed)
		}
	}
	if _, err := s.q.MarkMissingInactive(ctx, db.MarkMissingInactiveParams{Competition: competition, SeenBefore: startedAt}); err != nil {
		return rows, err
	}
	if !pool.ListedTeamsOnly {
		return rows, nil
	}
	// Teams outside the pool go too, once nothing points at them.
	if err := s.q.DetachUnlistedTeams(ctx, db.DetachUnlistedTeamsParams{Competition: competition, TeamIds: listed}); err != nil {
		return rows, err
	}
	_, err = s.q.DeleteUnlistedTeams(ctx, db.DeleteUnlistedTeamsParams{Competition: competition, TeamIds: listed})
	return rows, err
}

func (s *Syncer) syncTeam(ctx context.Context, competition string, src Source, team Team) (int, error) {
	teamID, err := s.q.UpsertProTeam(ctx, db.UpsertProTeamParams{
		Competition: competition,
		ProviderID:  team.ProviderID,
		Abbrev:      team.Abbrev,
		Name:        team.Name,
		LogoUrl:     team.LogoURL,
	})
	if err != nil {
		return 0, err
	}
	players, err := src.Roster(ctx, team)
	if err != nil {
		return 0, err
	}
	for i, p := range players {
		if err := s.upsertPlayer(ctx, competition, teamID, p); err != nil {
			return i, fmt.Errorf("player %s: %w", p.FullName, err)
		}
	}
	return len(players), nil
}

// upsertPlayer finds the player by external id, never by name.
func (s *Syncer) upsertPlayer(ctx context.Context, competition string, teamID pgtype.UUID, p Player) error {
	birth, positions := columns(p)
	p.Positions = positions

	id, err := s.find(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = s.q.InsertPlayer(ctx, db.InsertPlayerParams{
			Provider:    p.Provider,
			ProviderID:  p.ProviderID,
			Competition: competition,
			FullName:    p.FullName,
			Positions:   p.Positions,
			BirthDate:   birth,
			ProTeamID:   teamID,
			Class:       p.Class,
			HeadshotUrl: p.HeadshotURL,
		})
		return err
	}
	if err != nil {
		return err
	}
	previous, err := s.q.UpdatePlayer(ctx, db.UpdatePlayerParams{
		ID:          id,
		Competition: competition,
		FullName:    p.FullName,
		Positions:   p.Positions,
		BirthDate:   birth,
		ProTeamID:   teamID,
		Class:       p.Class,
		HeadshotUrl: p.HeadshotURL,
	})
	if err == nil && previous != competition && s.OnMove != nil {
		s.OnMove(ctx, id, previous, competition)
	}
	return err
}

func (s *Syncer) upsertProspect(ctx context.Context, competition string, p Player) error {
	birth, positions := columns(p)

	id, err := s.find(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = s.q.InsertProspect(ctx, db.InsertProspectParams{
			Provider:    p.Provider,
			ProviderID:  p.ProviderID,
			Competition: competition,
			FullName:    p.FullName,
			Positions:   positions,
			BirthDate:   birth,
			Note:        p.Note,
			HeadshotUrl: p.HeadshotURL,
		})
		return err
	}
	if err != nil {
		return err
	}
	return s.q.UpdateProspect(ctx, db.UpdateProspectParams{
		ID:          id,
		FullName:    p.FullName,
		Positions:   positions,
		BirthDate:   birth,
		Note:        p.Note,
		HeadshotUrl: p.HeadshotURL,
	})
}

// find returns the stored player a feed player is, by external id and
// never by name alone. If he is only known under one of his aliases, that
// row is him: it is given this feed's id so he is found directly next time.
func (s *Syncer) find(ctx context.Context, p Player) (pgtype.UUID, error) {
	id, err := s.q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: p.Provider, ProviderID: p.ProviderID})
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	for _, alias := range p.Aliases {
		id, err := s.q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: alias.Provider, ProviderID: alias.ProviderID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return id, err
		}
		return id, s.q.AddPlayerExternalID(ctx, db.AddPlayerExternalIDParams{Provider: p.Provider, ProviderID: p.ProviderID, PlayerID: id})
	}
	return pgtype.UUID{}, pgx.ErrNoRows
}

// columns converts a feed player's optional fields to their column values.
func columns(p Player) (pgtype.Date, []string) {
	positions := p.Positions
	if positions == nil {
		positions = []string{} // the column is not null; a nil slice would be sent as null
	}
	return pgtype.Date{Time: p.BirthDate, Valid: !p.BirthDate.IsZero()}, positions
}
