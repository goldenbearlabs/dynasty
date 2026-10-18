package roster

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// Season is where a franchise stands with a league's season roster: the
// season the commissioner has opened, if any, and when the franchise set
// its roster for it. Each manager decides who is on the main roster and who
// on reserve, once, whenever the season was opened: a season opened after
// its first day waits for its rosters all the same.
type Season struct {
	Season  *db.Season
	SetAt   pgtype.Timestamptz // when the roster was set; empty until it is
	Started bool               // the season's first day has come
	// Expired: the commissioner's deadline for rosters has passed, and a
	// roster not set by then holds as it stood.
	Expired bool
}

// Locked reports whether the lists hold for the rest of the season: once
// the manager has set the season's roster, or the deadline for it passes.
func (s Season) Locked() bool { return s.Season != nil && (s.SetAt.Valid || s.Expired) }

// SeasonOf is where a franchise stands with a league's open season.
func SeasonOf(ctx context.Context, q *db.Queries, leagueID, franchiseID pgtype.UUID) (Season, error) {
	today := sportsday.Today()
	season, err := q.OpenSeason(ctx, db.OpenSeasonParams{LeagueID: leagueID, Day: sportsday.Date(today)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Season{}, nil
	}
	if err != nil {
		return Season{}, err
	}
	setAt, err := q.SeasonRosterSetAt(ctx, db.SeasonRosterSetAtParams{SeasonID: season.ID, FranchiseID: franchiseID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Season{}, err
	}
	return Season{
		Season: &season, SetAt: setAt, Started: !today.Before(season.StartsOn.Time),
		Expired: season.RosterDeadline.Valid && !time.Now().Before(season.RosterDeadline.Time),
	}, nil
}

// heldForSeason refuses a manager's move between the main roster and the
// reserve list once the season's lists are locked. Drops, free agents,
// trades and injury swaps take other paths and stay open.
func heldForSeason(ctx context.Context, q *db.Queries, c Change, rules settings.League) error {
	if !rules.Roster.ReserveLockSeason || c.Source == Commissioner {
		return nil
	}
	season, err := SeasonOf(ctx, q, c.League.ID, c.Franchise.ID)
	if err != nil || !season.Locked() {
		return err
	}
	how := "Your %d season roster is set"
	if !season.SetAt.Valid {
		how = "The deadline for %d season rosters has passed, so your lists stand as they were"
	}
	return problem.New(how+": everyone stays on the list he is on until the season ends. You can still drop a player, add a free agent, trade, or use an injury swap.", season.Season.Year)
}

// SetSeason sets a franchise's roster for the season the commissioner has
// opened: the players named go on the reserve list and everyone else it
// has signed on the main roster. A manager does it once; a commissioner
// may set it again.
func (s *Service) SetSeason(ctx context.Context, c Change, reserve []pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		season, err := SeasonOf(ctx, q, c.League.ID, c.Franchise.ID)
		if err != nil {
			return err
		}
		forced := c.Source == Commissioner
		switch {
		case season.Season == nil:
			return problem.New("The commissioner has not opened this league's season yet.")
		case season.SetAt.Valid && !forced:
			return problem.New("Your %d season roster is already set.", season.Season.Year)
		case season.Expired && !forced:
			return problem.New("The deadline for %d season rosters has passed, so your lists stand as they were.", season.Season.Year)
		}
		if err := setIn(ctx, q, c, reserve); err != nil {
			return err
		}
		return q.MarkSeasonRosterSet(ctx, db.MarkSeasonRosterSetParams{SeasonID: season.Season.ID, FranchiseID: c.Franchise.ID})
	})
}

// SetDeadline sets, or with the zero time clears, the time by which every
// franchise must set its roster for a season.
func (s *Service) SetDeadline(ctx context.Context, seasonID pgtype.UUID, deadline time.Time) error {
	return db.New(s.pool).SetRosterDeadline(ctx, db.SetRosterDeadlineParams{
		ID: seasonID, RosterDeadline: pgtype.Timestamptz{Time: deadline, Valid: !deadline.IsZero()},
	})
}

// named reports whether a player is among those named for the reserve list.
func named(reserve []pgtype.UUID, id pgtype.UUID) bool { return slices.Contains(reserve, id) }
