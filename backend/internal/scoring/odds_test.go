package scoring

import (
	"math/rand/v2"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/settings"
)

// league builds n teams with weeks of round-robin matchups still to play.
func league(n, weeks int) ([]Row, []game, map[pgtype.UUID]string) {
	ids := make([]pgtype.UUID, n)
	rows := make([]Row, n)
	names := map[pgtype.UUID]string{}
	for i := range ids {
		ids[i] = pgtype.UUID{Bytes: [16]byte{byte(i + 1)}, Valid: true}
		rows[i] = Row{FranchiseID: ids[i]}
		names[ids[i]] = string(rune('A' + i))
	}
	rounds := roundRobin(ids)
	var games []game
	for w := range weeks {
		for _, p := range rounds[w%len(rounds)] {
			if p[1].Valid {
				games = append(games, game{home: p[0], away: p[1], daysLeft: 7, weeksOut: w + 1})
			}
		}
	}
	return rows, games, names
}

func chances(odds []TeamOdds) (lo, hi float64) {
	lo, hi = 1, 0
	for _, o := range odds {
		lo, hi = min(lo, o.Playoffs), max(hi, o.Playoffs)
	}
	return lo, hi
}

func TestPreseasonOddsStayNearEven(t *testing.T) {
	format := settings.Format{Type: settings.FormatHeadToHead, PlayoffTeams: 8}
	rows, games, names := league(12, 15)
	odds, _ := playOut(rows, nil, games, strengths(rows, nil, nil), 0, format, names, 8, rand.New(rand.NewPCG(1, 1)))
	// 8 of 12 make it: everyone should sit near two in three.
	lo, hi := chances(odds)
	t.Logf("preseason: %.3f to %.3f", lo, hi)
	if lo < 0.55 || hi > 0.78 {
		t.Errorf("preseason chances run %.3f to %.3f, want all near 0.667", lo, hi)
	}
}

func TestOneDayOfScoringBarelyMovesOdds(t *testing.T) {
	format := settings.Format{Type: settings.FormatHeadToHead, PlayoffTeams: 8}
	rows, games, names := league(12, 15)
	// Two days in, the first team has scored three times what the last has.
	scored, played := map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}
	for i, r := range rows {
		scored[r.FranchiseID], played[r.FranchiseID] = 30-float64(i)*20/11, 2
	}
	odds, _ := playOut(rows, nil, games, strengths(rows, scored, played), 0, format, names, 8, rand.New(rand.NewPCG(1, 1)))
	lo, hi := chances(odds)
	t.Logf("two days in: %.3f to %.3f", lo, hi)
	if lo < 0.4 || hi > 0.85 {
		t.Errorf("after two days chances run %.3f to %.3f, want no extremes", lo, hi)
	}
}

func TestPlayOutFavoursTheBetterTeam(t *testing.T) {
	format := settings.Format{Type: settings.FormatHeadToHead, PlayoffTeams: 2}
	rows, games, names := league(4, 4)
	rows[0].Wins, rows[1].Wins, rows[1].Losses, rows[2].Wins, rows[2].Losses, rows[3].Losses = 6, 3, 3, 3, 3, 6
	scored, played := map[pgtype.UUID]float64{}, map[pgtype.UUID]float64{}
	for i, r := range rows {
		scored[r.FranchiseID], played[r.FranchiseID] = []float64{600, 900, 450, 450}[i], 42
	}
	odds, places := playOut(rows, nil, games, strengths(rows, scored, played), 0, format, names, 2, rand.New(rand.NewPCG(1, 1)))
	if odds[1].Playoffs <= odds[2].Playoffs {
		t.Errorf("B %.3f should beat C %.3f: same record, more points a day", odds[1].Playoffs, odds[2].Playoffs)
	}
	if len(places[rows[0].FranchiseID]) != simulations {
		t.Errorf("want a place for every run")
	}
	if w := odds[0].Wins + odds[0].Losses + 0; w != 10 {
		t.Errorf("A's projected record adds up to %v games, want 10", w)
	}
}

func TestSettled(t *testing.T) {
	format := settings.Format{Type: settings.FormatHeadToHead, PlayoffTeams: 2}
	rows, games, _ := league(4, 2) // two matchups each still to play
	rows[0].Wins, rows[1].Wins, rows[2].Wins, rows[3].Wins = 9, 6, 5, 2
	s := settled(rows, games, 2, format)
	// A on 9 can be matched by nobody else (B tops out at 8): clinched.
	// D on 2 can reach 4, but A, B and C already have more: out.
	if s[rows[0].FranchiseID] != 1 || s[rows[3].FranchiseID] != -1 || s[rows[1].FranchiseID] != 0 || s[rows[2].FranchiseID] != 0 {
		t.Errorf("settled %v, want A in, D out, B and C open", s)
	}
	if s := settled(rows[:2], nil, 2, format); s[rows[0].FranchiseID] != 1 || s[rows[1].FranchiseID] != 1 {
		t.Errorf("everyone makes a two-team playoff in a two-team league")
	}
}
