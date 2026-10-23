package projection

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v; want %v", name, got, want)
	}
}

func TestBaseline(t *testing.T) {
	lengths := map[int]float64{2026: 10, 2025: 80}
	seasons := []Season{
		{Year: 2026, Games: 10, Stats: map[string]float64{"goals": 10}}, // a goal a game so far
		{Year: 2025, Games: 40, Stats: map[string]float64{"goals": 0}},  // none last season, in half the games
		{Year: 2020, Games: 82, Stats: map[string]float64{"goals": 82}}, // too old to count
	}
	perGame, plays := Baseline(seasons, lengths)
	// Last season counts a quarter: 10 goals over 10 + 10 games, in 20 of 10 + 20.
	near(t, "goals", perGame["goals"], 0.5)
	near(t, "plays", plays, 20.0/30)

	// A player who has not played this season is measured against it all the same.
	_, plays = Baseline([]Season{{Year: 2025, Games: 80, Stats: map[string]float64{"goals": 8}}}, lengths)
	near(t, "plays without this season", plays, 20.0/30)

	if perGame, plays := Baseline(nil, lengths); len(perGame) != 0 || plays != 0 {
		t.Errorf("no seasons = %v, %v", perGame, plays)
	}
}

func TestPoints(t *testing.T) {
	scoring := map[string]float64{"goals": 3, "saves": 0.2, "goalie_wins": 4}
	follows := map[string]string{"saves": "opponent", "goalie_wins": "win"}
	skater := map[string]float64{"goals": 1}
	goalie := map[string]float64{"saves": 25, "goalie_wins": 0.5}

	near(t, "no line", Points(skater, 1, Game{}, scoring, follows), 3)
	near(t, "half his games", Points(skater, 0.5, Game{}, scoring, follows), 1.5)
	// His side is expected to score 3.6 against an average of 3: 20% more.
	near(t, "good line", Points(skater, 1, Game{TeamScore: 3.6, Average: 3}, scoring, follows), 3.6)
	// The line is held to half again, however lopsided.
	near(t, "held", Points(skater, 1, Game{TeamScore: 9, Average: 3}, scoring, follows), 4.5)

	// A goalie facing a strong side makes more saves; a 60% favourite wins more.
	near(t, "goalie", Points(goalie, 0.6, Game{OpponentScore: 3.3, Win: 0.6, Average: 3, Starter: "expected"}, scoring, follows),
		25*1.1*0.2+0.5*1.2*4)
	near(t, "backup", Points(goalie, 0.6, Game{Starter: "other"}, scoring, follows), 0)
	near(t, "injured", Points(skater, 1, Game{Injured: true}, scoring, follows), 0)
}
