package lineup

import (
	"context"
	"slices"
	"time"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

// Keep this date in step with ListLineupPoints. Earlier windows always use
// the legacy rules, even when historical scores are recomputed.
const basketballLockInStarts = "2026-11-16"

func basketballLockIn(competition string, rules settings.Lineup, first time.Time) bool {
	return slices.Contains([]string{"nba", "cbb", "wnba"}, competition) &&
		rules.Period == settings.PeriodWeek && first.Format(time.DateOnly) >= basketballLockInStarts
}

func loadBasketballChoices(ctx context.Context, q *db.Queries, leagueID, franchiseID pgtype.UUID, view *View, now time.Time) error {
	first, _ := sportsday.Parse(view.Day)
	last, _ := sportsday.Parse(view.LastDay)
	choices, err := q.ListBasketballGameChoices(ctx, db.ListBasketballGameChoicesParams{
		LeagueID: leagueID, FranchiseID: franchiseID, FromDay: sportsday.Date(first), ToDay: sportsday.Date(last),
		AsOf: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return err
	}
	for i := range view.Players {
		p := &view.Players[i]
		p.Points, p.LocksUsed, p.Locked = 0, 0, false
		p.CountsFrom, p.CountsLast = "", true
		for j := range p.Games {
			g := &p.Games[j]
			g.Counts = false
			for _, choice := range choices {
				if choice.PlayerID != p.PlayerID || choice.GameID != g.GameID {
					continue
				}
				g.Eligible, g.LockedIn = choice.Eligible && p.StarterEligible, choice.LockedIn
				g.Counts = choice.Counts && p.StarterEligible
				g.CanLockIn = choice.CanLockIn && p.StarterEligible && view.Locked == ""
				g.LockDeadline = choice.LockDeadline.Time
				if g.LockedIn {
					p.LocksUsed++
				}
				if g.Counts {
					p.Points += g.Points
				}
				break
			}
			// A delayed feed must not allow a swap after the scheduled tipoff.
			if g.Status != "final" && !g.StartsAt.After(now) {
				p.Locked, p.LockReason = true, "his game is in progress."
			}
		}
		if p.LocksUsed > 0 {
			p.Locked, p.LockReason = true, "he is locked in for this matchup."
		}
	}
	return nil
}

func lockReason(p Player) string {
	if p.LockReason != "" {
		return p.LockReason
	}
	return "his game has started."
}

func validateBasketballLock(p Player, e Entry, now time.Time) error {
	if e.CountsFrom != "" {
		return problem.New("Choose a completed game with Lock in instead of picking a future game.")
	}
	if !e.LockGameID.Valid {
		return nil
	}
	for _, g := range p.Games {
		if g.GameID == e.LockGameID && g.CanLockIn && now.Before(g.LockDeadline) {
			return nil
		}
	}
	return problem.New("%s's game is not available to lock in. It must be completed, played in your starting lineup, and locked before his next game starts.", p.FullName)
}

func recordBasketballMove(ctx context.Context, q *db.Queries, leagueID, franchiseID pgtype.UUID, first time.Time, playerID pgtype.UUID, slot string, at time.Time) error {
	return q.RecordBasketballLineupMove(ctx, db.RecordBasketballLineupMoveParams{
		LeagueID: leagueID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(first), PlayerID: playerID,
		ChangedAt: pgtype.Timestamptz{Time: at, Valid: true}, Slot: slot,
	})
}

// Freeze the inherited lineup before replacing it, then record the exact
// time of each move. The initial snapshot also resets last week's locks.
func initializeBasketballWindow(ctx context.Context, q *db.Queries, leagueID, franchiseID pgtype.UUID, view View) error {
	first, _ := sportsday.Parse(view.Day)
	created, err := q.EnsureBasketballLineupWindow(ctx, db.EnsureBasketballLineupWindowParams{
		LeagueID: leagueID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(first),
	})
	if err != nil || created == 0 {
		return err
	}
	return q.SeedBasketballLineupWindow(ctx, db.SeedBasketballLineupWindowParams{
		LeagueID: leagueID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(first),
		ChangedAt: pgtype.Timestamptz{Time: sportsday.Start(first), Valid: true},
	})
}

func saveBasketballChoices(ctx context.Context, q *db.Queries, leagueID, franchiseID pgtype.UUID, view View, entries []Entry, now time.Time) error {
	if err := initializeBasketballWindow(ctx, q, leagueID, franchiseID, view); err != nil {
		return err
	}
	first, _ := sportsday.Parse(view.Day)
	at := now
	if start := sportsday.Start(first); at.Before(start) {
		at = start
	}
	wanted := map[pgtype.UUID]string{}
	for _, e := range entries {
		wanted[e.PlayerID] = e.Slot
	}
	for _, p := range view.Players {
		if p.Slot != wanted[p.PlayerID] {
			if err := recordBasketballMove(ctx, q, leagueID, franchiseID, first, p.PlayerID, wanted[p.PlayerID], at); err != nil {
				return err
			}
		}
	}
	for _, e := range entries {
		if e.LockGameID.Valid {
			if err := q.LockBasketballGame(ctx, db.LockBasketballGameParams{
				LeagueID: leagueID, FranchiseID: franchiseID, EffectiveOn: sportsday.Date(first),
				PlayerID: e.PlayerID, GameID: e.LockGameID, Slot: e.Slot,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// A drop, trade, or reserve move must also leave a timestamped bench entry;
// otherwise a later acquisition could make old bench games eligible.
func basketballBench(ctx context.Context, q *db.Queries, leagueID, franchiseID, playerID pgtype.UUID, day time.Time) error {
	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		return err
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"nba", "cbb", "wnba"}, league.Competition) || rules.Lineup.Period != settings.PeriodWeek {
		return nil
	}
	// A manager may already have prepared next Monday's lineup while the
	// current week still uses legacy rules.
	if err := q.BenchFutureBasketballLineups(ctx, db.BenchFutureBasketballLineupsParams{
		LeagueID: leagueID, FranchiseID: franchiseID, PlayerID: playerID, Day: sportsday.Date(day),
	}); err != nil {
		return err
	}
	first, _ := span(rules.Lineup, day)
	if !basketballLockIn(league.Competition, rules.Lineup, first) {
		return nil
	}
	view, err := load(ctx, q, league, franchiseID, day)
	if err != nil || !view.LockIn {
		return err
	}
	if err := initializeBasketballWindow(ctx, q, leagueID, franchiseID, view); err != nil {
		return err
	}
	first, _ = sportsday.Parse(view.Day)
	return recordBasketballMove(ctx, q, leagueID, franchiseID, first, playerID, "", time.Now())
}
