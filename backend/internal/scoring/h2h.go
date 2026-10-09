package scoring

import (
	"context"
	"errors"
	"math/bits"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// Head-to-head leagues. A season is cut into periods of matchup_days. In
// each regular-season period every franchise plays one other; the last few
// periods are the playoffs. A matchup's score is each side's points over
// its period, computed like every other total, so nothing here is stored
// but who plays whom.

// playoffRounds is how many rounds it takes to get from n teams to one.
func playoffRounds(teams int) int {
	return bits.Len(uint(teams - 1))
}

// GenerateSchedule builds, or rebuilds, the periods of a season that have
// not started yet. Use it after changing the season's dates or the
// league's matchup rules, or after adding a franchise.
func (s *Service) GenerateSchedule(ctx context.Context, seasonID pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		season, err := q.GetSeason(ctx, seasonID)
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
		return schedule(ctx, q, league, season)
	})
}

func schedule(ctx context.Context, q *db.Queries, league db.League, season db.Season) error {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	if rules.Format.Type != settings.FormatHeadToHead {
		return problem.New("%s is not a head-to-head league.", league.Name)
	}
	franchises, err := q.ListFranchises(ctx, league.DynastyID)
	if err != nil {
		return err
	}
	if len(franchises) < 2 {
		return problem.New("A head-to-head league needs at least two franchises.")
	}

	// Keep what has started; replace everything after it.
	tomorrow := sportsday.Today().AddDate(0, 0, 1)
	if err := q.DeletePeriodsFrom(ctx, db.DeletePeriodsFromParams{SeasonID: season.ID, FromDay: sportsday.Date(tomorrow)}); err != nil {
		return err
	}
	kept, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return err
	}
	start, seq := season.StartsOn.Time, int32(0)
	if len(kept) > 0 {
		last := kept[len(kept)-1]
		if last.IsPlayoff {
			return problem.New("The playoffs have begun, so the schedule can no longer change.")
		}
		start, seq = last.EndsOn.Time.AddDate(0, 0, 1), last.Seq
	}

	length := rules.Format.MatchupDays
	remaining := int(season.EndsOn.Time.Sub(start).Hours()/24+1) / length
	rounds := playoffRounds(min(rules.Format.PlayoffTeams, len(franchises)))
	regular := remaining - rounds
	if regular < 1 {
		return problem.New("%s's season is too short for %d-day matchups and %d playoff rounds. Lengthen it, or change the format.",
			league.Name, length, rounds)
	}

	ids := make([]pgtype.UUID, len(franchises))
	for i, f := range franchises {
		ids[i] = f.ID
	}
	rotation := roundRobin(ids)
	for i := range remaining {
		first := start.AddDate(0, 0, i*length)
		period, err := q.InsertPeriod(ctx, db.InsertPeriodParams{
			SeasonID: season.ID, Seq: seq + int32(i) + 1, IsPlayoff: i >= regular,
			StartsOn: sportsday.Date(first), EndsOn: sportsday.Date(first.AddDate(0, 0, length-1)),
		})
		if err != nil {
			return err
		}
		if period.IsPlayoff {
			continue // playoff matchups are set as each round is reached
		}
		// Carry on the rotation from where the kept periods left off.
		for _, pair := range rotation[(len(kept)+i)%len(rotation)] {
			if err := q.InsertMatchup(ctx, db.InsertMatchupParams{PeriodID: period.ID, HomeFranchiseID: pair[0], AwayFranchiseID: pair[1]}); err != nil {
				return err
			}
		}
	}
	return nil
}

// roundRobin returns rounds in which every team plays every other once,
// by the circle method: one team stays put while the rest rotate. With an
// odd number of teams, one sits out each round; its pair has no away side.
func roundRobin(teams []pgtype.UUID) [][][2]pgtype.UUID {
	circle := slices.Clone(teams)
	if len(circle)%2 == 1 {
		circle = append(circle, pgtype.UUID{}) // the bye
	}
	n := len(circle)
	var rounds [][][2]pgtype.UUID
	for round := range n - 1 {
		var pairs [][2]pgtype.UUID
		for i := range n / 2 {
			home, away := circle[i], circle[n-1-i]
			// Alternate who is at home, and keep a real team first in a bye.
			if (round%2 == 1 && away.Valid) || !home.Valid {
				home, away = away, home
			}
			pairs = append(pairs, [2]pgtype.UUID{home, away})
		}
		rounds = append(rounds, pairs)
		circle = append([]pgtype.UUID{circle[0], circle[n-1]}, circle[1:n-1]...)
	}
	return rounds
}

// result is one finished matchup between two franchises.
type result struct {
	Home, Away             pgtype.UUID
	HomePoints, AwayPoints float64
}

