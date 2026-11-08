// Package nbalive serves validated, pregame NBA model forecasts. The collector
// publishes complete snapshots atomically; requests never train or call feeds.
package nbalive

import (
	"crossover/internal/nbamodel"
	"fmt"
	"math"
	"time"
)

const Version = 1
const MaxAge = 6 * time.Hour

type Record struct {
	Target     nbamodel.Target            `json:"target"`
	Prediction nbamodel.Prediction        `json:"prediction"`
	ForecastAt time.Time                  `json:"forecast_at"`
	Out        bool                       `json:"out"`
	Official   *nbamodel.OfficialFeatures `json:"official_report,omitempty"`
}
type Snapshot struct {
	Sport          string            `json:"sport,omitempty"`
	Version        int               `json:"version"`
	GeneratedAt    time.Time         `json:"generated_at"`
	HistoryThrough time.Time         `json:"history_through"`
	ModelSHA256    string            `json:"model_sha256"`
	Records        map[string]Record `json:"records"`
}

func Key(game, player string) string { return game + "/" + player }
func (s Snapshot) Validate(now time.Time) error {
	if s.Version != Version || s.GeneratedAt.IsZero() || s.GeneratedAt.After(now.Add(time.Minute)) || s.HistoryThrough.IsZero() || s.HistoryThrough.After(s.GeneratedAt) || len(s.ModelSHA256) != 64 || s.Records == nil {
		return fmt.Errorf("invalid NBA snapshot header")
	}
	for key, r := range s.Records {
		if key != Key(r.Target.GameID, r.Target.PlayerID) || r.Target.GameID == "" || r.Target.PlayerID == "" || r.Target.Team == "" || r.Target.Opponent == "" || r.Target.Team == r.Target.Opponent || r.ForecastAt.IsZero() || r.ForecastAt.After(s.GeneratedAt) || !r.ForecastAt.Before(r.Target.StartsAt) || r.Prediction.Minutes < 0 || r.Prediction.Minutes > 48 || math.IsNaN(r.Prediction.Minutes) || math.IsInf(r.Prediction.Minutes, 0) {
			return fmt.Errorf("invalid NBA forecast %s", key)
		}
		for _, stat := range nbamodel.Stats {
			v, ok := r.Prediction.Stats[stat]
			if !ok || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("invalid NBA stat %s/%s", key, stat)
			}
		}
		if o := r.Official; o != nil && o.PublishedAt != nil && (o.PublishedAt.After(r.ForecastAt) || !o.PublishedAt.Before(r.Target.StartsAt)) {
			return fmt.Errorf("invalid NBA report timestamp %s", key)
		}
	}
	return nil
}

// Points returns the league-specific score. Live/final games retain their last
// pre-tip forecast; fresh scheduled forecasts are conditional on playing.
func (r Record) Points(scoring map[string]float64, now time.Time, scheduled bool) (float64, bool) {
	if scheduled && now.Sub(r.ForecastAt) > MaxAge {
		return 0, false
	}
	if r.Out {
		return 0, true
	}
	return nbamodel.Score(r.Prediction.Stats, scoring), true
}
