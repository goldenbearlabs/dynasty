package scoring

import (
	"crossover/internal/db"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"strings"
	"testing"
	"time"
)

func TestCustomScheduleDates(t *testing.T) {
	day := func(n int) time.Time { return sportsday.Today().AddDate(0, 0, n) }
	period := func(start, end int, playoff bool) CustomPeriod {
		return CustomPeriod{StartsOn: day(start).Format(time.DateOnly), EndsOn: day(end).Format(time.DateOnly), IsPlayoff: playoff}
	}
	season := db.Season{StartsOn: sportsday.Date(day(2)), EndsOn: sportsday.Date(day(40))}
	tests := []struct {
		name    string
		periods []CustomPeriod
		kept    []db.Period
		valid   bool
	}{
		{"variable lengths", []CustomPeriod{period(2, 4, false), period(5, 20, false), period(21, 23, true), period(24, 40, true)}, nil, true},
		{"rest days", []CustomPeriod{period(2, 4, false), period(8, 10, true), period(20, 30, true)}, nil, true},
		{"overlap", []CustomPeriod{period(2, 10, false), period(10, 20, true), period(21, 30, true)}, nil, false},
		{"backwards", []CustomPeriod{period(3, 2, false), period(10, 20, true), period(21, 30, true)}, nil, false},
		{"outside season", []CustomPeriod{period(2, 10, false), period(11, 20, true), period(21, 41, true)}, nil, false},
		{"today locked", []CustomPeriod{period(0, 10, false), period(11, 20, true), period(21, 30, true)}, nil, false},
		{"missing playoff round", []CustomPeriod{period(2, 10, false), period(11, 20, true)}, nil, false},
		{"regular after playoffs", []CustomPeriod{period(2, 10, true), period(11, 20, false), period(21, 30, true)}, nil, false},
		{"no regular season", []CustomPeriod{period(2, 10, true), period(11, 20, true)}, nil, false},
		{"kept regular season", []CustomPeriod{period(11, 20, true), period(21, 30, true)}, []db.Period{{EndsOn: sportsday.Date(day(10))}}, true},
		{"overlap kept", []CustomPeriod{period(10, 20, true), period(21, 30, true)}, []db.Period{{EndsOn: sportsday.Date(day(10))}}, false},
		{"invalid date", []CustomPeriod{{StartsOn: "invalid", EndsOn: "2026-10-20"}}, nil, false},
		{"empty", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateCustomPeriods(tt.periods, season, tt.kept, 2, settings.Lineup{})
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v, error=%v", tt.valid, err)
			}
		})
	}
}

// A doubled matchup in a weekly-lineup league must run whole lineup weeks.
func TestDoubleGamesWholeWeeks(t *testing.T) {
	day := func(n int) string { return sportsday.Today().AddDate(0, 0, n).Format(time.DateOnly) }
	season := db.Season{StartsOn: sportsday.Date(sportsday.Today().AddDate(0, 0, 2)), EndsOn: sportsday.Date(sportsday.Today().AddDate(0, 0, 60))}
	weekly := settings.Lineup{Period: settings.PeriodWeek, WeekStart: strings.ToLower(sportsday.Today().AddDate(0, 0, 2).Weekday().String())}
	daily := settings.Lineup{Period: "day", WeekStart: weekly.WeekStart}
	schedule := func(start, end int) []CustomPeriod {
		return []CustomPeriod{{StartsOn: day(start), EndsOn: day(end), DoubleGames: true}, {StartsOn: day(30), EndsOn: day(36), IsPlayoff: true}}
	}
	tests := []struct {
		name   string
		lineup settings.Lineup
		start  int
		end    int
		valid  bool
	}{
		{"two whole weeks", weekly, 2, 15, true},
		{"one and a half weeks", weekly, 2, 12, false},
		{"off the week start", weekly, 3, 16, false},
		{"daily lineups, any dates", daily, 3, 12, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateCustomPeriods(schedule(tt.start, tt.end), season, nil, 1, tt.lineup)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v, error=%v", tt.valid, err)
			}
		})
	}
}
