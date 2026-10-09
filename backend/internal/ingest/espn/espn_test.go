package espn

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"crossover/internal/ingest"
)

const teamsJSON = `{"sports":[{"leagues":[{"teams":[
  {"team":{"id":"150","abbreviation":"DUKE","displayName":"Duke Blue Devils","logos":[{"href":"https://img/duke.png"}]}},
  {"team":{"id":"2","abbreviation":"AUB","displayName":"Auburn Tigers"}}
]}]}]}`

const rosterJSON = `{"team":{"athletes":[
  {"id":"5041939","fullName":"Cooper Flagg","dateOfBirth":"2006-12-21T08:00Z",
   "position":{"abbreviation":"F"},"headshot":{"href":"https://img/flagg.png"},
   "experience":{"abbreviation":"FR"}},
  {"id":"7","fullName":"No Details"},
  {"id":"8","fullName":" Team","position":{"abbreviation":"ATH"}}
]}}`

func TestSource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/basketball/mens-college-basketball/teams", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(teamsJSON))
	})
	mux.HandleFunc("/basketball/mens-college-basketball/teams/150", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rosterJSON))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0), League{Path: "basketball/mens-college-basketball", Provider: "espn_basketball"})
	src.Base = server.URL

	teams, err := src.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 || teams[0] != (ingest.Team{ProviderID: "150", Abbrev: "DUKE", Name: "Duke Blue Devils", LogoURL: "https://img/duke.png"}) {
		t.Fatalf("teams = %+v", teams)
	}
	if teams[1].LogoURL != "" {
		t.Errorf("team without logos got LogoURL %q", teams[1].LogoURL)
	}

	players, err := src.Roster(context.Background(), teams[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 2 {
		t.Fatalf("got %d players, want 2 (the placeholder \"Team\" athlete is skipped)", len(players))
	}
	p := players[0]
	if p.Provider != "espn_basketball" || p.ProviderID != "5041939" || p.FullName != "Cooper Flagg" ||
		p.Class != "FR" || p.HeadshotURL != "https://img/flagg.png" ||
		len(p.Positions) != 1 || p.Positions[0] != "F" ||
		p.BirthDate.Format("2006-01-02") != "2006-12-21" {
		t.Errorf("player = %+v", p)
	}
	if bare := players[1]; !bare.BirthDate.IsZero() || len(bare.Positions) != 0 {
		t.Errorf("player without details = %+v", bare)
	}
}

func TestRecruits(t *testing.T) {
	const page = `{"pageCount":1,"items":[
	  {"athlete":{"id":"260209","fullName":"Beckham Black","position":{"abbreviation":"PG"},"highSchool":{"name":"Southeastern Prep Academy"}}},
	  {"athlete":{"fullName":"No Id"}}
	]}`
	// Two classes are published; the third is not there yet.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/basketball/leagues/mens-college-basketball/recruiting/2027/athletes",
			"/basketball/leagues/mens-college-basketball/recruiting/2028/athletes":
			w.Write([]byte(page))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	recruits := NewRecruits(ingest.NewClient(0))
	recruits.Base, recruits.Year = server.URL, 2027

	players, err := recruits.Prospects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 2 {
		t.Fatalf("got %d recruits, want 2 (the one with an id, in each published class)", len(players))
	}
	p := players[0]
	if p.Provider != "espn_recruit" || p.ProviderID != "260209" || p.FullName != "Beckham Black" ||
		p.Positions[0] != "PG" || p.Note != "Class of 2027, Southeastern Prep Academy" {
		t.Errorf("recruit = %+v", p)
	}
	if players[1].Note != "Class of 2028, Southeastern Prep Academy" {
		t.Errorf("second class recruit = %+v", players[1])
	}
}

func TestDraft(t *testing.T) {
	next := time.Now().Year()
	if time.Now().Month() > time.June {
		next++
	}
	board := fmt.Sprintf("/basketball/leagues/nba/seasons/%d/draft", next)
	last := fmt.Sprintf("/basketball/leagues/nba/seasons/%d/draft", next-1)

	mux := http.NewServeMux()
	mux.HandleFunc(last+"/rounds", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"picks":[
		  {"overall":32,"athlete":{"$ref":"http://espn.test/draft/athletes/110962?lang=en"}},
		  {"overall":33}
		]}]}`))
	})
	mux.HandleFunc(last+"/athletes/110962", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"fullName":"Nikola Djurisi&#263;","position":{"abbreviation":"F"},
		  "athlete":{"$ref":"http://espn.test/leagues/nba/athletes/5214637?lang=en"}}`))
	})
	mux.HandleFunc(board+"/athletes", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"$ref":"http://espn.test/draft/athletes/7"},{"$ref":"http://espn.test/draft/athletes/8"}]}`))
	})
	mux.HandleFunc(board+"/athletes/7", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"fullName":"Top Prospect","position":{"abbreviation":"G"},
		  "attributes":[{"name":"rank","displayValue":"1"},{"name":"overall","displayValue":"4"}],
		  "athlete":{"$ref":"http://espn.test/leagues/mens-college-basketball/athletes/5000001"}}`))
	})
	mux.HandleFunc(board+"/athletes/8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"fullName":"No Athlete Record"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	draft := NewDraft(ingest.NewClient(0), Draft{Path: "basketball/leagues/nba", Provider: "espn_basketball", Month: time.June, Classes: 1})
	draft.Base = server.URL

	players, err := draft.Prospects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 2 {
		t.Fatalf("got %d prospects, want 2 (an unmade pick and an entry without an athlete are skipped): %+v", len(players), players)
	}
	if p := players[0]; p.Provider != "espn_basketball" || p.ProviderID != "5214637" || p.FullName != "Nikola Djurisić" || p.Positions[0] != "F" ||
		p.Note != fmt.Sprintf("%d draft, pick 32", next-1) {
		t.Errorf("pick = %+v", p)
	}
	if p := players[1]; p.ProviderID != "5000001" || p.Note != fmt.Sprintf("%d draft prospect, ranked 4", next) {
		t.Errorf("ranked prospect = %+v", p)
	}
}

