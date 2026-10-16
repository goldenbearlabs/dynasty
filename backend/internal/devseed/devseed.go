// Package devseed fills a local database with a dynasty to work against:
// every sport on its default rules, a handful of franchises that can each
// sign in, full rosters of real players from the synced pool, lineups,
// seasons under way and future rookie drafts. It replaces whatever dynasty
// was there; the player pool, games and stat history are left alone.
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

// Run seeds the database. It refuses one that is not on this machine.
func Run(ctx context.Context, pool *pgxpool.Pool, registry competition.Registry, log *slog.Logger) error {
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
	today := sportsday.Today()
	for _, league := range leagues {
		c, _ := registry.Get(league.Competition)
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return err
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
		if _, err := scores.CreateSeason(ctx, scoring.NewSeason{
			LeagueID: league.ID, Year: starts.Year(), StartsOn: starts.Format(time.DateOnly), EndsOn: ends.Format(time.DateOnly),
		}); err != nil {
			return fmt.Errorf("%s season: %w", c.Name, err)
		}
		log.Info("seeded league", "league", c.Name, "season_starts", starts.Format(time.DateOnly))
	}
	// The startup draft is behind them: free agency is open.
	if _, err := pool.Exec(ctx, `update leagues set last_draft_at = now() where dynasty_id = $1`, d.ID); err != nil {
		return err
	}
	if _, err := draft.NewService(pool, hub.New(), log).CreateFuture(ctx, d.ID); err != nil {
		return fmt.Errorf("future drafts: %w", err)
	}

	for i, f := range franchises {
		log.Info("sign in", "franchise", f.Name, "email", email(i), "password", Password)
	}
	return nil
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
