package competition

import (
	"time"

	"crossover/internal/ingest"
	"crossover/internal/ingest/espn"
	"crossover/internal/settings"
)

// College and pro basketball share ESPN athlete ids, so they share a
// provider: a college player who reaches the NBA stays the same row.
const basketballProvider = "espn_basketball"

var (
	basketballPositions = []string{"G", "F", "C", "PG", "SG", "SF", "PF"}
	basketballStats     = []Stat{
		{"pts", "Points"}, {"reb", "Rebounds"}, {"ast", "Assists"}, {"stl", "Steals"},
		{"blk", "Blocks"}, {"tov", "Turnovers"}, {"tpm", "Three-pointers made"},
		{"fgm", "Field goals made"}, {"fga", "Field goals attempted"},
		{"ftm", "Free throws made"}, {"fta", "Free throws attempted"},
	}
	basketballScoring = map[string]float64{"pts": 1, "reb": 1.2, "ast": 1.5, "stl": 3, "blk": 3, "tov": -1}

	// ESPN's box score names for those stats.
	basketballFeed = map[string]string{
		"points": "pts", "rebounds": "reb", "assists": "ast", "steals": "stl", "blocks": "blk", "turnovers": "tov",
		"threePointFieldGoalsMade": "tpm", "fieldGoalsMade": "fgm", "fieldGoalsAttempted": "fga",
		"freeThrowsMade": "ftm", "freeThrowsAttempted": "fta",
	}
	basketballSeasonGroups = []string{"general", "offensive", "defensive"}
)

func cbb(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 10, 5
	rules.Lineup.Slots = []settings.Slot{
		slot("G", 2, "G", "PG", "SG"),
		slot("F", 2, "F", "SF", "PF"),
		slot("C", 1, "C"),
		slot("UTIL", 1, settings.AnyPosition),
	}
	rules.Scoring = basketballScoring
	// College players carry into the NBA league when the dynasty has one;
	// the setup wizard drops this if it does not.
	rules.Continuity = &settings.Continuity{Into: "nba", LandOn: settings.ListReserve}

	source := espn.New(client, espn.League{
		Path: "basketball/mens-college-basketball", Provider: basketballProvider, Stats: basketballFeed,
		Scoreboard:   "groups=50&limit=500", // every Division I game, not only the featured ones
		Groups:       collegeConferences,
		SeasonGroups: basketballSeasonGroups, SeasonStarts: time.November, SeasonSpansYears: true,
	})
	return Competition{
		Key: "cbb", Name: "College Basketball",
		Positions: basketballPositions, Stats: basketballStats, Defaults: rules,
		Season: [2]string{"11-03", "04-06"},
		Source: source, Games: source, Seasons: source,
		Prospects: espn.NewRecruits(client),
	}
}

// collegeConferences are the conferences the college league draws from, by
// ESPN group id. Players elsewhere in Division I are left out.
var collegeConferences = []string{
	"2",  // ACC
	"3",  // Atlantic 10
	"4",  // Big East
	"7",  // Big Ten
	"8",  // Big 12
	"21", // Pac-12
	"23", // SEC
	"44", // Mountain West
}

func nba(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 14, 6
	rules.Lineup.Slots = []settings.Slot{
		slot("G", 2, "G", "PG", "SG"),
		slot("F", 2, "F", "SF", "PF"),
		slot("C", 1, "C"),
		slot("UTIL", 2, settings.AnyPosition),
	}
	rules.Scoring = basketballScoring

	source := espn.New(client, espn.League{
		Path: "basketball/nba", Provider: basketballProvider, Stats: basketballFeed,
		SeasonGroups: basketballSeasonGroups, SeasonStarts: time.October, SeasonSpansYears: true,
	})
	return Competition{
		Key: "nba", Name: "NBA",
		Positions: basketballPositions, Stats: basketballStats, Defaults: rules,
		Season: [2]string{"10-20", "04-12"},
		Source: source, Games: source, Seasons: source,
		// College players are already in the pool; this adds the rest.
		Prospects: espn.NewDraft(client, espn.Draft{Path: "basketball/leagues/nba", Provider: basketballProvider, Month: time.June, Classes: 3}),
	}
}
