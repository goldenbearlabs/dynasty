package scoring

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
)

// Rescored is one franchise's settled score for one period, as stored and
// as it adds up now.
type Rescored struct {
	League    pgtype.UUID
	Period    int32
	StartsOn  time.Time
	EndsOn    time.Time
	Franchise string
	Before    float64
	After     float64
}

// Rescore adds up again the settled periods of a sport's leagues that end
// on or after a day, and reports each score that comes out differently. A
// settled period is only cleared by a change to the stat lines, lineups or
// rules behind it, so this is what brings one in line after a change to how
// points are counted. Only with apply are the new scores stored.
func (s *Service) Rescore(ctx context.Context, competition string, since time.Time, apply bool) ([]Rescored, error) {
	q := db.New(s.pool)
	leagues, err := q.ListLeaguesByCompetition(ctx, competition)
	if err != nil {
		return nil, err
	}
	var changed []Rescored
	for _, league := range leagues {
		seasons, err := q.ListSeasons(ctx, league.DynastyID)
		if err != nil {
			return nil, err
		}
		for _, season := range seasons {
			if season.LeagueID != league.ID {
				continue
			}
			periods, err := q.ListPeriods(ctx, season.ID)
			if err != nil {
				return nil, err
			}
			settled, err := q.ListPeriodScores(ctx, season.ID)
			if err != nil {
				return nil, err
			}
			for _, period := range periods {
				if period.EndsOn.Time.Before(since) {
					continue
				}
				stored := map[pgtype.UUID]float64{}
				for _, row := range settled {
					if row.PeriodID == period.ID {
						stored[row.FranchiseID] = row.Points
					}
				}
				if len(stored) == 0 {
					continue // not settled: it is added up when next needed
				}
				err := q.Tx(ctx, func(q *db.Queries) error {
					if err := q.LockPeriod(ctx, period.ID); err != nil {
						return err
					}
					scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time)
					if err != nil {
						return err
					}
					differs := false
					for id, before := range stored {
						after := scored[id].Points
						if math.Abs(after-before) < 1e-9 {
							continue
						}
						differs = true
						franchise, err := q.GetFranchise(ctx, id)
						if err != nil {
							return err
						}
						changed = append(changed, Rescored{
							League: league.ID, Period: period.Seq, StartsOn: period.StartsOn.Time, EndsOn: period.EndsOn.Time,
							Franchise: franchise.Name, Before: before, After: after,
						})
					}
					if !apply || !differs {
						return nil
					}
					if err := q.DeletePeriodScores(ctx, period.ID); err != nil {
						return err
					}
					for id := range stored {
						if err := q.InsertPeriodScore(ctx, db.InsertPeriodScoreParams{PeriodID: period.ID, FranchiseID: id, Points: scored[id].Points}); err != nil {
							return err
						}
					}
					return q.BumpPublicCache(ctx)
				})
				if err != nil {
					return changed, err
				}
			}
		}
	}
	return changed, nil
}
