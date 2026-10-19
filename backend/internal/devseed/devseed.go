// Package devseed fills a local database with a dynasty to work against:
// every sport on its default rules, a handful of franchises that can each
// sign in, full rosters of real players from the synced pool and future
// rookie drafts, at one of two points in a dynasty's life:
//
//   - PostDraft: each league's startup draft has just finished. Rosters are
//     full, as the draft left them, and nothing has been set: no season is
//     open yet, no season roster chosen, no lineup.
//   - InSeason: rosters full, lineups set and seasons under way.
//
// It replaces whatever dynasty was there; the player pool, games and stat
// history are left alone.
package devseed

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/auth"
	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/draft"
	"crossover/internal/dynasty"
	"crossover/internal/hub"
	"crossover/internal/lineup"
	"crossover/internal/scoring"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// Password signs in every seeded manager.
const Password = "password"

// The points in a dynasty's life the seed can leave it at.
const (
	PostDraft = "draft"
	InSeason  = "season"
)

var managers = []dynasty.FranchiseSetup{
	{Name: "Test1", ManagerName: "Luke"},
	{Name: "Ridge Runners", ManagerName: "Sam"},
	{Name: "Night Owls", ManagerName: "Jordan"},
	{Name: "Iron Pines", ManagerName: "Alex"},
	{Name: "Harbor Kings", ManagerName: "Riley"},
	{Name: "Summit FC", ManagerName: "Casey"},
}

// email is a manager's sign-in: dev@example.com for the commissioner.
func email(i int) string {
	if i == 0 {
		return "dev@example.com"
	}
	return fmt.Sprintf("manager%d@example.com", i+1)
}

// Run seeds the database at a point in the dynasty's life: PostDraft or
// InSeason. It refuses a database that is not on this machine.
func Run(ctx context.Context, pool *pgxpool.Pool, registry competition.Registry, log *slog.Logger, state string) error {
	if state != PostDraft && state != InSeason {
		return fmt.Errorf("seed to %q or %q, not %q", PostDraft, InSeason, state)
	}
	if host := pool.Config().ConnConfig.Host; host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("refusing to seed the database on %q: seed only a local one", host)
	}
	q := db.New(pool)

	// The same tables the tests empty: everything a dynasty owns.
	if _, err := pool.Exec(ctx, `truncate periods, lineups, seasons, trades, drafts, transactions, roster_entries, leagues, franchises, dynasties, users cascade`); err != nil {
		return err
	}

	setup := dynasty.Setup{
		Name:       "Dev Dynasty",
		Settings:   settings.Dynasty{OverallTitle: settings.OverallTitle{Enabled: true, PointsByFinish: []float64{12, 9, 8, 7, 6, 5, 4, 3}}},
		Franchises: managers,
		Account:    dynasty.Account{Email: email(0), Password: Password},
	}
	for _, c := range registry {
		setup.Leagues = append(setup.Leagues, dynasty.LeagueSetup{Competition: c.Key, Settings: c.Defaults})
	}
	if _, err := dynasty.NewService(pool, registry).Create(ctx, setup); err != nil {
		return fmt.Errorf("create dynasty: %w", err)
	}
	d, err := q.GetDynasty(ctx)
	if err != nil {
		return err
	}
	franchises, err := q.ListFranchises(ctx, d.ID)
	if err != nil {
		return err
	}
	// The setup's order, the commissioner first, rather than by name.
	slices.SortFunc(franchises, func(a, b db.Franchise) int {
		return slices.IndexFunc(managers, func(m dynasty.FranchiseSetup) bool { return m.Name == a.Name }) -
			slices.IndexFunc(managers, func(m dynasty.FranchiseSetup) bool { return m.Name == b.Name })
	})
	for i, f := range franchises[1:] {
		user, err := auth.CreateUser(ctx, q, email(i+1), Password)
		if err != nil {
			return err
		}
		if err := q.ClaimFranchise(ctx, db.ClaimFranchiseParams{ID: f.ID, UserID: user.ID}); err != nil {
			return err
		}
	}

	leagues, err := q.ListLeagues(ctx, d.ID)
	if err != nil {
		return err
	}
	lineups := lineup.NewService(pool)
	scores := scoring.NewService(pool)
	drafts := draft.NewService(pool, hub.New(), log)
	today := sportsday.Today()
	for _, league := range leagues {
		c, _ := registry.Get(league.Competition)
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return err
		}
		if state == PostDraft {
			// No season is open: the commissioner opens each one, and then
			// every manager sets a roster for it.
			if err := startupDraft(ctx, q, drafts, d.ID, league, rules, franchises); err != nil {
				return fmt.Errorf("%s startup draft: %w", c.Name, err)
			}
			log.Info("seeded league", "league", c.Name)
			continue
		}
		if err := fillRosters(ctx, q, league, rules, franchises); err != nil {
			return fmt.Errorf("%s rosters: %w", c.Name, err)
		}
		for _, f := range franchises {
			if err := lineups.AutoSet(ctx, league, f.ID, today); err != nil {
				log.Warn("seed lineup", "league", c.Name, "franchise", f.Name, "err", err)
			}
		}
		starts, ends := seasonDates(today, c.Season, rules.Lineup.WeekStart)
		season, err := scores.CreateSeason(ctx, scoring.NewSeason{
			LeagueID: league.ID, Year: starts.Year(), StartsOn: starts.Format(time.DateOnly), EndsOn: ends.Format(time.DateOnly),
		})
		if err != nil {
			return fmt.Errorf("%s season: %w", c.Name, err)
		}
		// Under way, so every roster has been set for it.
		for _, f := range franchises {
			if err := q.MarkSeasonRosterSet(ctx, db.MarkSeasonRosterSetParams{SeasonID: season.ID, FranchiseID: f.ID}); err != nil {
				return err
			}
		}
		log.Info("seeded league", "league", c.Name, "season_starts", starts.Format(time.DateOnly))
	}
	if state == InSeason {
		// The startup draft is behind them: free agency is open. (After a real
		// startup draft, finishing it does this and creates the future drafts.)
		if _, err := pool.Exec(ctx, `update leagues set last_draft_at = now() where dynasty_id = $1`, d.ID); err != nil {
			return err
		}
		if _, err := drafts.CreateFuture(ctx, d.ID); err != nil {
			return fmt.Errorf("future drafts: %w", err)
		}
	}

	for i, f := range franchises {
		log.Info("sign in", "franchise", f.Name, "email", email(i), "password", Password)
	}
	return nil
}

