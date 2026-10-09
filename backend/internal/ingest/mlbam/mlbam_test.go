package mlbam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crossover/internal/ingest"
)

const teamsJSON = `{"teams":[{"id":119,"abbreviation":"LAD","name":"Los Angeles Dodgers"}]}`

const rosterJSON = `{"roster":[
  {"person":{"id":660271,"fullName":"Shohei Ohtani","birthDate":"1994-07-05"},"position":{"abbreviation":"TWP"}}
]}`

func TestSource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/teams", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(teamsJSON))
	})
	mux.HandleFunc("/teams/119/roster", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("rosterType") != "40Man" {
			t.Errorf("rosterType = %q, want 40Man", r.URL.Query().Get("rosterType"))
		}
		w.Write([]byte(rosterJSON))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base = server.URL

	teams, err := src.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].ProviderID != "119" || teams[0].Abbrev != "LAD" || teams[0].Name != "Los Angeles Dodgers" {
		t.Fatalf("teams = %+v", teams)
	}

	players, err := src.Roster(context.Background(), teams[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 {
		t.Fatalf("got %d players, want 1", len(players))
	}
	p := players[0]
	if p.Provider != "mlbam" || p.ProviderID != "660271" || p.FullName != "Shohei Ohtani" ||
		p.Positions[0] != "TWP" || p.BirthDate.Format("2006-01-02") != "1994-07-05" {
		t.Errorf("player = %+v", p)
	}
}

func TestProspects(t *testing.T) {
	const draft = `{"drafts":{"rounds":[{"picks":[
	  {"pickRound":"1","person":{"id":800604,"fullName":"Roch Cholowsky","birthDate":"2005-04-07","primaryPosition":{"abbreviation":"SS"}},"school":{"name":"UCLA"}},
	  {"pickRound":"1"}
	]}]}}`
	years := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/draft/", func(w http.ResponseWriter, r *http.Request) {
		years++
		w.Write([]byte(draft))
	})
	mux.HandleFunc("/teams", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"teams":[{"id":512,"parentOrgName":"Detroit Tigers","sport":{"name":"Triple-A"}}]}`))
	})
	// The same three players at every level: one new, one already a draft
	// pick, one who has played in the majors.
	mux.HandleFunc("/sports/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"people":[
		  {"id":700001,"fullName":"Josue Briceno","birthDate":"2004-09-23","primaryPosition":{"abbreviation":"C"},"currentTeam":{"id":512}},
		  {"id":800604,"fullName":"Roch Cholowsky","currentTeam":{"id":512}},
		  {"id":600002,"fullName":"Old Hand","mlbDebutDate":"2019-04-01","currentTeam":{"id":512}}
		]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base = server.URL

	players, err := src.Prospects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if years != draftYears || len(players) != draftYears+2 {
		t.Fatalf("fetched %d draft years and got %d prospects, want %d picks (passed picks skipped) and 2 minor leaguers", years, len(players), draftYears)
	}
	p := players[0]
	if p.Provider != "mlbam" || p.ProviderID != "800604" || p.FullName != "Roch Cholowsky" ||
		p.Positions[0] != "SS" || p.BirthDate.Format("2006-01-02") != "2005-04-07" || !strings.HasSuffix(p.Note, "draft, round 1, UCLA") {
		t.Errorf("draft pick = %+v", p)
	}
	// Minor leaguers follow the picks, so a pick who has started playing is
	// described by his club.
	p = players[draftYears]
	if p.ProviderID != "700001" || p.Positions[0] != "C" || p.Note != "Triple-A, Detroit Tigers" {
		t.Errorf("minor leaguer = %+v", p)
	}
	if p = players[draftYears+1]; p.ProviderID != "800604" || p.Note != "Triple-A, Detroit Tigers" {
		t.Errorf("drafted minor leaguer = %+v", p)
	}
}

func TestGamesAndBoxScore(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/schedule", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"dates":[{"games":[
		  {"gamePk":849819,"gameDate":"2026-10-06T22:00:00Z","status":{"abstractGameState":"Final"},
		   "teams":{"home":{"score":3,"team":{"id":144}},"away":{"score":1,"team":{"id":119}}},
		   "linescore":{"currentInningOrdinal":"9th","inningState":"Bottom"}},
		  {"gamePk":849820,"gameDate":"2026-10-07T00:08:00Z","status":{"abstractGameState":"Preview"},
		   "teams":{"home":{"team":{"id":147}},"away":{"team":{"id":111}}}},
		  {"gamePk":849821,"gameDate":"2026-10-07T00:38:00Z","status":{"abstractGameState":"Live"},
		   "teams":{"home":{"score":2,"team":{"id":121}},"away":{"score":4,"team":{"id":143}}},
		   "linescore":{"currentInningOrdinal":"5th","inningState":"Top"}}
		]}]}`))
	})
	mux.HandleFunc("/game/849819/boxscore", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"teams":{
		  "home":{"players":{
		    "ID1":{"person":{"id":1},"stats":{"batting":{"runs":1,"hits":2,"homeRuns":1,"rbi":3,"stolenBases":0,"baseOnBalls":1,"strikeOuts":1},"pitching":{}}},
		    "ID2":{"person":{"id":2},"stats":{"batting":{},"pitching":{"inningsPitched":"6.1","strikeOuts":8,"wins":1,"saves":0,"earnedRuns":2,"hits":5,"baseOnBalls":1}}},
		    "ID3":{"person":{"id":3},"stats":{"batting":{},"pitching":{}}}
		  }},
		  "away":{"players":{}}
		}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base = server.URL

	day, _ := time.Parse(time.DateOnly, "2026-10-06")
	games, err := src.Games(context.Background(), day)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 || games[0].ProviderID != "849819" || games[0].Status != ingest.GameFinal ||
		games[0].HomeTeam != "144" || games[0].AwayTeam != "119" || games[1].Status != ingest.GameScheduled {
		t.Fatalf("games = %+v", games)
	}
	if games[0].HomeScore != 3 || games[0].AwayScore != 1 || games[0].Detail != "Final" ||
		games[2].Status != ingest.GameLive || games[2].AwayScore != 4 || games[2].Detail != "Top 5th" {
		t.Errorf("scores and state = %+v", games)
	}

	lines, err := src.BoxScore(context.Background(), games[0])
	if err != nil {
		t.Fatal(err)
	}
	byPlayer := map[string]map[string]float64{}
	for _, l := range lines {
		byPlayer[l.ProviderID] = l.Stats
	}
	if len(byPlayer) != 2 {
		t.Fatalf("got lines for %d players, want 2: the one who did not play is left out", len(byPlayer))
	}
	if s := byPlayer["1"]; s["bat_r"] != 1 || s["bat_h"] != 2 || s["bat_hr"] != 1 || s["bat_rbi"] != 3 || s["bat_bb"] != 1 || s["bat_so"] != 1 {
		t.Errorf("batter = %v", s)
	}
	// "6.1" innings is six and a third.
	if s := byPlayer["2"]; s["pit_ip"] < 6.33 || s["pit_ip"] > 6.34 || s["pit_so"] != 8 || s["pit_w"] != 1 || s["pit_er"] != 2 {
		t.Errorf("pitcher = %v", s)
	}
}

