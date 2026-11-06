package nbamodel

import (
	"fmt"
	"math"
	"slices"
	"time"
)

type Linear struct {
	Coefficients []float64 `json:"coefficients"`
}

func (m Linear) predict(features []float64) float64 {
	var value float64
	for i, x := range features {
		value += m.Coefficients[i] * x
	}
	return value
}

type moments struct {
	xx     [][]float64
	xy     []float64
	weight float64
}

func newMoments(n int) *moments {
	m := &moments{xx: make([][]float64, n), xy: make([]float64, n)}
	for i := range m.xx {
		m.xx[i] = make([]float64, n)
	}
	return m
}

func (m *moments) add(x []float64, y, weight float64) {
	m.weight += weight
	for i, a := range x {
		m.xy[i] += weight * a * y
		for j, b := range x {
			m.xx[i][j] += weight * a * b
		}
	}
}

func (m *moments) fit(ridge float64) (Linear, error) {
	if m.weight == 0 {
		return Linear{}, fmt.Errorf("no training examples")
	}
	n := len(m.xy)
	a := make([][]float64, n)
	for i := range a {
		a[i] = make([]float64, n+1)
		for j := 0; j < n; j++ {
			a[i][j] = m.xx[i][j] / m.weight
		}
		a[i][i] += ridge
		a[i][n] = m.xy[i] / m.weight
	}
	for col := 0; col < n; col++ {
		pivot := col
		for i := col + 1; i < n; i++ {
			if math.Abs(a[i][col]) > math.Abs(a[pivot][col]) {
				pivot = i
			}
		}
		if math.Abs(a[pivot][col]) < 1e-12 {
			return Linear{}, fmt.Errorf("singular training matrix")
		}
		a[col], a[pivot] = a[pivot], a[col]
		v := a[col][col]
		for j := col; j <= n; j++ {
			a[col][j] /= v
		}
		for i := 0; i < n; i++ {
			if i == col {
				continue
			}
			v = a[i][col]
			for j := col; j <= n; j++ {
				a[i][j] -= v * a[col][j]
			}
		}
	}
	result := Linear{Coefficients: make([]float64, n)}
	for i := range result.Coefficients {
		result.Coefficients[i] = a[i][n]
	}
	return result, nil
}

type Model struct {
	Version          int                `json:"version"`
	Config           Config             `json:"config"`
	TrainedBefore    time.Time          `json:"trained_before"`
	TrainingExamples int                `json:"training_examples"`
	MinuteFeatures   []string           `json:"minute_features"`
	RateFeatures     []string           `json:"rate_features"`
	Minutes          Linear             `json:"minutes"`
	Rates            map[string]Linear  `json:"rates"`
	Scoring          map[string]float64 `json:"selection_scoring"`
}

type Prediction struct {
	Minutes       float64            `json:"minutes"`
	Stats         map[string]float64 `json:"stats"`
	FantasyPoints float64            `json:"fantasy_points_if_active"`
}

func (m Model) Validate() error {
	if (m.Version != Version && m.Version != 1) || !slices.Equal(m.MinuteFeatures, minuteFeatureNames(m.Config.Features)) || !slices.Equal(m.RateFeatures, rateFeatureNames(m.Config.Features)) {
		return fmt.Errorf("unsupported model version or feature schema")
	}
	if m.Config.HalfLife <= 0 || m.Config.PriorMinutes <= 0 || m.Config.Ridge <= 0 || m.Config.SeasonDecay <= 0 || m.Config.SeasonDecay > 1 || m.TrainedBefore.IsZero() {
		return fmt.Errorf("invalid model configuration")
	}
	check := func(l Linear, n int) bool {
		if len(l.Coefficients) != n {
			return false
		}
		for _, v := range l.Coefficients {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
		}
		return true
	}
	if !check(m.Minutes, len(m.MinuteFeatures)) {
		return fmt.Errorf("invalid minute coefficients")
	}
	for _, stat := range Stats {
		if !check(m.Rates[stat], len(m.RateFeatures)) {
			return fmt.Errorf("invalid %s coefficients", stat)
		}
	}
	return nil
}

