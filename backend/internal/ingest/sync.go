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
		err = s.q.Tx(ctx, func(q *db.Queries) error {
			for _, p := range prospects {
				if err := upsertProspect(ctx, q, competition, p); err != nil {
					return fmt.Errorf("prospect %s: %w", p.FullName, err)
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
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
			if err := s.upsertGames(ctx, competition, games); err != nil {
				return 0, err
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

// upsertGames stores one day's games in a single transaction, so however
// many of them changed, readers see one new state.
func (s *Syncer) upsertGames(ctx context.Context, competition string, games []Game) error {
	return s.q.Tx(ctx, func(q *db.Queries) error {
		teams, err := q.ListProTeamIDs(ctx, competition)
		if err != nil {
			return err
		}
		ids := map[string]pgtype.UUID{}
		for _, t := range teams {
			ids[t.ProviderID] = t.ID
		}
		for _, game := range games {
			// A team we do not know, such as an all-star side, is left empty.
			home, away := ids[game.HomeTeam], ids[game.AwayTeam]
			if !home.Valid && !away.Valid {
				continue // a game between teams outside the competition is not ours to follow
			}
			if err := q.UpsertGame(ctx, db.UpsertGameParams{
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
			}); err != nil {
				return err
			}
		}
		return nil
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
		if err := s.upsertGames(ctx, competition, games); err != nil {
			return LiveUpdate{}, err
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
	refs := make([]ExternalID, len(lines))
	for i, line := range lines {
		refs[i] = ExternalID{Provider: line.Provider, ProviderID: line.ProviderID}
	}
	stored := 0
	err = s.q.Tx(ctx, func(q *db.Queries) error {
		players, err := playerIDs(ctx, q, refs)
		if err != nil {
			return err
		}
		for i, line := range lines {
			playerID, known := players[refs[i]]
			if !known {
				continue // someone the roster feed has never listed
			}
			stats, _ := json.Marshal(line.Stats)
			if err := q.UpsertStatLine(ctx, db.UpsertStatLineParams{GameID: game.ID, PlayerID: playerID, Stats: stats}); err != nil {
				return err
			}
			stored++
		}
		return q.MarkGameStatsSynced(ctx, game.ID)
	})
	if err != nil {
		return 0, err
	}
	return stored, nil
}

// playerIDs finds the stored player behind each feed id, in one query.
// Ids nobody here has are left out.
func playerIDs(ctx context.Context, q *db.Queries, refs []ExternalID) (map[ExternalID]pgtype.UUID, error) {
	arg := db.FindPlayersByExternalIDsParams{Providers: make([]string, len(refs)), ProviderIds: make([]string, len(refs))}
	for i, ref := range refs {
		arg.Providers[i], arg.ProviderIds[i] = ref.Provider, ref.ProviderID
	}
	rows, err := q.FindPlayersByExternalIDs(ctx, arg)
	if err != nil {
		return nil, err
	}
	players := make(map[ExternalID]pgtype.UUID, len(rows))
	for _, row := range rows {
		players[ExternalID{Provider: row.Provider, ProviderID: row.ProviderID}] = row.PlayerID
	}
	return players, nil
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
		missing, err := s.q.ListSeasonsMissingResearchMetadata(ctx, competition)
		if err != nil {
			return 0, err
		}
		latest := src.LatestSeason(time.Now())
		years := []int{}
		for year := latest; year > latest-backfill; year-- {
			years = append(years, year)
		}
		for _, year := range missing {
			if !slices.Contains(years, int(year)) {
				years = append(years, int(year))
			}
		}
		lines := 0
		for _, year := range years {
			if year < latest-1 && slices.Contains(stored, int32(year)) && !slices.Contains(missing, int32(year)) {
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
	if len(lines) == 0 {
		return 0, nil // a season not played yet: leave whatever is stored alone
	}
	refs := make([]ExternalID, len(lines))
	for i, line := range lines {
		refs[i] = ExternalID{Provider: line.Provider, ProviderID: line.ProviderID}
	}
	stored := 0
	err = s.q.Tx(ctx, func(q *db.Queries) error {
		players, err := playerIDs(ctx, q, refs)
		if err != nil {
			return err
		}
		for i, line := range lines {
			playerID, known := players[refs[i]]
			if !known {
				continue // someone who is not in our player pool
			}
			if err := storeSeason(ctx, q, playerID, competition, line.Season); err != nil {
				return err
			}
			stored++
		}
		return q.DeleteStalePlayerSeasons(ctx, db.DeleteStalePlayerSeasonsParams{
			Competition: competition, Year: int32(year), SyncedBefore: startedAt,
		})
	})
	if err != nil {
		return 0, err
	}
	return stored, nil
}

// StoreSeason writes one season line for a player.
func (s *Syncer) StoreSeason(ctx context.Context, playerID pgtype.UUID, competition string, season Season) error {
	return storeSeason(ctx, s.q, playerID, competition, season)
}

func storeSeason(ctx context.Context, q *db.Queries, playerID pgtype.UUID, competition string, season Season) error {
	stats, _ := json.Marshal(season.Stats)
	return q.UpsertPlayerSeason(ctx, db.UpsertPlayerSeasonParams{
		Conference: season.Conference,
		PlayerID:   playerID, Competition: competition, Year: int32(season.Year), Label: season.Label,
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

// syncTeam stores one team and its roster in a single transaction: the
// roster is fetched first, so the transaction only spans the writes.
func (s *Syncer) syncTeam(ctx context.Context, competition string, src Source, team Team) (int, error) {
	players, err := src.Roster(ctx, team)
	if err != nil {
		return 0, err
	}
	type move struct {
		player pgtype.UUID
		from   string
	}
	var moves []move
	err = s.q.Tx(ctx, func(q *db.Queries) error {
		teamID, err := q.UpsertProTeam(ctx, db.UpsertProTeamParams{
			Competition: competition,
			ProviderID:  team.ProviderID,
			Abbrev:      team.Abbrev,
			Name:        team.Name,
			LogoUrl:     team.LogoURL,
			Conference:  team.Conference,
		})
		if err != nil {
			return err
		}
		for _, p := range players {
			id, previous, err := upsertPlayer(ctx, q, competition, teamID, p)
			if err != nil {
				return fmt.Errorf("player %s: %w", p.FullName, err)
			}
			if previous != competition {
				moves = append(moves, move{id, previous})
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if s.OnMove != nil {
		for _, m := range moves {
			s.OnMove(ctx, m.player, m.from, competition)
		}
	}
	return len(players), nil
}

// upsertPlayer finds the player by external id, never by name. It returns
// his id and the competition he was in before, which differs from this one
// when he has moved.
func upsertPlayer(ctx context.Context, q *db.Queries, competition string, teamID pgtype.UUID, p Player) (pgtype.UUID, string, error) {
	birth, positions := columns(p)
	p.Positions = positions

	id, err := find(ctx, q, p)
	if errors.Is(err, pgx.ErrNoRows) {
		id, err = q.InsertPlayer(ctx, db.InsertPlayerParams{
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
		return id, competition, err
	}
	if err != nil {
		return id, "", err
	}
	previous, err := q.UpdatePlayer(ctx, db.UpdatePlayerParams{
		ID:          id,
		Competition: competition,
		FullName:    p.FullName,
		Positions:   p.Positions,
		BirthDate:   birth,
		ProTeamID:   teamID,
		Class:       p.Class,
		HeadshotUrl: p.HeadshotURL,
	})
	return id, previous, err
}

func upsertProspect(ctx context.Context, q *db.Queries, competition string, p Player) error {
	birth, positions := columns(p)

	id, err := find(ctx, q, p)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = q.InsertProspect(ctx, db.InsertProspectParams{
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
	return q.UpdateProspect(ctx, db.UpdateProspectParams{
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
func find(ctx context.Context, q *db.Queries, p Player) (pgtype.UUID, error) {
	id, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: p.Provider, ProviderID: p.ProviderID})
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	for _, alias := range p.Aliases {
		id, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: alias.Provider, ProviderID: alias.ProviderID})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return id, err
		}
		return id, q.AddPlayerExternalID(ctx, db.AddPlayerExternalIDParams{Provider: p.Provider, ProviderID: p.ProviderID, PlayerID: id})
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
