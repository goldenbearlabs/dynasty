package scoring

import (
	"context"
	"errors"
	"math/bits"
	"slices"
	"time"

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

	byHand, err := pairingsByHand(ctx, q, season.ID)
	if err != nil {
		return err
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
	spans := periodSpans(start, season.EndsOn.Time, rules)
	remaining := len(spans)
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
	for i, span := range spans {
		period, err := q.InsertPeriod(ctx, db.InsertPeriodParams{
			SeasonID: season.ID, Seq: seq + int32(i) + 1, IsPlayoff: i >= regular,
			StartsOn: sportsday.Date(span[0]), EndsOn: sportsday.Date(span[1]),
		})
		if err != nil {
			return err
		}
		pairs, set := byHand[spanKey(period)]
		switch {
		case set:
			if err := q.SetPeriodByHand(ctx, db.SetPeriodByHandParams{ID: period.ID, ByHand: true}); err != nil {
				return err
			}
		case period.IsPlayoff:
			continue // playoff matchups are set as each round is reached
		default:
			pairs = rotation[int(period.Seq-1)%len(rotation)]
		}
		if err := pair(ctx, q, period.ID, pairs); err != nil {
			return err
		}
	}
	return nil
}

func pair(ctx context.Context, q *db.Queries, periodID pgtype.UUID, pairs [][2]pgtype.UUID) error {
	for _, p := range pairs {
		if err := q.InsertMatchup(ctx, db.InsertMatchupParams{PeriodID: periodID, HomeFranchiseID: p[0], AwayFranchiseID: p[1]}); err != nil {
			return err
		}
	}
	return nil
}

// ---- the commissioner's own matchups ----

// spanKey names a period by its days, which is how one is recognised again
// after the schedule is rebuilt.
func spanKey(p db.Period) string {
	return p.StartsOn.Time.Format(time.DateOnly) + "/" + p.EndsOn.Time.Format(time.DateOnly)
}

// pairingsByHand returns the matchups of every period the commissioner set
// by hand, by the period's days.
func pairingsByHand(ctx context.Context, q *db.Queries, seasonID pgtype.UUID) (map[string][][2]pgtype.UUID, error) {
	periods, err := q.ListPeriods(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	matchups, err := q.ListSeasonMatchups(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	set := map[string][][2]pgtype.UUID{}
	for _, p := range periods {
		for _, m := range matchups {
			if p.ByHand && m.PeriodID == p.ID {
				set[spanKey(p)] = append(set[spanKey(p)], [2]pgtype.UUID{m.HomeFranchiseID, m.AwayFranchiseID})
			}
		}
	}
	return set, nil
}

// Pairing is one matchup as the commissioner sets it. Without an away side
// it is a bye.
type Pairing struct {
	Home pgtype.UUID `json:"home_franchise_id"`
	Away pgtype.UUID `json:"away_franchise_id"`
}

// SetMatchups replaces a period's matchups with the commissioner's own, in
// the regular season or the playoffs. They stay as set: rebuilding the
// schedule keeps them, and a playoff round set this way is not seeded over.
// A franchise left out has no matchup that period. A period that is over
// cannot change.
func (s *Service) SetMatchups(ctx context.Context, periodID pgtype.UUID, pairings []Pairing) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		period, _, league, err := openPeriod(ctx, q, periodID)
		if err != nil {
			return err
		}
		franchises, err := q.ListFranchises(ctx, league.DynastyID)
		if err != nil {
			return err
		}
		if len(pairings) == 0 {
			return problem.New("Set at least one matchup, or go back to the automatic ones.")
		}
		playing := map[pgtype.UUID]bool{}
		pairs := make([][2]pgtype.UUID, len(pairings))
		for i, p := range pairings {
			if !p.Home.Valid {
				return problem.New("Every matchup needs a home side.")
			}
			for _, id := range []pgtype.UUID{p.Home, p.Away} {
				if !id.Valid {
					continue // a bye
				}
				at := slices.IndexFunc(franchises, func(f db.Franchise) bool { return f.ID == id })
				if at < 0 {
					return problem.New("Every side must be a franchise in this league.")
				}
				if playing[id] {
					return problem.New("%s is in more than one matchup.", franchises[at].Name)
				}
				playing[id] = true
			}
			pairs[i] = [2]pgtype.UUID{p.Home, p.Away}
		}
		if err := q.DeleteMatchups(ctx, period.ID); err != nil {
			return err
		}
		if err := q.SetPeriodByHand(ctx, db.SetPeriodByHandParams{ID: period.ID, ByHand: true}); err != nil {
			return err
		}
		return pair(ctx, q, period.ID, pairs)
	})
}