func TestGamesAndBoxScore(t *testing.T) {
	const scoreboard = `{"events":[
	  {"id":"401","date":"2026-10-04T17:00Z","status":{"type":{"state":"post","shortDetail":"Final/OT"}},
	   "competitions":[{"competitors":[{"homeAway":"home","score":"13","team":{"id":"28"}},{"homeAway":"away","score":"30","team":{"id":"11"}}]}]},
	  {"id":"402","date":"2026-10-05T00:20Z","status":{"type":{"state":"in","shortDetail":"7:32 - 3rd"}},
	   "competitions":[{"competitors":[{"homeAway":"away","score":"7","team":{"id":"1"}},{"homeAway":"home","score":"10","team":{"id":"2"}}]}]},
	  {"id":"403","date":"2026-10-06T00:15Z","status":{"type":{"state":"pre"}},"competitions":[{"competitors":[]}]}
	]}`
	// One quarterback appears in three groups; "interceptions" is both a
	// passing stat and a defensive one.
	const summary = `{"boxscore":{"players":[{"statistics":[
	  {"name":"passing","keys":["completions/passingAttempts","passingYards","passingTouchdowns","interceptions","sacks-sackYardsLost"],
	   "athletes":[{"athlete":{"id":"10"},"stats":["19/34","143","2","1","2-14"]}]},
	  {"name":"rushing","keys":["rushingAttempts","rushingYards","rushingTouchdowns"],
	   "athletes":[{"athlete":{"id":"10"},"stats":["3","-4","1"]},{"athlete":{"id":"20"},"stats":["20","95","2"]}]},
	  {"name":"interceptions","keys":["interceptions","interceptionYards"],
	   "athletes":[{"athlete":{"id":"30"},"stats":["1","7"]}]},
	  {"name":"kicking","keys":["fieldGoalsMade/fieldGoalAttempts","extraPointsMade/extraPointAttempts"],
	   "athletes":[{"athlete":{"id":"40"},"stats":["3/4","2/2"]}]}
	]}]}}`

	mux := http.NewServeMux()
	mux.HandleFunc("/football/nfl/scoreboard", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dates") != "20261004" || r.URL.Query().Get("limit") != "9" {
			t.Errorf("scoreboard query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(scoreboard))
	})
	mux.HandleFunc("/football/nfl/summary", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(summary))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	saved := 0
	client := ingest.NewClient(0)
	client.Save = func(context.Context, string, []byte) { saved++ }
	src := New(client, League{
		Path: "football/nfl", Provider: "espn_football", Scoreboard: "limit=9",
		Stats: map[string]string{
			"passingYards": "pass_yds", "passingTouchdowns": "pass_td", "passing.interceptions": "interceptions",
			"rushingYards": "rush_yds", "rushingTouchdowns": "rush_td",
			"fieldGoalsMade": "fg_made", "extraPointsMade": "xp_made",
		},
	})
	src.Base = server.URL

	day, _ := time.Parse(time.DateOnly, "2026-10-04")
	games, err := src.Games(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 {
		t.Fatalf("got %d games, want 3", len(games))
	}
	first := games[0]
	if first.ProviderID != "401" || first.Status != ingest.GameFinal || first.HomeTeam != "28" || first.AwayTeam != "11" ||
		first.StartsAt.Format(time.RFC3339) != "2026-10-04T17:00:00Z" {
		t.Errorf("first game = %+v", first)
	}
	if first.HomeScore != 13 || first.AwayScore != 30 || first.Detail != "Final/OT" ||
		games[1].HomeScore != 10 || games[1].AwayScore != 7 || games[1].Detail != "7:32 - 3rd" || games[2].Detail != "" {
		t.Errorf("scores and state = %+v", games)
	}
	if games[1].Status != ingest.GameLive || games[1].HomeTeam != "2" || games[2].Status != ingest.GameScheduled {
		t.Errorf("statuses = %s, %s; second home = %s", games[1].Status, games[2].Status, games[1].HomeTeam)
	}

	lines, err := src.BoxScore(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	byPlayer := map[string]map[string]float64{}
	for _, l := range lines {
		if l.Provider != "espn_football" {
			t.Errorf("provider = %q", l.Provider)
		}
		byPlayer[l.ProviderID] = l.Stats
	}
	want := map[string]map[string]float64{
		"10": {"pass_yds": 143, "pass_td": 2, "interceptions": 1, "rush_yds": -4, "rush_td": 1},
		"20": {"rush_yds": 95, "rush_td": 2},
		"40": {"fg_made": 3, "xp_made": 2},
		// player 30's defensive interception is not a stat this league scores
	}
	if !reflect.DeepEqual(byPlayer, want) {
		t.Errorf("box score = %v\nwant %v", byPlayer, want)
	}
	if saved != 0 {
		t.Errorf("%d per-day and per-game responses were saved; they would pile up forever", saved)
	}
}

func TestSeasonStats(t *testing.T) {
	// Every athlete's totals follow the order of the names given once per
	// group. Touchdowns appear under "rushing" and again under "scoring",
	// and must be counted once.
	page := func(pages int, athletes string) string {
		return `{"pagination":{"pages":` + strconv.Itoa(pages) + `},"requestedSeason":{"displayName":"2025"},
		  "categories":[
		    {"name":"general","names":["gamesPlayed","fumblesForced"]},
		    {"name":"passing","names":["passingYards","passingTouchdowns","interceptions"]},
		    {"name":"rushing","names":["rushingYards","rushingTouchdowns","rushingFumblesLost"]},
		    {"name":"scoring","names":["rushingTouchdowns","totalPoints"]},
		    {"name":"defensiveinterceptions","names":["interceptions"]}],
		  "athletes":[` + athletes + `]}`
	}
	const quarterback = `{"athlete":{"id":"3918298","teamShortName":"BUF"},"categories":[
	  {"name":"general","totals":["17","0"]},{"name":"passing","totals":["3,668","25","10"]},
	  {"name":"rushing","totals":["579","14","2"]},{"name":"scoring","totals":["14","84"]},
	  {"name":"defensiveinterceptions","totals":["1"]}]}`
	const unused = `{"athlete":{"id":"1","teamShortName":"BUF"},"categories":[{"name":"general","totals":["0","0"]},{"name":"passing","totals":["-","-","-"]}]}`
	const runner = `{"athlete":{"id":"4242335","teamShortName":"IND"},"categories":[{"name":"general","totals":["16","0"]},{"name":"rushing","totals":["1,431","18","1"]}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/football/nfl/statistics/byathlete" || q.Get("season") != "2025" || q.Get("isqualified") != "false" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if q.Get("page") == "1" {
			w.Write([]byte(page(2, quarterback+","+unused)))
		} else {
			w.Write([]byte(page(2, runner)))
		}
	}))
	defer server.Close()

	src := New(ingest.NewClient(0), League{
		Path: "football/nfl", Provider: "espn_football",
		Stats: map[string]string{
			"passingYards": "pass_yds", "passingTouchdowns": "pass_td", "passing.interceptions": "interceptions",
			"rushingYards": "rush_yds", "rushingTouchdowns": "rush_td", "rushingFumblesLost": "fumbles_lost",
		},
		SeasonGroups: []string{"passing", "rushing"},
	})
	src.WebBase = server.URL

	lines, err := src.SeasonStats(context.Background(), 2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: both pages, without the player who never played", len(lines))
	}
	qb := lines[0]
	want := map[string]float64{"pass_yds": 3668, "pass_td": 25, "interceptions": 10, "rush_yds": 579, "rush_td": 14, "fumbles_lost": 2}
	if qb.Provider != "espn_football" || qb.ProviderID != "3918298" || qb.Year != 2025 || qb.Label != "2025" ||
		qb.Team != "BUF" || qb.Games != 17 || !reflect.DeepEqual(qb.Stats, want) {
		t.Errorf("quarterback = %+v\nwant 17 games for BUF with %v", qb, want)
	}
	if lines[1].ProviderID != "4242335" || lines[1].Stats["rush_yds"] != 1431 {
		t.Errorf("second page = %+v", lines[1])
	}
}

func TestLatestSeason(t *testing.T) {
	on := func(date string) time.Time { d, _ := time.Parse(time.DateOnly, date); return d }
	basketball := New(nil, League{SeasonStarts: time.October, SeasonSpansYears: true})
	football := New(nil, League{SeasonStarts: time.September})
	tests := []struct {
		name string
		src  *Source
		date string
		want int
	}{
		{"basketball in the autumn it starts", basketball, "2026-10-21", 2027},
		{"basketball in the spring", basketball, "2027-03-01", 2027},
		{"basketball in the summer after", basketball, "2027-07-01", 2027},
		{"football in the autumn it starts", football, "2026-09-10", 2026},
		{"football in its January", football, "2027-01-02", 2026},
		{"football in the summer before the next", football, "2027-07-01", 2026},
	}
	for _, tt := range tests {
		if got := tt.src.LatestSeason(on(tt.date)); got != tt.want {
			t.Errorf("%s: LatestSeason(%s) = %d, want %d", tt.name, tt.date, got, tt.want)
		}
	}
}

// A league narrowed to some groups and positions lists only those teams
// and players, and refuses to carry on if a group comes back empty.
func TestSourcePool(t *testing.T) {
	groupTeams := `{"items":[{"$ref":"http://espn.test/seasons/2027/teams/150?lang=en"}]}`
	mux := http.NewServeMux()
	mux.HandleFunc("/basketball/mens-college-basketball/teams", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(teamsJSON))
	})
	mux.HandleFunc("/basketball/mens-college-basketball/teams/150", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rosterJSON))
	})
	mux.HandleFunc("/basketball/leagues/mens-college-basketball/seasons/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(groupTeams))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0), League{
		Path: "basketball/mens-college-basketball", Provider: "espn_basketball",
		Groups: []string{"2"}, Positions: []string{"F"},
	})
	src.Base, src.CoreBase = server.URL, server.URL

	teams, err := src.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].ProviderID != "150" {
		t.Fatalf("teams = %+v, want only the one in the group", teams)
	}
	players, err := src.Roster(context.Background(), teams[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].FullName != "Cooper Flagg" {
		t.Errorf("players = %+v, want only the forward", players)
	}
	if pool := src.Pool(); !pool.ListedTeamsOnly || len(pool.Positions) != 1 {
		t.Errorf("pool = %+v", pool)
	}

	groupTeams = `{"items":[]}`
	if _, err := src.Teams(context.Background()); err == nil {
		t.Error("an empty group was accepted; it must stop the sync")
	}
}

// Conference metadata never narrows ingestion; season totals keep the
// membership from their own season rather than the current team conference.
func TestConferenceMetadataKeepsFullPoolAndHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/basketball/mens-college-basketball/teams":
			w.Write([]byte(teamsJSON))
		case r.URL.Path == "/basketball/mens-college-basketball/statistics/byathlete":
			w.Write([]byte(`{"pagination":{"pages":1},"requestedSeason":{"displayName":"2024-25"},"categories":[{"name":"general","names":["gamesPlayed","points"]}],"athletes":[{"athlete":{"id":"1","teamId":"150","teamShortName":"DUKE"},"categories":[{"name":"general","totals":["10","100"]}]}]}`))
		case strings.Contains(r.URL.Path, "/groups/"):
			id := "150"
			if strings.Contains(r.URL.Path, "/groups/12/") {
				id = "2"
			}
			// The older season has opposite membership to exercise a conference move.
			if strings.Contains(r.URL.Path, "/seasons/2025/") {
				if id == "150" {
					id = "2"
				} else {
					id = "150"
				}
			}
			fmt.Fprintf(w, `{"items":[{"$ref":"http://espn.test/teams/%s"}]}`, id)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	src := New(ingest.NewClient(0), League{Path: "basketball/mens-college-basketball", Provider: "espn_basketball", ConferenceGroups: []string{"2", "12"}, Stats: map[string]string{"points": "pts"}, SeasonGroups: []string{"general"}})
	src.Base, src.CoreBase, src.WebBase = server.URL, server.URL, server.URL
	teams, err := src.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 || teams[0].Conference != "2" || teams[1].Conference != "12" || src.Pool().ListedTeamsOnly {
		t.Fatalf("conference metadata narrowed the pool: %+v", teams)
	}
	lines, err := src.SeasonStats(context.Background(), 2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Conference != "12" {
		t.Fatalf("historical membership: %+v", lines)
	}
}
