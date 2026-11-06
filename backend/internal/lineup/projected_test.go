package lineup

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"crossover/internal/projection"
)

// A game under way adds what is left of it; on auto, his points in it so
// far too, since they are not in the score until it is over.
func TestProjectedStarterLive(t *testing.T) {
	now := time.Date(2026, 11, 5, 21, 0, 0, 0, time.UTC)
	play := projection.Shape{Periods: 4, Minutes: 12, Spread: 0.4}
	live := Game{Day: "2026-11-05", StartsAt: now.Add(-time.Hour), Status: "live", detail: "Halftime", Points: 14, Expected: 30, Counts: true, Stats: json.RawMessage(`{"pts":14}`)}
	later := Game{Day: "2026-11-07", StartsAt: now.Add(48 * time.Hour), Status: "scheduled", Projected: 25}

	var picked projection.Outlook
	projectedStarter(&picked, Player{Slot: "G", CountsFrom: "2026-11-05", Games: []Game{live, later}}, 1, "2026-11-05", "2026-11-09", play, now)
	if math.Abs(picked.Points-15) > 1e-9 || math.Abs(picked.Variance-0.16*900*0.5) > 1e-9 {
		t.Errorf("picked for the live game: %+v; want half its 30 to come, and the later game left out", picked)
	}

	onAuto := live
	onAuto.Counts = false
	var auto projection.Outlook
	projectedStarter(&auto, Player{Slot: "G", CountsLast: true, Games: []Game{onAuto}}, 1, "2026-11-05", "2026-11-09", play, now)
	if math.Abs(auto.Points-29) > 1e-9 {
		t.Errorf("on auto in his last game: %+v; want his 14 so far and 15 to come", auto)
	}

	hurt := live
	hurt.Stats, hurt.market.Injured = json.RawMessage(`{}`), true
	var out projection.Outlook
	projectedStarter(&out, Player{Slot: "G", CountsFrom: "2026-11-05", Games: []Game{hurt}}, 1, "2026-11-05", "2026-11-09", play, now)
	if out.Points != 0 {
		t.Errorf("hurt and not in the game: %+v; want nothing to come", out)
	}
}
