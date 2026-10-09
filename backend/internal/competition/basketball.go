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
		{"dd", "Double-doubles"}, {"td", "Triple-doubles"},
		{"pts40", "40-point games"}, {"pts50", "50-point games"},
	}
	// A 50-point game is also a 40-point game, so it earns both bonuses.
	basketballScoring = map[string]float64{
		"pts": 0.5, "reb": 1, "ast": 1, "stl": 2, "blk": 2, "tov": -1, "tpm": 0.5,
		"dd": 1, "td": 2, "pts40": 2, "pts50": 2,
	}

	// ESPN's names for those stats, in box scores and season totals.
	basketballFeed = map[string]string{
		"points": "pts", "rebounds": "reb", "assists": "ast", "steals": "stl", "blocks": "blk", "turnovers": "tov",
		"threePointFieldGoalsMade": "tpm", "fieldGoalsMade": "fgm", "fieldGoalsAttempted": "fga",
		"freeThrowsMade": "ftm", "freeThrowsAttempted": "fta",
		"doubleDouble": "dd", "tripleDouble": "td", // season totals only; in a game they are worked out below
	}
	basketballSeasonGroups = []string{"general", "offensive", "defensive"}
)

// basketballGame adds what a single game's line amounts to: a double- or
// triple-double (ten or more in two or three of points, rebounds, assists,
// steals and blocks) and a 40- or 50-point game.
func basketballGame(stats map[string]float64) {
	tens := 0
	for _, stat := range []string{"pts", "reb", "ast", "stl", "blk"} {
		if stats[stat] >= 10 {
			tens++
		}
	}
	for stat, earned := range map[string]bool{"dd": tens >= 2, "td": tens >= 3, "pts40": stats["pts"] >= 40, "pts50": stats["pts"] >= 50} {
		if earned {
			stats[stat] = 1
		}
	}
}

// basketballLineup is three guards, three forwards and a centre, then any
// extra players. Every slot counts one game a week: the manager picks which.
// The feeds mostly label players only G, F or C, so the slots do too.
func basketballLineup(guards, forwards, util int) settings.Lineup {
	slots := []settings.Slot{
		slot("G", guards, "G", "PG", "SG"),
		slot("F", forwards, "F", "SF", "PF"),
		slot("C", 1, "C"),
	}
	if util > 0 {
		slots = append(slots, slot("UTIL", util, settings.AnyPosition))
	}
	for i := range slots {
		slots[i].GamesPerWeek = 1
	}
	return settings.Lineup{Period: settings.PeriodWeek, WeekStart: "monday", Lock: settings.LockGameStart, Slots: slots}
}

func cbb(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 12, 5 // 7 starters and 5 on the bench
	rules.Lineup = basketballLineup(3, 3, 0)
	rules.Lineup.Conferences = settings.DefaultCollegeConferences()
	rules.Roster.ReserveEligibility = settings.ReserveProspectsOrIneligible
	rules.Scoring = basketballScoring
	// College players carry into the NBA league when the dynasty has one;
	// the setup wizard drops this if it does not.
	rules.Continuity = &settings.Continuity{Into: "nba", LandOn: settings.ListReserve}

	source := espn.New(client, espn.League{
		Path: "basketball/mens-college-basketball", Provider: basketballProvider, Stats: basketballFeed, Derive: basketballGame,
		Scoreboard:       "groups=50&limit=500", // every Division I game, not only the featured ones
		ConferenceGroups: collegeConferenceIDs(),
		SeasonGroups:     basketballSeasonGroups, SeasonStarts: time.November, SeasonSpansYears: true,
	})
	return Competition{
		Key: "cbb", Name: "College Basketball", Conferences: collegeConferenceCatalog,
		Positions: basketballPositions, Stats: basketballStats, Defaults: rules,
		Season: [2]string{"11-03", "04-06"},
		Source: source, Games: source, Seasons: source,
		Prospects: espn.NewRecruits(client),
	}
}

// All Division I conferences stay in the draft and reserve pool.
var collegeConferenceCatalog = []Conference{
	{"1", "America East"}, {"62", "American"}, {"3", "A-10"}, {"2", "ACC"},
	{"46", "ASUN"}, {"8", "Big 12"}, {"4", "Big East"}, {"5", "Big Sky"},
	{"6", "Big South"}, {"7", "Big Ten"}, {"9", "Big West"}, {"10", "CAA"},
	{"11", "Conference USA"}, {"45", "Horizon"}, {"12", "Ivy League"},
	{"13", "MAAC"}, {"14", "MAC"}, {"16", "MEAC"}, {"18", "Missouri Valley"},
	{"44", "MWC"}, {"19", "NEC"}, {"20", "Ohio Valley"}, {"21", "Pac-12"},
	{"22", "Patriot"}, {"23", "SEC"}, {"24", "Southern"}, {"25", "Southland"},
	{"26", "SWAC"}, {"49", "Summit"}, {"27", "Sun Belt"}, {"30", "WAC"}, {"29", "WCC"},
}

func collegeConferenceIDs() []string {
	ids := make([]string, len(collegeConferenceCatalog))
	for i, c := range collegeConferenceCatalog {
		ids[i] = c.ID
	}
	return ids
}

func nba(client *ingest.Client) Competition {
	rules := defaults()
	rules.Roster.Main, rules.Roster.Reserve = 14, 3 // 9 starters and 5 on the bench
	rules.Lineup = basketballLineup(3, 3, 2)
	rules.Scoring = basketballScoring

	source := espn.New(client, espn.League{
		Path: "basketball/nba", Provider: basketballProvider, Stats: basketballFeed, Derive: basketballGame,
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

// The WNBA's athlete ids are shared with the women's college game, not
// with the men's leagues above.
const womensBasketballProvider = "espn_womens_basketball"

func wnba(client *ingest.Client) Competition {
	rules := defaults()
	// The league is 15 teams of 12, so 16 franchises can hold little more
	// than their starters before the waiver wire is empty: 6 and 2.
	rules.Roster.Main, rules.Roster.Reserve = 8, 1
	rules.Lineup = basketballLineup(2, 2, 1)
	rules.Scoring = basketballScoring

	source := espn.New(client, espn.League{
		Path: "basketball/wnba", Provider: womensBasketballProvider, Stats: basketballFeed, Derive: basketballGame,
		SeasonGroups: basketballSeasonGroups, SeasonStarts: time.May, // numbered by the year it is played in
	})
	return Competition{
		Key: "wnba", Name: "WNBA",
		Positions: basketballPositions, Stats: basketballStats, Defaults: rules,
		Season: [2]string{"05-08", "09-25"},
		Source: source, Games: source, Seasons: source,
		// Recent picks who are not on a roster, and the next class once ESPN ranks it.
		Prospects: espn.NewDraft(client, espn.Draft{
			Path: "basketball/leagues/wnba", Provider: womensBasketballProvider, Month: time.April, Classes: 2,
		}),
	}
}
