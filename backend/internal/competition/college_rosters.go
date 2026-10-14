package competition

import (
	"crossover/internal/ingest"
	"crossover/internal/ingest/espn"
)

// Verified 2026–27 school roster entries missing from ESPN, checked October
// 13, 2026. These expire when roster imports move to the following season.
// Keep athlete ids rather than recruit ids or names as the player identity.
var collegeRosterAdditions = []espn.RosterAddition{
	{
		Season: 2027, TeamID: "2599",
		Player:    ingest.Player{ProviderID: "5075625", FullName: "Keyshawn Hall", Positions: []string{"F"}, Class: "GR"},
		SourceURL: "https://redstormsports.com/sports/mens-basketball/roster/keyshawn-hall/6431",
	},
	{
		Season: 2027, TeamID: "2599",
		Player:    ingest.Player{ProviderID: "5095152", FullName: "Tounde Yessoufou", Positions: []string{"G"}, Class: "SO"},
		SourceURL: "https://redstormsports.com/sports/mens-basketball/roster",
	},
	{
		Season: 2027, TeamID: "2250",
		Player:    ingest.Player{ProviderID: "5311903", FullName: "Massamba Diop", Positions: []string{"C"}, Class: "SO"},
		SourceURL: "https://gozags.com/sports/mens-basketball/roster/massamba-diop/6670",
	},
}
