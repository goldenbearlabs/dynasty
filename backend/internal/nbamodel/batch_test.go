package nbamodel

import (
	"math"
	"testing"
	"time"
)

func TestBatchMatchesSingleAndRejectsFutureInformation(t *testing.T) {
	rows := fixture()
	before := sportsdayOf(rows[180*12].StartsAt)
	model, err := FitBefore(rows, Config{HalfLife: 25, PriorMinutes: 240, Ridge: .01, SeasonDecay: .65}, before, map[string]float64{"pts": .5, "reb": 1})
	if err != nil {
		t.Fatal(err)
	}
	target := rows[220*12].Target()
	observed := target.StartsAt.Add(-time.Hour)
	batch, _, err := PredictBatch(rows, model, []Target{target}, nil, observed)
	if err != nil {
		t.Fatal(err)
	}
	single, _, err := PredictWithReport(rows, model, target, model.Scoring, nil)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(batch[0].FantasyPoints-single.FantasyPoints) > 1e-10 {
		t.Fatalf("batch %v != single %v", batch[0], single)
	}
	for i := 220 * 12; i < len(rows); i++ {
		rows[i].Minutes = 48
		rows[i].Stats["pts"] = 200
	}
	changed, _, err := PredictBatch(rows, model, []Target{target}, nil, observed)
	if err != nil {
		t.Fatal(err)
	}
	if changed[0].FantasyPoints != batch[0].FantasyPoints {
		t.Fatal("same-day outcomes leaked")
	}
	reports := map[string]*PregameReport{target.GameID: {StartsAt: target.StartsAt, PublishedAt: observed.Add(time.Minute)}}
	if _, _, err = PredictBatch(rows, model, []Target{target}, reports, observed); err == nil {
		t.Fatal("accepted report after capture")
	}
	futureObservation := observed.Add(time.Minute)
	reports[target.GameID].PublishedAt = observed.Add(-time.Hour)
	reports[target.GameID].Teams = map[string]ReportedTeam{"a": {ObservedAt: &futureObservation}}
	if _, _, err = PredictBatch(rows, model, []Target{target}, reports, observed); err == nil {
		t.Fatal("accepted team observation after capture")
	}
	if _, _, err = PredictBatch(rows, model, []Target{target}, nil, target.StartsAt); err == nil {
		t.Fatal("accepted started target")
	}
}
