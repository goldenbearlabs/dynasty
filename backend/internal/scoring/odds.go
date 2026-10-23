package scoring

import (
	"cmp"
	"context"
	"errors"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// Playoff odds. Each league's regular season is played out many times from
// where it stands. What has happened is fixed: finished matchups, and the
// points already scored in the one in progress. What is to come is drawn
// around what each team has scored a day so far, pulled hard toward the
// league's average until there are weeks of results to go on. Nobody's
// lineup today is used: rosters, lineups and schedules change.
//
// The uncertainty is deliberately wide. Each run gives every team its own
// strength for the season, which then drifts week by week, so the matchups
// furthest out are the least certain. Pairing the leagues' runs gives the
// chance of each franchise taking the overall title.

const (
	simulations = 5000
	// priorDays is how many days of the league's average a team's own
	// results are weighed against: early on everyone looks alike.
	priorDays = 28.0
	// strengthSpread is how far a team's true strength may sit from what
	// its results say before any are in, as a share of it. It narrows as
	// results come in.
	strengthSpread = 0.15
	// drift is how far a team's strength wanders from one week to the next.
	drift = 0.05
	// spread is how far one week's score strays from what was expected, as
	// a share of it.
	spread = 0.25
	// floor and ceiling bound a chance that is not settled for certain.
	floor, ceiling = 0.001, 0.999
)

type Odds struct {
	Leagues []LeagueOdds  `json:"leagues"`
	Overall []OverallOdds `json:"overall"` // empty without an overall title
}

type LeagueOdds struct {
	LeagueID    pgtype.UUID `json:"league_id"`
	Competition string      `json:"competition"`
	Rows        []TeamOdds  `json:"rows"` // in standings order
}

type TeamOdds struct {
	FranchiseID pgtype.UUID `json:"franchise_id"`
	// The chance of a playoff place, 0 to 1. It is exactly 1 or 0 only once
	// a place is clinched or out of reach.
	Playoffs   float64 `json:"playoffs"`
	Clinched   bool    `json:"clinched"`
	Eliminated bool    `json:"eliminated"`
	Wins       float64 `json:"wins"` // projected at the season's end
	Losses     float64 `json:"losses"`
	Place      float64 `json:"place"` // the average final place, 1 being first
}

type OverallOdds struct {
	FranchiseID pgtype.UUID `json:"franchise_id"`
	Title       float64     `json:"title"`  // the chance of winning the overall title
	Points      float64     `json:"points"` // projected overall points
}

// Odds simulates every league's latest season.
func (s *Service) Odds(ctx context.Context, dynasty db.Dynasty) (Odds, error) {
	q := db.New(s.pool)
	out := Odds{Leagues: []LeagueOdds{}, Overall: []OverallOdds{}}
	dynastyRules, err := settings.Parse[settings.Dynasty](dynasty.Settings)
	if err != nil {
		return out, err
	}
	leagues, err := q.ListLeagues(ctx, dynasty.ID)
	if err != nil {
		return out, err
	}

	// Each league's final places in every run, by franchise; the overall
	// title counts the leagues of the latest year.
	type run struct {
		year   int
		active bool
		places map[pgtype.UUID][]int
	}
	var runs []run
	for _, league := range leagues {
		season, err := q.LatestSeason(ctx, league.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, err
		}
		odds, places, err := simulate(ctx, q, league, season)
		if err != nil {
			return out, err
		}
		out.Leagues = append(out.Leagues, odds)
		runs = append(runs, run{int(season.Year), season.Status == "active", places})
	}

	if !dynastyRules.OverallTitle.Enabled || len(runs) == 0 {
		return out, nil
	}
	latest := slices.MaxFunc(runs, func(a, b run) int { return cmp.Compare(a.year, b.year) }).year
	totals := map[pgtype.UUID][]float64{}
	settled := true // every league of the year is over
	for _, r := range runs {
		if r.year != latest {
			continue
		}
		settled = settled && !r.active
		for id, places := range r.places {
			if totals[id] == nil {
				totals[id] = make([]float64, simulations)
			}
			for i, place := range places {
				if place < len(dynastyRules.OverallTitle.PointsByFinish) {
					totals[id][i] += dynastyRules.OverallTitle.PointsByFinish[place]
				}
			}
		}
	}
	titles := map[pgtype.UUID]float64{}
	for i := range simulations {
		best, winners := math.Inf(-1), []pgtype.UUID{}
		for id, t := range totals {
			switch {
			case t[i] > best:
				best, winners = t[i], []pgtype.UUID{id}
			case t[i] == best:
				winners = append(winners, id)
			}
		}
		for _, id := range winners { // a tie for the title is shared
			titles[id] += 1 / float64(len(winners))
		}
	}
	for id, t := range totals {
		sum := 0.0
		for _, v := range t {
			sum += v
		}
		title := titles[id] / simulations
		if !settled && len(totals) > 1 {
			title = min(max(title, floor), ceiling) // still to be decided
		}
		out.Overall = append(out.Overall, OverallOdds{FranchiseID: id, Title: title, Points: sum / simulations})
	}
	slices.SortFunc(out.Overall, func(a, b OverallOdds) int {
		return cmp.Or(cmp.Compare(b.Title, a.Title), cmp.Compare(b.Points, a.Points))
	})
	return out, nil
}

// game is a matchup still to be decided: one to come, or the rest of the
// one in progress.
type game struct {
	home, away           pgtype.UUID
	homeSoFar, awaySoFar float64 // points already scored in it
	daysLeft             float64
	weeksOut             int // 0 for the matchup in progress
}

// simulate plays out the rest of one league's regular season and returns
// the odds and every run's final places (0 being first) by franchise.
func simulate(ctx context.Context, q *db.Queries, league db.League, season db.Season) (LeagueOdds, map[pgtype.UUID][]int, error) {
	odds := LeagueOdds{LeagueID: league.ID, Competition: league.Competition, Rows: []TeamOdds{}}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return odds, nil, err
	}
	rows, err := table(ctx, q, league, &season)
	if err != nil {
		return odds, nil, err
	}
	franchises, err := q.ListFranchises(ctx, league.DynastyID)
	if err != nil {
		return odds, nil, err
	}
	names := map[pgtype.UUID]string{}
	for _, f := range franchises {
		names[f.ID] = f.Name
	}
	headToHead := rules.Format.Type == settings.FormatHeadToHead
	active := season.Status == "active"
	today := sportsday.Today()

	// What has been scored, and over how many days: the truth to build on.
	scored, played := map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}
	var games []game
	var results []result
	if headToHead && active {
		if results, err = regularSeasonResults(ctx, q, league, season); err != nil {
			return odds, nil, err
		}
		periods, err := q.ListPeriods(ctx, season.ID)
		if err != nil {
			return odds, nil, err
		}
		settledScores, err := q.ListPeriodScores(ctx, season.ID)
		if err != nil {
			return odds, nil, err
		}
		for _, s := range settledScores {
			i := slices.IndexFunc(periods, func(p db.Period) bool { return p.ID == s.PeriodID })
			if i >= 0 && !periods[i].IsPlayoff {
				scored[s.FranchiseID] += s.Points
				played[s.FranchiseID] += days(periods[i].StartsOn.Time, periods[i].EndsOn.Time)
			}
		}
		live, current, err := liveResults(ctx, q, league, season)
		if err != nil {
			return odds, nil, err
		}
		if current != nil {
			// Today is half played, on average.
			elapsed := days(current.StartsOn.Time, today) - 0.5
			left := days(today, current.EndsOn.Time) - 0.5
			for _, m := range live {
				games = append(games, game{m.Home, m.Away, m.HomePoints, m.AwayPoints, left, 0})
				for id, points := range map[pgtype.UUID]float64{m.Home: m.HomePoints, m.Away: m.AwayPoints} {
					scored[id] += points
					played[id] += elapsed
				}
			}
		}
		matchups, err := q.ListSeasonMatchups(ctx, season.ID)
		if err != nil {
			return odds, nil, err
		}
		weeks := 0
		for _, p := range periods {
			if p.IsPlayoff || !p.StartsOn.Time.After(today) {
				continue
			}
			weeks++
			n := days(p.StartsOn.Time, p.EndsOn.Time)
			for _, m := range matchups {
				if m.PeriodID == p.ID && m.AwayFranchiseID.Valid {
					games = append(games, game{m.HomeFranchiseID, m.AwayFranchiseID, 0, 0, n, weeks})
				}
			}
		}
	} else if active && !season.StartsOn.Time.After(today) {
		// A total-points league: the season so far.
		n := days(season.StartsOn.Time, today) - 0.5
		for _, r := range rows {
			scored[r.FranchiseID], played[r.FranchiseID] = r.Points, n
		}
	}
	seasonLeft := 0.0
	if !headToHead && active {
		seasonLeft = days(maxTime(today, season.StartsOn.Time), season.EndsOn.Time)
	}

	seed := fnv.New64a()
	seed.Write(league.ID.Bytes[:])
	seed.Write([]byte(today.Format(time.DateOnly)))
	rng := rand.New(rand.NewPCG(seed.Sum64(), 1)) // the same odds all day for the same results
	playoffTeams := 0
	if headToHead {
		playoffTeams = rules.Format.PlayoffTeams
	}
	var places map[pgtype.UUID][]int
	odds.Rows, places = playOut(rows, results, games, strengths(rows, scored, played), seasonLeft, rules.Format, names, playoffTeams, rng)
	if active {
		settle := settled(rows, games, playoffTeams, rules.Format)
		for i := range odds.Rows {
			o := &odds.Rows[i]
			switch settle[o.FranchiseID] {
			case 1:
				o.Playoffs, o.Clinched = 1, true
			case -1:
				o.Playoffs, o.Eliminated = 0, true
			default:
				o.Playoffs = min(max(o.Playoffs, floor), ceiling)
			}
		}
	}
	return odds, places, nil
}