// regularSeasonResults returns every finished regular-season matchup. A
// period is finished once its last day has passed.
func regularSeasonResults(ctx context.Context, q *db.Queries, league db.League, season db.Season) ([]result, error) {
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil || len(periods) == 0 {
		return nil, err
	}
	matchups, err := q.ListSeasonMatchups(ctx, season.ID)
	if err != nil {
		return nil, err
	}
	var results []result
	for _, period := range periods {
		if period.IsPlayoff || !finished(period) {
			continue
		}
		scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time)
		if err != nil {
			return nil, err
		}
		for _, m := range matchups {
			if m.PeriodID == period.ID && m.AwayFranchiseID.Valid {
				results = append(results, result{
					Home: m.HomeFranchiseID, Away: m.AwayFranchiseID,
					HomePoints: scored[m.HomeFranchiseID].Points, AwayPoints: scored[m.AwayFranchiseID].Points,
				})
			}
		}
	}
	return results, nil
}

func finished(period db.Period) bool {
	return period.EndsOn.Time.Before(sportsday.Today())
}

// AdvancePlayoffs sets the matchups of any playoff round that has been
// reached, in every head-to-head season in progress. It is safe to call at
// any time and does nothing until a round is due.
func (s *Service) AdvancePlayoffs(ctx context.Context) error {
	seasons, err := db.New(s.pool).ListActiveSeasons(ctx)
	if err != nil {
		return err
	}
	for _, season := range seasons {
		if err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
			league, err := q.GetLeague(ctx, season.LeagueID)
			if err != nil {
				return err
			}
			return advance(ctx, q, league, season)
		}); err != nil {
			return err
		}
	}
	return nil
}

// advance fills in the next playoff round once the period before it is
// over. The first round takes the top teams from the standings; each later
// round takes the winners. Teams are re-seeded every round, the best
// playing the worst, and with an odd number the top seed sits out.
func advance(ctx context.Context, q *db.Queries, league db.League, season db.Season) error {
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return err
	}
	matchups, err := q.ListSeasonMatchups(ctx, season.ID)
	if err != nil {
		return err
	}
	in := func(period db.Period) []db.ListSeasonMatchupsRow {
		var found []db.ListSeasonMatchupsRow
		for _, m := range matchups {
			if m.PeriodID == period.ID {
				found = append(found, m)
			}
		}
		return found
	}

	next := slices.IndexFunc(periods, func(p db.Period) bool { return p.IsPlayoff && len(in(p)) == 0 })
	if next < 1 || !finished(periods[next-1]) {
		return nil // no round is due
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	standings, err := table(ctx, q, league, &season)
	if err != nil {
		return err
	}
	seed := func(id pgtype.UUID) int {
		return slices.IndexFunc(standings, func(r Row) bool { return r.FranchiseID == id })
	}

	var alive []pgtype.UUID
	if previous := periods[next-1]; !previous.IsPlayoff {
		for _, row := range standings[:min(rules.Format.PlayoffTeams, len(standings))] {
			alive = append(alive, row.FranchiseID)
		}
	} else {
		scored, err := scores(ctx, q, league, previous.StartsOn.Time, previous.EndsOn.Time)
		if err != nil {
			return err
		}
		for _, m := range in(previous) {
			alive = append(alive, winner(m.HomeFranchiseID, m.AwayFranchiseID, scored))
		}
		slices.SortFunc(alive, func(a, b pgtype.UUID) int { return seed(a) - seed(b) })
	}
	if len(alive) < 2 {
		return nil // the final has been played
	}

	if len(alive)%2 == 1 { // the top seed sits this round out
		if err := q.InsertMatchup(ctx, db.InsertMatchupParams{PeriodID: periods[next].ID, HomeFranchiseID: alive[0]}); err != nil {
			return err
		}
		alive = alive[1:]
	}
	for i := range len(alive) / 2 {
		if err := q.InsertMatchup(ctx, db.InsertMatchupParams{
			PeriodID: periods[next].ID, HomeFranchiseID: alive[i], AwayFranchiseID: alive[len(alive)-1-i],
		}); err != nil {
			return err
		}
	}
	return nil
}

// winner of a matchup: the side with more points. The home side, which in
// the playoffs is the better seed, takes a tie, and a bye has no away side.
func winner(home, away pgtype.UUID, scored map[pgtype.UUID]Row) pgtype.UUID {
	if away.Valid && scored[away].Points > scored[home].Points {
		return away
	}
	return home
}

