package nbamodel

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

type Metrics struct {
	N                         int     `json:"n"`
	MAE                       float64 `json:"mae"`
	RMSE                      float64 `json:"rmse"`
	Bias                      float64 `json:"bias"`
	absolute, squared, signed float64
}

func (m *Metrics) Add(predicted, actual float64) {
	delta := predicted - actual
	m.N++
	m.absolute += math.Abs(delta)
	m.squared += delta * delta
	m.signed += delta
}

func (m Metrics) Final() Metrics {
	if m.N > 0 {
		m.MAE = m.absolute / float64(m.N)
		m.RMSE = math.Sqrt(m.squared / float64(m.N))
		m.Bias = m.signed / float64(m.N)
	}
	return m
}

type Evaluation struct {
	From                time.Time          `json:"from"`
	To                  time.Time          `json:"to_exclusive"`
	Games               int                `json:"games"`
	SkippedDNP          int                `json:"skipped_dnp_or_zero_minutes"`
	SkippedOriginRoster int                `json:"skipped_unknown_frozen_origin_roster"`
	Model               Metrics            `json:"model"`
	Proxy               Metrics            `json:"proxy_without_archived_market_or_injuries"`
	ProxyIfActive       Metrics            `json:"proxy_if_active"`
	Recent10            Metrics            `json:"recent_10"`
	Minutes             Metrics            `json:"minutes"`
	RecentMinutes       Metrics            `json:"recent_10_minutes"`
	Stats               map[string]Metrics `json:"stats"`
	Groups              map[string]Metrics `json:"groups"`
	Notes               []string           `json:"notes"`
}

type EvaluationOptions struct {
	From, To           time.Time
	GameIDs, PlayerIDs map[string]bool
	Predictions        io.Writer
	Errors             io.Writer
	Diagnostics        bool
	FreezeWeek         bool
	RankFeatures       bool
}

type Result struct {
	Target        Target             `json:"target"`
	Name          string             `json:"name"`
	ActualMinutes float64            `json:"actual_minutes"`
	ActualStats   map[string]float64 `json:"actual_stats"`
	ActualPoints  float64            `json:"actual_points"`
	Prediction    Prediction         `json:"prediction"`
	Proxy         float64            `json:"proxy"`
	ProxyIfActive float64            `json:"proxy_if_active"`
	Recent10      float64            `json:"recent_10"`
	Features      Features           `json:"features"`
}

