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
// where it stands: the record so far, the matchup in progress, and the
// schedule still to come. A team's score in a matchup is drawn around what
// it should score, which blends what it has scored per day so far with what
// its lineup is projected to score, leaning on the projection early and on
// the results as they build up. Pairing the leagues' runs gives the chance
// of each franchise taking the overall title.

// Projector is what a franchise's starters are projected to add between
// two days, from today on.
type Projector func(ctx context.Context, league db.League, franchiseID pgtype.UUID, from, to time.Time) (float64, error)

const (
	simulations = 5000
	// projectionDays is how many days of results count as much as the
	// projection does.
	projectionDays = 14.0
	// spread is how far a week's score strays from what was expected, as a
	// share of it, until there are results enough to measure it.
	spread = 0.22
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
	Playoffs    float64     `json:"playoffs"` // the chance of a playoff place, 0 to 1
	Wins        float64     `json:"wins"`     // projected at the season's end
	Losses      float64     `json:"losses"`
	Place       float64     `json:"place"` // the average final place, 1 being first
}

type OverallOdds struct {
	FranchiseID pgtype.UUID `json:"franchise_id"`
	Title       float64     `json:"title"`  // the chance of winning the overall title
	Points      float64     `json:"points"` // projected overall points
}

// Odds simulates every league's latest season.
func (s *Service) Odds(ctx context.Context, dynasty db.Dynasty, project Projector) (Odds, error) {
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
		odds, places, err := simulate(ctx, q, league, season, project)
		if err != nil {
			return out, err
		}
		out.Leagues = append(out.Leagues, odds)
		runs = append(runs, run{int(season.Year), places})
	}

	if !dynastyRules.OverallTitle.Enabled || len(runs) == 0 {
		return out, nil
	}
	latest := slices.MaxFunc(runs, func(a, b run) int { return cmp.Compare(a.year, b.year) }).year
	totals := map[pgtype.UUID][]float64{}
	for _, r := range runs {
		if r.year != latest {
			continue
		}
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
		out.Overall = append(out.Overall, OverallOdds{FranchiseID: id, Title: titles[id] / simulations, Points: sum / simulations})
	}
	slices.SortFunc(out.Overall, func(a, b OverallOdds) int {
		return cmp.Or(cmp.Compare(b.Title, a.Title), cmp.Compare(b.Points, a.Points))
	})
	return out, nil
}

