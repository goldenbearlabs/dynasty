package competition

import (
	"time"

	"crossover/internal/ingest"
	"crossover/internal/ingest/espn"
	"crossover/internal/settings"
)

func nfl(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 20, 6
	rules.Lineup.Period, rules.Lineup.WeekStart = settings.PeriodWeek, "tuesday" // a week runs Thursday to Monday
	rules.Lineup.Slots = []settings.Slot{
		slot("QB", 1, "QB"),
		slot("RB", 2, "RB", "FB"),
		slot("WR", 2, "WR"),
		slot("TE", 1, "TE"),
		slot("FLEX", 1, "RB", "FB", "WR", "TE"),
		slot("K", 1, "PK"),
	}
	rules.Scoring = map[string]float64{
		"pass_yds": 0.04, "pass_td": 4, "interceptions": -2,
		"rush_yds": 0.1, "rush_td": 6,
		"receptions": 0.5, "rec_yds": 0.1, "rec_td": 6,
		"fumbles_lost": -2, "fg_made": 3, "xp_made": 1,
	}
	rules.Draft.Rounds = 4

	source := espn.New(client, espn.League{
		Path: "football/nfl", Provider: "espn_football",
		// "interceptions" is a passing stat and a defensive one, so it is named by group.
		Stats: map[string]string{
			"passingYards": "pass_yds", "passingTouchdowns": "pass_td", "passing.interceptions": "interceptions",
			"rushingYards": "rush_yds", "rushingTouchdowns": "rush_td",
			"receptions": "receptions", "receivingYards": "rec_yds", "receivingTouchdowns": "rec_td",
			"fumblesLost": "fumbles_lost", "fieldGoalsMade": "fg_made", "extraPointsMade": "xp_made",
			// Season totals split fumbles by how the ball was being carried.
			"rushingFumblesLost": "fumbles_lost", "receivingFumblesLost": "fumbles_lost",
		},
		SeasonGroups: []string{"passing", "rushing", "receiving", "kicking"},
		SeasonStarts: time.September, // numbered by the year it starts in
	})
	return Competition{
		Key: "nfl", Name: "NFL",
		Positions: []string{
			"QB", "RB", "FB", "WR", "TE", "PK", "P",
			"LB", "CB", "S", "DB", "DT", "DE", "OT", "G", "C", "OL", "LS",
		},
		Stats: []Stat{
			{"pass_yds", "Passing yards"}, {"pass_td", "Passing touchdowns"}, {"interceptions", "Interceptions thrown"},
			{"rush_yds", "Rushing yards"}, {"rush_td", "Rushing touchdowns"},
			{"receptions", "Receptions"}, {"rec_yds", "Receiving yards"}, {"rec_td", "Receiving touchdowns"},
			{"fumbles_lost", "Fumbles lost"}, {"fg_made", "Field goals made"}, {"xp_made", "Extra points made"},
		},
		Defaults: rules,
		Season:   [2]string{"09-09", "01-03"},
		Source:   source,
		Games:    source,
		Seasons:  source,
		// Drafted rookies arrive through rosters; this adds the class to come.
		Prospects: espn.NewDraft(client, espn.Draft{Path: "football/leagues/nfl", Provider: "espn_football", Month: time.April}),
	}
}
