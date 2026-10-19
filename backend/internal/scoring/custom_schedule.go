package scoring

import (
	"context"
	"time"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

// CustomPeriod describes a future matchup period. Null matchups use the
// round robin in the regular season and automatic seeding in the playoffs.
type CustomPeriod struct {
	StartsOn  string    `json:"starts_on"`
	EndsOn    string    `json:"ends_on"`
	IsPlayoff bool      `json:"is_playoff"`
	Matchups  []Pairing `json:"matchups"`
}

func validateCustomPeriods(input []CustomPeriod, season db.Season, kept []db.Period, rounds int) ([][2]time.Time, error) {
	if len(input) == 0 || len(input) > 366 {
		return nil, problem.New("Set between 1 and 366 future matchup periods.")
	}
	previous := season.StartsOn.Time.AddDate(0, 0, -1)
	regular := len(kept)
	if regular > 0 {
		previous = kept[regular-1].EndsOn.Time
	}
	spans := make([][2]time.Time, len(input))
	playoffs := 0
	for i, p := range input {
		start, e1 := sportsday.Parse(p.StartsOn)
		end, e2 := sportsday.Parse(p.EndsOn)
		if e1 != nil || e2 != nil || end.Before(start) {
			return nil, problem.New("Matchup %d needs valid first and last days.", i+1)
		}
		if !start.After(sportsday.Today()) || !start.After(previous) || start.Before(season.StartsOn.Time) || end.After(season.EndsOn.Time) {
			return nil, problem.New("Matchup %d must be in the future, within the season, and after the previous matchup.", i+1)
		}
		if p.IsPlayoff {
			playoffs++
		} else {
			if playoffs > 0 {
				return nil, problem.New("Regular-season matchups must come before the playoffs.")
			}
			regular++
		}
		spans[i] = [2]time.Time{start, end}
		previous = end
	}
	if regular == 0 || playoffs != rounds {
		return nil, problem.New("Include a regular season and exactly %d playoff rounds for this league's playoff teams.", rounds)
	}
	return spans, nil
}

// SetSchedule replaces future periods atomically; started periods retain
// their IDs, scores and pairings. Custom dates survive automatic rescheduling.
func (s *Service) SetSchedule(ctx context.Context, id pgtype.UUID, input []CustomPeriod) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if err := q.LockSchedule(ctx, id); err != nil {
			return err
		}
		season, err := q.GetSeason(ctx, id)
		if err != nil {
			return err
		}
		if season.Status != "active" {
			return problem.New("That season is closed.")
		}
		league, err := q.GetLeague(ctx, season.LeagueID)
		if err != nil {
			return err
		}
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return err
		}
		if rules.Format.Type != settings.FormatHeadToHead {
			return problem.New("Custom schedules need a head-to-head league.")
		}
		franchises, err := q.ListFranchises(ctx, league.DynastyID)
		if err != nil {
			return err
		}
		if len(franchises) < 2 {
			return problem.New("A schedule needs at least two franchises.")
		}
		periods, err := q.ListPeriods(ctx, id)
		if err != nil {
			return err
		}
		var kept []db.Period
		for _, p := range periods {
			if !p.StartsOn.Time.After(sportsday.Today()) {
				if p.IsPlayoff {
					return problem.New("The playoffs have begun, so the schedule can no longer change.")
				}
				kept = append(kept, p)
			}
		}
		spans, err := validateCustomPeriods(input, season, kept, scheduleRounds(rules.Format, len(franchises)))
		if err != nil {
			return err
		}
		ids := make([]pgtype.UUID, len(franchises))
		for i, f := range franchises {
			ids[i] = f.ID
		}
		rotation := roundRobin(ids)
		if err := q.DeletePeriodsFrom(ctx, db.DeletePeriodsFromParams{SeasonID: id, FromDay: sportsday.Date(sportsday.Today().AddDate(0, 0, 1))}); err != nil {
			return err
		}
		for i, p := range input {
			seq := int32(len(kept) + i + 1)
			period, err := q.InsertPeriod(ctx, db.InsertPeriodParams{SeasonID: id, Seq: seq, StartsOn: sportsday.Date(spans[i][0]), EndsOn: sportsday.Date(spans[i][1]), IsPlayoff: p.IsPlayoff})
			if err != nil {
				return err
			}
			var pairs [][2]pgtype.UUID
			if p.Matchups != nil {
				pairs, err = validatePairings(p.Matchups, franchises)
				if err != nil {
					return err
				}
				if err := q.SetPeriodByHand(ctx, db.SetPeriodByHandParams{ID: period.ID, ByHand: true}); err != nil {
					return err
				}
			} else if !p.IsPlayoff {
				pairs = rotation[int(seq-1)%len(rotation)]
			}
			if p.Matchups != nil {
				if err := pairCustom(ctx, q, period, p.Matchups, pairs); err != nil {
					return err
				}
			} else if err := pair(ctx, q, period.ID, pairs); err != nil {
				return err
			}
		}
		return q.SetCustomSchedule(ctx, id, true)
	})
}

// RestartSeason moves an active season's first day into the future. It
// explicitly replaces the previous schedule, including started periods,
// so those periods do not prevent a commissioner correcting the start.
func (s *Service) RestartSeason(ctx context.Context, id pgtype.UUID, startsOn, endsOn string) error {
	starts, ends, err := dates(startsOn, endsOn)
	if err != nil {
		return err
	}
	if !starts.Time.After(sportsday.Today()) {
		return problem.New("Restart the season on a future day.")
	}
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if err := q.LockSchedule(ctx, id); err != nil {
			return err
		}
		season, err := q.GetSeason(ctx, id)
		if err != nil {
			return err
		}
		if season.Status != "active" {
			return problem.New("Reopen the season before changing its start.")
		}
		league, err := q.GetLeague(ctx, season.LeagueID)
		if err != nil {
			return err
		}
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return err
		}
		if err := q.ClearSeasonSchedule(ctx, id); err != nil {
			return err
		}
		if err := q.SetCustomSchedule(ctx, id, false); err != nil {
			return err
		}
		if err := q.SetSeasonDates(ctx, db.SetSeasonDatesParams{ID: id, StartsOn: starts, EndsOn: ends}); err != nil {
			return err
		}
		season.StartsOn, season.EndsOn = starts, ends
		if rules.Format.Type != settings.FormatHeadToHead {
			return nil
		}
		return schedule(ctx, q, league, season)
	})
}
