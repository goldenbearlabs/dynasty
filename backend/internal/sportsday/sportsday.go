// Package sportsday defines the day a game belongs to. Every league here
// plays on the US calendar, so a sports day follows US Eastern time, and it
// turns over at 5am rather than midnight: a game that tips off late on the
// West Coast or in Hawaii belongs to the evening it is played on.
//
// Days are passed around as a time.Time at midnight UTC, which is what the
// database driver uses for a date column.
package sportsday

import (
	"strings"
	"time"
	_ "time/tzdata" // the zone is needed even on a minimal server image

	"github.com/jackc/pgx/v5/pgtype"
)

var eastern = mustLoad("America/New_York")

func mustLoad(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return zone
}

// rollover is how far past midnight Eastern a sports day runs.
const rollover = 5 * time.Hour

// Of returns the sports day a moment falls on.
func Of(moment time.Time) time.Time {
	y, m, d := moment.In(eastern).Add(-rollover).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Calendar returns the plain Eastern calendar date of a moment, with no
// late-night allowance. The feeds file games under this date.
func Calendar(moment time.Time) time.Time {
	y, m, d := moment.In(eastern).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Clock writes a moment for a manager to read: "Oct 14, 9:30 PM ET".
func Clock(moment time.Time) string {
	return moment.In(eastern).Format("Jan 2, 3:04 PM") + " ET"
}

// Today is the current sports day.
func Today() time.Time {
	return Of(time.Now())
}

// Start is the moment a sports day begins.
func Start(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, eastern).Add(rollover)
}

// WeekStart returns the first day of the week containing day, for weeks
// that begin on the named weekday ("monday" ... "sunday").
func WeekStart(day time.Time, weekday string) time.Time {
	back := (int(day.Weekday()) - int(Weekday(weekday)) + 7) % 7
	return day.AddDate(0, 0, -back)
}

// Weekdays are the names a week can start on.
var Weekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// Weekday converts a name to a weekday, defaulting to Monday.
func Weekday(name string) time.Weekday {
	for i, w := range Weekdays {
		if w == strings.ToLower(name) {
			return time.Weekday(i)
		}
	}
	return time.Monday
}

// Parse reads a day written as YYYY-MM-DD.
func Parse(s string) (time.Time, error) {
	return time.Parse(time.DateOnly, s)
}

// Date converts a day to its database form.
func Date(day time.Time) pgtype.Date {
	return pgtype.Date{Time: day, Valid: true}
}
