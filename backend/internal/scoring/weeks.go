package scoring

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// seasonScores is scores over a whole season without adding up again the
// weeks that are over: each is added up once and stored, until something
// behind it changes and the database clears it. Only the week under way is
// added up from its box scores on every read.
func seasonScores(ctx context.Context, q *db.Queries, league db.League, season db.Season) (map[pgtype.UUID]Row, error) {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return nil, err
	}
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return nil, err
	}
	span := db.ListSettledWeeksParams{LeagueID: league.ID, FromDay: season.StartsOn, ToDay: season.EndsOn}
	settled, err := q.ListSettledWeeks(ctx, span)
	if err != nil {
		return nil, err
	}
	ends := map[string]string{} // a settled week's last day, by its first
	for _, w := range settled {
		ends[day(w.StartsOn)] = day(w.EndsOn)
	}
	rows, err := q.ListWeekScores(ctx, db.ListWeekScoresParams(span))
	if err != nil {
		return nil, err
	}
	stored := map[string]map[pgtype.UUID]Row{}
	for _, r := range rows {
		week := day(r.StartsOn)
		if stored[week] == nil {
			stored[week] = map[pgtype.UUID]Row{}
		}
		row := stored[week][r.FranchiseID]
		row.FranchiseID = r.FranchiseID
		row.Points += r.Points
		row.Players = append(row.Players, PlayerPoints{
			PlayerID: r.PlayerID, FullName: r.FullName, Nickname: r.Nickname, HeadshotURL: r.HeadshotUrl,
			Games: int64(r.Games), Points: r.Points,
		})
		stored[week][r.FranchiseID] = row
	}

	total := map[pgtype.UUID]Row{}
	today := sportsday.Today()
	for _, w := range weeks(season.StartsOn.Time, season.EndsOn.Time, rules.Lineup.WeekStart, periods) {
		first, last := w[0], w[1]
		var scored map[pgtype.UUID]Row
		switch {
		case first.After(today):
			continue // to come
		case !last.Before(today):
			scored, err = scores(ctx, q, league, first, last) // under way
		case ends[day(sportsday.Date(first))] == day(sportsday.Date(last)):
			scored = stored[day(sportsday.Date(first))]
		default:
			scored, err = settleWeek(ctx, q, league, first, last)
		}
		if err != nil {
			return nil, err
		}
		add(total, scored)
	}
	return total, nil
}

// weeks cuts a season into the spans its games are counted in: a doubled
// matchup as a whole, or else a lineup week. Each can be added up on its
// own, and together they add up to the season.
func weeks(first, last time.Time, weekStart string, periods []db.Period) [][2]time.Time {
	window := func(d time.Time) time.Time {
		for _, p := range periods {
			if p.DoubleGames && !d.Before(p.StartsOn.Time) && !d.After(p.EndsOn.Time) {
				return p.StartsOn.Time
			}
		}
		return sportsday.WeekStart(d, weekStart)
	}
	var spans [][2]time.Time
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		if n := len(spans); n > 0 && window(d).Equal(window(spans[n-1][1])) {
			spans[n-1][1] = d
			continue
		}
		spans = append(spans, [2]time.Time{d, d})
	}
	return spans
}

// settleWeek adds up a week that is over and stores it.
func settleWeek(ctx context.Context, q *db.Queries, league db.League, first, last time.Time) (map[pgtype.UUID]Row, error) {
	var scored map[pgtype.UUID]Row
	err := q.Tx(ctx, func(q *db.Queries) error {
		start := sportsday.Date(first)
		if err := q.LockWeek(ctx, db.LockWeekParams{LeagueID: league.ID, StartsOn: start}); err != nil {
			return err
		}
		var err error
		if scored, err = scores(ctx, q, league, first, last); err != nil {
			return err
		}
		added, err := q.InsertSettledWeek(ctx, db.InsertSettledWeekParams{LeagueID: league.ID, StartsOn: start, EndsOn: sportsday.Date(last)})
		if err != nil || added == 0 {
			return err // or someone else stored it first
		}
		for _, row := range scored {
			for _, p := range row.Players {
				if err := q.InsertWeekScore(ctx, db.InsertWeekScoreParams{
					LeagueID: league.ID, StartsOn: start, FranchiseID: row.FranchiseID, PlayerID: p.PlayerID,
					Games: int32(p.Games), Points: p.Points,
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return scored, err
}

// add puts one span's scores into a running total, player by player.
func add(total, scored map[pgtype.UUID]Row) {
	for id, s := range scored {
		row := total[id]
		row.FranchiseID = id
		row.Points += s.Points
		for _, p := range s.Players {
			found := false
			for i := range row.Players {
				if row.Players[i].PlayerID == p.PlayerID {
					row.Players[i].Games += p.Games
					row.Players[i].Points += p.Points
					found = true
					break
				}
			}
			if !found {
				row.Players = append(row.Players, p)
			}
		}
		total[id] = row
	}
}

func day(d pgtype.Date) string { return d.Time.Format(time.DateOnly) }
