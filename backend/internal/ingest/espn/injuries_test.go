package espn

import (
	"context"
	"crossover/internal/ingest"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
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

func TestPreviewsFeed(t *testing.T) {
	payload := `{"events":[{"id":"9","competitions":[{
		"competitors":[
			{"homeAway":"home","team":{"name":"Rangers"},"probables":[{"name":"probableStartingGoalie","athlete":{"id":"1","fullName":"Expected Goalie"},"status":{"type":"expected"}}]},
			{"homeAway":"away","team":{"name":"Ducks"},"probables":[{"name":"probableStartingGoalie","athlete":{"id":"2","fullName":"Confirmed Goalie"},"status":{"type":"confirmed"}},{"name":"somethingElse","athlete":{"id":"3","fullName":"Not A Starter"}}]}],
		"odds":[{"overUnder":6,"spread":-1.5,"moneyline":{"home":{"close":{"odds":"-150"}},"away":{"close":{"odds":"+150"}}}}]}]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hockey/nhl/scoreboard" || r.URL.Query().Get("dates") != "20261019" {
			t.Errorf("requested %s", r.URL)
		}
		w.Write([]byte(payload))
	}))
	defer server.Close()
	source := New(ingest.NewClient(0), League{Path: "hockey/nhl", Provider: "espn_nhl"})
	source.Base = server.URL
	previews, err := source.Previews(context.Background(), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(previews) != 1 || previews[0].GameID != "9" || previews[0].HomeTeam != "Rangers" {
		t.Fatalf("previews = %+v", previews)
	}
	want := []ingest.Starter{
		{Provider: "espn_nhl", ProviderID: "1", FullName: "Expected Goalie"},
		{Provider: "espn_nhl", ProviderID: "2", FullName: "Confirmed Goalie", Confirmed: true},
	}
	if !slices.Equal(previews[0].Starters, want) {
		t.Fatalf("starters = %+v", previews[0].Starters)
	}
	// -150 against +150 is a 60% favourite, who takes 54.5% of 6 goals on a fixed line.
	if o := previews[0].Odds; o == nil || math.Abs(o.HomeWin-0.6) > 1e-9 || math.Abs(o.HomeScore-3.27) > 1e-9 || math.Abs(o.AwayScore-2.73) > 1e-9 {
		t.Fatalf("odds = %+v", o)
	}
}

func TestGameOdds(t *testing.T) {
	// Basketball splits the total by the spread: a 4-point home favourite in a 220 game.
	if o := gameOdds(220, -4, 0, false); o.HomeScore != 112 || o.AwayScore != 108 || o.HomeWin != 0 {
		t.Fatalf("spread odds = %+v", o)
	}
	if got := chance("-110", "-110"); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("even chance = %v", got)
	}
	if got := chance("", "+120"); got != 0 {
		t.Fatalf("missing price = %v", got)
	}
}
