package nbamodel

import (
	"math"
	"time"

	"crossover/internal/projection"
	"crossover/internal/sportsday"
)

// Hyperparameters are selected on the chronological validation window only.
type Config struct {
	HalfLife     float64    `json:"half_life_games"`
	PriorMinutes float64    `json:"prior_minutes"`
	Ridge        float64    `json:"ridge"`
	SeasonDecay  float64    `json:"season_decay"`
	Features     FeatureSet `json:"features,omitempty"`
}

var MinuteFeatures = []string{"intercept", "recent_3_minutes", "recent_10_minutes", "season_minutes", "previous_season_minutes", "recent_start_share", "back_to_back", "home", "season_experience", "recent_team_change", "days_since_last_game"}
var RateFeatures = []string{"intercept", "baseline_rate", "recent_rate_delta", "season_rate_delta", "opponent_position_delta", "last_meeting_delta", "home", "back_to_back", "minutes_trend", "recent_start_share", "season_experience", "recent_team_change"}

type total struct {
	Games, Minutes float64
	Stats          map[string]float64
}

func (t *total) add(r Row) {
	if t.Stats == nil {
		t.Stats = map[string]float64{}
	}
	t.Games++
	t.Minutes += r.Minutes
	for _, stat := range Stats {
		t.Stats[stat] += r.Stats[stat]
	}
	t.Stats["tpa"] += r.Stats["tpa"]
}

type defense struct {
	Minutes          float64
	Actual, Expected map[string]float64
}

type State struct {
	players   map[string][]Row
	seasons   map[int]map[string]*total
	positions map[int]map[string]*total
	defenses  map[int]map[string]*defense
	team      *teamState
	reports   map[string]*PregameReport
}

func NewState() *State {
	return &State{players: map[string][]Row{}, seasons: map[int]map[string]*total{}, positions: map[int]map[string]*total{}, defenses: map[int]map[string]*defense{}}
}

type Features struct {
	Minutes            []float64            `json:"minutes_features"`
	Rates              map[string][]float64 `json:"rate_features"`
	BaseMinutes        float64              `json:"baseline_minutes"`
	BaseRates          map[string]float64   `json:"baseline_rates"`
	Scales             map[string]float64   `json:"rate_scales"`
	Position           string               `json:"position"`
	HistoryGames       int                  `json:"history_games"`
	SeasonGames        int                  `json:"season_games"`
	LastMeetingMinutes float64              `json:"last_meeting_minutes"`
	OpponentMinutes    float64              `json:"opponent_position_minutes"`
	OpponentFactors    map[string]float64   `json:"opponent_position_factors"`
	RecentPoints       map[string]float64   `json:"recent_10_stats"`
	Context            *ContextFeatures     `json:"context,omitempty"`
	Shooting           *ShootingFeatures    `json:"shooting,omitempty"`
	Official           *OfficialFeatures    `json:"official_report,omitempty"`
}

func (s *State) priors(season int, pos string) (float64, map[string]float64) {
	var games, minutes float64
	stats := map[string]float64{}
	for year := season - 2; year <= season; year++ {
		t := s.positions[year][pos]
		if t == nil || t.Minutes < 300 {
			t = s.positions[year]["ALL"]
		}
		if t == nil {
			continue
		}
		w := math.Pow(0.5, float64(season-year))
		games += w * t.Games
		minutes += w * t.Minutes
		for _, stat := range Stats {
			stats[stat] += w * t.Stats[stat]
		}
	}
	// Used only at the very beginning of the dataset, before league evidence.
	fallback := map[string]float64{"pts": .5, "reb": .2, "ast": .1, "stl": .03, "blk": .02, "tov": .06, "tpm": .05, "fgm": .18, "fga": .4, "ftm": .09, "fta": .12, "dd": .002, "td": .0001, "pts40": .0005, "pts50": .0001}
	if minutes == 0 || games == 0 {
		return 20, fallback
	}
	for _, stat := range Stats {
		stats[stat] /= minutes
	}
	return minutes / games, stats
}