// strength is what a team should score a day, and how sure that is.
type strength struct {
	perDay float64
	spread float64 // the doubt about perDay, as a share of it
}

// strengths weighs each team's points a day against the league's average:
// a few days of results barely move it, a few weeks do.
func strengths(rows []Row, scored, played map[pgtype.UUID]float64) map[pgtype.UUID]strength {
	var points, days float64
	for _, r := range rows {
		points += scored[r.FranchiseID]
		days += played[r.FranchiseID]
	}
	average := 1.0 // with nothing to go on, every team alike
	if points > 0 && days > 0 {
		average = points / days
	}
	out := map[pgtype.UUID]strength{}
	for _, r := range rows {
		n := max(played[r.FranchiseID], 0)
		out[r.FranchiseID] = strength{
			perDay: (scored[r.FranchiseID] + priorDays*average) / (n + priorDays),
			spread: strengthSpread * math.Sqrt(priorDays/(n+priorDays)),
		}
	}
	return out
}

// playOut plays the rest of a season many times from the standings as they
// are. A total-points league has seasonLeft days still to score.
func playOut(rows []Row, results []result, games []game, teams map[pgtype.UUID]strength, seasonLeft float64, format settings.Format, names map[pgtype.UUID]string, playoffTeams int, rng *rand.Rand) ([]TeamOdds, map[pgtype.UUID][]int) {
	weeks := int(math.Ceil(seasonLeft / 7))
	for _, g := range games {
		weeks = max(weeks, g.weeksOut)
	}
	// Each run's strength for every team, week by week from now.
	form := map[pgtype.UUID][]float64{}
	for _, r := range rows {
		form[r.FranchiseID] = make([]float64, weeks+1)
	}
	// A score over n days: one week strays by spread of what is expected, a
	// shorter or longer span by the square root of its length.
	draw := func(id pgtype.UUID, n float64, week int) float64 {
		mean := form[id][week] * n
		return max(0, mean+rng.NormFloat64()*spread*form[id][week]*math.Sqrt(n*7))
	}

	places := map[pgtype.UUID][]int{}
	wins, losses, place := map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}
	playoffs := map[pgtype.UUID]float64{}
	for _, r := range rows {
		places[r.FranchiseID] = make([]int, simulations)
	}
	sim := make([]Row, len(rows))
	for i := range simulations {
		for id, f := range form {
			t := teams[id]
			level := t.perDay * max(0.2, 1+rng.NormFloat64()*t.spread)
			for w := range f {
				if w > 0 {
					level *= max(0.2, 1+rng.NormFloat64()*drift)
				}
				f[w] = level
			}
		}
		copy(sim, rows)
		index := map[pgtype.UUID]*Row{}
		for j := range sim {
			index[sim[j].FranchiseID] = &sim[j]
		}
		played := slices.Clip(results)
		for _, g := range games {
			home, away := index[g.home], index[g.away]
			if home == nil || away == nil {
				continue
			}
			homeAdd, awayAdd := draw(g.home, g.daysLeft, g.weeksOut), draw(g.away, g.daysLeft, g.weeksOut)
			hp, ap := g.homeSoFar+homeAdd, g.awaySoFar+awayAdd
			home.PointsFor += homeAdd
			home.PointsAgainst += awayAdd
			away.PointsFor += awayAdd
			away.PointsAgainst += homeAdd
			switch {
			case hp > ap:
				home.Wins++
				away.Losses++
			case ap > hp:
				away.Wins++
				home.Losses++
			default:
				home.Ties++
				away.Ties++
			}
			played = append(played, result{g.home, g.away, hp, ap})
		}
		for j := range sim {
			for w, left := 0, seasonLeft; left > 0; w, left = w+1, left-7 {
				sim[j].Points += draw(sim[j].FranchiseID, min(left, 7), w)
			}
		}
		rankStandings(sim, played, format, names)
		for p, r := range sim {
			places[r.FranchiseID][i] = p
			wins[r.FranchiseID] += float64(r.Wins)
			losses[r.FranchiseID] += float64(r.Losses)
			place[r.FranchiseID] += float64(p + 1)
			if p < playoffTeams {
				playoffs[r.FranchiseID]++
			}
		}
	}
	odds := make([]TeamOdds, 0, len(rows))
	for _, r := range rows {
		id := r.FranchiseID
		odds = append(odds, TeamOdds{
			FranchiseID: id, Playoffs: playoffs[id] / simulations,
			Wins: wins[id] / simulations, Losses: losses[id] / simulations, Place: place[id] / simulations,
		})
	}
	return odds, places
}