// ResetMatchups hands a period back to the schedule: its place in the
// round robin, or in the playoffs the seeding, once the round is due.
func (s *Service) ResetMatchups(ctx context.Context, periodID pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		period, season, league, err := openPeriod(ctx, q, periodID)
		if err != nil {
			return err
		}
		if err := q.DeleteMatchups(ctx, period.ID); err != nil {
			return err
		}
		if err := q.SetPeriodByHand(ctx, db.SetPeriodByHandParams{ID: period.ID, ByHand: false}); err != nil {
			return err
		}
		if period.IsPlayoff {
			return advance(ctx, q, league, season)
		}
		franchises, err := q.ListFranchises(ctx, league.DynastyID)
		if err != nil || len(franchises) < 2 {
			return err
		}
		ids := make([]pgtype.UUID, len(franchises))
		for i, f := range franchises {
			ids[i] = f.ID
		}
		rotation := roundRobin(ids)
		return pair(ctx, q, period.ID, rotation[int(period.Seq-1)%len(rotation)])
	})
}

// openPeriod loads a period whose matchups can still change, with its
// season and league.
func openPeriod(ctx context.Context, q *db.Queries, id pgtype.UUID) (db.Period, db.Season, db.League, error) {
	period, season, league, err := periodLeague(ctx, q, id)
	switch {
	case err != nil:
	case season.Status != "active":
		err = problem.New("That season is closed.")
	case finished(period):
		err = problem.New("That matchup is over, so it can no longer change.")
	}
	return period, season, league, err
}

// periodLeague loads a period with the season and league it belongs to.
func periodLeague(ctx context.Context, q *db.Queries, id pgtype.UUID) (period db.Period, season db.Season, league db.League, err error) {
	if period, err = q.GetPeriod(ctx, id); err != nil {
		return
	}
	if season, err = q.GetSeason(ctx, period.SeasonID); err != nil {
		return
	}
	league, err = q.GetLeague(ctx, season.LeagueID)
	return
}

// periodSpans cuts the days from start to end into matchup periods.
//
// One-week matchups in a league with weekly lineups follow the lineup's
// weeks, so a matchup and the lineup that plays it cover the same days. A
// season rarely starts or ends on a week boundary: a first or last stretch
// of fewer than four days joins the week beside it and makes that one
// matchup longer.
//
// Any other length is cut into equal periods from the start, and days left
// over at the end are not played.
func periodSpans(start, end time.Time, rules settings.League) [][2]time.Time {
	length := rules.Format.MatchupDays
	var spans [][2]time.Time
	if length != 7 || rules.Lineup.Period != settings.PeriodWeek {
		for first := start; !first.AddDate(0, 0, length-1).After(end); first = first.AddDate(0, 0, length) {
			spans = append(spans, [2]time.Time{first, first.AddDate(0, 0, length-1)})
		}
		return spans
	}
	const shortest = 4 // days a stretch needs to stand as its own matchup
	days := func(first, last time.Time) int { return int(last.Sub(first).Hours()/24) + 1 }
	for first := start; !first.After(end); {
		last := sportsday.WeekStart(first, rules.Lineup.WeekStart).AddDate(0, 0, 6)
		if days(first, last) < shortest {
			last = last.AddDate(0, 0, 7)
		}
		if last.After(end) {
			last = end
		}
		if days(first, last) < shortest && len(spans) > 0 {
			spans[len(spans)-1][1] = last
			break
		}
		spans = append(spans, [2]time.Time{first, last})
		first = last.AddDate(0, 0, 1)
	}
	return spans
}

// Reschedule brings a league's schedule into line after something it was
// built from has changed: the format, the season's dates, or who is in the
// league. Matchups that have begun are kept. In a league that is no longer
// head to head, the matchups still to come are removed. It does nothing
// without a season in progress, or once the playoffs have begun.
func (s *Service) Reschedule(ctx context.Context, leagueID pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error { return reschedule(ctx, q, leagueID) })
}

