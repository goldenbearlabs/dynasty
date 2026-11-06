package nbamodel

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestPriorityFeaturesAndShotConsistency(t *testing.T) {
	rows := fixture()
	features, _ := ParseFeatureSet("workload,role_state,stat_windows,opportunities")
	for i := range rows {
		rows[i].Stats["tpa"] = 2
		rows[i].Stats["tpm"] = 1
		rows[i].Stats["fta"] = 2
		rows[i].Stats["ftm"] = 1
		rows[i].Stats["pf"] = 2
	}
	c := Config{HalfLife: 25, PriorMinutes: 240, Ridge: .001, SeasonDecay: .65, Features: features}
	var target Features
	err := Walk(rows, c, func(r Row, f Features, _ *State) error {
		if r.GameID == "10" && r.PlayerID == "1" {
			target = f
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Minutes) != len(minuteFeatureNames(features)) || len(target.Rates["pts"]) != len(rateFeatureNames(features)) {
		t.Fatal("priority schemas differ from feature vectors")
	}
	m, err := FitBefore(rows, c, sportsdayOf(rows[200*12].StartsAt), map[string]float64{"pts": 1})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := Predict(rows, m, rows[205*12+1].Target(), m.Scoring)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.Stats["pts"]-(2*p.Stats["fgm"]+p.Stats["tpm"]+p.Stats["ftm"])) > 1e-9 {
		t.Fatal("opportunity projection has incoherent point total")
	}
	if p.Stats["fgm"] > p.Stats["fga"] || p.Stats["tpm"] > p.Stats["fgm"] || p.Stats["ftm"] > p.Stats["fta"] {
		t.Fatal("projected makes exceed attempts")
	}
	changed := append([]Row(nil), rows...)
	for i := 10 * 12; i < 11*12; i++ {
		changed[i].Minutes = 45
		changed[i].Stats = map[string]float64{"pts": 100, "tpa": 30}
	}
	var after Features
	Walk(changed, c, func(r Row, f Features, _ *State) error {
		if r.GameID == "10" && r.PlayerID == "1" {
			after = f
		}
		return nil
	})
	if !reflect.DeepEqual(target, after) {
		t.Fatal("target-day production leaked into priority features")
	}
}

func TestTimestampedReportedAvailability(t *testing.T) {
	rows := fixture()
	c := Config{HalfLife: 25, PriorMinutes: 240, Ridge: .001, SeasonDecay: .65, Features: FeatureSet{Workload: true, TeammateMinutes: true, ReportedAvailability: true}}
	stamp := rows[10*12].StartsAt
	report := &PregameReport{PublishedAt: stamp.Add(-time.Hour), StartsAt: stamp, Teams: map[string]ReportedTeam{"a": {Submitted: true, Players: []ReportedPlayer{{PlayerID: "0", Status: "Out"}}}, "b": {Submitted: false}}}
	data, _ := json.Marshal(map[string]any{"reports": map[string]*PregameReport{"10": report}})
	if _, err := ReadPregameReports(bytes.NewReader(data), rows); err != nil {
		t.Fatal(err)
	}
	var found Features
	Walk(rows, c, func(r Row, f Features, s *State) error {
		if r.GameID == "10" && r.PlayerID == "1" {
			found = f
			for _, p := range s.context(r.Target(), c).Players {
				if p.ID == "0" && p.Presence != 0 {
					t.Fatal("official out did not override inferred participation")
				}
			}
		}
		return nil
	})
	n := len(found.Minutes)
	if found.Minutes[n-4] != 1 || found.Minutes[n-3] <= 0 {
		t.Fatal("official report absence features missing")
	}
	report.PublishedAt = stamp.Add(time.Minute)
	data, _ = json.Marshal(map[string]any{"reports": map[string]*PregameReport{"10": report}})
	if _, err := ReadPregameReports(bytes.NewReader(data), rows); err == nil {
		t.Fatal("accepted report published after tipoff")
	}
}

func TestFrozenWeekCannotUseLaterPregameReports(t *testing.T) {
	rows := fixture()
	cutoff := sportsdayOf(rows[200*12].StartsAt)
	future := rows[201*12].StartsAt
	report := &PregameReport{PublishedAt: future.Add(-time.Hour), StartsAt: future, Teams: map[string]ReportedTeam{"a": {Submitted: true, Players: []ReportedPlayer{{PlayerID: "0", Status: "Out"}}}}}
	for i := 201 * 12; i < 202*12; i++ {
		rows[i].Pregame = report
	}
	c := Config{HalfLife: 25, PriorMinutes: 240, Ridge: .001, SeasonDecay: .65, Features: FeatureSet{Workload: true, ReportedAvailability: true}}
	m, err := FitBefore(rows, c, cutoff, map[string]float64{"pts": 1})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	_, err = Evaluate(rows, m, m.Scoring, EvaluationOptions{From: cutoff, To: cutoff.AddDate(0, 0, 2), FreezeWeek: true, Predictions: &output})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'}) {
		var r Result
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatal(err)
		}
		if r.Target.GameID == "201" && r.Target.PlayerID == "1" {
			found = true
			if r.Features.Official == nil || r.Features.Official.Available || r.Features.Official.PublishedAt != nil {
				t.Fatal("later report leaked into an earlier frozen origin")
			}
		}
	}
	if !found {
		t.Fatal("missing frozen forecast")
	}
}
