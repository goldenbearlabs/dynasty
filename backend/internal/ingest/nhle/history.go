package nhle

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"crossover/internal/ingest"
)

// Discover players from this season and the preceding two, including injured
// players and unsigned veterans absent from the current roster feed. Older
// seasons remain research history, rather than admitting every retired player.
func (s *Source) DiscoverySeasons() int { return 3 }

func (s *Source) seasonPlayers(ctx context.Context, year int) (map[string]ingest.Player, error) {
	if year < s.LatestSeason(time.Now())-s.DiscoverySeasons()+1 {
		return nil, nil
	}
	type bio struct {
		PlayerID   int    `json:"playerId"`
		SkaterName string `json:"skaterFullName"`
		GoalieName string `json:"goalieFullName"`
		Position   string `json:"positionCode"`
		BirthDate  string `json:"birthDate"`
	}
	players := map[string]ingest.Player{}
	for _, kind := range []string{"skater", "goalie"} {
		rows, err := report[bio](ctx, s, kind+"/bios", year)
		if err != nil {
			return nil, fmt.Errorf("%d %s identities: %w", year, kind, err)
		}
		for _, row := range rows {
			name, pos := row.SkaterName, row.Position
			if kind == "goalie" {
				name, pos = row.GoalieName, "G"
			}
			if row.PlayerID == 0 || name == "" {
				continue
			}
			p := ingest.Player{Provider: provider, ProviderID: strconv.Itoa(row.PlayerID), FullName: name, BirthDate: ingest.ParseDate(row.BirthDate)}
			if pos != "" {
				p.Positions = []string{position(pos)}
			}
			players[p.ProviderID] = p
		}
	}
	return players, nil
}