func (m Model) Predict(f Features, scoring map[string]float64) Prediction {
	minutes := min(48, max(2, f.BaseMinutes+12*m.Minutes.predict(f.Minutes)))
	return m.project(f, minutes, scoring)
}

func (m Model) project(f Features, minutes float64, scoring map[string]float64) Prediction {
	p := Prediction{Minutes: minutes, Stats: map[string]float64{}}
	for _, stat := range Stats {
		rate := max(0, f.BaseRates[stat]+f.Scales[stat]*m.Rates[stat].predict(f.Rates[stat]))
		p.Stats[stat] = rate * p.Minutes
	}
	if m.Config.Features.Opportunities && f.Shooting != nil {
		x := f.Shooting
		two := p.Stats["fga"] * (1 - x.ThreeShare) * x.TwoPct
		three := p.Stats["fga"] * x.ThreeShare * x.ThreePct
		p.Stats["tpm"] = three
		p.Stats["fgm"] = two + three
		p.Stats["ftm"] = p.Stats["fta"] * x.FreePct
		p.Stats["pts"] = 2*two + 3*three + p.Stats["ftm"]
	}
	for _, stat := range []string{"dd", "td", "pts40", "pts50"} {
		p.Stats[stat] = min(1, p.Stats[stat])
	}
	p.Stats["td"] = min(p.Stats["td"], p.Stats["dd"])
	p.Stats["pts50"] = min(p.Stats["pts50"], p.Stats["pts40"])
	p.FantasyPoints = Score(p.Stats, scoring)
	return p
}

type training struct {
	minutes *moments
	rates   map[string]*moments
	n       int
}

func newTraining(c Config) *training {
	t := &training{minutes: newMoments(len(minuteFeatureNames(c.Features))), rates: map[string]*moments{}}
	for _, stat := range Stats {
		t.rates[stat] = newMoments(len(rateFeatureNames(c.Features)))
	}
	return t
}

func (t *training) add(row Row, f Features, latestSeason int) {
	weight := math.Pow(.65, float64(max(0, latestSeason-row.Season)))
	t.minutes.add(f.Minutes, (row.Minutes-f.BaseMinutes)/12, weight)
	for _, stat := range Stats {
		t.rates[stat].add(f.Rates[stat], (row.Stats[stat]/row.Minutes-f.BaseRates[stat])/f.Scales[stat], weight*row.Minutes/30)
	}
	t.n++
}

func (t *training) fit(c Config, before time.Time, scoring map[string]float64) (Model, error) {
	m := Model{Version: Version, Config: c, TrainedBefore: before, TrainingExamples: t.n, MinuteFeatures: minuteFeatureNames(c.Features), RateFeatures: rateFeatureNames(c.Features), Rates: map[string]Linear{}, Scoring: scoring}
	var err error
	if m.Minutes, err = t.minutes.fit(c.Ridge); err != nil {
		return m, err
	}
	for _, stat := range Stats {
		if m.Rates[stat], err = t.rates[stat].fit(c.Ridge); err != nil {
			return m, err
		}
	}
	return m, m.Validate()
}

type Candidate struct {
	Config     Config  `json:"config"`
	Validation Metrics `json:"validation"`
}

