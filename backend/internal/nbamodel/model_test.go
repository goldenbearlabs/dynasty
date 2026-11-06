package nbamodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

func fixture() []Row {
	var rows []Row
	start := time.Date(2024, 10, 1, 23, 0, 0, 0, time.UTC)
	for day := 0; day < 240; day++ {
		for player := 0; player < 12; player++ {
			minutes := 16 + float64(player)*2
			if day > 90 && player < 3 {
				minutes += 5
			}
			rate := .35 + float64(player)*.025
			home := day%2 == 0
			if home {
				rate += .03
			}
			stats := map[string]float64{}
			for _, stat := range Stats {
				stats[stat] = 0
			}
			stats["pts"] = minutes * rate
			stats["reb"] = minutes * .2
			stats["ast"] = minutes * .1
			stats["fgm"] = stats["pts"] * .4
			stats["fga"] = stats["fgm"] * 2
			stats["tov"] = minutes * .04
			team, opponent := "a", "b"
			if player >= 6 {
				team, opponent = opponent, team
			}
			rows = append(rows, Row{GameID: fmt.Sprint(day), PlayerID: fmt.Sprint(player), StartsAt: start.AddDate(0, 0, day), Season: 2025, Team: team, Opponent: opponent, Position: "G", Home: home, Minutes: minutes, Played: true, Starter: player%6 < 5, Stats: stats})
		}
	}
	return rows
}

func TestTrainHoldoutAndFilters(t *testing.T) {
	rows := fixture()
	trainBefore := sportsdayOf(rows[140*12].StartsAt)
	testFrom := sportsdayOf(rows[200*12].StartsAt)
	scoring := map[string]float64{"pts": .5, "reb": 1, "ast": 1, "tov": -1}
	m, candidates, err := Train(rows, trainBefore, testFrom, scoring, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 6 {
		t.Fatalf("candidates = %d", len(candidates))
	}
	options := EvaluationOptions{From: testFrom, To: sportsdayOf(rows[len(rows)-1].StartsAt).AddDate(0, 0, 1)}
	e, err := Evaluate(rows, m, scoring, options)
	if err != nil {
		t.Fatal(err)
	}
	if e.Model.N != 40*12 || e.Games != 40 {
		t.Fatalf("unexpected evaluation size: %+v", e)
	}
	if e.Model.MAE >= e.Proxy.MAE || e.Minutes.MAE > 2 {
		t.Fatalf("failed to learn stable synthetic production: model %+v, proxy %+v, minutes %+v", e.Model, e.Proxy, e.Minutes)
	}
	// Changing every held-out outcome must leave training and model selection
	// unchanged, including test outcomes used in later days' rolling history.
	modified := append([]Row(nil), rows...)
	for i := 200 * 12; i < len(modified); i++ {
		modified[i].Minutes = 45
		modified[i].Stats = map[string]float64{"pts": 100}
	}
	m2, choices2, err := Train(modified, trainBefore, testFrom, scoring, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, m2) || !reflect.DeepEqual(candidates, choices2) {
		t.Fatal("held-out outcomes changed training or selection")
	}
	options.GameIDs = map[string]bool{"205": true}
	options.PlayerIDs = map[string]bool{"2": true}
	var predictions bytes.Buffer
	options.Predictions = &predictions
	single, err := Evaluate(rows, m, scoring, options)
	if err != nil {
		t.Fatal(err)
	}
	if single.Model.N != 1 || single.Games != 1 {
		t.Fatalf("filters not applied: %+v", single)
	}
	var result Result
	if err := json.Unmarshal(bytes.TrimSpace(predictions.Bytes()), &result); err != nil {
		t.Fatal(err)
	}
	p, features, err := Predict(rows, m, rows[205*12+2].Target(), scoring)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, result.Prediction) || !reflect.DeepEqual(features, result.Features) {
		t.Fatal("single-game prediction differs from harness")
	}
	options.From = trainBefore
	if _, err := Evaluate(rows, m, scoring, options); err == nil {
		t.Fatal("accepted evaluation overlapping training")
	}
}

