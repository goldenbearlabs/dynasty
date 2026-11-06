package nbalive

import (
	"context"
	"crossover/internal/nbamodel"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func example(now time.Time) Snapshot {
	stats := map[string]float64{}
	for _, s := range nbamodel.Stats {
		stats[s] = 0
	}
	stats["pts"] = 20
	stats["reb"] = 6
	r := Record{Target: nbamodel.Target{GameID: "g", PlayerID: "p", Team: "a", Opponent: "b", StartsAt: now.Add(time.Hour)}, Prediction: nbamodel.Prediction{Minutes: 30, Stats: stats}, ForecastAt: now}
	return Snapshot{Version: Version, GeneratedAt: now, HistoryThrough: now.Add(-24 * time.Hour), ModelSHA256: strings.Repeat("a", 64), Records: map[string]Record{"g/p": r}}
}
func TestScoresFreshnessAndPreservedPregameForecast(t *testing.T) {
	now := time.Now()
	s := example(now)
	if err := s.Validate(now); err != nil {
		t.Fatal(err)
	}
	r := s.Records["g/p"]
	if points, ok := r.Points(map[string]float64{"pts": .5, "reb": 1}, now, true); !ok || points != 16 {
		t.Fatal(points, ok)
	}
	if points, ok := r.Points(map[string]float64{"pts": 1}, now, true); !ok || points != 20 {
		t.Fatal("scoring not league-specific")
	}
	if _, ok := r.Points(nil, now.Add(7*time.Hour), true); ok {
		t.Fatal("stale scheduled prediction served")
	}
	if points, ok := r.Points(map[string]float64{"pts": 1}, now.Add(7*time.Hour), false); !ok || points != 20 {
		t.Fatal("lost locked pregame expectation")
	}
	r.Out = true
	if p, ok := r.Points(map[string]float64{"pts": 1}, now, true); !ok || p != 0 {
		t.Fatal("reported Out not zero")
	}
}
func TestRejectInvalidAndFutureSnapshots(t *testing.T) {
	now := time.Now()
	s := example(now)
	r := s.Records["g/p"]
	r.ForecastAt = r.Target.StartsAt
	s.Records["g/p"] = r
	if s.Validate(now) == nil {
		t.Fatal("accepted post-tip forecast")
	}
	s = example(now)
	r = s.Records["g/p"]
	r.Prediction.Stats["pts"] = math.NaN()
	s.Records["g/p"] = r
	if s.Validate(now) == nil {
		t.Fatal("accepted nonfinite stat")
	}
	s = example(now)
	r = s.Records["g/p"]
	future := now.Add(time.Minute)
	r.Official = &nbamodel.OfficialFeatures{PublishedAt: &future}
	s.Records["g/p"] = r
	if s.Validate(now) == nil {
		t.Fatal("accepted later injury news")
	}
}
func TestMalformedReloadRetainsPreviousSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	prior := &loaded{snapshot: example(time.Now())}
	service.current.Store(prior)
	if service.Reload(context.Background()) == nil {
		t.Fatal("malformed reload accepted")
	}
	if service.current.Load() != prior {
		t.Fatal("lost last valid snapshot")
	}
}
