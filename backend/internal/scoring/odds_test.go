package scoring

import (
	"math/rand/v2"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/settings"
)

func TestPlayOut(t *testing.T) {
	id := func(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{b}, Valid: true} }
	a, b, c, d := id(1), id(2), id(3), id(4)
	format := settings.Format{Type: settings.FormatHeadToHead, PlayoffTeams: 2}
	names := map[pgtype.UUID]string{a: "A", b: "B", c: "C", d: "D"}
	rng := rand.New(rand.NewPCG(1, 1))

	// A is far ahead and clinched; D cannot catch up. B and C are level,
	// but B scores twice as much a day, so it should take the last place.
	rows := []Row{
		{FranchiseID: a, Wins: 10},
		{FranchiseID: b, Wins: 5, Losses: 5},
		{FranchiseID: c, Wins: 5, Losses: 5},
		{FranchiseID: d, Losses: 10},
	}
	perDay := map[pgtype.UUID]float64{a: 10, b: 20, c: 10, d: 10}
	games := []game{
		{home: a, away: d, daysLeft: 7},
		{home: b, away: c, daysLeft: 7},
		{home: a, away: c, daysLeft: 7},
		{home: b, away: d, daysLeft: 7},
	}
	odds, places := playOut(rows, nil, games, perDay, spread, 0, format, names, 2, rng)
	chance := map[pgtype.UUID]float64{}
	for _, o := range odds {
		chance[o.FranchiseID] = o.Playoffs
	}
	if chance[a] != 1 || chance[d] != 0 {
		t.Errorf("clinched %v and eliminated %v, want 1 and 0", chance[a], chance[d])
	}
	if chance[b] < 0.9 || chance[c] > 0.1 || chance[b]+chance[c] < 0.999 {
		t.Errorf("B %v and C %v: the stronger side should be the likely second playoff team", chance[b], chance[c])
	}
	if len(places[a]) != simulations || places[a][0] != 0 {
		t.Errorf("A's places %v, want first in every run", places[a][:1])
	}
	// The rest of the season is played: two more matchups each.
	if w := odds[0].Wins + odds[0].Losses; w != 12 {
		t.Errorf("A's projected record adds up to %v games, want 12", w)
	}
}