func TestCurrentGameAndSameDayOutcomesExcluded(t *testing.T) {
	rows := fixture()
	config := Config{HalfLife: 10, PriorMinutes: 240, SeasonDecay: .65, Ridge: .01}
	var before Features
	if err := Walk(rows, config, func(row Row, f Features, _ *State) error {
		if row.GameID == "10" && row.PlayerID == "1" {
			before = f
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	changed := append([]Row(nil), rows...)
	// Alter the target and a different player earlier in the same day's input.
	for _, i := range []int{10 * 12, 10*12 + 1} {
		changed[i].Minutes = 60
		changed[i].Position = "C"
		changed[i].Starter = !changed[i].Starter
		changed[i].Stats = map[string]float64{"pts": 150}
	}
	var after Features
	if err := Walk(changed, config, func(row Row, f Features, _ *State) error {
		if row.GameID == "10" && row.PlayerID == "1" {
			after = f
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("target or same-day outcomes leaked into features")
	}
	if before.Position != "G" || before.LastMeetingMinutes == 0 || before.OpponentMinutes == 0 {
		t.Fatalf("position/opponent/meeting features missing: %+v", before)
	}
}

func TestReadRowsRejectsCorruptData(t *testing.T) {
	row := fixture()[0]
	encode := func(r Row) []byte {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		return append(b, '\n')
	}
	valid := encode(row)
	if _, err := ReadRows(bytes.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRows(bytes.NewReader(append(append([]byte(nil), valid...), valid...))); err == nil {
		t.Fatal("accepted duplicate player-game")
	}
	delete(row.Stats, "reb")
	if _, err := ReadRows(bytes.NewReader(encode(row))); err == nil {
		t.Fatal("accepted missing stat as zero")
	}
	row.Played = false
	row.Minutes = 20
	if _, err := ReadRows(bytes.NewReader(encode(row))); err == nil {
		t.Fatal("accepted DNP with positive minutes")
	}
}

func TestMetricsAndArtifactValidation(t *testing.T) {
	var m Metrics
	m.Add(10, 8)
	m.Add(10, 14)
	f := m.Final()
	if f.MAE != 3 || math.Abs(f.RMSE-math.Sqrt(10)) > 1e-10 || f.Bias != -1 {
		t.Fatalf("metrics %+v", f)
	}
	if err := (Model{Version: Version}).Validate(); err == nil {
		t.Fatal("accepted incomplete model artifact")
	}
}

func TestRefitBeforeAndFrozenOriginExcludeLaterOutcomes(t *testing.T) {
	rows := fixture()
	cutoff := sportsdayOf(rows[200*12].StartsAt)
	c := Config{HalfLife: 25, PriorMinutes: 240, SeasonDecay: .65, Ridge: .001, Features: FeatureSet{Workload: true}}
	scoring := map[string]float64{"pts": 1}
	m, err := FitBefore(rows, c, cutoff, scoring)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]Row(nil), rows...)
	for i := 200 * 12; i < len(changed); i++ {
		changed[i].Stats = map[string]float64{"pts": 100}
		changed[i].Minutes = 45
	}
	m2, err := FitBefore(changed, c, cutoff, scoring)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, m2) {
		t.Fatal("refit included outcomes at or after cutoff")
	}
	end := cutoff.AddDate(0, 0, 7)
	get := func(data []Row) map[string]float64 {
		var buffer bytes.Buffer
		_, err := Evaluate(data, m, scoring, EvaluationOptions{From: cutoff, To: end, FreezeWeek: true, Errors: &buffer})
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]float64{}
		for _, line := range bytes.Split(bytes.TrimSpace(buffer.Bytes()), []byte{'\n'}) {
			var r struct {
				Game       string  `json:"game"`
				Player     string  `json:"player"`
				Prediction float64 `json:"predicted"`
				Origin     string  `json:"forecast_origin"`
			}
			if err := json.Unmarshal(line, &r); err != nil {
				t.Fatal(err)
			}
			if r.Origin == cutoff.Format(time.DateOnly) {
				result[r.Game+"/"+r.Player] = r.Prediction
			}
		}
		return result
	}
	a, b := get(rows), get(changed)
	if len(a) == 0 || !reflect.DeepEqual(a, b) {
		t.Fatal("later same-week outcomes changed frozen-origin forecasts")
	}
	// Turning oracle diagnostics on must leave the scored forecasts unchanged.
	ae, err := Evaluate(rows, m, scoring, EvaluationOptions{From: cutoff, To: end})
	if err != nil {
		t.Fatal(err)
	}
	be, err := Evaluate(rows, m, scoring, EvaluationOptions{From: cutoff, To: end, Diagnostics: true})
	if err != nil {
		t.Fatal(err)
	}
	if ae.Model != be.Model {
		t.Fatal("oracle diagnostics changed primary predictions")
	}
}
