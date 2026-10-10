package espn

import (
	"context"
	"fmt"
	"regexp"

	"crossover/internal/ingest"
)

var athleteLinkID = regexp.MustCompile(`/id/([0-9]+)(?:/|$)`)

// Injuries reads only list placements, or basketball's explicit Out status.
// Questionable, day-to-day, suspension and generic Out in other sports are excluded.
func (s *Source) Injuries(ctx context.Context) ([]ingest.Injury, error) {
	var res struct {
		Status   string `json:"status"`
		Injuries *[]struct {
			Injuries []struct {
				Type struct {
					Name string `json:"name"`
				} `json:"type"`
				Athlete struct {
					ID          string `json:"id"`
					DisplayName string `json:"displayName"`
					Links       []struct {
						Href string `json:"href"`
					} `json:"links"`
				} `json:"athlete"`
			} `json:"injuries"`
		} `json:"injuries"`
	}
	if err := s.client.GetTransient(ctx, s.Base+"/"+s.Path+"/injuries", &res); err != nil {
		return nil, err
	}
	if res.Status != "success" || res.Injuries == nil {
		return nil, fmt.Errorf("espn %s: incomplete injury response", s.Path)
	}
	var injuries []ingest.Injury
	for _, team := range *res.Injuries {
		for _, entry := range team.Injuries {
			designation := injuryDesignation(s.Path, entry.Type.Name)
			if designation == "" {
				continue
			}
			id := entry.Athlete.ID
			for _, link := range entry.Athlete.Links {
				if match := athleteLinkID.FindStringSubmatch(link.Href); id == "" && len(match) > 1 {
					id = match[1]
				}
			}
			injuries = append(injuries, ingest.Injury{Provider: s.Provider, ProviderID: id, FullName: entry.Athlete.DisplayName, Designation: designation})
		}
	}
	return injuries, nil
}

func injuryDesignation(path, code string) string {
	switch code {
	case "INJURY_STATUS_IR":
		return "IR"
	case "INJURY_STATUS_7DAYIL":
		return "IL7"
	case "INJURY_STATUS_10DAYIL":
		return "IL10"
	case "INJURY_STATUS_15DAYIL":
		return "IL15"
	case "INJURY_STATUS_60DAYIL":
		return "IL60"
	case "INJURY_STATUS_OUT":
		if path == "basketball/nba" || path == "basketball/wnba" || path == "basketball/mens-college-basketball" {
			return "OUT"
		}
	}
	return ""
}
