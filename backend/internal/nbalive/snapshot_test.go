package nbalive

import (
	"context"
	"crossover/internal/nbamodel"
	"encoding/json"
	"fmt"
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

func TestCompetitionIsolationBeforeDatabaseMapping(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		sport, competition string
		minutes            float64
	}{
		{"nba", "cbb", 30}, {"cbb", "nba", 30}, {"cbb", "cbb", 41},
	} {
		t.Run(tc.sport+"_"+tc.competition+fmt.Sprint(tc.minutes), func(t *testing.T) {
			snapshot := example(now)
			snapshot.Sport = tc.sport
			r := snapshot.Records["g/p"]
			r.Prediction.Minutes = tc.minutes
			snapshot.Records["g/p"] = r
			path := filepath.Join(t.TempDir(), "snapshot.json")
			data, _ := json.Marshal(snapshot)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			service := NewCompetitionService(nil, path, slog.New(slog.NewTextHandler(io.Discard, nil)), tc.competition)
			if err := service.Reload(context.Background()); err == nil {
				t.Fatal("accepted wrong sport or invalid CBB minutes")
			}
		})
	}
}

// Set CBB_TEST_SNAPSHOT to exercise the Python collector -> Go serving contract.
func TestCBBCollectorSnapshot(t *testing.T) {
	path := os.Getenv("CBB_TEST_SNAPSHOT")
	if path == "" {
		t.Skip("set CBB_TEST_SNAPSHOT for the collector integration check")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if snapshot.Sport != "cbb" || len(snapshot.Records) == 0 {
		t.Fatal("expected populated CBB snapshot")
	}
	for _, r := range snapshot.Records {
		s := r.Prediction.Stats
		if r.Prediction.Minutes > 40 || math.Abs(s["pts"]-(2*s["fgm"]+s["tpm"]+s["ftm"])) > 1e-7 {
			t.Fatal("incoherent CBB forecast")
		}
	}
	t.Logf("validated %d collector forecasts", len(snapshot.Records))
}
