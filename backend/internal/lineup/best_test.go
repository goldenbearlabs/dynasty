package lineup

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/settings"
)

func id(n byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{n}, Valid: true} }

func playing(n byte, projected float64, now time.Time, positions ...string) Player {
	return Player{PlayerID: id(n), Positions: positions, StarterEligible: true,
		Games: []Game{{StartsAt: now.Add(time.Hour), Projected: projected}}}
}

func starters(entries []Entry) map[pgtype.UUID]string {
	m := map[pgtype.UUID]string{}
	for _, e := range entries {
		m[e.PlayerID] = e.Slot
	}
	return m
}

// A player who fits two slots must not take the only slot another player fits,
// leaving someone with a game on the bench.
func TestBestStartsEveryoneWhoCanPlay(t *testing.T) {
	now := time.Now()
	view := View{
		Slots: []settings.Slot{{Name: "C", Positions: []string{"C"}, Count: 1}, {Name: "LW", Positions: []string{"LW"}, Count: 1}},
		Players: []Player{playing(1, 5, now, "C", "LW"), playing(2, 3, now, "C")},
	}
	got := starters(best(view, map[pgtype.UUID]int{}, now))
	if got[id(1)] != "LW" || got[id(2)] != "C" {
		t.Fatalf("want 1 at LW and 2 at C, got %v", got)
	}
}

// With more players playing than slots, the higher projections start.
func TestBestPrefersHigherProjection(t *testing.T) {
	now := time.Now()
	idle := Player{PlayerID: id(4), Positions: []string{"C"}, StarterEligible: true}
	view := View{
		Slots:   []settings.Slot{{Name: "C", Positions: []string{"C"}, Count: 1}, {Name: "UTIL", Positions: []string{"C", "LW"}, Count: 1}},
		Players: []Player{idle, playing(1, 2, now, "C"), playing(2, 6, now, "LW"), playing(3, 4, now, "C")},
	}
	got := starters(best(view, map[pgtype.UUID]int{}, now))
	if len(got) != 2 || got[id(2)] != "UTIL" || got[id(3)] != "C" {
		t.Fatalf("want 2 at UTIL and 3 at C, got %v", got)
	}
}
