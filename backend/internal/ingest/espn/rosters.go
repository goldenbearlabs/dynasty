package espn

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"crossover/internal/ingest"
)

// ESPN's default roster uses {"href": ...}; season-specific rosters may
// instead use a URL string, or null when no headshot is available.
type rosterHeadshot struct {
	Href string `json:"href"`
}

func (h *rosterHeadshot) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &h.Href)
	}
	type object rosterHeadshot
	return json.Unmarshal(data, (*object)(h))
}

// RosterAddition fills a verified omission in ESPN's roster for one season.
// Use the ordinary ESPN athlete id so later feed updates keep the same row.
// SourceURL records the official school roster used to verify membership.
type RosterAddition struct {
	Season    int
	TeamID    string
	Player    ingest.Player
	SourceURL string
}

func (s *Source) rosterSeason(now time.Time) int {
	if s.RosterSeasonStarts == 0 {
		return 0
	}
	year := now.Year()
	if s.SeasonSpansYears && now.Month() >= s.RosterSeasonStarts {
		year++
	}
	return year
}

func (s *Source) rosterURL(url string) string {
	if year := s.rosterSeason(time.Now()); year != 0 {
		url += fmt.Sprintf("&season=%d", year)
	}
	return url
}

func (s *Source) addRosterPlayers(players []ingest.Player, teamID string, now time.Time) []ingest.Player {
	season := s.rosterSeason(now)
	for _, addition := range s.RosterAdditions {
		if addition.Season != season || addition.TeamID != teamID {
			continue
		}
		if slices.ContainsFunc(players, func(p ingest.Player) bool {
			return p.ProviderID == addition.Player.ProviderID
		}) {
			continue // prefer the feed's details once it lists the player
		}
		p := addition.Player
		p.Provider = s.Provider
		if len(s.Positions) > 0 && !slices.ContainsFunc(p.Positions, func(position string) bool {
			return slices.Contains(s.Positions, position)
		}) {
			continue
		}
		players = append(players, p)
	}
	return players
}
