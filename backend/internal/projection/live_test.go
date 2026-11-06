package projection

import (
	"math"
	"testing"
	"time"
)

func TestLeft(t *testing.T) {
	nba := Shape{Periods: 4, Minutes: 12, Length: 2 * time.Hour}
	cbb := Shape{Periods: 2, Minutes: 20, Length: 2 * time.Hour}
	mlb := Shape{Periods: 9, Length: 3 * time.Hour}
	start := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC)
	tests := []struct {
		shape  Shape
		detail string
		after  time.Duration
		want   float64
	}{
		{nba, "6:00 - 1st", 0, 0.875},
		{nba, "Halftime", 0, 0.5},
		{nba, "End of 3rd", 0, 0.25},
		{nba, "0:45 - OT", 0, 0.02},
		{cbb, "10:00 - 2nd Half", 0, 0.25},
		{cbb, "End of 1st Half", 0, 0.5},
		{mlb, "Top 1st", 0, 1},
		{mlb, "Bottom 5th", 0, 1 - 4.5/9},
		{mlb, "End 9th", 0, 0.02},
		{nba, "Delayed", 30 * time.Minute, 0.75}, // no clock: by the time since it began
	}
	for _, tc := range tests {
		if got := tc.shape.Left(tc.detail, start, start.Add(tc.after)); math.Abs(got-tc.want) > 0.001 {
			t.Errorf("Left(%q) = %v, want %v", tc.detail, got, tc.want)
		}
	}
}

func TestWinChance(t *testing.T) {
	even := WinChance(Outlook{Points: 100, Variance: 50}, Outlook{Points: 100, Variance: 50})
	ahead := WinChance(Outlook{Points: 110, Variance: 50}, Outlook{Points: 100, Variance: 50})
	behind := WinChance(Outlook{Points: 100, Variance: 50}, Outlook{Points: 110, Variance: 50})
	if math.Abs(even-0.5) > 1e-9 || math.Abs(ahead+behind-1) > 1e-9 || ahead < 0.84 || ahead > 0.85 {
		t.Fatalf("even %v, ahead %v, behind %v; want 0.5 and about 0.84 either way", even, ahead, behind)
	}
	if done := WinChance(Outlook{Points: 90}, Outlook{Points: 80}); done != 1 {
		t.Fatalf("with nothing left, the side ahead wins: got %v", done)
	}
	// A game left to play keeps it open.
	var us Outlook
	us.Points = 80
	us.Add(30, 1, 0.4)
	if got := WinChance(us, Outlook{Points: 100}); got < 0.75 || got > 0.85 {
		t.Fatalf("80 with 30 to come against 100 = %v, want about 0.8: 110 expected, give or take 12", got)
	}
}
