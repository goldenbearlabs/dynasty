package competition

import (
	"crossover/internal/ingest"
	"crossover/internal/ingest/mlbam"
	"crossover/internal/settings"
)

func mlb(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 20, 14 // 15 starters and 5 on the bench
	hitters := []string{"C", "1B", "2B", "3B", "SS", "LF", "CF", "RF", "OF", "DH", "TWP"}
	// Lineups are set by the day. A team's pitchers score in four starts a
	// week between them: a fifth start, by anyone, scores nothing.
	rules.Lineup.PitcherStartsPerWeek = 4
	rules.Lineup.Slots = []settings.Slot{
		slot("C", 1, "C"),
		slot("1B", 1, "1B"),
		slot("2B", 1, "2B"),
		slot("3B", 1, "3B"),
		slot("SS", 1, "SS"),
		slot("OF", 3, "LF", "CF", "RF", "OF"),
		slot("UTIL", 1, hitters...), // any hitter, which is where a designated hitter plays
		slot("SP", 4, "SP", "TWP"),
		slot("RP", 2, "RP", "P"),
	}
	// An inning is three outs, so three points.
	rules.Scoring = map[string]float64{
		"bat_tb": 1, "bat_r": 1, "bat_rbi": 1, "bat_bb": 1, "bat_sb": 1, "bat_so": -1,
		"pit_ip": 3, "pit_so": 1, "pit_h": -1, "pit_bb": -1, "pit_er": -2,
		"pit_w": 2, "pit_l": -2, "pit_sv": 5, "pit_hld": 2,
	}

	source := mlbam.New(client)
	return Competition{
		Key: "mlb", Name: "MLB",
		Positions: []string{"SP", "RP", "P", "C", "1B", "2B", "3B", "SS", "LF", "CF", "RF", "OF", "DH", "TWP"},
		Stats: []Stat{
			{"bat_tb", "Total bases"}, {"bat_r", "Runs"}, {"bat_h", "Hits"}, {"bat_hr", "Home runs"}, {"bat_rbi", "Runs batted in"},
			{"bat_sb", "Stolen bases"}, {"bat_bb", "Walks"}, {"bat_so", "Strikeouts (batting)"},
			{"pit_ip", "Innings pitched"}, {"pit_so", "Strikeouts (pitching)"}, {"pit_w", "Wins"}, {"pit_l", "Losses"},
			{"pit_games", "Pitching appearances"}, {"bat_games", "Batting appearances"},
			{"pit_sv", "Saves"}, {"pit_hld", "Holds"}, {"pit_gs", "Games started"}, {"pit_er", "Earned runs allowed"}, {"pit_h", "Hits allowed"},
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
