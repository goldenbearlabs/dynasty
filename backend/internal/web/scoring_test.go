package web_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/sportsday"
)

// TestScoringFlow plays a short season: starting it, setting lineups under
// the lock rules, scoring from stat lines, a rule change, players leaving
// the roster, and crowning a champion.
func TestScoringFlow(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "scoring_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zs-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'zs-%'`)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann, bob := newBrowser(t, server.URL), newBrowser(t, server.URL)

	// --- an NBA league with two starting slots, and a weekly NFL league ---
	nba, _ := registry.Get("nba")
	nfl, _ := registry.Get("nfl")
	rules := nba.Defaults
	rules.Roster.Main, rules.Roster.Reserve, rules.Roster.ReserveEligibility = 3, 2, "anyone"
	rules.Lineup.Slots = rules.Lineup.Slots[:0:0]
	rules.Lineup.Slots = append(rules.Lineup.Slots, nba.Defaults.Lineup.Slots[0], nba.Defaults.Lineup.Slots[3]) // G and UTIL
	rules.Lineup.Slots[0].Count, rules.Lineup.Slots[1].Count = 1, 1
	rules.Scoring = map[string]float64{"pts": 1, "reb": 1.2}
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name:     "Scoring Test",
		Settings: dynastySettings(10, 7),
		Leagues: []dynasty.LeagueSetup{
			{Competition: "nba", Settings: rules},
			{Competition: "nfl", Settings: nfl.Defaults},
		},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)

	var view struct {
		Leagues []struct {
			ID, Competition string
			Settings        json.RawMessage
		}
		Franchises []struct{ ID, Name string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, nflLeague := view.Leagues[0], view.Leagues[1]
	team := map[string]string{}
	for _, f := range view.Franchises {
		team[f.Name] = f.ID
	}
	var invites []struct {
		Name        string
		InviteToken string `json:"invite_token"`
	}
	ann.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	bob.claim(invites[1].InviteToken, "bob@example.com")

	// --- players on two real teams, with games around today ---------------
	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nba", FullName: "Zs Early Guard", Positions: []string{"G"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zs Late Guard", Positions: []string{"G"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zs Late Forward", Positions: []string{"F"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zs Other Guard", Positions: []string{"G"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zs+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}

	today := sportsday.Today()
	// One team's game today has started; the other's has not. Both stay
	// inside today's sports day whenever the test runs.
	started, later := time.Now().Add(-time.Minute), time.Now().Add(3*time.Hour)
	if !sportsday.Of(started).Equal(today) {
		started = sportsday.Start(today)
	}
	if !sportsday.Of(later).Equal(today) {
		later = sportsday.Start(today.AddDate(0, 0, 1)).Add(-time.Minute)
	}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	sql(`insert into pro_teams (competition, provider_id, abbrev, name) values ('nba', 'zs-early', 'ZSE', 'Early'), ('nba', 'zs-late', 'ZSL', 'Late')`)
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zs-early') where id = $1`, id["Zs Early Guard"])
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zs-late') where id = any($1::uuid[])`,
		[]string{id["Zs Late Guard"], id["Zs Late Forward"], id["Zs Other Guard"]})
	game := func(providerID, home string, day time.Time, startsAt time.Time, status string) {
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
		     values ('nba', $1, $2, $3, $4, (select id from pro_teams where provider_id = $5))`, providerID, day, startsAt, status, home)
	}
	line := func(gameID, player, stats string) {
		sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, $3::jsonb)`, gameID, id[player], stats)
	}
	yesterday := today.AddDate(0, 0, -1)
	game("zs-yesterday", "zs-early", yesterday, sportsday.Start(yesterday).Add(14*time.Hour), "final")
	game("zs-started", "zs-early", today, started, "live")
	game("zs-later", "zs-late", today, later, "scheduled")
	line("zs-yesterday", "Zs Early Guard", `{"pts": 50}`)
	line("zs-started", "Zs Early Guard", `{"pts": 10, "reb": 5, "ast": 9}`) // assists are not scored here
	line("zs-started", "Zs Other Guard", `{"pts": 40}`)                     // Bob's player, who is not starting

	place := func(player, franchise string) {
		t.Helper()
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league.ID+"/roster/add",
			map[string]any{"player_id": id[player], "list": "main", "franchise_id": team[franchise], "force": true}, nil)
	}
	place("Zs Early Guard", "Ann")
	place("Zs Late Guard", "Ann")
	place("Zs Late Forward", "Ann")
	place("Zs Other Guard", "Bob")

	// Inline research shows recorded finals under current scoring rules,
	// without mixing in today's live line or another player's games.
	var research players.Research
	ann.want(http.StatusOK, "GET", "/api/players/"+id["Zs Early Guard"], nil, &research)
	if research.Player.FullName != "Zs Early Guard" || research.Player.OwnerName != "Ann" || research.Player.TeamAbbrev != "ZSE" {
		t.Fatalf("research identity = %+v", research.Player)
	}
	if len(research.Games) != 1 || research.Games[0].Points != 50 {
		t.Fatalf("research history = %+v, want yesterday's 50-point final only", research.Games)
	}
	ann.want(http.StatusOK, "GET", "/api/players/"+id["Zs Late Guard"], nil, &research)
	if research.Games == nil || len(research.Games) != 0 {
		t.Fatalf("research without games = %+v, want an empty list", research.Games)
	}
	ann.want(http.StatusNotFound, "GET", "/api/players/not-a-uuid", nil, nil)
	ann.want(http.StatusNotFound, "GET", "/api/players/00000000-0000-0000-0000-000000000000", nil, nil)

	// --- seasons -----------------------------------------------------------
	type standings struct {
		Competition string
		Season      *struct {
			ID, Status          string
			ChampionFranchiseID *string `json:"champion_franchise_id"`
		}
		Rows []struct {
			FranchiseID string `json:"franchise_id"`
			Points      float64
			Players     []struct {
				FullName string `json:"full_name"`
				Games    int
				Points   float64
			}
		}
	}
	table := func() standings {
		t.Helper()
		var all []standings
		ann.want(http.StatusOK, "GET", "/api/standings", nil, &all)
		return all[0]
	}
	points := func() (annPoints, bobPoints float64) {
		t.Helper()
		for _, row := range table().Rows {
			if row.FranchiseID == team["Ann"] {
				annPoints = row.Points
			} else {
				bobPoints = row.Points
			}
		}
		return
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.001 }

	if before := table(); before.Season != nil || len(before.Rows) != 2 {
		t.Fatalf("before any season: %+v", before)
	}
	season := map[string]any{
		"league_id": league.ID, "year": today.Year(),
		"starts_on": today.AddDate(0, 0, -7).Format(time.DateOnly), "ends_on": today.AddDate(0, 0, 30).Format(time.DateOnly),
	}
	bob.want(http.StatusForbidden, "POST", "/api/admin/seasons", season, nil)
	backwards := map[string]any{"league_id": league.ID, "year": today.Year(), "starts_on": "2026-05-01", "ends_on": "2026-04-01"}
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/seasons", backwards, nil)
	var created struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", season, &created)
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/seasons", season, nil) // one season at a time

	// --- lineups and locks -------------------------------------------------
	lineupURL := "/api/leagues/" + league.ID + "/lineup"
	start := func(day time.Time, slots ...string) map[string]any {
		entries := []map[string]string{}
		for i := 0; i < len(slots); i += 2 {
			entries = append(entries, map[string]string{"slot": slots[i], "player_id": id[slots[i+1]]})
		}
		return map[string]any{"day": day.Format(time.DateOnly), "entries": entries}
	}
	bob.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(today, "G", "Zs Late Guard"), nil)                             // not Bob's player
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(yesterday, "G", "Zs Late Guard"), nil)                         // the day has passed
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(today, "G", "Zs Late Forward"), nil)                           // a forward at guard
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(today, "G", "Zs Late Guard", "G", "Zs Early Guard"), nil)      // one G slot
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(today, "G", "Zs Late Guard", "UTIL", "Zs Late Guard"), nil)    // the same player twice
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(today, "BENCH", "Zs Late Guard"), nil)                         // no such slot
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(today, "G", "Zs Early Guard", "UTIL", "Zs Late Forward"), nil) // his game has started
	ann.want(http.StatusNoContent, "PUT", lineupURL, start(today, "G", "Zs Late Guard", "UTIL", "Zs Late Forward"), nil)

	type lineupView struct {
		Day, LastDay string `json:"-"`
		Players      []struct {
			FullName string `json:"full_name"`
			Slot     string
			Locked   bool
			Points   float64
			Games    []struct{ Opponent, Status string }
		}
	}
	lineupOn := func(franchise string, day time.Time) map[string]string {
		t.Helper()
		var v lineupView
		bob.want(http.StatusOK, "GET", lineupURL+"?franchise="+team[franchise]+"&day="+day.Format(time.DateOnly), nil, &v) // lineups are public
		slots := map[string]string{}
		for _, p := range v.Players {
			slots[p.FullName] = p.Slot
			if p.FullName == "Zs Early Guard" && day.Equal(today) && (!p.Locked || len(p.Games) != 1 || !near(p.Points, 16)) {
				t.Errorf("the early guard today = %+v, want locked with one game worth 16", p)
			}
		}
		return slots
	}
	if got := lineupOn("Ann", today); got["Zs Late Guard"] != "G" || got["Zs Late Forward"] != "UTIL" || got["Zs Early Guard"] != "" {
		t.Fatalf("Ann's lineup today = %v", got)
	}
	if a, _ := points(); a != 0 {
		t.Fatalf("Ann has %v points with only players who have not played", a)
	}

	// The commissioner can override a lock. Now the early guard starts.
	forced := start(today, "G", "Zs Early Guard", "UTIL", "Zs Late Forward")
	bob.want(http.StatusForbidden, "PUT", lineupURL, map[string]any{"day": forced["day"], "entries": forced["entries"], "franchise_id": team["Ann"]}, nil)
	forced["force"] = true
	ann.want(http.StatusNoContent, "PUT", lineupURL, forced, nil)

	// --- scoring -----------------------------------------------------------
	// 10 points and 5 rebounds at 1 and 1.2. Yesterday's 50 came before the
	// lineup took effect, and Bob's 40 came from a player he did not start.
	if a, b := points(); !near(a, 16) || b != 0 {
		t.Fatalf("points = Ann %v, Bob %v; want 16 and 0", a, b)
	}
	if top := table().Rows[0]; top.FranchiseID != team["Ann"] || len(top.Players) != 1 || top.Players[0].Games != 1 {
		t.Fatalf("standings leader = %+v", top)
	}

	// Changing a rule changes every total at once.
	var saved map[string]any
	json.Unmarshal(league.Settings, &saved)
	saved["scoring"] = map[string]float64{"pts": 2, "reb": 1.2}
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league.ID+"/settings", saved, nil)
	if a, _ := points(); !near(a, 26) {
		t.Fatalf("after doubling points Ann has %v, want 26", a)
	}

	// A lineup stays in force until replaced: had it been set two days ago,
	// yesterday's game would count too.
	earlier := today.AddDate(0, 0, -2)
	sql(`insert into lineups (league_id, franchise_id, effective_on) select league_id, franchise_id, $1 from lineups where franchise_id = $2`, earlier, team["Ann"])
	sql(`update lineup_entries set effective_on = $1 where franchise_id = $2`, earlier, team["Ann"])
	sql(`delete from lineups where franchise_id = $1 and effective_on = $2`, team["Ann"], today)
	if a, _ := points(); !near(a, 126) {
		t.Fatalf("with the lineup in force since two days ago Ann has %v, want 126", a)
	}
	if got := lineupOn("Ann", today.AddDate(0, 0, 5)); got["Zs Early Guard"] != "G" {
		t.Errorf("five days on, the lineup is %v; it should still be in force", got)
	}

	// --- leaving the roster ------------------------------------------------
	// The early guard goes to reserve after his game began: today's points
	// stand, and from tomorrow he no longer starts.
	move := map[string]any{"player_id": id["Zs Early Guard"], "list": "reserve"}
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league.ID+"/roster/move", move, nil)
	if a, _ := points(); !near(a, 126) {
		t.Errorf("after benching a player mid-game Ann has %v, want the 126 already scored", a)
	}
	if got := lineupOn("Ann", today.AddDate(0, 0, 1)); len(got) != 2 || got["Zs Late Forward"] != "UTIL" {
		t.Errorf("tomorrow's lineup = %v, want the forward still starting and the guard gone from the roster view", got)
	}
	// The forward is dropped before his game: he is out of today's lineup.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league.ID+"/roster/drop", map[string]any{"player_id": id["Zs Late Forward"]}, nil)
	sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = 'zs-later'), $1, '{"pts": 30}')`, id["Zs Late Forward"])
	if a, _ := points(); !near(a, 126) {
		t.Errorf("a dropped player's later game changed Ann's points to %v", a)
	}

	// --- weekly lineups ----------------------------------------------------
	var week struct {
		Day     string
		LastDay string `json:"last_day"`
	}
	ann.want(http.StatusOK, "GET", "/api/leagues/"+nflLeague.ID+"/lineup?franchise="+team["Ann"]+"&day=2026-10-08", nil, &week)
	if week.Day != "2026-10-06" || week.LastDay != "2026-10-12" { // Thursday falls in the Tuesday-to-Monday week
		t.Errorf("the NFL week of 2026-10-08 = %s to %s, want 2026-10-06 to 2026-10-12", week.Day, week.LastDay)
	}

	// --- titles ------------------------------------------------------------
	type overallYear struct {
		Year  int
		Final bool
		Rows  []struct {
			FranchiseID string `json:"franchise_id"`
			Points      float64
			Finishes    map[string]int
		}
	}
	var overall []overallYear
	ann.want(http.StatusOK, "GET", "/api/overall", nil, &overall)
	if len(overall) != 1 || overall[0].Final || overall[0].Rows[0].FranchiseID != team["Ann"] || overall[0].Rows[0].Points != 10 {
		t.Fatalf("overall during the season = %+v, want Ann leading 10 to 7 and not final", overall)
	}

	closeURL := "/api/admin/seasons/" + created.ID + "/close"
	bob.want(http.StatusForbidden, "POST", closeURL, map[string]any{}, nil)
	ann.want(http.StatusNoContent, "POST", closeURL, map[string]any{}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", closeURL, map[string]any{}, nil)
	if done := table().Season; done.Status != "complete" || done.ChampionFranchiseID == nil || *done.ChampionFranchiseID != team["Ann"] {
		t.Fatalf("closed season = %+v, want Ann the champion", done)
	}
	// The year is not final overall while the NFL league has not played it.
	ann.want(http.StatusOK, "GET", "/api/overall", nil, &overall)
	if overall[0].Final || overall[0].Rows[1].Finishes["nba"] != 2 {
		t.Errorf("overall after one of two leagues finished = %+v", overall[0])
	}

	// A closed season can be reopened, and the next one must be a later year.
	ann.want(http.StatusNoContent, "POST", "/api/admin/seasons/"+created.ID+"/reopen", nil, nil)
	if table().Season.Status != "active" {
		t.Error("the reopened season is not active")
	}
	ann.want(http.StatusNoContent, "POST", closeURL, map[string]any{"champion_franchise_id": team["Bob"]}, nil) // the commissioner's call
	if champion := table().Season.ChampionFranchiseID; champion == nil || *champion != team["Bob"] {
		t.Error("the commissioner's choice of champion was not kept")
	}
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/seasons", season, nil) // that year is taken
	season["year"] = today.Year() + 1
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", season, nil)
}
