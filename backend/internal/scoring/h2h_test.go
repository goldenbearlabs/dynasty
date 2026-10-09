package scoring

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/settings"
)

func team(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{n}, Valid: true}
}

func TestRoundRobin(t *testing.T) {
	for _, size := range []int{2, 3, 4, 5, 8, 11} {
		var teams []pgtype.UUID
		for i := range size {
			teams = append(teams, team(byte(i+1)))
		}
		rounds := roundRobin(teams)

		met := map[[2]pgtype.UUID]int{}
		for r, round := range rounds {
			playing := map[pgtype.UUID]bool{}
			byes := 0
			for _, pair := range round {
				home, away := pair[0], pair[1]
				if !home.Valid {
					t.Fatalf("%d teams, round %d: a matchup has no home side", size, r)
				}
				if !away.Valid {
					byes++
				} else if home.Bytes[0] < away.Bytes[0] {
					met[[2]pgtype.UUID{home, away}]++
				} else {
					met[[2]pgtype.UUID{away, home}]++
				}
				for _, side := range pair {
					if !side.Valid {
						continue
					}
					if playing[side] {
						t.Fatalf("%d teams, round %d: a team plays twice", size, r)
					}
					playing[side] = true
				}
			}
			// Every team appears each round, one of them on a bye when the count is odd.
			if byes != size%2 || len(playing) != size {
				t.Errorf("%d teams, round %d: %d byes and %d teams accounted for", size, r, byes, len(playing))
			}
		}
		// Everyone meets everyone exactly once.
		if want := size * (size - 1) / 2; len(met) != want {
			t.Errorf("%d teams: %d distinct pairings, want %d", size, len(met), want)
		}
		for pair, times := range met {
			if times != 1 {
				t.Errorf("%d teams: %v met %d times", size, pair, times)
			}
		}
	}
}

func TestPlayoffRounds(t *testing.T) {
	for teams, want := range map[int]int{2: 1, 3: 2, 4: 2, 5: 3, 6: 3, 8: 3, 9: 4} {
		if got := playoffRounds(teams); got != want {
			t.Errorf("playoffRounds(%d) = %d, want %d", teams, got, want)
		}
	}
}

func TestRecordOrder(t *testing.T) {
	// One win in three beats no wins and two ties only on points: both have
	// won a third of their matchups.
	if a, b := (Row{Wins: 1, Losses: 2}), (Row{Losses: 1, Ties: 2}); share(a) != share(b) {
		t.Errorf("1-2 and 0-1-2 should be level, got %v and %v", share(a), share(b))
	}
	if better, worse := (Row{Wins: 2, Ties: 1}), (Row{Wins: 2, Losses: 1}); share(better) <= share(worse) {
		t.Error("2-0-1 should rank above 2-1")
	}
	// A team with a bye has played fewer matchups.
	if perfect, good := (Row{Wins: 2}), (Row{Wins: 2, Losses: 1}); share(perfect) <= share(good) {
		t.Error("2-0 should rank above 2-1")
	}
}

// One-week matchups in a weekly league follow the lineup's weeks; a short
// stretch at either end of the season joins the week beside it.
func TestPeriodSpans(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	show := func(spans [][2]time.Time) string {
		out := ""
		for _, s := range spans {
			out += s[0].Format("01-02") + ".." + s[1].Format("01-02") + " "
		}
		return out
	}
	weekly := settings.League{
		Format: settings.Format{MatchupDays: 7},
		Lineup: settings.Lineup{Period: settings.PeriodWeek, WeekStart: "monday"},
	}
	tests := []struct {
		name, start, end, want string
		rules                  settings.League
	}{
		// 2026-10-20 is a Tuesday: the opening six days are a matchup of their own.
		{"starts mid-week", "2026-10-20", "2026-11-08", "10-20..10-25 10-26..11-01 11-02..11-08 ", weekly},
		// 2026-10-23 is a Friday: three days are too few, so they join the first full week.
		{"starts late in the week", "2026-10-23", "2026-11-08", "10-23..11-01 11-02..11-08 ", weekly},
		// Two days left over at the end join the last week.
		{"ends early in the week", "2026-10-19", "2026-11-03", "10-19..10-25 10-26..11-03 ", weekly},
		{"ends late in the week", "2026-10-19", "2026-11-05", "10-19..10-25 10-26..11-01 11-02..11-05 ", weekly},
		// Daily lineups, or another length: equal periods from the first day, the remainder unplayed.
		{"daily lineups", "2026-10-20", "2026-11-08", "10-20..10-26 10-27..11-02 ",
			settings.League{Format: settings.Format{MatchupDays: 7}, Lineup: settings.Lineup{Period: settings.PeriodDay, WeekStart: "monday"}}},
		{"three-day matchups", "2026-10-20", "2026-10-27", "10-20..10-22 10-23..10-25 ",
			settings.League{Format: settings.Format{MatchupDays: 3}, Lineup: weekly.Lineup}},
	}
	for _, tt := range tests {
		if got := show(periodSpans(day(tt.start), day(tt.end), tt.rules)); got != tt.want {
			t.Errorf("%s:\n got  %s\n want %s", tt.name, got, tt.want)
		}
	}
}