func (s *State) Build(t Target, c Config) Features {
	history := s.players[t.PlayerID]
	f := Features{Position: "UNK", Rates: map[string][]float64{}, BaseRates: map[string]float64{}, Scales: map[string]float64{}, RecentPoints: map[string]float64{}, OpponentFactors: map[string]float64{}}
	if len(history) > 0 {
		f.Position = Position(history[len(history)-1].Position)
	}
	priorMinutes, priorRates := s.priors(t.Season, f.Position)
	var weightedMinutes, weight, recentMinutes, seasonMinutes, previousMinutes, previousGames, recentGames, starts, last3Minutes, last3Games, changed float64
	weighted, recent, seasonStats := map[string]float64{}, map[string]float64{}, map[string]float64{}
	var last *Row
	var meeting *Row
	for i := len(history) - 1; i >= 0; i-- {
		r := &history[i]
		if r.Season < t.Season-2 {
			break
		}
		age := len(history) - 1 - i
		w := math.Exp2(-float64(age)/c.HalfLife) * math.Pow(c.SeasonDecay, float64(t.Season-r.Season))
		weight += w
		weightedMinutes += w * r.Minutes
		for _, stat := range Stats {
			weighted[stat] += w * r.Stats[stat]
		}
		f.HistoryGames++
		if last == nil {
			last = r
		}
		if r.Opponent == t.Opponent && meeting == nil && t.StartsAt.Sub(r.StartsAt) <= 365*24*time.Hour {
			meeting = r
		}
		if age < 10 {
			recentGames++
			recentMinutes += r.Minutes
			if r.Starter {
				starts++
			}
			if r.Team != t.Team {
				changed++
			}
			for _, stat := range Stats {
				recent[stat] += r.Stats[stat]
			}
		}
		if age < 3 {
			last3Games++
			last3Minutes += r.Minutes
		}
		if r.Season == t.Season {
			f.SeasonGames++
			seasonMinutes += r.Minutes
			for _, stat := range Stats {
				seasonStats[stat] += r.Stats[stat]
			}
		}
		if r.Season == t.Season-1 {
			previousGames++
			previousMinutes += r.Minutes
		}
	}
	base := (priorMinutes*5 + weightedMinutes) / (5 + weight)
	f.BaseMinutes = base
	mean := func(sum, n float64) float64 {
		if n == 0 {
			return base
		}
		return sum / n
	}
	recent3 := mean(last3Minutes, last3Games)
	recent10 := mean(recentMinutes, recentGames)
	seasonMean := mean(seasonMinutes, float64(f.SeasonGames))
	previousMean := mean(previousMinutes, previousGames)
	var startShare, teamChange, backToBack, home, days float64
	if recentGames > 0 {
		startShare = starts / recentGames
		teamChange = changed / recentGames
	}
	if last != nil {
		days = min(14, max(0, t.StartsAt.Sub(last.StartsAt).Hours()/24))
		if days < 1.6 {
			backToBack = 1
		}
	}
	if t.Home {
		home = 1
	}
	experience := float64(f.SeasonGames) / (float64(f.SeasonGames) + 15)
	f.Minutes = []float64{1, recent3 / 36, recent10 / 36, seasonMean / 36, previousMean / 36, startShare, backToBack, home, experience, teamChange, days / 14}
	def := s.defenses[t.Season][t.Opponent+"/"+f.Position]
	if def != nil {
		f.OpponentMinutes = def.Minutes
	}
	if meeting != nil {
		f.LastMeetingMinutes = meeting.Minutes
	}
	for _, stat := range Stats {
		prior := priorRates[stat]
		scale := max(prior, .001)
		baseRate := (weighted[stat] + c.PriorMinutes*prior) / (weightedMinutes + c.PriorMinutes)
		f.BaseRates[stat] = baseRate
		f.Scales[stat] = scale
		recentDelta, seasonDelta, meetingDelta, defenseDelta := 0.0, 0.0, 0.0, 0.0
		if recentMinutes > 0 {
			recentDelta = (recent[stat]/recentMinutes - baseRate) / scale
		}
		if seasonMinutes > 0 {
			seasonDelta = (seasonStats[stat]/seasonMinutes - baseRate) / scale * experience
		}
		factor := 1.0
		if def != nil {
			// Compare opponents' actual production with their own pregame
			// baseline, rather than confusing a difficult schedule with defense.
			factor = (def.Actual[stat] + 1000*scale) / (def.Expected[stat] + 1000*scale)
			factor = min(1.5, max(.5, factor))
			defenseDelta = (factor - 1) * baseRate / scale
		}
		f.OpponentFactors[stat] = factor
		if meeting != nil && meeting.Minutes > 0 {
			meetingDelta = (meeting.Stats[stat]/meeting.Minutes - baseRate) / scale * meeting.Minutes / (meeting.Minutes + 120)
		}
		f.Rates[stat] = []float64{1, baseRate / scale, recentDelta, seasonDelta, defenseDelta, meetingDelta, home, backToBack, (recent3 - recent10) / 36, startShare, experience, teamChange}
		if recentGames > 0 {
			f.RecentPoints[stat] = recent[stat] / recentGames
		} else {
			f.RecentPoints[stat] = prior * priorMinutes
		}
	}
	if c.Features.StatWindows {
		s.statSpecificRates(t, c, &f)
	}
	if c.Features != (FeatureSet{}) {
		s.extend(t, c, &f)
	}
	return f
}