// playoffChampion returns the winner of the playoff final, if it has been
// played, and an invalid id otherwise.
func playoffChampion(ctx context.Context, q *db.Queries, league db.League, season db.Season) (pgtype.UUID, error) {
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil || len(periods) == 0 {
		return pgtype.UUID{}, err
	}
	final := periods[len(periods)-1]
	if !final.IsPlayoff || !finished(final) {
		return pgtype.UUID{}, nil
	}
	matchups, err := q.ListSeasonMatchups(ctx, season.ID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	var played []db.ListSeasonMatchupsRow
	for _, m := range matchups {
		if m.PeriodID == final.ID {
			played = append(played, m)
		}
	}
	if len(played) != 1 {
		return pgtype.UUID{}, nil
	}
	scored, err := scores(ctx, q, league, final.StartsOn.Time, final.EndsOn.Time)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return winner(played[0].HomeFranchiseID, played[0].AwayFranchiseID, scored), nil
}

// ---- reading ----

// Matchups is one period of a head-to-head season, with the score of each
// matchup so far.
type Matchups struct {
	Periods  []db.Period `json:"periods"` // the whole season, for moving between them
	Period   *db.Period  `json:"period"`  // the one shown; nil when there is no schedule
	Matchups []Matchup   `json:"matchups"`
}

type Matchup struct {
	ID         pgtype.UUID `json:"id"`
	Home       pgtype.UUID `json:"home_franchise_id"`
	Away       pgtype.UUID `json:"away_franchise_id"` // null for a bye
	HomePoints float64     `json:"home_points"`
	AwayPoints float64     `json:"away_points"`
	Final      bool        `json:"final"`
}

// Matchups returns a period's matchups for a league's latest season. seq
// picks the period; zero means the one in progress, or failing that the
// most recent.
func (s *Service) Matchups(ctx context.Context, league db.League, seq int) (Matchups, error) {
	q := db.New(s.pool)
	view := Matchups{Periods: []db.Period{}, Matchups: []Matchup{}}
	season, err := q.LatestSeason(ctx, league.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	if view.Periods, err = q.ListPeriods(ctx, season.ID); err != nil || len(view.Periods) == 0 {
		return view, err
	}

	i := slices.IndexFunc(view.Periods, func(p db.Period) bool { return int(p.Seq) == seq })
	if i < 0 { // the period in progress or next to start
		i = slices.IndexFunc(view.Periods, func(p db.Period) bool { return !finished(p) })
	}
	if i < 0 { // the season is over
		i = len(view.Periods) - 1
	}
	period := view.Periods[i]
	view.Period = &period

	all, err := q.ListSeasonMatchups(ctx, season.ID)
	if err != nil {
		return view, err
	}
	scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time)
	if err != nil {
		return view, err
	}
	for _, m := range all {
		if m.PeriodID == period.ID {
			view.Matchups = append(view.Matchups, Matchup{
				ID: m.ID, Home: m.HomeFranchiseID, Away: m.AwayFranchiseID,
				HomePoints: scored[m.HomeFranchiseID].Points, AwayPoints: scored[m.AwayFranchiseID].Points,
				Final: finished(period),
			})
		}
	}
	return view, nil
}

// MatchupDetail is one matchup with the players behind each side's score.
type MatchupDetail struct {
	Matchup
	Competition string         `json:"competition"`
	Period      db.Period      `json:"period"`
	HomePlayers []PlayerPoints `json:"home_players"`
	AwayPlayers []PlayerPoints `json:"away_players"`
}

func (s *Service) Matchup(ctx context.Context, id pgtype.UUID) (MatchupDetail, error) {
	q := db.New(s.pool)
	m, err := q.GetMatchup(ctx, id)
	if err != nil {
		return MatchupDetail{}, err
	}
	period, err := q.GetPeriod(ctx, m.PeriodID)
	if err != nil {
		return MatchupDetail{}, err
	}
	season, err := q.GetSeason(ctx, period.SeasonID)
	if err != nil {
		return MatchupDetail{}, err
	}
	league, err := q.GetLeague(ctx, season.LeagueID)
	if err != nil {
		return MatchupDetail{}, err
	}
	scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time)
	if err != nil {
		return MatchupDetail{}, err
	}
	players := func(id pgtype.UUID) []PlayerPoints {
		if scored[id].Players == nil {
			return []PlayerPoints{}
		}
		return scored[id].Players
	}
	return MatchupDetail{
		Matchup: Matchup{
			ID: m.ID, Home: m.HomeFranchiseID, Away: m.AwayFranchiseID,
			HomePoints: scored[m.HomeFranchiseID].Points, AwayPoints: scored[m.AwayFranchiseID].Points,
			Final: finished(period),
		},
		Competition: league.Competition, Period: period,
		HomePlayers: players(m.HomeFranchiseID), AwayPlayers: players(m.AwayFranchiseID),
	}, nil
}