func reschedule(ctx context.Context, q *db.Queries, leagueID pgtype.UUID) error {
	season, err := q.LatestSeason(ctx, leagueID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && season.Status != "active") {
		return nil
	}
	if err != nil {
		return err
	}
	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		return err
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(periods, func(p db.Period) bool { return p.IsPlayoff && !p.StartsOn.Time.After(sportsday.Today()) }) {
		return nil
	}
	if rules.Format.Type != settings.FormatHeadToHead {
		tomorrow := sportsday.Today().AddDate(0, 0, 1)
		return q.DeletePeriodsFrom(ctx, db.DeletePeriodsFromParams{SeasonID: season.ID, FromDay: sportsday.Date(tomorrow)})
	}
	return schedule(ctx, q, league, season)
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
	settled, err := q.ListPeriodScores(ctx, season.ID)
	if err != nil {
		return nil, err
	}
	points := map[[2]pgtype.UUID]float64{} // period and franchise
	done := map[pgtype.UUID]bool{}
	for _, s := range settled {
		points[[2]pgtype.UUID{s.PeriodID, s.FranchiseID}] = s.Points
		done[s.PeriodID] = true
	}
	var results []result
	for _, period := range periods {
		if period.IsPlayoff || !finished(period) {
			continue
		}
		var played []db.ListSeasonMatchupsRow
		for _, m := range matchups {
			if m.PeriodID == period.ID && m.AwayFranchiseID.Valid {
				played = append(played, m)
			}
		}
		if !done[period.ID] && len(played) > 0 {
			if err := settle(ctx, q, league, period, played, points); err != nil {
				return nil, err
			}
		}
		for _, m := range played {
			results = append(results, result{
				Home: m.HomeFranchiseID, Away: m.AwayFranchiseID,
				HomePoints: points[[2]pgtype.UUID{period.ID, m.HomeFranchiseID}],
				AwayPoints: points[[2]pgtype.UUID{period.ID, m.AwayFranchiseID}],
			})
		}
	}
	return results, nil
}

// settle works out what each side of a finished period's matchups scored
// and stores it, so the period is never added up again unless something
// behind it changes: the database clears a period's scores when it does.
func settle(ctx context.Context, q *db.Queries, league db.League, period db.Period, played []db.ListSeasonMatchupsRow, points map[[2]pgtype.UUID]float64) error {
	return q.Tx(ctx, func(q *db.Queries) error {
		if err := q.LockPeriod(ctx, period.ID); err != nil {
			return err
		}
		scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time)
		if err != nil {
			return err
		}
		for _, m := range played {
			for _, id := range []pgtype.UUID{m.HomeFranchiseID, m.AwayFranchiseID} {
				points[[2]pgtype.UUID{period.ID, id}] = scored[id].Points
				if err := q.InsertPeriodScore(ctx, db.InsertPeriodScoreParams{PeriodID: period.ID, FranchiseID: id, Points: scored[id].Points}); err != nil {
					return err
				}
			}
		}
		return nil
	})
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

// SchedulePeriod is one period of a season's schedule with its matchups.
// One that has not started has no points yet.
type SchedulePeriod struct {
	db.Period
	Matchups []Matchup `json:"matchups"`
}

// Schedule returns every period of a league's latest season, in order,
// with who plays whom and the scores so far. It is empty without one.
func (s *Service) Schedule(ctx context.Context, league db.League) ([]SchedulePeriod, error) {
	q := db.New(s.pool)
	schedule := []SchedulePeriod{}
	season, err := q.LatestSeason(ctx, league.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule, nil
	}
	if err != nil {
		return nil, err
	}
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return nil, err
	}
	matchups, err := q.ListSeasonMatchups(ctx, season.ID)
	if err != nil {
		return nil, err
	}
	// Settles the finished regular season, so only the matchup being
	// played and the playoffs are added up here.
	if _, err := regularSeasonResults(ctx, q, league, season); err != nil {
		return nil, err
	}
	settled, err := q.ListPeriodScores(ctx, season.ID)
	if err != nil {
		return nil, err
	}
	for _, period := range periods {
		points := map[pgtype.UUID]float64{}
		for _, score := range settled {
			if score.PeriodID == period.ID {
				points[score.FranchiseID] = score.Points
			}
		}
		if len(points) == 0 && !period.StartsOn.Time.After(sportsday.Today()) {
			scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time)
			if err != nil {
				return nil, err
			}
			for id, row := range scored {
				points[id] = row.Points
			}
		}
		entry := SchedulePeriod{Period: period, Matchups: []Matchup{}}
		for _, m := range matchups {
			if m.PeriodID == period.ID {
				entry.Matchups = append(entry.Matchups, Matchup{
					ID: m.ID, Home: m.HomeFranchiseID, Away: m.AwayFranchiseID,
					HomePoints: points[m.HomeFranchiseID], AwayPoints: points[m.AwayFranchiseID],
					Final: finished(period),
				})
			}
		}
		schedule = append(schedule, entry)
	}
	return schedule, nil
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
	period, _, league, err := periodLeague(ctx, q, m.PeriodID)
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