func Evaluate(rows []Row, m Model, scoring map[string]float64, options EvaluationOptions) (Evaluation, error) {
	e := Evaluation{From: options.From, To: options.To, Stats: map[string]Metrics{}, Groups: map[string]Metrics{}, Notes: []string{
		"Conditional on appearing with positive minutes; DNP outcomes are retained in data but excluded from these accuracy metrics. Probability of playing is not trained.",
		"All features use completed prior sports days; current box-score minutes, starts, position and stats are outcomes only.",
		"Proxy reuses the application's Baseline and Points functions on reconstructed prior game totals. The proxy replay does not use archived pregame market or injury adjustments; market factors are neutral and current injury flags are not replayed.",
		"Opponent defense uses current-season G/F/C buckets and schedule-adjusted per-minute residuals, not actual individual defensive matchups.",
		"Coefficients are frozen during evaluation; player history and opponent features update after each completed day.",
	}}
	if err := m.Validate(); err != nil {
		return e, err
	}
	if m.Config.Features.ReportedAvailability {
		e.Notes = append(e.Notes, "Teammate participation uses prior box scores and timestamp-validated official pregame reports; missing reports or unlisted players are not automatically healthy. Target-game outcomes and final participant lists are not pregame inputs. Frozen-origin forecasts cannot see later reports.")
	} else if m.Config.Features != (FeatureSet{}) {
		e.Notes = append(e.Notes, "Teammate participation and historical roster membership are estimated using prior box scores only; target-game DNPs, appearances, starts and final roster lists are not supplied as pregame information.")
	}
	if m.Config.Features.TeamBudget {
		e.Notes = append(e.Notes, "The budget constrains probability-weighted expected minutes plus an unknown-rotation reserve to 240; conditional-on-playing minutes alone do not sum to 240. It is not an active-lineup scenario model.")
	}
	if options.From.Before(m.TrainedBefore) {
		return e, fmt.Errorf("evaluation starts before model training cutoff %s", m.TrainedBefore.Format(time.DateOnly))
	}
	if !options.To.After(options.From) {
		return e, fmt.Errorf("evaluation end must follow its start")
	}
	games := map[string]bool{}
	type frozen struct {
		prediction Prediction
		features   Features
		reference  float64
	}
	frozenPredictions := map[string]frozen{}
	frozenWeek, originDay := "", ""
	schedule := map[string]map[string]Target{}
	if options.FreezeWeek {
		e.Notes = append(e.Notes, "Same-origin benchmark: forecasts for the remaining week's scheduled games are frozen before the first eligible sports day. Roster membership is inferred at that origin; unknown/new/traded players without a matching prior roster are excluded and counted separately. No subsequent results update these forecasts.")
		for _, r := range rows {
			day := sportsdayOf(r.StartsAt)
			if day.Before(options.From) || !day.Before(options.To) {
				continue
			}
			week := day.AddDate(0, 0, -(int(day.Weekday())+6)%7).Format(time.DateOnly)
			if schedule[week] == nil {
				schedule[week] = map[string]Target{}
			}
			t := r.Target()
			t.PlayerID = ""
			schedule[week][r.GameID+"/"+r.Team] = t
		}
	}
	encoder := json.NewEncoder(options.Predictions)
	errorEncoder := json.NewEncoder(options.Errors)
	err := Walk(rows, m.Config, func(row Row, f Features, state *State) error {
		day := sportsdayOf(row.StartsAt)
		if day.Before(options.From) || !day.Before(options.To) {
			return nil
		}
		p := m.Forecast(row.Target(), f, state, scoring)
		originReference := 0.0
		knownOrigin := true
		if options.FreezeWeek {
			week := day.AddDate(0, 0, -(int(day.Weekday())+6)%7).Format(time.DateOnly)
			if week != frozenWeek {
				frozenWeek = week
				originDay = day.Format(time.DateOnly)
				frozenPredictions = map[string]frozen{}
				var targets []Target
				for _, t := range schedule[week] {
					targets = append(targets, t)
				}
				slices.SortFunc(targets, func(a, b Target) int {
					if c := a.StartsAt.Compare(b.StartsAt); c != 0 {
						return c
					}
					return strings.Compare(a.GameID+"/"+a.Team, b.GameID+"/"+b.Team)
				})
				var ids []string
				if state.team != nil {
					for id := range state.team.Roster {
						ids = append(ids, id)
					}
				}
				slices.Sort(ids)
				for _, t := range targets {
					for _, id := range ids {
						r := state.team.Roster[id]
						if r.Team != t.Team || r.Season < t.Season-1 || t.StartsAt.Sub(r.StartsAt) > 210*24*time.Hour {
							continue
						}
						t.PlayerID = id
						// Matchup-only diagnostic: hold form, elapsed rest and
						// workload at the same origin for every future matchup.
						featureTarget := t
						featureTarget.StartsAt = row.StartsAt
						features := state.Build(featureTarget, m.Config)
						history := state.players[id]
						reference, n := 0.0, 0
						for i := len(history) - 1; i >= 0 && n < 20; i-- {
							reference += Score(history[i].Stats, scoring)
							n++
						}
						if n > 0 {
							reference /= float64(n)
						}
						frozenPredictions[t.GameID+"/"+id] = frozen{m.Forecast(featureTarget, features, state, scoring), features, reference}
					}
				}
			}
			entry, ok := frozenPredictions[row.GameID+"/"+row.PlayerID]
			knownOrigin = ok
			if ok {
				p, f, originReference = entry.prediction, entry.features, entry.reference
			}
		}
		if len(options.GameIDs) > 0 && !options.GameIDs[row.GameID] {
			return nil
		}
		if len(options.PlayerIDs) > 0 && !options.PlayerIDs[row.PlayerID] {
			return nil
		}
		if !row.Played || row.Minutes <= 0 {
			e.SkippedDNP++
			return nil
		}
		if !knownOrigin {
			e.SkippedOriginRoster++
			return nil
		}
		actual := Score(row.Stats, scoring)
		proxy, conditional := state.Proxy(row.Target(), scoring)
		recent := Score(f.RecentPoints, scoring)
		e.Model.Add(p.FantasyPoints, actual)
		e.Proxy.Add(proxy, actual)
		e.ProxyIfActive.Add(conditional, actual)
		e.Recent10.Add(recent, actual)
		e.Minutes.Add(p.Minutes, row.Minutes)
		e.RecentMinutes.Add(36*f.Minutes[2], row.Minutes)
		for _, stat := range Stats {
			metric := e.Stats[stat]
			metric.Add(p.Stats[stat], row.Stats[stat])
			e.Stats[stat] = metric
		}
		group := "established"
		if f.HistoryGames < 5 {
			group = "cold_start"
		}
		groups := []string{group, fmt.Sprintf("season_%d", row.Season), "position_" + f.Position}
		if f.Context != nil {
			if f.Context.RosterKnown {
				groups = append(groups, "known_prior_roster")
			} else {
				groups = append(groups, "unknown_prior_roster")
			}
			if f.Context.MissingTeammateMinutes >= 60 {
				groups = append(groups, "estimated_missing_teammate_minutes_60_plus")
			}
			if f.Context.OffseasonReset > 0 {
				groups = append(groups, "offseason_reset")
			}
		}
		if f.Minutes[6] == 1 {
			groups = append(groups, "back_to_back")
		}
		if f.LastMeetingMinutes > 0 {
			groups = append(groups, "previous_meeting_available")
		}
		if math.Abs(f.Minutes[1]-f.Minutes[2])*36 >= 5 {
			groups = append(groups, "minutes_changing")
		}
		for _, group := range groups {
			metric := e.Groups[group]
			metric.Add(p.FantasyPoints, actual)
			e.Groups[group] = metric
		}
		games[row.GameID] = true
		if options.Errors != nil {
			type diagnostic struct {
				ActualMinutesOracle float64 `json:"actual_minutes_oracle"`
				ActualBonus         float64 `json:"actual_bonus_points"`
				PredictedBonus      float64 `json:"predicted_bonus_points"`
			}
			var extra *diagnostic
			if options.Diagnostics {
				extra = &diagnostic{ActualMinutesOracle: m.project(f, row.Minutes, scoring).FantasyPoints}
				for _, stat := range []string{"dd", "td", "pts40", "pts50"} {
					extra.ActualBonus += row.Stats[stat] * scoring[stat]
					extra.PredictedBonus += p.Stats[stat] * scoring[stat]
				}
			}
			var rank []float64
			if options.RankFeatures {
				rank = []float64{p.FantasyPoints / 20, p.Minutes / 36, f.Minutes[7]}
				for _, stat := range Stats {
					rank = append(rank, f.Rates[stat][4], f.Rates[stat][5])
				}
			}
			if err := errorEncoder.Encode(struct {
				Game             string      `json:"game"`
				Player           string      `json:"player"`
				Actual           float64     `json:"actual"`
				Predicted        float64     `json:"predicted"`
				ActualMinutes    float64     `json:"actual_minutes"`
				PredictedMinutes float64     `json:"predicted_minutes"`
				Diagnostics      *diagnostic `json:"diagnostics,omitempty"`
				ForecastOrigin   string      `json:"forecast_origin,omitempty"`
				OriginReference  float64     `json:"origin_reference_mean,omitempty"`
				RankFeatures     []float64   `json:"rank_features,omitempty"`
			}{row.GameID, row.PlayerID, actual, p.FantasyPoints, row.Minutes, p.Minutes, extra, originDay, originReference, rank}); err != nil {
				return err
			}
		}
		if options.Predictions != nil {
			return encoder.Encode(Result{row.Target(), row.Name, row.Minutes, row.Stats, actual, p, proxy, conditional, recent, f})
		}
		return nil
	})
	if err != nil {
		return e, err
	}
	if e.Model.N == 0 {
		return e, fmt.Errorf("no eligible test appearances matched the date/game/player filters")
	}
	e.Games = len(games)
	e.Model = e.Model.Final()
	e.Proxy = e.Proxy.Final()
	e.ProxyIfActive = e.ProxyIfActive.Final()
	e.Recent10 = e.Recent10.Final()
	e.Minutes = e.Minutes.Final()
	e.RecentMinutes = e.RecentMinutes.Final()
	for k, v := range e.Stats {
		e.Stats[k] = v.Final()
	}
	for k, v := range e.Groups {
		e.Groups[k] = v.Final()
	}
	return e, nil
}

