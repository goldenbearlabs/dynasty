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
	if teams < 2 {
		return 0
	}
	return bits.Len(uint(teams - 1))
}

// GenerateSchedule builds, or rebuilds, the periods of a season that have
// not started yet. Use it after changing the season's dates or the
// league's matchup rules, or after adding a franchise.
func (s *Service) GenerateSchedule(ctx context.Context, seasonID pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		if err := q.LockSchedule(ctx, seasonID); err != nil {
			return err
		}
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
		if err := q.SetCustomSchedule(ctx, season.ID, false); err != nil {
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
	spans := leaguePeriodSpans(start, season.EndsOn.Time, rules, league.Competition)
	remaining := len(spans)
	rounds := scheduleRounds(rules.Format, len(franchises))
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
		custom, set := byHand[spanKey(period)]
		var pairs [][2]pgtype.UUID
		switch {
		case set:
			if err := q.SetPeriodByHand(ctx, db.SetPeriodByHandParams{ID: period.ID, ByHand: true}); err != nil {
				return err
			}
			pairs = make([][2]pgtype.UUID, len(custom))
			for i, p := range custom {
				pairs[i] = [2]pgtype.UUID{p.Home, p.Away}
			}
			if err := pairCustom(ctx, q, period, custom, pairs); err != nil {
				return err
			}
			continue
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
func pairingsByHand(ctx context.Context, q *db.Queries, seasonID pgtype.UUID) (map[string][]Pairing, error) {
	periods, err := q.ListPeriods(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	matchups, err := q.ListSeasonMatchups(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	set := map[string][]Pairing{}
	for _, p := range periods {
		for _, m := range matchups {
			if p.ByHand && m.PeriodID == p.ID {
				slot := m.BracketSlot
				set[spanKey(p)] = append(set[spanKey(p)], Pairing{Home: m.HomeFranchiseID, Away: m.AwayFranchiseID, Bracket: m.Bracket, Slot: &slot})
			}
		}
	}
	return set, nil
}

// Pairing is one matchup as the commissioner sets it. Without an away side
// it is a bye.
type Pairing struct {
	Home    pgtype.UUID `json:"home_franchise_id"`
	Away    pgtype.UUID `json:"away_franchise_id"`
	Bracket string      `json:"bracket,omitempty"`
	Slot    *int32      `json:"bracket_slot,omitempty"`
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
		pairs, err := validatePairings(pairings, franchises)
		if err != nil {
			return err
		}
		if err := q.DeleteMatchups(ctx, period.ID); err != nil {
			return err
		}
		if err := q.SetPeriodByHand(ctx, db.SetPeriodByHandParams{ID: period.ID, ByHand: true}); err != nil {
			return err
		}
		return pairCustom(ctx, q, period, pairings, pairs)
	})
}

func validatePairings(pairings []Pairing, franchises []db.Franchise) ([][2]pgtype.UUID, error) {
	if len(pairings) == 0 {
		return nil, problem.New("Set at least one matchup, or go back to the automatic ones.")
	}
	playing := map[pgtype.UUID]bool{}
	pairs := make([][2]pgtype.UUID, len(pairings))
	for i, p := range pairings {
		if !p.Home.Valid {
			return nil, problem.New("Every matchup needs a home side.")
		}
		for _, id := range []pgtype.UUID{p.Home, p.Away} {
			if !id.Valid {
				continue // a bye
			}
			at := slices.IndexFunc(franchises, func(f db.Franchise) bool { return f.ID == id })
			if at < 0 {
				return nil, problem.New("Every side must be a franchise in this league.")
			}
			if playing[id] {
				return nil, problem.New("%s is in more than one matchup.", franchises[at].Name)
			}
			playing[id] = true
		}
		pairs[i] = [2]pgtype.UUID{p.Home, p.Away}
	}
	return pairs, nil
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

// NHL matchups follow calendar weeks even though managers set daily lineups.
// WeekStart defaults to Monday; an explicitly configured week start is honored.
func leaguePeriodSpans(start, end time.Time, rules settings.League, competition string) [][2]time.Time {
	if competition == "nhl" && rules.Format.MatchupDays == 7 {
		rules.Lineup.Period = settings.PeriodWeek
		if rules.Lineup.WeekStart == "" {
			rules.Lineup.WeekStart = "monday"
		}
	}
	return periodSpans(start, end, rules)
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
		if err := q.SetCustomSchedule(ctx, season.ID, false); err != nil {
			return err
		}
		tomorrow := sportsday.Today().AddDate(0, 0, 1)
		return q.DeletePeriodsFrom(ctx, db.DeletePeriodsFromParams{SeasonID: season.ID, FromDay: sportsday.Date(tomorrow)})
	}
	custom, err := q.HasCustomSchedule(ctx, season.ID)
	if err != nil {
		return err
	}
	if custom {
		for _, p := range periods {
			if p.StartsOn.Time.Before(season.StartsOn.Time) || p.EndsOn.Time.After(season.EndsOn.Time) {
				return problem.New("The season dates must contain the custom schedule. Edit the schedule first, or rebuild it with automatic dates.")
			}
		}
		return nil
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

// advance fills every due round of the championship and consolation
// brackets independently. Slot order carries winners through a fixed tree.
func advance(ctx context.Context, q *db.Queries, league db.League, season db.Season) error {
	if err := q.LockSchedule(ctx, season.ID); err != nil {
		return err
	}
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return err
	}
	matchups, err := q.ListSeasonMatchups(ctx, season.ID)
	if err != nil {
		return err
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	first := slices.IndexFunc(periods, func(p db.Period) bool { return p.IsPlayoff })
	if first < 1 || !finished(periods[first-1]) {
		return nil
	}
	standings, err := table(ctx, q, league, &season)
	if err != nil {
		return err
	}
	top := min(rules.Format.PlayoffTeams, len(standings))
	fields := map[string][]pgtype.UUID{championshipBracket: {}, consolationBracket: {}}
	for i, r := range standings {
		bracket := championshipBracket
		if i >= top {
			bracket = consolationBracket
		}
		fields[bracket] = append(fields[bracket], r.FranchiseID)
	}
	if rules.Format.BottomFirst() {
		slices.Reverse(fields[consolationBracket])
	}
	in := func(period pgtype.UUID, bracket string) []db.ListSeasonMatchupsRow {
		var found []db.ListSeasonMatchupsRow
		for _, m := range matchups {
			if m.PeriodID == period && m.Bracket == bracket {
				found = append(found, m)
			}
		}
		return found
	}
	for i := first; i < len(periods); i++ {
		p := periods[i]
		if !p.IsPlayoff || p.ByHand || !finished(periods[i-1]) {
			continue
		}
		for _, bracket := range []string{championshipBracket, consolationBracket} {
			if bracket == consolationBracket && !rules.Format.ConsolationEnabled() {
				continue
			}
			teams := fields[bracket]
			rounds := playoffRounds(len(teams))
			offset := len(periods) - first - rounds
			if rounds == 0 || i-first < offset || len(in(p.ID, bracket)) > 0 {
				continue
			}
			var pairs []bracketPair
			if i-first == offset {
				pairs = seededBracket(teams)
			} else {
				prev := periods[i-1]
				previous := in(prev.ID, bracket)
				scored, err := scores(ctx, q, league, prev.StartsOn.Time, prev.EndsOn.Time)
				if err != nil {
					return err
				}
				if rules.Format.Reseed {
					alive := []pgtype.UUID{}
					for _, m := range previous {
						alive = append(alive, winner(m.HomeFranchiseID, m.AwayFranchiseID, scored))
					}
					slices.SortFunc(alive, func(a, b pgtype.UUID) int { return slices.Index(teams, a) - slices.Index(teams, b) })
					pairs = seededBracket(alive)
				} else {
					pairs = fixedNext(previous, scored)
					// Home wins a tie, so preserve the better original bracket seed at home.
					for j, pair := range pairs {
						if pair.Away.Valid && slices.Index(teams, pair.Away) < slices.Index(teams, pair.Home) {
							pairs[j].Home, pairs[j].Away = pair.Away, pair.Home
						}
					}
				}
			}
			if err := insertBracket(ctx, q, p.ID, bracket, pairs); err != nil {
				return err
			}
			// Make newly generated rounds visible to later due rounds in this call.
			for _, pair := range pairs {
				matchups = append(matchups, db.ListSeasonMatchupsRow{PeriodID: p.ID, HomeFranchiseID: pair.Home, AwayFranchiseID: pair.Away, Bracket: bracket, BracketSlot: pair.Slot})
			}
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
	return bracketChampion(ctx, q, league, season, championshipBracket)
}

func bracketChampion(ctx context.Context, q *db.Queries, league db.League, season db.Season, bracket string) (pgtype.UUID, error) {
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
		if m.PeriodID == final.ID && m.Bracket == bracket {
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
	ID          pgtype.UUID `json:"id"`
	Bracket     string      `json:"bracket"`
	BracketSlot int32       `json:"bracket_slot"`
	Home        pgtype.UUID `json:"home_franchise_id"`
	Away        pgtype.UUID `json:"away_franchise_id"` // null for a bye
	HomePoints  float64     `json:"home_points"`
	AwayPoints  float64     `json:"away_points"`
	Final       bool        `json:"final"`
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
				ID: m.ID, Home: m.HomeFranchiseID, Away: m.AwayFranchiseID, Bracket: m.Bracket, BracketSlot: m.BracketSlot,
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
					ID: m.ID, Home: m.HomeFranchiseID, Away: m.AwayFranchiseID, Bracket: m.Bracket, BracketSlot: m.BracketSlot,
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
	scored, err := scores(ctx, q, league, period.StartsOn.Time, period.EndsOn.Time, true)
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
			ID: m.ID, Home: m.HomeFranchiseID, Away: m.AwayFranchiseID, Bracket: m.Bracket, BracketSlot: m.BracketSlot,
			HomePoints: scored[m.HomeFranchiseID].Points, AwayPoints: scored[m.AwayFranchiseID].Points,
			Final: finished(period),
		},
		Competition: league.Competition, Period: period,
		HomePlayers: players(m.HomeFranchiseID), AwayPlayers: players(m.AwayFranchiseID),
	}, nil
}
