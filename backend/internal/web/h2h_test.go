package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// TestHeadToHeadFlow plays a four-team head-to-head season that has already
// happened: a three-day round robin, then a one-day final between the top
// two. It also follows a game over the live connections, and a college
// player's rights into the NBA league.
func TestHeadToHeadFlow(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "h2h_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zh-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann, bob := newBrowser(t, server.URL), newBrowser(t, server.URL)

	// --- an NBA league decided head to head, a day at a time ---------------
	nba, _ := registry.Get("nba")
	cbb, _ := registry.Get("cbb")
	rules := nba.Defaults
	rules.Lineup.Slots = []settings.Slot{{Name: "UTIL", Positions: []string{settings.AnyPosition}, Count: 1}}
	rules.Lineup.Period = "day"
	rules.Roster.ReserveEligibility = "anyone"
	rules.Scoring = map[string]float64{"pts": 1}
	rules.Format.Type, rules.Format.MatchupDays, rules.Format.PlayoffTeams = "head_to_head", 1, 2
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name:     "Head To Head Test",
		Settings: dynastySettings(10, 7, 5, 3),
		Leagues: []dynasty.LeagueSetup{
			{Competition: "cbb", Settings: cbb.Defaults}, // carries players into the NBA league
			{Competition: "nba", Settings: rules},
		},
		Franchises: []dynasty.FranchiseSetup{
			{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"},
			{Name: "Cy", ManagerName: "Cy"}, {Name: "Di", ManagerName: "Di"},
		},
		Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)

	var view struct {
		Leagues    []struct{ ID, Competition string }
		Franchises []struct{ ID, Name, Slug string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	college, league := view.Leagues[0], view.Leagues[1]
	team, name := map[string]string{}, map[string]string{}
	for _, f := range view.Franchises {
		team[f.Name], name[f.ID] = f.ID, f.Name
	}
	var invites []struct {
		Name        string
		InviteToken string `json:"invite_token"`
	}
	ann.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	bob.claim(invites[1].InviteToken, "bob@example.com")

	// --- one player each, who scores the same every day: Ann's 40, Bob's 30,
	// Cy's 20, Di's 10. Then the final, where Bob outscores Ann.
	daily := map[string]int{"Ann": 40, "Bob": 30, "Cy": 20, "Di": 10}
	var roster []players.NewPlayer
	for owner := range daily {
		roster = append(roster, players.NewPlayer{Competition: "nba", FullName: "Zh " + owner + " Star", Status: "active", Note: marker})
	}
	roster = append(roster, players.NewPlayer{Competition: "cbb", FullName: "Zh College Kid", Status: "active", Note: marker})
	ann.want(http.StatusCreated, "POST", "/api/admin/players", roster, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zh+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}

	today := sportsday.Today()
	day := func(ago int) time.Time { return today.AddDate(0, 0, -ago) }
	for owner := range daily {
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league.ID+"/roster/add",
			map[string]any{"player_id": id["Zh "+owner+" Star"], "list": "main", "franchise_id": team[owner], "force": true}, nil)
		// Everyone has started their star since before the season.
		sql(`insert into lineups (league_id, franchise_id, effective_on) values ($1, $2, $3)`, league.ID, team[owner], day(10))
		sql(`insert into lineup_entries (league_id, franchise_id, effective_on, slot, slot_index, player_id) values ($1, $2, $3, 'UTIL', 0, $4)`,
			league.ID, team[owner], day(10), id["Zh "+owner+" Star"])
	}
	for ago := 4; ago >= 1; ago-- {
		game := "zh-" + day(ago).Format(time.DateOnly)
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_score, away_score, detail)
		     values ('nba', $1, $2, $3, 'final', 101, 99, 'Final')`, game, day(ago), sportsday.Start(day(ago)).Add(14*time.Hour))
		for owner, points := range daily {
			if ago == 1 && owner == "Bob" {
				points = 99 // the final
			}
			sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, jsonb_build_object('pts', $3::int))`,
				game, id["Zh "+owner+" Star"], points)
		}
	}

	// --- the season and its schedule ---------------------------------------
	season := func(first, last time.Time) map[string]any {
		return map[string]any{"league_id": league.ID, "year": today.Year(),
			"starts_on": first.Format(time.DateOnly), "ends_on": last.Format(time.DateOnly)}
	}
	// One day cannot hold both a regular-season matchup and a final.
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/seasons", season(day(1), day(1)), nil)
	var created struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", season(day(4), day(1)), &created)

	type matchups struct {
		Periods []struct {
			Seq       int
			IsPlayoff bool `json:"is_playoff"`
		}
		Period   *struct{ Seq int }
		Matchups []struct {
			ID         string
			Home       string  `json:"home_franchise_id"`
			Away       *string `json:"away_franchise_id"`
			HomePoints float64 `json:"home_points"`
			AwayPoints float64 `json:"away_points"`
			Final      bool
		}
	}
	period := func(seq string) matchups {
		t.Helper()
		var m matchups
		bob.want(http.StatusOK, "GET", "/api/leagues/"+league.ID+"/matchups?period="+seq, nil, &m)
		return m
	}

	// Three regular days in which everyone meets everyone, then a playoff day.
	met := map[string]bool{}
	for _, seq := range []string{"1", "2", "3"} {
		m := period(seq)
		if len(m.Periods) != 4 || !m.Periods[3].IsPlayoff || m.Periods[2].IsPlayoff || len(m.Matchups) != 2 {
			t.Fatalf("period %s = %+v", seq, m)
		}
		for _, game := range m.Matchups {
			home, away := name[game.Home], name[*game.Away]
			if game.HomePoints != float64(daily[home]) || game.AwayPoints != float64(daily[away]) || !game.Final {
				t.Errorf("period %s: %s %v, %s %v, final %v", seq, home, game.HomePoints, away, game.AwayPoints, game.Final)
			}
			met[min(home, away)+max(home, away)] = true
		}
	}
	if len(met) != 6 {
		t.Errorf("the round robin produced %d distinct pairings, want all 6", len(met))
	}

	// --- standings by record -----------------------------------------------
	type standings struct {
		Competition string
		Format      string
		Season      *struct {
			Status              string
			ChampionFranchiseID *string `json:"champion_franchise_id"`
		}
		Rows []struct {
			FranchiseID        string `json:"franchise_id"`
			Wins, Losses, Ties int
			Points             float64
		}
	}
	nbaTable := func() standings {
		t.Helper()
		var all []standings
		ann.want(http.StatusOK, "GET", "/api/standings", nil, &all)
		return all[1]
	}
	table := nbaTable()
	if table.Format != "head_to_head" {
		t.Fatalf("format = %q", table.Format)
	}
	for i, want := range []struct {
		name         string
		wins, losses int
	}{{"Ann", 3, 0}, {"Bob", 2, 1}, {"Cy", 1, 2}, {"Di", 0, 3}} {
		row := table.Rows[i]
		if name[row.FranchiseID] != want.name || row.Wins != want.wins || row.Losses != want.losses {
			t.Errorf("place %d = %s %d-%d, want %s %d-%d", i+1, name[row.FranchiseID], row.Wins, row.Losses, want.name, want.wins, want.losses)
		}
	}

	// --- the playoffs ------------------------------------------------------
	if final := period("4"); len(final.Matchups) != 0 {
		t.Fatalf("the final was set before anyone asked: %+v", final.Matchups)
	}
	bob.want(http.StatusForbidden, "POST", "/api/admin/playoffs/advance", nil, nil)
	ann.want(http.StatusNoContent, "POST", "/api/admin/playoffs/advance", nil, nil)
	ann.want(http.StatusNoContent, "POST", "/api/admin/playoffs/advance", nil, nil) // asking twice changes nothing
	final := period("4")
	if len(final.Matchups) != 1 || name[final.Matchups[0].Home] != "Ann" || name[*final.Matchups[0].Away] != "Bob" {
		t.Fatalf("the final = %+v, want first seed Ann at home to second seed Bob", final.Matchups)
	}
	if got := final.Matchups[0]; got.HomePoints != 40 || got.AwayPoints != 99 || !got.Final {
		t.Errorf("the final's score = %v to %v, final %v; want 40 to 99, finished", got.HomePoints, got.AwayPoints, got.Final)
	}
	// With no period named, the last one is shown once the season is over.
	if shown := period(""); shown.Period == nil || shown.Period.Seq != 4 {
		t.Errorf("default period = %+v, want the final", shown.Period)
	}

	var detail struct {
		Competition string
		HomePlayers []struct {
			FullName string `json:"full_name"`
			Points   float64
		} `json:"home_players"`
		AwayPoints float64 `json:"away_points"`
	}
	ann.want(http.StatusOK, "GET", "/api/matchups/"+final.Matchups[0].ID, nil, &detail)
	if detail.Competition != "nba" || len(detail.HomePlayers) != 1 || detail.HomePlayers[0].FullName != "Zh Ann Star" || detail.AwayPoints != 99 {
		t.Errorf("matchup detail = %+v", detail)
	}

	// The schedule cannot be rebuilt once the playoffs have begun.
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/seasons/"+created.ID+"/schedule", nil, nil)

	// The champion is whoever won the final, not whoever led the table.
	ann.want(http.StatusNoContent, "POST", "/api/admin/seasons/"+created.ID+"/close", map[string]any{}, nil)
	if champion := nbaTable().Season.ChampionFranchiseID; champion == nil || name[*champion] != "Bob" {
		t.Errorf("champion = %v, want Bob, who won the final", champion)
	}

	// --- scoreboards and game pages, pushed over a websocket ---------------
	ws := "ws" + strings.TrimPrefix(server.URL, "http")
	listen := func(path string, into any) {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, ws+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.CloseNow()
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, raw, err := conn.Read(readCtx)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatal(err)
		}
	}
	type board struct {
		Day   string
		Games []struct {
			ID             string
			Status, Detail string
			HomeScore      int `json:"home_score"`
			AwayScore      int `json:"away_score"`
		}
	}
	// Today's board arrives on connecting; past days come over plain HTTP.
	var live, past board
	listen("/api/scores/nba/ws", &live)
	if live.Day != today.Format(time.DateOnly) {
		t.Errorf("the live scoreboard is for %s, want today", live.Day)
	}
	ann.want(http.StatusOK, "GET", "/api/scores/nba?day="+day(1).Format(time.DateOnly), nil, &past)
	var finalGame string
	for _, g := range past.Games {
		if g.Detail == "Final" && g.HomeScore == 101 && g.AwayScore == 99 {
			finalGame = g.ID
		}
	}
	if finalGame == "" {
		t.Fatalf("yesterday's scoreboard %+v does not have the test's game", past.Games)
	}
	ann.want(http.StatusNotFound, "GET", "/api/scores/curling", nil, nil)

	// A game page lists each player's line, its fantasy value and his owner.
	var game struct {
		HomeScore int `json:"home_score"`
		Lines     []struct {
			FullName  string `json:"full_name"`
			Points    float64
			OwnerSlug string `json:"owner_slug"`
			Stats     map[string]float64
		}
	}
	listen("/api/games/"+finalGame+"/ws", &game)
	if game.HomeScore != 101 || len(game.Lines) != 4 || game.Lines[0].FullName != "Zh Bob Star" ||
		game.Lines[0].Points != 99 || game.Lines[0].OwnerSlug != "bob" || game.Lines[0].Stats["pts"] != 99 {
		t.Errorf("game page = %+v, want four lines led by Bob's star with 99", game)
	}

	// --- a college player's rights follow him to the NBA -------------------
	kid := id["Zh College Kid"]
	holdings := func() map[string]string { // player -> "league competition/list" for Bob
		t.Helper()
		var d struct {
			Rosters []struct {
				Competition string
				Players     []struct {
					FullName string `json:"full_name"`
					List     string
				}
			}
		}
		ann.want(http.StatusOK, "GET", "/api/franchises/bob", nil, &d)
		held := map[string]string{}
		for _, r := range d.Rosters {
			for _, p := range r.Players {
				held[p.FullName] = r.Competition + "/" + p.List
			}
		}
		return held
	}
	place := func() {
		t.Helper()
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+college.ID+"/roster/add",
			map[string]any{"player_id": kid, "list": "main", "franchise_id": team["Bob"], "force": true}, nil)
	}
	var kidID pgtype.UUID
	kidID.Scan(kid)

	// He leaves college without appearing in the NBA feed: the commissioner decides.
	place()
	sql(`update players set status = 'inactive' where id = $1`, kid)
	var queue []struct {
		FullName      string `json:"full_name"`
		FranchiseName string `json:"franchise_name"`
		LeagueID      string `json:"league_id"`
	}
	bob.want(http.StatusForbidden, "GET", "/api/admin/review-queue", nil, nil)
	ann.want(http.StatusOK, "GET", "/api/admin/review-queue", nil, &queue)
	if len(queue) != 1 || queue[0].FullName != "Zh College Kid" || queue[0].FranchiseName != "Bob" || queue[0].LeagueID != college.ID {
		t.Fatalf("review queue = %+v", queue)
	}
	carry := map[string]string{"player_id": kid}
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/leagues/"+league.ID+"/carry-over", carry, nil) // the NBA league carries nobody on
	ann.want(http.StatusNoContent, "POST", "/api/admin/leagues/"+college.ID+"/carry-over", carry, nil)
	if held := holdings(); held["Zh College Kid"] != "nba/reserve" {
		t.Fatalf("after carrying him over Bob holds %v, want the kid on his NBA reserve list", held)
	}
	ann.want(http.StatusOK, "GET", "/api/admin/review-queue", nil, &queue)
	if len(queue) != 0 {
		t.Errorf("review queue after carrying him over = %+v", queue)
	}

	// The ordinary way: the roster sync finds him in the NBA.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league.ID+"/roster/drop",
		map[string]any{"player_id": kid, "franchise_id": team["Bob"], "force": true}, nil)
	sql(`update players set competition = 'cbb', status = 'active' where id = $1`, kid)
	place()
	sql(`update players set competition = 'nba' where id = $1`, kid)
	movedPlayer(ctx, kidID, "cbb", "nba")
	if held := holdings(); held["Zh College Kid"] != "nba/reserve" {
		t.Fatalf("after the sync moved him Bob holds %v, want the kid on his NBA reserve list", held)
	}

	var activity []struct {
		Kind       string
		PlayerName string `json:"player_name"`
	}
	ann.want(http.StatusOK, "GET", "/api/activity", nil, &activity)
	graduations := 0
	for _, a := range activity {
		if a.Kind == "graduated" && a.PlayerName == "Zh College Kid" {
			graduations++
		}
	}
	if graduations != 2 {
		t.Errorf("the feed has %d graduations, want 2", graduations)
	}
}