// Predict rebuilds history through completed prior sports days. This is the
// same feature builder and prediction path used by the evaluation harness.
func Predict(rows []Row, model Model, target Target, scoring map[string]float64) (Prediction, Features, error) {
	var report *PregameReport
	for _, r := range rows {
		if r.GameID == target.GameID && r.Pregame != nil {
			report = r.Pregame
			break
		}
	}
	return PredictWithReport(rows, model, target, scoring, report)
}

func PredictWithReport(rows []Row, model Model, target Target, scoring map[string]float64, report *PregameReport) (Prediction, Features, error) {
	if err := model.Validate(); err != nil {
		return Prediction{}, Features{}, err
	}
	if sportsdayOf(target.StartsAt).Before(model.TrainedBefore) {
		return Prediction{}, Features{}, fmt.Errorf("target precedes model training cutoff")
	}
	state := NewState()
	for first := 0; first < len(rows); {
		day := sportsdayOf(rows[first].StartsAt)
		if !day.Before(sportsdayOf(target.StartsAt)) {
			break
		}
		last := first + 1
		for last < len(rows) && sportsdayOf(rows[last].StartsAt).Equal(day) {
			last++
		}
		features := make([]Features, last-first)
		if state.reports == nil {
			state.reports = map[string]*PregameReport{}
		}
		for _, r := range rows[first:last] {
			if r.Pregame != nil {
				state.reports[r.GameID] = r.Pregame
			}
		}
		for i := first; i < last; i++ {
			features[i-first] = state.Build(rows[i].Target(), model.Config)
		}
		state.ObserveDay(rows[first:last], features)
		first = last
	}
	if state.reports == nil {
		state.reports = map[string]*PregameReport{}
	}
	if report != nil {
		if !report.PublishedAt.Before(target.StartsAt) || report.StartsAt.Sub(target.StartsAt).Abs() > time.Minute {
			return Prediction{}, Features{}, fmt.Errorf("report is not before target tipoff")
		}
		state.reports[target.GameID] = report
	}
	f := state.Build(target, model.Config)
	return model.Forecast(target, f, state, scoring), f, nil
}
