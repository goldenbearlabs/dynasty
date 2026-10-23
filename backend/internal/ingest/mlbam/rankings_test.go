package mlbam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crossover/internal/ingest"
)

func rankingFixture(n int) []map[string]any {
	rows := make([]map[string]any, n)
	for i := range rows {
		pos := "SS"
		if i == 1 {
			pos = "RHP"
		}
		if i == 2 {
			pos = "SS/3B"
		}
		rows[i] = map[string]any{"rank": i + 1, "playerEntity": map[string]any{"position": pos, "eta": "2027", "player": map[string]any{"id": 815908 + i, "useName": "Jesús", "useLastName": fmt.Sprintf("Player %d", i+1), "birthDate": "2007-05-08"}}}
	}
	return rows
}

func TestMLBPipelineTop100(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing GraphQL media type")
		}
		if !strings.Contains(r.URL.Query().Get("query"), "getPlayerRankingsFromSelection") || !strings.Contains(r.URL.Query().Get("query"), "limit:100") {
			t.Error("not the complete official selection")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"getPlayerRankingsFromSelection": rankingFixture(100)}})
	}))
	defer server.Close()
	source := New(ingest.NewClient(0))
	source.RankingsBase = server.URL
	list, err := source.ProspectRankings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Rankings) != 100 || list.Source != "mlb_pipeline" || list.Year != time.Now().Year() {
		t.Fatalf("list = %+v", list)
	}
	first := list.Rankings[0]
	if first.Player.Provider != "mlbam" || first.Player.ProviderID != "815908" || first.Rank != 1 || first.ETA != "2027" || first.Player.FullName != "Jesús Player 1" || first.Player.BirthDate.Format(time.DateOnly) != "2007-05-08" {
		t.Fatalf("first = %+v", first)
	}
	if list.Rankings[1].Player.Positions[0] != "P" || strings.Join(list.Rankings[2].Player.Positions, "/") != "SS/3B" {
		t.Fatal("positions were not normalized")
	}
}

func TestPipelineKeepsYearWhenNewListUnpublished(t *testing.T) {
	calls := 0
	year := time.Now().Year()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		rows := rankingFixture(100)
		if strings.Contains(r.URL.Query().Get("query"), fmt.Sprintf("sel-pr-%d-top100", year)) {
			rows = nil
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"getPlayerRankingsFromSelection": rows}})
	}))
	defer server.Close()
	source := New(ingest.NewClient(0))
	source.RankingsBase = server.URL
	list, err := source.ProspectRankings(context.Background())
	if err != nil || list.Year != year-1 || calls != 2 {
		t.Fatalf("year=%d calls=%d err=%v", list.Year, calls, err)
	}
}

func TestPipelineRejectsIncompleteAndInvalidSnapshots(t *testing.T) {
	for _, kind := range []string{"partial", "duplicate_rank", "duplicate_id", "invalid_rank", "missing_id", "graphql_error"} {
		t.Run(kind, func(t *testing.T) {
			rows := rankingFixture(100)
			errors := []map[string]string{}
			switch kind {
			case "partial":
				rows = rows[:99]
			case "duplicate_rank":
				rows[1]["rank"] = 1
			case "invalid_rank":
				rows[1]["rank"] = 101
			case "duplicate_id":
				rows[1]["playerEntity"].(map[string]any)["player"].(map[string]any)["id"] = 815908
			case "missing_id":
				rows[1]["playerEntity"].(map[string]any)["player"].(map[string]any)["id"] = 0
			case "graphql_error":
				errors = append(errors, map[string]string{"message": "selection failed"})
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"getPlayerRankingsFromSelection": rows}, "errors": errors})
			}))
			defer server.Close()
			source := New(ingest.NewClient(0))
			source.RankingsBase = server.URL
			if _, err := source.ProspectRankings(context.Background()); err == nil {
				t.Fatal("invalid ranking snapshot accepted")
			}
		})
	}
}
