package espn

import (
	"math"
	"testing"
)

func TestMinutesFormats(t *testing.T) {
	for _, value := range []struct {
		text    string
		minutes float64
		valid   bool
	}{
		{"32", 32, true}, {"32:30", 32.5, true}, {"0:03", .05, true},
		{"--", 0, false}, {"DNP", 0, false}, {"12:99", 0, false},
	} {
		stats := split("minutes", value.text)
		minutes, ok := stats["minutes"]
		if ok != value.valid || math.Abs(minutes-value.minutes) > 1e-9 {
			t.Errorf("%q: minutes=%v present=%v", value.text, minutes, ok)
		}
	}
}