func TestSeasonStats(t *testing.T) {
	// A two-way player appears in both listings; his two lines become one.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/stats" || q.Get("season") != "2025" || q.Get("playerPool") != "all" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if q.Get("group") == "hitting" {
			w.Write([]byte(`{"stats":[{"splits":[
			  {"player":{"id":660271},"team":{"name":"Los Angeles Dodgers"},"stat":{"gamesPlayed":158,"runs":146,"hits":172,"homeRuns":55,"rbi":102,"stolenBases":20,"baseOnBalls":109,"strikeOuts":187}},
			  {"player":{"id":592450},"team":{"name":"New York Yankees"},"stat":{"gamesPlayed":152,"runs":137,"hits":179,"homeRuns":53,"rbi":114,"stolenBases":12,"baseOnBalls":124,"strikeOuts":160}}]}]}`))
		} else {
			w.Write([]byte(`{"stats":[{"splits":[
			  {"player":{"id":660271},"team":{"name":"Los Angeles Dodgers"},"stat":{"gamesPlayed":14,"inningsPitched":"47.1","strikeOuts":62,"wins":1,"saves":0,"earnedRuns":15,"hits":40,"baseOnBalls":9}}]}]}`))
		}
	}))
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base = server.URL

	lines, err := src.SeasonStats(context.Background(), 2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 players", len(lines))
	}
	s := lines[0]
	if s.ProviderID != "660271" || s.Year != 2025 || s.Label != "2025" || s.Team != "Los Angeles Dodgers" || s.Games != 158 ||
		s.Stats["bat_hr"] != 55 || s.Stats["bat_so"] != 187 || s.Stats["pit_so"] != 62 || s.Stats["pit_w"] != 1 ||
		s.Stats["pit_ip"] < 47.33 || s.Stats["pit_ip"] > 47.34 {
		t.Errorf("two-way player = %+v", s)
	}
	if hitter := lines[1]; hitter.Stats["bat_hr"] != 53 || hitter.Stats["pit_ip"] != 0 {
		t.Errorf("hitter = %+v", hitter)
	}

	march, _ := time.Parse(time.DateOnly, "2026-03-01")
	may, _ := time.Parse(time.DateOnly, "2026-05-01")
	if src.LatestSeason(march) != 2025 || src.LatestSeason(may) != 2026 {
		t.Errorf("LatestSeason = %d in March and %d in May 2026, want 2025 and 2026", src.LatestSeason(march), src.LatestSeason(may))
	}
}
