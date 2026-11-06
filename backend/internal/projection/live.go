package projection

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Shape is how a competition's games run, and how far a player's fantasy
// points in one game typically fall from what was projected.
type Shape struct {
	Periods int           // quarters, halves, periods or innings in regulation
	Minutes float64       // on the clock in each; zero for innings
	Length  time.Duration // a game from start to finish, for a feed that gives no clock
	// Spread is the typical miss on one game, as a share of his projection:
	// small where scoring is steady (basketball), large where it is lumpy
	// (a goal, a home run).
	Spread float64
}

var (
	ordinalAt = regexp.MustCompile(`(\d+)(st|nd|rd|th)`)
	clockAt   = regexp.MustCompile(`(\d+):(\d+)`)
	overtime  = regexp.MustCompile(`\b\d*(ot|so)\b`) // "OT", "2OT", "SO"
)

// Left is the share of a game still to be played, from the feed's short
// description of where it stands: "7:32 - 3rd", "End of 1st Half",
// "Halftime", "Bottom 5th". Failing that, from the time since it began.
func (s Shape) Left(detail string, startsAt, now time.Time) float64 {
	done, ok := s.done(detail)
	if !ok && s.Length > 0 {
		done = float64(now.Sub(startsAt)) / float64(s.Length)
	}
	// A game under way always has a little left: overtime, extra innings.
	return min(1, max(0.02, 1-done))
}

// done is the share of regulation played, and whether the description said.
func (s Shape) done(detail string) (float64, bool) {
	text := strings.ToLower(detail)
	switch {
	case s.Periods == 0:
		return 0, false
	case strings.Contains(text, "halftime"):
		return 0.5, true
	case overtime.MatchString(text):
		return 1, true
	}
	match := ordinalAt.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(match[1])
	part := 0.5 // somewhere in it
	switch {
	case strings.HasPrefix(text, "end"):
		part = 1
	case strings.HasPrefix(text, "top"):
		part = 0
	case s.Minutes > 0:
		if c := clockAt.FindStringSubmatch(text); c != nil {
			minutes, _ := strconv.Atoi(c[1])
			seconds, _ := strconv.Atoi(c[2])
			part = 1 - (float64(minutes)+float64(seconds)/60)/s.Minutes
		}
	}
	return min(1, (float64(n-1)+max(0, part))/float64(s.Periods)), true
}

// Outlook is a side's expected final score and how uncertain it is: the
// variance of what it has still to add.
type Outlook struct {
	Points   float64
	Variance float64
}

// Add counts what is left of one game in which he is expected to score
// whole: all of it for a game to come, share of it for one under way.
func (o *Outlook) Add(whole, share, spread float64) {
	o.Points += whole * share
	miss := spread * whole
	o.Variance += miss * miss * share
}

// WinChance is the chance one side finishes ahead of the other, taking
// each final score as normally spread around its expected value. A tie
// counts as half.
func WinChance(us, them Outlook) float64 {
	lead := us.Points - them.Points
	spread := math.Sqrt(us.Variance + them.Variance)
	if spread == 0 {
		switch {
		case lead > 0:
			return 1
		case lead < 0:
			return 0
		}
		return 0.5
	}
	return 0.5 * math.Erfc(-lead/(spread*math.Sqrt2))
}