// ObserveDay is called only after every prediction on the sports day has been
// made. This excludes the target game and all other same-day outcomes.
func (s *State) ObserveDay(rows []Row, features []Features) {
	s.observeTeams(rows)
	for i, r := range rows {
		if !r.Played || r.Minutes <= 0 {
			continue
		}
		if s.seasons[r.Season] == nil {
			s.seasons[r.Season] = map[string]*total{}
			s.positions[r.Season] = map[string]*total{}
			s.defenses[r.Season] = map[string]*defense{}
		}
		if s.seasons[r.Season][r.PlayerID] == nil {
			s.seasons[r.Season][r.PlayerID] = &total{}
		}
		s.seasons[r.Season][r.PlayerID].add(r)
		for _, pos := range []string{Position(r.Position), "ALL"} {
			if s.positions[r.Season][pos] == nil {
				s.positions[r.Season][pos] = &total{}
			}
			s.positions[r.Season][pos].add(r)
		}
		key := r.Opponent + "/" + Position(r.Position)
		if s.defenses[r.Season][key] == nil {
			s.defenses[r.Season][key] = &defense{Actual: map[string]float64{}, Expected: map[string]float64{}}
		}
		d := s.defenses[r.Season][key]
		// Cold-start guesses should not teach the opponent defense feature.
		if features[i].HistoryGames >= 5 {
			d.Minutes += r.Minutes
			for _, stat := range Stats {
				d.Actual[stat] += r.Stats[stat]
				d.Expected[stat] += features[i].BaseRates[stat] * r.Minutes
			}
		}
		s.players[r.PlayerID] = append(s.players[r.PlayerID], r)
		if len(s.players[r.PlayerID]) > 180 {
			s.players[r.PlayerID] = s.players[r.PlayerID][1:]
		}
	}
}

func (s *State) Proxy(t Target, scoring map[string]float64) (float64, float64) {
	lengths := map[int]float64{}
	var seasons []projection.Season
	for year := t.Season - 1; year <= t.Season; year++ {
		for _, p := range s.seasons[year] {
			lengths[year] = max(lengths[year], p.Games)
		}
		if p := s.seasons[year][t.PlayerID]; p != nil {
			seasons = append(seasons, projection.Season{Year: year, Games: p.Games, Stats: p.Stats})
		}
	}
	stats, plays := projection.Baseline(seasons, lengths)
	conditional := projection.Points(stats, 1, projection.Game{}, scoring, nil)
	return conditional * plays, conditional
}

func Walk(rows []Row, c Config, visit func(Row, Features, *State) error) error {
	s := NewState()
	for first := 0; first < len(rows); {
		last := first + 1
		day := sportsday.Of(rows[first].StartsAt)
		for last < len(rows) && sportsday.Of(rows[last].StartsAt).Equal(day) {
			last++
		}
		features := make([]Features, last-first)
		if s.reports == nil {
			s.reports = map[string]*PregameReport{}
		}
		for _, r := range rows[first:last] {
			if r.Pregame != nil {
				s.reports[r.GameID] = r.Pregame
			}
		}
		for i := first; i < last; i++ {
			features[i-first] = s.Build(rows[i].Target(), c)
			if err := visit(rows[i], features[i-first], s); err != nil {
				return err
			}
		}
		s.ObserveDay(rows[first:last], features)
		first = last
	}
	return nil
}