// startupDraft runs a league's startup draft from first pick to last, the
// commissioner making each one: the best player left who fits a starting
// slot the team has yet to fill, then the best player left. The draft
// itself decides each player's list, as it would for a real one.
func startupDraft(ctx context.Context, q *db.Queries, drafts *draft.Service, dynastyID pgtype.UUID, league db.League, rules settings.League, franchises []db.Franchise) error {
	order := make([]pgtype.UUID, len(franchises))
	for i, f := range franchises {
		order[i] = f.ID
	}
	created, err := drafts.Create(ctx, dynastyID, draft.NewDraft{
		Name: fmt.Sprintf("%d %s Startup Draft", sportsday.Today().Year(), league.Name), Kind: "startup", Year: sportsday.Today().Year(),
		LeagueIDs: []pgtype.UUID{league.ID}, Rounds: rules.Roster.Main + rules.Roster.Reserve,
		Order: settings.OrderSnake, FranchiseOrder: order,
	})
	if err != nil {
		return err
	}
	if err := drafts.Start(ctx, created.ID); err != nil {
		return err
	}
	ranked, err := q.ListRankedPlayers(ctx, db.ListRankedPlayersParams{Competition: league.Competition, PageSize: 5000})
	if err != nil {
		return err
	}
	var seats []settings.Slot // the starting places each team fills first
	for _, slot := range rules.Lineup.Slots {
		for range slot.Count {
			seats = append(seats, slot)
		}
	}
	commissioner := franchises[0]
	taken := map[pgtype.UUID]bool{}
	refused := map[[2]pgtype.UUID]bool{} // a team and a player its roster rules turned away
	made := map[pgtype.UUID]int{}        // franchise -> picks made
	for {
		state, err := drafts.State(ctx, created.ID)
		if err != nil {
			return err
		}
		if !state.OnClock.Valid {
			return nil // the draft is over
		}
		i := slices.IndexFunc(state.Picks, func(p db.ListDraftPicksRow) bool { return p.ID == state.OnClock })
		team := state.Picks[i].CurrentFranchiseID
		fits := func(p db.ListRankedPlayersRow) bool { return true }
		if n := made[team]; n < len(seats) {
			seat := seats[n]
			fits = func(p db.ListRankedPlayersRow) bool {
				return rules.CanStart(league.Competition, p.Conference) && lineup.Fits(p.Positions, seat)
			}
		}
		// The best who fits; if the roster rules turn him away, the next.
		picked := false
		for pass := 0; pass < 2 && !picked; pass++ {
			for _, p := range ranked {
				if taken[p.ID] || refused[[2]pgtype.UUID{team, p.ID}] || (pass == 0 && !fits(p)) {
					continue
				}
				if err := drafts.Pick(ctx, draft.PickRequest{DraftID: created.ID, Actor: commissioner, PlayerID: p.ID}); err != nil {
					refused[[2]pgtype.UUID{team, p.ID}] = true
					continue
				}
				taken[p.ID] = true
				picked = true
				break
			}
		}
		if !picked {
			return fmt.Errorf("nobody left that %s can draft", team)
		}
		made[team]++
	}
}

