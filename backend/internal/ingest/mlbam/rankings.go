package mlbam

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"crossover/internal/ingest"
)

// ProspectRankings reads the same MLB Pipeline selection as the official
// Top 100 page. January refreshes may precede the new list's publication;
// in that case the previous year's list keeps its explicit year label.
func (s *Source) ProspectRankings(ctx context.Context) (ingest.ProspectRankingList, error) {
	year := time.Now().Year()
	for _, year := range []int{year, year - 1} {
		list, err := s.rankingsForYear(ctx, year)
		if err != nil {
			return list, err
		}
		if len(list.Rankings) > 0 {
			return list, nil
		}
	}
	return ingest.ProspectRankingList{}, fmt.Errorf("MLB Pipeline has not published a Top 100 selection for the current or previous year")
}

func (s *Source) rankingsForYear(ctx context.Context, year int) (ingest.ProspectRankingList, error) {
	list := ingest.ProspectRankingList{Source: "mlb_pipeline", Year: year, URL: "https://www.mlb.com/milb/prospects/top100/"}
	query := fmt.Sprintf(`{getPlayerRankingsFromSelection(slug:"sel-pr-%d-top100",limit:100){rank playerEntity{position eta player{id useName useLastName birthDate}}}}`, year)
	var response struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Players []struct {
				Rank   int `json:"rank"`
				Entity struct {
					Position string `json:"position"`
					ETA      string `json:"eta"`
					Player   struct {
						ID    int    `json:"id"`
						First string `json:"useName"`
						Last  string `json:"useLastName"`
						Birth string `json:"birthDate"`
					} `json:"player"`
				} `json:"playerEntity"`
			} `json:"getPlayerRankingsFromSelection"`
		} `json:"data"`
	}
	headers := http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}}
	if err := s.client.GetJSONWithHeaders(ctx, s.RankingsBase+"?"+url.Values{"query": {query}}.Encode(), &response, headers); err != nil {
		return list, err
	}
	if len(response.Errors) > 0 {
		return list, fmt.Errorf("MLB Pipeline: %s", response.Errors[0].Message)
	}
	if len(response.Data.Players) == 0 {
		return list, nil
	}
	if len(response.Data.Players) != 100 {
		return list, fmt.Errorf("MLB Pipeline returned %d rankings; need the complete Top 100", len(response.Data.Players))
	}
	seenRanks, seenIDs := map[int]bool{}, map[int]bool{}
	for _, r := range response.Data.Players {
		p := r.Entity.Player
		name := strings.TrimSpace(p.First + " " + p.Last)
		if r.Rank < 1 || r.Rank > 100 || p.ID < 1 || name == "" || seenRanks[r.Rank] || seenIDs[p.ID] {
			return list, fmt.Errorf("MLB Pipeline returned an invalid or duplicate rank/player ID")
		}
		seenRanks[r.Rank], seenIDs[p.ID] = true, true
		positions := strings.FieldsFunc(r.Entity.Position, func(r rune) bool { return r == '/' || r == ',' })
		for i, pos := range positions {
			positions[i] = strings.TrimSpace(pos)
			if positions[i] == "LHP" || positions[i] == "RHP" {
				positions[i] = "P"
			}
		}
		list.Rankings = append(list.Rankings, ingest.ProspectRanking{
			Rank: r.Rank, ETA: r.Entity.ETA,
			Player: ingest.Player{Provider: provider, ProviderID: strconv.Itoa(p.ID), FullName: name, BirthDate: ingest.ParseDate(p.Birth), Positions: positions, HeadshotURL: fmt.Sprintf(headshotURL, p.ID), Note: "MLB Pipeline Top 100 prospect"},
		})
	}
	return list, nil
}
