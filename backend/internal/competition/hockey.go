package competition

import (
	"crossover/internal/ingest"
	"crossover/internal/ingest/nhle"
	"crossover/internal/settings"
)

func nhl(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 18, 8
	rules.Lineup.Slots = []settings.Slot{
		slot("C", 2, "C"),
		slot("W", 4, "L", "R"),
		slot("D", 4, "D"),
		slot("G", 1, "G"),
	}
	rules.Scoring = map[string]float64{
		"goals": 3, "assists": 2, "shots": 0.4, "hits": 0.2, "blocks": 0.4,
		"saves": 0.2, "goals_against": -1, "goalie_wins": 4,
	}

	source := nhle.New(client)
	return Competition{
		Key: "nhl", Name: "NHL",
		Positions: []string{"C", "L", "R", "D", "G"},
		Stats: []Stat{
			{"goals", "Goals"}, {"assists", "Assists"}, {"shots", "Shots on goal"},
			{"hits", "Hits"}, {"blocks", "Blocked shots"}, {"pp_goals", "Power-play goals"},
			{"pim", "Penalty minutes"}, {"plus_minus", "Plus/minus"},
			{"saves", "Saves"}, {"goals_against", "Goals against"}, {"goalie_wins", "Goalie wins"},
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
