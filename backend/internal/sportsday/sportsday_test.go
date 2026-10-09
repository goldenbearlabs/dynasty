package sportsday

import (
	"testing"
	"time"
)

func TestOf(t *testing.T) {
	tests := []struct{ name, moment, want string }{
		{"an afternoon game", "2026-11-14T18:00:00Z", "2026-11-14"},
		// 10:30pm Pacific is 1:30am Eastern, but it is still Saturday's game.
		{"a late West Coast tip-off", "2026-11-15T06:30:00Z", "2026-11-14"},
		// 9:30am Eastern, a London kickoff.
		{"an early morning game", "2026-11-15T14:30:00Z", "2026-11-15"},
	}
	for _, tt := range tests {
		moment, _ := time.Parse(time.RFC3339, tt.moment)
		if got := Of(moment).Format(time.DateOnly); got != tt.want {
			t.Errorf("%s: Of(%s) = %s, want %s", tt.name, tt.moment, got, tt.want)
		}
	}

	// A day contains its own start, and the moment before belongs to the day before.
	day, _ := Parse("2026-11-14")
	if !Of(Start(day)).Equal(day) || !Of(Start(day).Add(-time.Second)).Equal(day.AddDate(0, 0, -1)) {
		t.Errorf("Start(%v) = %v does not sit on the day's boundary", day, Start(day))
	}
}

func TestWeekStart(t *testing.T) {
	day := func(s string) time.Time { d, _ := Parse(s); return d }
	tests := []struct{ day, weekday, want string }{
		{"2026-10-08", "tuesday", "2026-10-06"}, // a Thursday in an NFL week
		{"2026-10-06", "tuesday", "2026-10-06"}, // the first day is its own start
		{"2026-10-05", "tuesday", "2026-09-29"}, // Monday night belongs to the week before
		{"2026-10-08", "monday", "2026-10-05"},
		{"2026-10-08", "", "2026-10-05"}, // unset means Monday
	}
	for _, tt := range tests {
		if got := WeekStart(day(tt.day), tt.weekday).Format(time.DateOnly); got != tt.want {
			t.Errorf("WeekStart(%s, %q) = %s, want %s", tt.day, tt.weekday, got, tt.want)
		}
	}
}