// settled says which teams are certain of a playoff place (1) and which
// cannot reach one (-1), whatever happens in the matchups left. Only a
// league ranked by record can be settled early; one ranked by points or
// win percentage is settled only when every team makes it.
func settled(rows []Row, games []game, playoffTeams int, format settings.Format) map[pgtype.UUID]int {
	out := map[pgtype.UUID]int{}
	if playoffTeams <= 0 {
		return out
	}
	if len(rows) <= playoffTeams {
		for _, r := range rows {
			out[r.FranchiseID] = 1
		}
		return out
	}
	if format.Ranking() != "record" {
		return out
	}
	left := map[pgtype.UUID]int{}
	for _, g := range games {
		left[g.home]++
		left[g.away]++
	}
	for _, r := range rows {
		id := r.FranchiseID
		// Clinched: fewer than playoffTeams others could match the wins it
		// already has. Out: playoffTeams others already have more wins than
		// it can reach.
		catch, ahead := 0, 0
		for _, o := range rows {
			if o.FranchiseID == id {
				continue
			}
			if o.Wins+left[o.FranchiseID] >= r.Wins {
				catch++
			}
			if o.Wins > r.Wins+left[id] {
				ahead++
			}
		}
		switch {
		case catch < playoffTeams:
			out[id] = 1
		case ahead >= playoffTeams:
			out[id] = -1
		}
	}
	return out
}

// days counts the days from one to another, both included.
func days(from, to time.Time) float64 {
	return max(0, math.Round(to.Sub(from).Hours()/24)+1)
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