// simulate plays out the rest of one league's regular season and returns
// the odds and every run's final places (0 being first) by franchise.
func simulate(ctx context.Context, q *db.Queries, league db.League, season db.Season, project Projector) (LeagueOdds, map[pgtype.UUID][]int, error) {
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
	today := sportsday.Today()

	var games []game
	var results []result
	var next *db.Period // the first matchup not yet begun, to project a full one
	var current *db.Period
	if headToHead && season.Status == "active" {
		if results, err = regularSeasonResults(ctx, q, league, season); err != nil {
			return odds, nil, err
		}
		live, period, err := liveResults(ctx, q, league, season)
		if err != nil {
			return odds, nil, err
		}
		current = period
		if period != nil {
			left := days(today, period.EndsOn.Time)
			for _, m := range live {
				games = append(games, game{m.Home, m.Away, m.HomePoints, m.AwayPoints, left})
			}
		}
		periods, err := q.ListPeriods(ctx, season.ID)
		if err != nil {
			return odds, nil, err
		}
		matchups, err := q.ListSeasonMatchups(ctx, season.ID)
		if err != nil {
			return odds, nil, err
		}
		for _, p := range periods {
			if p.IsPlayoff || !p.StartsOn.Time.After(today) {
				continue
			}
			if next == nil {
				next = &p
			}
			n := days(p.StartsOn.Time, p.EndsOn.Time)
			for _, m := range matchups {
				if m.PeriodID == p.ID && m.AwayFranchiseID.Valid {
					games = append(games, game{m.HomeFranchiseID, m.AwayFranchiseID, 0, 0, n})
				}
			}
		}
	}

	// What each franchise should score a day, and how far a matchup strays.
	perDay, sd, err := strength(ctx, q, league, season, rows, current, next, project)
	if err != nil {
		return odds, nil, err
	}
	seasonLeft := 0.0
	if !headToHead && season.Status == "active" {
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
	odds.Rows, places = playOut(rows, results, games, perDay, sd, seasonLeft, rules.Format, names, playoffTeams, rng)
	return odds, places, nil
}

// game is a matchup still to be decided: one to come, or the rest of the
// one in progress.
type game struct {
	home, away           pgtype.UUID
	homeSoFar, awaySoFar float64 // points already scored in it
	daysLeft             float64
}

// playOut plays the rest of a season many times from the standings as they
// are. A total-points league has seasonLeft days still to score.
func playOut(rows []Row, results []result, games []game, perDay map[pgtype.UUID]float64, sd, seasonLeft float64, format settings.Format, names map[pgtype.UUID]string, playoffTeams int, rng *rand.Rand) ([]TeamOdds, map[pgtype.UUID][]int) {
	// A score over n days: a week strays by sd of what is expected, a
	// shorter or longer span by the square root of its length.
	draw := func(id pgtype.UUID, n float64) float64 {
		return max(0, perDay[id]*n+rng.NormFloat64()*sd*perDay[id]*math.Sqrt(n*7))
	}
	places := map[pgtype.UUID][]int{}
	wins, losses, place := map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}
	playoffs := map[pgtype.UUID]float64{}
	for _, r := range rows {
		places[r.FranchiseID] = make([]int, simulations)
	}
	sim := make([]Row, len(rows))
	for i := range simulations {
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
			homeAdd, awayAdd := draw(g.home, g.daysLeft), draw(g.away, g.daysLeft)
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
		if seasonLeft > 0 {
			for j := range sim {
				sim[j].Points += draw(sim[j].FranchiseID, seasonLeft)
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

// strength is what each franchise should score a day, and how far a week's
// score strays from that, as a share of it.
func strength(ctx context.Context, q *db.Queries, league db.League, season db.Season, rows []Row, current, next *db.Period, project Projector) (map[pgtype.UUID]float64, float64, error) {
	today := sportsday.Today()
	// What has been scored, and over how many days.
	scored, played := map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}
	weekly := map[pgtype.UUID][]float64{} // per-day scores, matchup by matchup
	periods, err := q.ListPeriods(ctx, season.ID)
	if err != nil {
		return nil, 0, err
	}
	if len(periods) > 0 {
		settled, err := q.ListPeriodScores(ctx, season.ID)
		if err != nil {
			return nil, 0, err
		}
		for _, s := range settled {
			i := slices.IndexFunc(periods, func(p db.Period) bool { return p.ID == s.PeriodID })
			if i < 0 || periods[i].IsPlayoff {
				continue
			}
			n := days(periods[i].StartsOn.Time, periods[i].EndsOn.Time)
			scored[s.FranchiseID] += s.Points
			played[s.FranchiseID] += n
			weekly[s.FranchiseID] = append(weekly[s.FranchiseID], s.Points/n)
		}
	} else if season.Status == "active" && !season.StartsOn.Time.After(today) {
		// A total-points league: the season so far, up to yesterday.
		n := days(season.StartsOn.Time, today) - 1
		for _, r := range rows {
			if n > 0 {
				scored[r.FranchiseID], played[r.FranchiseID] = r.Points, n
			}
		}
	}

	// What each lineup is projected to score a day: over the next whole
	// matchup, or failing that the rest of this one, or the coming week.
	from, to := today, today.AddDate(0, 0, 6)
	switch {
	case next != nil:
		from, to = next.StartsOn.Time, next.EndsOn.Time
	case current != nil:
		to = current.EndsOn.Time
	}
	span := days(from, to)
	perDay := map[pgtype.UUID]float64{}
	sum, counted := 0.0, 0
	for _, r := range rows {
		id := r.FranchiseID
		projected := 0.0
		if season.Status == "active" {
			if projected, err = project(ctx, league, id, from, to); err != nil {
				return nil, 0, err
			}
		}
		actual, n := scored[id]/max(played[id], 1), played[id]
		switch {
		case projected <= 0 && n == 0:
			continue
		case projected <= 0:
			perDay[id] = actual
		default:
			perDay[id] = (actual*n + projected/span*projectionDays) / (n + projectionDays)
		}
		sum += perDay[id]
		counted++
	}
	// Anyone with nothing to go on is taken as the league's average team.
	average := 1.0
	if counted > 0 && sum > 0 {
		average = sum / float64(counted)
	}
	for _, r := range rows {
		if perDay[r.FranchiseID] <= 0 {
			perDay[r.FranchiseID] = average
		}
	}

	// How far one matchup strays from a team's own average, once teams have
	// played a few.
	var squares float64
	var samples int
	for _, scores := range weekly {
		if len(scores) < 2 {
			continue
		}
		mean := 0.0
		for _, s := range scores {
			mean += s
		}
		mean /= float64(len(scores))
		for _, s := range scores {
			if mean > 0 {
				squares += math.Pow(s/mean-1, 2)
				samples++
			}
		}
	}
	sd := spread
	if samples >= 10 {
		sd = min(max(math.Sqrt(squares/float64(samples)), 0.1), 0.4)
	}
	return perDay, sd, nil
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
