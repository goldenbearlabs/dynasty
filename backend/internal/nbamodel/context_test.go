package nbamodel

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestExtendedFeaturesExcludeTargetAndSameDayRoster(t *testing.T) {
	rows := fixture()
	all, _ := ParseFeatureSet("teammate_minutes,teammate_rates,team_budget,workload,offseason,pace,competitiveness")
	c := Config{HalfLife: 25, PriorMinutes: 240, SeasonDecay: .65, Ridge: .001, Features: all}
	get := func(rows []Row) Features {
		var found Features
		err := Walk(rows, c, func(row Row, f Features, _ *State) error {
			if row.GameID == "10" && row.PlayerID == "1" {
				found = f
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	before := get(rows)
	changed := append([]Row(nil), rows...)
	for i := 10 * 12; i < 11*12; i++ {
		changed[i].Played = false
		changed[i].Minutes = 0
		changed[i].Position = "C"
		changed[i].Starter = false
		changed[i].Stats = map[string]float64{}
	}
	after := get(changed)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("current-game teammate appearances or box scores leaked into context")
	}
	if len(before.Minutes) != len(minuteFeatureNames(all)) || len(before.Rates["pts"]) != len(rateFeatureNames(all)) {
		t.Fatal("feature schema does not match vectors")
	}
	if !before.Context.RosterKnown || before.Context.EstimatedPresence <= 0 || before.Context.ExpectedRotationMinutes <= 0 {
		t.Fatalf("missing prior-team context: %+v", before.Context)
	}
}

func TestExpectedTeamBudget(t *testing.T) {
	raw := map[string]float64{"a": 40, "b": 35, "c": 35, "d": 30, "e": 30, "f": 24, "g": 20, "h": 18, "i": 12, "j": 8}
	q := map[string]float64{"a": .9, "b": .9, "c": .8, "d": .95, "e": .9, "f": .9, "g": .7, "h": .7, "i": .4, "j": .3}
	a := allocateMinutes(raw, q, 8)
	if math.Abs(a.Total-240) > 1e-8 {
		t.Fatalf("expected team minutes %v", a.Total)
	}
	for _, m := range a.Minutes {
		if m < 0 || m > 48 {
			t.Fatalf("invalid allocated minutes %v", m)
		}
	}
	// If a major teammate becomes less likely to play, others receive minutes;
	// no target-game participation is needed to calculate this response.
	q2 := map[string]float64{}
	for id, p := range q {
		q2[id] = p
	}
	q2["a"] = .05
	b := allocateMinutes(raw, q2, 8)
	if b.Minutes["b"] <= a.Minutes["b"] {
		t.Fatal("missing teammate did not free any minutes")
	}
	if math.Abs(b.Total-240) > 1e-8 {
		t.Fatal("absence broke the budget")
	}
	unknown := allocateMinutes(map[string]float64{}, map[string]float64{}, 8)
	if math.Abs(unknown.Reserve-240) > 1e-8 {
		t.Fatal("unknown rotation did not retain unallocated reserve")
	}
}

func TestOffseasonResetAndUnknownRoster(t *testing.T) {
	rows := fixture()
	s := NewState()
	base := Config{HalfLife: 25, PriorMinutes: 240, SeasonDecay: .65, Ridge: .001}
	for first := 0; first < len(rows); first += 12 {
		features := make([]Features, 12)
		for i := range features {
			features[i] = s.Build(rows[first+i].Target(), base)
		}
		s.ObserveDay(rows[first:first+12], features)
	}
	target := rows[len(rows)-12].Target()
	target.GameID = "new-season"
	target.Season = 2026
	target.StartsAt = target.StartsAt.AddDate(0, 5, 0)
	reset := base
	reset.Features.Offseason = true
	f := s.Build(target, reset)
	var minutes, games float64
	for _, r := range s.players[target.PlayerID] {
		if r.Season == 2025 {
			minutes += r.Minutes
			games++
		}
	}
	if f.Context.OffseasonReset != 1 || math.Abs(f.BaseMinutes-minutes/games) > 1e-9 {
		t.Fatal("opening-season baseline did not reset to prior-season minutes")
	}
	target.PlayerID = "never-seen-rookie"
	reset.Features.TeammateMinutes = true
	f = s.Build(target, reset)
	if f.Context.RosterKnown {
		t.Fatal("target's identity was inserted into the inferred prior roster")
	}
	for _, p := range s.context(target, reset).Players {
		if p.ID == target.PlayerID {
			t.Fatal("unknown target contaminated team budget pool")
		}
	}
}

func TestExtendedTrainingHoldoutAndPredictionParity(t *testing.T) {
	rows := fixture()
	features, _ := ParseFeatureSet("teammate_minutes,teammate_rates,team_budget,workload,offseason,pace,competitiveness")
	trainBefore := sportsdayOf(rows[140*12].StartsAt)
	testFrom := sportsdayOf(rows[200*12].StartsAt)
	scoring := map[string]float64{"pts": 1}
	m, candidates, err := TrainVariant(rows, trainBefore, testFrom, scoring, features, nil)
	if err != nil {
		t.Fatal(err)
	}
	modified := append([]Row(nil), rows...)
	for i := 200 * 12; i < len(modified); i++ {
		modified[i].Minutes = 45
		modified[i].Stats = map[string]float64{"pts": 100}
		modified[i].Played = i%2 == 0
	}
	m2, candidates2, err := TrainVariant(modified, trainBefore, testFrom, scoring, features, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, m2) || !reflect.DeepEqual(candidates, candidates2) {
		t.Fatal("held-out teammate outcomes changed training/selection")
	}
	var replay Prediction
	err = Walk(rows, m.Config, func(r Row, f Features, state *State) error {
		if r.GameID == "205" && r.PlayerID == "2" {
			replay = m.Forecast(r.Target(), f, state, scoring)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := Predict(rows, m, rows[205*12+2].Target(), scoring)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, replay) {
		t.Fatal("team-budget forecast differs between single prediction and replay")
	}
	// Artifact schema and coefficient sizes survive serialization.
	body, _ := json.Marshal(m)
	var restored Model
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
}