// fillRosters hands out a sport's best players in snake order: first one
// for each starting slot, so every lineup can be filled, then the best left
// until each main roster is full.
func fillRosters(ctx context.Context, q *db.Queries, league db.League, rules settings.League, franchises []db.Franchise) error {
	ranked, err := q.ListRankedPlayers(ctx, db.ListRankedPlayersParams{Competition: league.Competition, PageSize: 5000})
	if err != nil {
		return err
	}
	taken := map[pgtype.UUID]bool{}
	held := map[pgtype.UUID]int{} // franchise -> players
	give := func(f db.Franchise, fits func(db.ListRankedPlayersRow) bool) error {
		i := slices.IndexFunc(ranked, func(p db.ListRankedPlayersRow) bool { return !taken[p.ID] && fits(p) })
		if i < 0 {
			return nil // nobody left who fits: the slot stays open
		}
		taken[ranked[i].ID] = true
		held[f.ID]++
		return q.InsertRosterEntry(ctx, db.InsertRosterEntryParams{
			LeagueID: league.ID, FranchiseID: f.ID, PlayerID: ranked[i].ID, List: settings.ListMain, AcquiredVia: "draft", Startup: true,
		})
	}

	round := 0
	snake := func() []db.Franchise {
		order := slices.Clone(franchises)
		if round%2 == 1 {
			slices.Reverse(order)
		}
		round++
		return order
	}
	for _, slot := range rules.Lineup.Slots {
		for range slot.Count {
			for _, f := range snake() {
				if err := give(f, func(p db.ListRankedPlayersRow) bool {
					return rules.CanStart(league.Competition, p.Conference) && lineup.Fits(p.Positions, slot)
				}); err != nil {
					return err
				}
			}
		}
	}
	for full := false; !full; {
		full = true
		for _, f := range snake() {
			if held[f.ID] < rules.Roster.Main {
				full = false
				if err := give(f, func(db.ListRankedPlayersRow) bool { return true }); err != nil {
					return err
				}
			}
		}
		if round > 500 {
			break // the pool ran dry
		}
	}
	return nil
}

// seasonDates is the real season under way, from the start of this week so
// it has a few days of scores, or the next one when the sport is resting.
// window is the competition's usual season, as "MM-DD" start and end.
func seasonDates(today time.Time, window [2]string, weekStart string) (starts, ends time.Time) {
	for year := today.Year() - 1; year <= today.Year()+1; year++ {
		starts, _ = time.ParseInLocation("2006-01-02", fmt.Sprintf("%d-%s", year, window[0]), today.Location())
		ends, _ = time.ParseInLocation("2006-01-02", fmt.Sprintf("%d-%s", year, window[1]), today.Location())
		if ends.Before(starts) {
			ends = ends.AddDate(1, 0, 0) // a season that crosses the new year
		}
		if !ends.Before(today) {
			break
		}
	}
	if week := sportsday.WeekStart(today, weekStart); starts.Before(week) {
		starts = week
	}
	return starts, ends
}
