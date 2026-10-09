package competition

import (
	"crossover/internal/ingest"
	"crossover/internal/ingest/mlbam"
	"crossover/internal/settings"
)

func mlb(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 24, 10
	rules.Lineup.Slots = []settings.Slot{
		slot("C", 1, "C"),
		slot("1B", 1, "1B"),
		slot("2B", 1, "2B"),
		slot("3B", 1, "3B"),
		slot("SS", 1, "SS"),
		slot("OF", 3, "LF", "CF", "RF", "OF"),
		slot("UTIL", 1, settings.AnyPosition),
		slot("P", 7, "P", "TWP"),
	}
	rules.Scoring = map[string]float64{
		"bat_r": 1, "bat_h": 1, "bat_hr": 3, "bat_rbi": 1, "bat_sb": 2, "bat_bb": 1, "bat_so": -0.5,
		"pit_ip": 3, "pit_so": 1, "pit_w": 4, "pit_sv": 5, "pit_er": -2, "pit_h": -0.5, "pit_bb": -0.5,
	}
	rules.Draft.Rounds = 5

	source := mlbam.New(client)
	return Competition{
		Key: "mlb", Name: "MLB",
		Positions: []string{"P", "C", "1B", "2B", "3B", "SS", "LF", "CF", "RF", "OF", "DH", "TWP"},
		Stats: []Stat{
			{"bat_r", "Runs"}, {"bat_h", "Hits"}, {"bat_hr", "Home runs"}, {"bat_rbi", "Runs batted in"},
			{"bat_sb", "Stolen bases"}, {"bat_bb", "Walks"}, {"bat_so", "Strikeouts (batting)"},
			{"pit_ip", "Innings pitched"}, {"pit_so", "Strikeouts (pitching)"}, {"pit_w", "Wins"},
			{"pit_sv", "Saves"}, {"pit_er", "Earned runs allowed"}, {"pit_h", "Hits allowed"},
			{"pit_bb", "Walks allowed"},
		},
		Defaults:  rules,
		Season:    [2]string{"03-26", "09-27"},
		Source:    source,
		Games:     source,
		Seasons:   source,
		Prospects: source,
	}
}