// FitBefore refits an already specified configuration without selecting any
// hyperparameters. Useful for chronological calibration forecasts: the fitted
// model cannot see the validation outcomes it will be asked to predict.
func FitBefore(rows []Row, c Config, before time.Time, scoring map[string]float64) (Model, error) {
	if i := slices.IndexFunc(rows, func(r Row) bool { return !sportsdayOf(r.StartsAt).Before(before) }); i >= 0 {
		rows = rows[:i]
	}
	latest := before.Year()
	if before.Month() >= time.July {
		latest++
	}
	t := newTraining(c)
	err := Walk(rows, c, func(r Row, f Features, _ *State) error {
		if r.Played && r.Minutes > 0 && f.HistoryGames >= 3 {
			t.add(r, f, latest)
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return t.fit(c, before, scoring)
}

// Train selects hyperparameters on [trainBefore, testFrom), then refits the
// selected configuration on all examples strictly before testFrom. No held-out
// test labels influence coefficients, hyperparameters, or scoring selection.
func Train(rows []Row, trainBefore, testFrom time.Time, scoring map[string]float64, progress func(string)) (Model, []Candidate, error) {
	return TrainVariant(rows, trainBefore, testFrom, scoring, FeatureSet{}, progress)
}

func TrainVariant(rows []Row, trainBefore, testFrom time.Time, scoring map[string]float64, features FeatureSet, progress func(string)) (Model, []Candidate, error) {
	if !trainBefore.Before(testFrom) {
		return Model{}, nil, fmt.Errorf("train-before must precede test-from")
	}
	// Do not even build features from held-out outcomes during fitting.
	if i := slices.IndexFunc(rows, func(row Row) bool { return !sportsdayOf(row.StartsAt).Before(testFrom) }); i >= 0 {
		rows = rows[:i]
	}
	var candidates []Candidate
	bestMAE := math.Inf(1)
	var best Model
	latestSeason := testFrom.Year()
	if testFrom.Month() >= time.July {
		latestSeason++
	}
	for _, halfLife := range []float64{10, 25} {
		c := Config{HalfLife: halfLife, PriorMinutes: 240, SeasonDecay: .65, Ridge: .01, Features: features}
		initial, combined := newTraining(c), newTraining(c)
		var models []Model
		scores := []Metrics{{}, {}, {}}
		fit := func() error {
			if len(models) > 0 {
				return nil
			}
			if initial.n < 100 {
				return fmt.Errorf("need at least 100 training appearances before %s; found %d", trainBefore.Format(time.DateOnly), initial.n)
			}
			for _, ridge := range []float64{.001, .01, .1} {
				config := c
				config.Ridge = ridge
				m, err := initial.fit(config, trainBefore, scoring)
				if err != nil {
					return err
				}
				models = append(models, m)
			}
			return nil
		}
		err := Walk(rows, c, func(row Row, f Features, state *State) error {
			day := sportsdayOf(row.StartsAt)
			if !day.Before(testFrom) {
				return nil
			}
			if !row.Played || row.Minutes <= 0 {
				return nil
			}
			if day.Before(trainBefore) {
				if f.HistoryGames >= 3 {
					initial.add(row, f, latestSeason)
				}
			} else {
				if err := fit(); err != nil {
					return err
				}
				for i, m := range models {
					scores[i].Add(m.Forecast(row.Target(), f, state, scoring).FantasyPoints, Score(row.Stats, scoring))
				}
			}
			if f.HistoryGames >= 3 {
				combined.add(row, f, latestSeason)
			}
			return nil
		})
		if err != nil {
			return Model{}, nil, err
		}
		if len(models) == 0 || scores[0].N < 100 {
			return Model{}, nil, fmt.Errorf("need at least 100 validation appearances in [%s, %s)", trainBefore.Format(time.DateOnly), testFrom.Format(time.DateOnly))
		}
		for i, m := range models {
			score := scores[i].Final()
			candidates = append(candidates, Candidate{m.Config, score})
			if progress != nil {
				progress(fmt.Sprintf("half-life %.0f, ridge %.3f: validation MAE %.3f (%d appearances)", halfLife, m.Config.Ridge, score.MAE, score.N))
			}
			if score.MAE < bestMAE {
				bestMAE = score.MAE
				best, err = combined.fit(m.Config, testFrom, scoring)
				if err != nil {
					return Model{}, nil, err
				}
			}
		}
	}
	return best, candidates, nil
}
