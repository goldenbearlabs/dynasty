package lineup

import (
	"encoding/json"
	"testing"
	"time"

	"crossover/internal/settings"
)

// A picked game he finished without a stat gives way to his next game, or,
// with none left to play, to his latest earlier one since he joined the slot.
func TestSettleMissedPick(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	rules := settings.Lineup{Lock: settings.LockGameStart, Slots: []settings.Slot{{Name: "G", Count: 1, GamesPerWeek: 1}}}
	played := func(day string, hoursAgo int, stats string, points float64) Game {
		return Game{Day: day, StartsAt: now.Add(-time.Duration(hoursAgo) * time.Hour), Status: "final", Stats: json.RawMessage(stats), Points: points}
	}
	ahead := Game{Day: "2026-10-30", StartsAt: now.Add(48 * time.Hour), Status: "scheduled", Stats: json.RawMessage(`{}`)}
	counted := func(p Player) (days []string) {
		for _, g := range p.Games {
			if g.Counts {
				days = append(days, g.Day)
			}
		}
		return days
	}
	tests := []struct {
		name   string
		joined string
		games  []Game
		want   []string
		points float64
		locked bool
	}{
		{"missed, a game to come", "", []Game{played("2026-10-26", 50, `{"pts":20}`, 20), played("2026-10-28", 2, `{}`, 0), ahead}, []string{"2026-10-30"}, 0, false},
		{"missed the last", "", []Game{played("2026-10-26", 50, `{"pts":20}`, 20), played("2026-10-27", 26, `{"pts":8}`, 8), played("2026-10-28", 2, `{"pts":0}`, 0)}, []string{"2026-10-27"}, 8, true},
		{"missed the last, joined late", "2026-10-28", []Game{played("2026-10-27", 26, `{"pts":8}`, 8), played("2026-10-28", 2, `{}`, 0)}, nil, 0, false},
		{"played the pick", "", []Game{played("2026-10-27", 26, `{"pts":8}`, 8), played("2026-10-28", 2, `{"pts":5}`, 5)}, []string{"2026-10-28"}, 5, true},
	}
	for _, tc := range tests {
		p := Player{Slot: "G", CountsFrom: "2026-10-28", joined: tc.joined, Games: tc.games}
		settle(&p, rules, now, 1)
		got := counted(p)
		if len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) || p.Points != tc.points || p.Locked != tc.locked {
			t.Errorf("%s: counted %v, %v points, locked %v; want %v, %v, %v", tc.name, got, p.Points, p.Locked, tc.want, tc.points, tc.locked)
		}
	}
}
