package nhle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crossover/internal/ingest"
)

const standingsJSON = `{"standings":[
  {"teamAbbrev":{"default":"TOR"},"teamName":{"default":"Toronto Maple Leafs","fr":"Maple Leafs de Toronto"},"teamLogo":"https://img/tor.svg"}
]}`

const rosterJSON = `{
  "forwards":[{"id":8479318,"firstName":{"default":"Auston"},"lastName":{"default":"Matthews"},"positionCode":"C","birthDate":"1997-09-17","headshot":"https://img/am.png"}],
  "defensemen":[{"id":2,"firstName":{"default":"Morgan"},"lastName":{"default":"Rielly"},"positionCode":"D","birthDate":"1994-03-09"}],
  "goalies":[{"id":3,"firstName":{"default":"Joseph"},"lastName":{"default":"Woll"},"positionCode":"G","birthDate":"1998-07-12"}]
}`

func TestSource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/standings/now", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(standingsJSON))
	})
	// The live API answers "current" with a redirect to the season's roster.
	mux.HandleFunc("/roster/TOR/current", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/roster/TOR/20262027", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/roster/TOR/20262027", func(w http.ResponseWriter, r *http.Request) {
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
	if len(teams) != 1 || teams[0] != (ingest.Team{ProviderID: "TOR", Abbrev: "TOR", Name: "Toronto Maple Leafs", LogoURL: "https://img/tor.svg"}) {
		t.Fatalf("teams = %+v", teams)
	}

	players, err := src.Roster(context.Background(), teams[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 3 {
		t.Fatalf("got %d players, want all three position groups", len(players))
	}
	p := players[0]
	if p.Provider != "nhle" || p.ProviderID != "8479318" || p.FullName != "Auston Matthews" ||
		p.Positions[0] != "C" || p.BirthDate.Format("2006-01-02") != "1997-09-17" || p.HeadshotURL != "https://img/am.png" {
		t.Errorf("player = %+v", p)
	}
	if players[2].Positions[0] != "G" {
		t.Errorf("last player = %+v, want the goalie", players[2])
	}
}

func TestProspects(t *testing.T) {
	// One draft is on record; one player is ranked for the next. The pick
	// who was ranked a year earlier is listed with the key he was ranked
	// under, so the two are known to be one person.
	var askedDrafts, askedRankings []string
	mux := http.NewServeMux()
	mux.HandleFunc("/standings/now", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(standingsJSON))
	})
	mux.HandleFunc("/prospects/TOR", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"forwards":[{"id":8479599,"firstName":{"default":"Nikolai"},"lastName":{"default":"Chebykin"},"positionCode":"L","birthDate":"1997-08-01"}],"defensemen":[],"goalies":[]}`))
	})
	mux.HandleFunc("/records/draft", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("cayenneExp")
		askedDrafts = append(askedDrafts, filter)
		if len(askedDrafts) > 1 {
			w.Write([]byte(`{"data":[]}`))
			return
		}
		w.Write([]byte(`{"data":[
		  {"playerId":8486065,"firstName":"Caleb","lastName":"Malhotra","position":"C","birthDate":"2008-06-02","roundNumber":1,"overallPickNumber":3,"triCode":"VAN","amateurClubName":"BRANTFORD","amateurLeague":"OHL"},
		  {"playerId":8486067,"firstName":"Gavin","lastName":"McKenna","position":"LW","birthDate":"2007-12-20","roundNumber":1,"overallPickNumber":1,"triCode":"TOR","amateurClubName":"PENN STATE","amateurLeague":"BIG10"},
		  {"playerId":0,"firstName":"","lastName":"","roundNumber":2,"overallPickNumber":40,"triCode":"OTT"}]}`))
	})
	mux.HandleFunc("/draft/rankings/", func(w http.ResponseWriter, r *http.Request) {
		askedRankings = append(askedRankings, r.URL.Path)
		if !strings.HasSuffix(r.URL.Path, "/1") {
			w.Write([]byte(`{"rankings":[]}`))
			return
		}
		w.Write([]byte(`{"rankings":[{"firstName":"Future","lastName":"Star","positionCode":"RW","birthDate":"2009-01-15","lastAmateurClub":"LONDON","lastAmateurLeague":"OHL","midtermRank":2,"finalRank":0}]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base, src.RecordsBase = server.URL, server.URL+"/records"

	players, err := src.Prospects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(askedDrafts) != draftClasses || len(askedRankings) != 4 {
		t.Errorf("asked for %d drafts and %d ranking lists, want %d and 4", len(askedDrafts), len(askedRankings), draftClasses)
	}
	byName := map[string]ingest.Player{}
	for _, p := range players {
		byName[p.FullName] = p
	}
	if len(players) != 4 {
		t.Fatalf("got %d prospects, want a club prospect, two picks and a ranked player: %v", len(players), byName)
	}

	// A club's prospect has the id he will keep in the NHL.
	if p := byName["Nikolai Chebykin"]; p.Provider != "nhle" || p.ProviderID != "8479599" || p.Note != "TOR prospect" {
		t.Errorf("club prospect = %+v", p)
	}
	// So does a draft pick, with where he was taken and from.
	pick := byName["Caleb Malhotra"]
	if pick.Provider != "nhle" || pick.ProviderID != "8486065" || pick.Positions[0] != "C" ||
		pick.BirthDate.Format(time.DateOnly) != "2008-06-02" ||
		!strings.Contains(pick.Note, "round 1, #3 overall to VAN. BRANTFORD (OHL)") {
		t.Errorf("draft pick = %+v", pick)
	}
	if len(pick.Aliases) != 1 || pick.Aliases[0] != (ingest.ExternalID{Provider: "nhle_ranking", ProviderID: "caleb malhotra|2008-06-02"}) {
		t.Errorf("draft pick aliases = %+v, want the key he would have been ranked under", pick.Aliases)
	}
	if byName["Gavin McKenna"].Positions[0] != "L" {
		t.Errorf("a left wing is written %q, want L as on a roster", byName["Gavin McKenna"].Positions[0])
	}
	// A ranked player has no id yet, only his name and birth date.
	ranked := byName["Future Star"]
	if ranked.Provider != "nhle_ranking" || ranked.ProviderID != "future star|2009-01-15" || ranked.Positions[0] != "R" ||
		!strings.Contains(ranked.Note, "ranked #2 among North American skaters (midterm). LONDON (OHL)") {
		t.Errorf("ranked player = %+v", ranked)
	}
}

// Before the rankings for the next draft are published the feed answers
// 404, which is not a failure.
func TestProspectsBeforeRankings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/standings/now", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"standings":[]}`)) })
	mux.HandleFunc("/records/draft", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":[]}`)) })
	server := httptest.NewServer(mux) // no rankings route: 404
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base, src.RecordsBase = server.URL, server.URL+"/records"
	if players, err := src.Prospects(context.Background()); err != nil || len(players) != 0 {
		t.Errorf("got %d prospects and error %v, want none and no error", len(players), err)
	}
}

func TestGamesAndBoxScore(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/score/2026-10-06", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"games":[
		  {"id":2026020044,"startTimeUTC":"2026-10-06T23:00:00Z","gameState":"OFF","homeTeam":{"abbrev":"TOR","score":5},"awayTeam":{"abbrev":"NSH","score":4},
		   "period":4,"periodDescriptor":{"periodType":"OT"},"clock":{"timeRemaining":"00:00"}},
		  {"id":2026020045,"startTimeUTC":"2026-10-07T02:00:00Z","gameState":"CRIT","homeTeam":{"abbrev":"VAN","score":2},"awayTeam":{"abbrev":"CGY","score":0},
		   "period":3,"periodDescriptor":{"periodType":"REG"},"clock":{"timeRemaining":"02:46"}},
		  {"id":2026020046,"startTimeUTC":"2026-10-07T02:30:00Z","gameState":"FUT","homeTeam":{"abbrev":"LAK"},"awayTeam":{"abbrev":"SJS"}}
		]}`))
	})
	mux.HandleFunc("/gamecenter/2026020044/boxscore", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"playerByGameStats":{
		  "homeTeam":{"forwards":[{"playerId":1,"goals":2,"assists":1,"sog":5,"hits":3,"blockedShots":1,"powerPlayGoals":1,"pim":2,"plusMinus":-1}],
		              "defense":[{"playerId":2,"assists":2}],
		              "goalies":[{"playerId":3,"toi":"64:46","saves":21,"goalsAgainst":4,"decision":"W"},{"playerId":4,"toi":"00:00"}]},
		  "awayTeam":{"forwards":[],"defense":[],"goalies":[{"playerId":5,"toi":"62:10","saves":30,"goalsAgainst":5,"decision":"L"}]}
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
	if len(games) != 3 || games[0].Status != ingest.GameFinal || games[1].Status != ingest.GameLive || games[2].Status != ingest.GameScheduled {
		t.Fatalf("games = %+v", games)
	}
	if games[0].HomeScore != 5 || games[0].AwayScore != 4 || games[0].Detail != "Final/OT" ||
		games[1].HomeScore != 2 || games[1].Detail != "02:46 - 3rd" || games[2].Detail != "" {
		t.Errorf("scores and state = %+v", games)
	}
	if games[0].ProviderID != "2026020044" || games[0].HomeTeam != "TOR" || games[0].AwayTeam != "NSH" {
		t.Errorf("first game = %+v", games[0])
	}

	lines, err := src.BoxScore(context.Background(), games[0])
	if err != nil {
		t.Fatal(err)
	}
	byPlayer := map[string]map[string]float64{}
	for _, l := range lines {
		byPlayer[l.ProviderID] = l.Stats
	}
	if len(byPlayer) != 4 {
		t.Fatalf("got lines for %d players, want 4: the goalie who did not play is left out", len(byPlayer))
	}
	if s := byPlayer["1"]; s["goals"] != 2 || s["assists"] != 1 || s["shots"] != 5 || s["hits"] != 3 || s["blocks"] != 1 ||
		s["pp_goals"] != 1 || s["pim"] != 2 || s["plus_minus"] != -1 {
		t.Errorf("skater = %v", s)
	}
	if s := byPlayer["3"]; s["saves"] != 21 || s["goals_against"] != 4 || s["goalie_wins"] != 1 {
		t.Errorf("winning goalie = %v", s)
	}
	if _, won := byPlayer["5"]["goalie_wins"]; won {
		t.Errorf("losing goalie = %v", byPlayer["5"])
	}
}

func TestSeasonStats(t *testing.T) {
	mux := http.NewServeMux()
	report := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("cayenneExp"); got != "seasonId=20252026 and gameTypeId=2" {
				t.Errorf("filter = %q", got)
			}
			w.Write([]byte(`{"data":[` + body + `]}`))
		}
	}
	mux.HandleFunc("/skater/summary", report(`{"playerId":1,"teamAbbrevs":"EDM","gamesPlayed":82,"goals":48,"assists":90,"shots":306,"ppGoals":13,"penaltyMinutes":44,"plusMinus":17}`))
	mux.HandleFunc("/skater/realtime", report(`{"playerId":1,"hits":71,"blockedShots":33}`))
	mux.HandleFunc("/goalie/summary", report(`{"playerId":2,"teamAbbrevs":"TOR","gamesPlayed":26,"wins":10,"saves":632,"goalsAgainst":76}`))
	server := httptest.NewServer(mux)
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.StatsBase = server.URL

	lines, err := src.SeasonStats(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want a skater and a goalie", len(lines))
	}
	// The skater's scoring and his hits and blocks come from different reports.
	skater := lines[0]
	if skater.ProviderID != "1" || skater.Year != 2026 || skater.Label != "2025-26" || skater.Team != "EDM" || skater.Games != 82 ||
		skater.Stats["goals"] != 48 || skater.Stats["pp_goals"] != 13 || skater.Stats["pim"] != 44 ||
		skater.Stats["hits"] != 71 || skater.Stats["blocks"] != 33 {
		t.Errorf("skater = %+v", skater)
	}
	if g := lines[1]; g.ProviderID != "2" || g.Stats["saves"] != 632 || g.Stats["goals_against"] != 76 || g.Stats["goalie_wins"] != 10 {
		t.Errorf("goalie = %+v", g)
	}

	october, _ := time.Parse(time.DateOnly, "2026-10-07")
	june, _ := time.Parse(time.DateOnly, "2026-06-15")
	if src.LatestSeason(october) != 2027 || src.LatestSeason(june) != 2026 {
		t.Errorf("LatestSeason = %d in October 2026 and %d in June 2026, want 2027 and 2026", src.LatestSeason(october), src.LatestSeason(june))
	}
}

func TestCareer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"position":"C","seasonTotals":[
		  {"season":20232024,"gameTypeId":2,"leagueAbbrev":"OHL","teamName":{"default":"Erie Otters"},"gamesPlayed":47,"goals":44,"assists":76,"pim":48},
		  {"season":20232024,"gameTypeId":3,"leagueAbbrev":"OHL","teamName":{"default":"Erie Otters"},"gamesPlayed":9,"goals":8},
		  {"season":20252026,"gameTypeId":2,"leagueAbbrev":"NHL","teamName":{"default":"Edmonton Oilers"},"gamesPlayed":82,"goals":50}]}`))
	}))
	defer server.Close()

	src := New(ingest.NewClient(0))
	src.Base = server.URL

	seasons, err := src.Career(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	// Only the junior regular season: the NHL comes from SeasonStats, and playoffs are left out.
	if len(seasons) != 1 {
		t.Fatalf("got %d seasons, want 1", len(seasons))
	}
	if s := seasons[0]; s.Year != 2024 || s.Label != "2023-24" || s.League != "OHL" || s.Team != "Erie Otters" ||
		s.Games != 47 || s.Stats["goals"] != 44 || s.Stats["assists"] != 76 {
		t.Errorf("junior season = %+v", s)
	}
}
