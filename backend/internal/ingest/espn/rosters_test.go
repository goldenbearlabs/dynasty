package espn

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crossover/internal/ingest"
)

func TestCollegeRosterSeason(t *testing.T) {
	src := New(ingest.NewClient(0), League{RosterSeasonStarts: time.July, SeasonSpansYears: true})
	for _, tt := range []struct {
		date string
		want int
	}{
		{"2026-06-30", 2026}, {"2026-07-01", 2027},
		{"2026-10-13", 2027}, {"2027-01-01", 2027},
	} {
		now, _ := time.Parse(time.DateOnly, tt.date)
		if got := src.rosterSeason(now); got != tt.want {
			t.Errorf("rosterSeason(%s) = %d, want %d", tt.date, got, tt.want)
		}
	}
	pro := New(ingest.NewClient(0), League{SeasonStarts: time.October, SeasonSpansYears: true})
	if got := pro.rosterURL("http://espn.test/teams?limit=500"); got != "http://espn.test/teams?limit=500" {
		t.Fatalf("changed pro league URL: %s", got)
	}
}

func TestRosterHeadshotFormats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"team":{"athletes":[
		 {"id":"1","fullName":"Object","headshot":{"href":"https://img/object.png"}},
		 {"id":"2","fullName":"String","headshot":"https://img/string.png"},
		 {"id":"3","fullName":"Null","headshot":null},
		 {"id":"4","fullName":"Absent"}
		]}}`)
	}))
	defer server.Close()
	src := New(ingest.NewClient(0), League{Path: "basketball/mens-college-basketball"})
	src.Base = server.URL
	players, err := src.Roster(context.Background(), ingest.Team{ProviderID: "1"})
	if err != nil || len(players) != 4 {
		t.Fatalf("roster = %+v, err = %v", players, err)
	}
	for i, want := range []string{"https://img/object.png", "https://img/string.png", "", ""} {
		if players[i].HeadshotURL != want {
			t.Errorf("player %d headshot = %q, want %q", i, players[i].HeadshotURL, want)
		}
	}
}

func TestCollegeTeamsAndRostersUseSelectedSeason(t *testing.T) {
	year := time.Now().Year()
	if time.Now().Month() >= time.July {
		year++
	}
	season := fmt.Sprint(year)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/groups/46/teams"):
			fmt.Fprint(w, `{"items":[{"$ref":"http://espn.test/teams/2511"},{"$ref":"http://espn.test/teams/150"}]}`)
		case r.URL.Path == "/basketball/mens-college-basketball/teams":
			if r.URL.Query().Get("season") != season {
				t.Errorf("team listing season = %q, want %s", r.URL.Query().Get("season"), season)
			}
			fmt.Fprint(w, teamsJSON) // Queens is absent, Duke is already present.
		case r.URL.Path == "/basketball/mens-college-basketball/teams/2511":
			if r.URL.Query().Get("season") != season || r.URL.Query().Get("enable") != "roster" {
				http.NotFound(w, r) // a default/old-season roster lacks Watford
				return
			}
			fmt.Fprint(w, `{"team":{"id":"2511","abbreviation":"QU","displayName":"Queens University Royals","logos":[{"href":"https://img/queens.png"}],"athletes":[{"id":"5144110","fullName":"Jordan Watford","position":{"abbreviation":"G"}}]}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	src := New(ingest.NewClient(0), League{
		Path: "basketball/mens-college-basketball", Provider: "espn_basketball",
		ConferenceGroups: []string{"46"}, RosterSeasonStarts: time.July, SeasonSpansYears: true,
	})
	src.Base, src.CoreBase = server.URL, server.URL
	teams, err := src.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 3 || teams[2].ProviderID != "2511" || teams[2].Conference != "46" || teams[2].LogoURL == "" {
		t.Fatalf("missing conference team or duplicated listed team: %+v", teams)
	}
	players, err := src.Roster(context.Background(), teams[2])
	if err != nil || len(players) != 1 || players[0].FullName != "Jordan Watford" {
		t.Fatalf("Queens roster = %+v, err = %v", players, err)
	}
}

func TestRosterAdditionsExpireAndPreferFeed(t *testing.T) {
	src := New(ingest.NewClient(0), League{
		Provider: "espn_basketball", RosterSeasonStarts: time.July, SeasonSpansYears: true,
		RosterAdditions: []RosterAddition{{Season: 2027, TeamID: "2599",
			Player: ingest.Player{ProviderID: "5075625", FullName: "Keyshawn Hall", Positions: []string{"F"}}}},
	})
	now := time.Date(2026, time.October, 13, 0, 0, 0, 0, time.UTC)
	players := src.addRosterPlayers(nil, "2599", now)
	if len(players) != 1 || players[0].Provider != "espn_basketball" {
		t.Fatalf("missing roster addition: %+v", players)
	}
	players[0].HeadshotURL = "https://img/feed.png"
	players = src.addRosterPlayers(players, "2599", now)
	if len(players) != 1 || players[0].HeadshotURL == "" {
		t.Fatalf("addition duplicated or replaced feed player: %+v", players)
	}
	for _, date := range []time.Time{now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0)} {
		if players := src.addRosterPlayers(nil, "2599", date); len(players) != 0 {
			t.Fatalf("addition applied outside verified season: %+v", players)
		}
	}
	if players := src.addRosterPlayers(nil, "2", now); len(players) != 0 {
		t.Fatal("addition applied to another team")
	}
	src.Positions = []string{"G"}
	if players := src.addRosterPlayers(nil, "2599", now); len(players) != 0 {
		t.Fatal("addition bypassed position filter")
	}
}
