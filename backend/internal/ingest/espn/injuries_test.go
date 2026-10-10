package espn

import (
	"context"
	"crossover/internal/ingest"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInjuryDesignations(t *testing.T) {
	for _, tc := range []struct{ path, code, want string }{
		{"football/nfl", "INJURY_STATUS_IR", "IR"},
		{"hockey/nhl", "INJURY_STATUS_IR", "IR"},
		{"baseball/mlb", "INJURY_STATUS_7DAYIL", "IL7"},
		{"baseball/mlb", "INJURY_STATUS_10DAYIL", "IL10"},
		{"baseball/mlb", "INJURY_STATUS_15DAYIL", "IL15"},
		{"baseball/mlb", "INJURY_STATUS_60DAYIL", "IL60"},
		{"basketball/nba", "INJURY_STATUS_OUT", "OUT"},
		{"basketball/wnba", "INJURY_STATUS_OUT", "OUT"},
		{"basketball/mens-college-basketball", "INJURY_STATUS_OUT", "OUT"},
		{"football/nfl", "INJURY_STATUS_OUT", ""},
		{"basketball/nba", "INJURY_STATUS_DAYTODAY", ""},
		{"baseball/mlb", "INJURY_STATUS_SUSPENSION", ""},
		{"baseball/mlb", "INJURY_STATUS_PATERNITY", ""},
	} {
		if got := injuryDesignation(tc.path, tc.code); got != tc.want {
			t.Errorf("%s %s = %q; want %q", tc.path, tc.code, got, tc.want)
		}
	}
}

func TestInjuriesFeed(t *testing.T) {
	payload := `{"status":"success","injuries":[{"injuries":[{"type":{"name":"INJURY_STATUS_IR"},"athlete":{"displayName":"Test Player","links":[{"href":"https://www.espn.com/nfl/player/_/id/123/test-player"}]}},{"type":{"name":"INJURY_STATUS_QUESTIONABLE"},"athlete":{"displayName":"Questionable Player"}}]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(payload)) }))
	defer server.Close()
	source := New(ingest.NewClient(0), League{Path: "football/nfl", Provider: "espn_football"})
	source.Base = server.URL
	entries, err := source.Injuries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ProviderID != "123" || entries[0].Designation != "IR" {
		t.Fatalf("injuries = %+v", entries)
	}
	payload = `{"status":"success"}`
	if _, err := source.Injuries(context.Background()); err == nil {
		t.Fatal("missing injury list accepted")
	}
	payload = `{"status":"success","injuries":[]}`
	if entries, err := source.Injuries(context.Background()); err != nil || len(entries) != 0 {
		t.Fatalf("empty feed = %+v, %v", entries, err)
	}
}
