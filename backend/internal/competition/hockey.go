package competition

import (
	"crossover/internal/ingest"
	"crossover/internal/ingest/nhle"
	"crossover/internal/settings"
)

func nhl(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 25, 10 // 13 starters and 12 on the bench
	rules.Lineup.Slots = []settings.Slot{
		slot("F", 8, "C", "L", "R"),
		slot("D", 4, "D"),
		slot("G", 1, "G"),
	}
	// Shots carry real weight because they are the steadiest sign of who
	// the good players are; goalies are scored up to keep pace with skaters.
	rules.Scoring = map[string]float64{
		"goals": 2, "assists": 1, "pp_points": 1, "shots": 0.5, "blocks": 0.25, "hits": 0.1,
		"goalie_wins": 4, "shutouts": 3, "saves": 0.25, "goals_against": -1,
	}

	source := nhle.New(client)
	return Competition{
		Key: "nhl", Name: "NHL",
		Positions: []string{"C", "L", "R", "D", "G"},
		Stats: []Stat{
			{"goals", "Goals"}, {"assists", "Assists"}, {"shots", "Shots on goal"},
			{"hits", "Hits"}, {"blocks", "Blocked shots"}, {"pp_points", "Power-play points"}, {"pp_goals", "Power-play goals"},
			{"pim", "Penalty minutes"}, {"plus_minus", "Plus/minus"},
			{"saves", "Saves"}, {"goals_against", "Goals against"}, {"goalie_wins", "Goalie wins"}, {"shutouts", "Shutouts"},
		},
		Defaults:  rules,
		Season:    [2]string{"10-07", "04-16"},
		Source:    source,
		Games:     source,
		Seasons:   source,
		Career:    source,
		Prospects: source,
	}
}
