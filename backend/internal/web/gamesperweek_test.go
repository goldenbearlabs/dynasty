package web_test

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// TestOneGameAWeek covers slots that count one game a week: which game
// counts, how a manager picks another, and when a player locks.
func TestOneGameAWeek(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "one_game_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zo-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'zo-%'`)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)

	// The week is made to start two days ago, so there are days either side of today.
	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Roster.Main = 4
	rules.Lineup.WeekStart = strings.ToLower(today.AddDate(0, 0, -2).Weekday().String())
	rules.Lineup.Slots = []settings.Slot{
		{Name: "G", Positions: []string{"G"}, Count: 1, GamesPerWeek: 1},
		{Name: "UTIL", Positions: []string{settings.AnyPosition}, Count: 1, GamesPerWeek: 1},
	}
	rules.Scoring = map[string]float64{"pts": 1}
	rules.Format.Type = "total_points" // one franchise: nobody to play

	daily := rules
	daily.Lineup.Period = "day"
	setup := dynasty.Setup{
		Name: "One Game Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: daily}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/dynasty", setup, nil) // counting games by the week needs weekly lineups
	setup.Leagues[0].Settings = rules
	ann.want(http.StatusCreated, "POST", "/api/dynasty", setup, nil)

	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, franchise := view.Leagues[0].ID, view.Franchises[0].ID

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nba", FullName: "Zq Played", Positions: []string{"G"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zq Tonight", Positions: []string{"F"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zq Bench", Positions: []string{"G"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zq+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add",
			map[string]any{"player_id": p.ID, "list": "main", "force": true}, nil)
	}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	sql(`insert into pro_teams (competition, provider_id, abbrev, name) values ('nba', 'zo-x', 'ZOX', 'X'), ('nba', 'zo-y', 'ZOY', 'Y')`)
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zo-x') where id = any($1::uuid[])`, []string{id["Zq Played"], id["Zq Bench"]})
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zo-y') where id = $1`, id["Zq Tonight"])
	game := func(providerID, home string, offset int, startsAt time.Time, status string) {
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
		     values ('nba', $1, $2, $3, $4, (select id from pro_teams where provider_id = $5))`, providerID, today.AddDate(0, 0, offset), startsAt, status, home)
	}
	line := func(gameID, player, stats string) {
		sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, $3::jsonb)`, gameID, id[player], stats)
	}
	later := time.Now().Add(2 * time.Hour)
	if !sportsday.Of(later).Equal(today) {
		later = sportsday.Start(today.AddDate(0, 0, 1)).Add(-time.Minute)
	}
	// Team X played yesterday and plays again in two and three days; team Y plays later today and in two days.
	game("zo-x1", "zo-x", -1, sportsday.Start(today).Add(-10*time.Hour), "final")
	game("zo-x2", "zo-x", 2, sportsday.Start(today.AddDate(0, 0, 2)).Add(14*time.Hour), "scheduled")
	game("zo-x3", "zo-x", 3, sportsday.Start(today.AddDate(0, 0, 3)).Add(14*time.Hour), "scheduled")
	game("zo-y1", "zo-y", 0, later, "scheduled")
	game("zo-y2", "zo-y", 2, sportsday.Start(today.AddDate(0, 0, 2)).Add(14*time.Hour), "scheduled")
	line("zo-x1", "Zq Played", `{"pts": 30}`)

	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(), "starts_on": day(-7), "ends_on": day(30),
	}, nil)

	lineupURL := "/api/leagues/" + league + "/lineup"
	start := func(entries ...map[string]string) map[string]any {
		return map[string]any{"day": day(0), "entries": entries}
	}
	entry := func(slot, player, countsFrom string) map[string]string {
		return map[string]string{"slot": slot, "player_id": id[player], "counts_from": countsFrom}
	}
	type seen struct {
		CountsFrom string `json:"counts_from"`
		Locked     bool
		Points     float64
		Games      []struct {
			Day    string
			Counts bool
		}
	}
	lineup := func() map[string]seen {
		t.Helper()
		var v struct {
			Players []struct {
				FullName string `json:"full_name"`
				seen
			}
		}
		ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise+"&day="+day(0), nil, &v)
		byName := map[string]seen{}
		for _, p := range v.Players {
			byName[p.FullName] = p.seen
		}
		return byName
	}
	total := func() float64 {
		t.Helper()
		assertCountedGameTotals(t, pool, league)
		var all []struct{ Rows []struct{ Points float64 } }
		ann.want(http.StatusOK, "GET", "/api/standings", nil, &all)
		return all[0].Rows[0].Points
	}

	// --- a game already played cannot be counted after the fact ------------
	if p := lineup()["Zq Played"]; p.Locked {
		t.Fatalf("a bench player with games left is locked: %+v", p)
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(entry("G", "Zq Played", day(-1))), nil) // yesterday's game
	ann.want(http.StatusNoContent, "PUT", lineupURL, start(entry("G", "Zq Played", ""), entry("UTIL", "Zq Tonight", "")), nil)
	now := lineup()
	if p := now["Zq Played"]; p.CountsFrom != day(2) || p.Locked || p.Points != 0 ||
		len(p.Games) != 3 || p.Games[0].Counts || !p.Games[1].Counts || p.Games[2].Counts {
		t.Fatalf("started after his first game: %+v; want his next game to be the one that counts", p)
	}
	if p := now["Zq Tonight"]; p.CountsFrom != "" || p.Locked || !p.Games[0].Counts || p.Games[1].Counts {
		t.Fatalf("started before any game: %+v; want his first game to count", p)
	}
	if got := total(); got != 0 {
		t.Fatalf("total = %v, want 0: yesterday's 30 came before he was started", got)
	}

	// --- the manager picks a later game ------------------------------------
	ann.want(http.StatusNoContent, "PUT", lineupURL, start(entry("G", "Zq Played", day(3)), entry("UTIL", "Zq Tonight", day(2))), nil)
	now = lineup()
	if p := now["Zq Played"]; p.CountsFrom != day(3) || p.Games[1].Counts || !p.Games[2].Counts {
		t.Fatalf("picked his third game: %+v", p)
	}
	if p := now["Zq Tonight"]; p.CountsFrom != day(2) || p.Games[0].Counts || !p.Games[1].Counts {
		t.Fatalf("picked his second game: %+v", p)
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(entry("G", "Zq Played", day(5))), nil) // no game from then on
	// Back to tonight's game for him.
	ann.want(http.StatusNoContent, "PUT", lineupURL, start(entry("G", "Zq Played", day(3)), entry("UTIL", "Zq Tonight", day(0))), nil)

	// --- only the picked game scores, and it locks him ----------------------
	sql(`update games set starts_at = now() - interval '1 minute', status = 'live' where provider_id = 'zo-y1'`)
	line("zo-y1", "Zq Tonight", `{"pts": 12}`)
	if p := lineup()["Zq Tonight"]; !p.Locked || p.Points != 12 {
		t.Fatalf("after his picked game began: %+v; want locked with 12 points", p)
	}
	if got := total(); math.Abs(got-12) > 0.001 {
		t.Fatalf("total = %v, want 12", got)
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(entry("G", "Zq Played", day(3)), entry("UTIL", "Zq Bench", "")), nil)       // he is locked in
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, start(entry("G", "Zq Played", day(3)), entry("UTIL", "Zq Tonight", day(2))), nil) // nor can his pick move
	ann.want(http.StatusNoContent, "PUT", lineupURL, start(entry("G", "Zq Bench", ""), entry("UTIL", "Zq Tonight", "")), nil)                    // the other slot is still free

	// A second game in the same week adds nothing: one game a week counts.
	sql(`update games set day = $1, starts_at = now() - interval '30 seconds' where provider_id = 'zo-y2'`, today)
	line("zo-y2", "Zq Tonight", `{"pts": 40}`)
	if got := total(); math.Abs(got-12) > 0.001 {
		t.Fatalf("total = %v after a second game, want still 12", got)
	}
}

// TestAutoLastGame covers auto set and the auto pick in slots that count
// one game a week: a player's last game of the week with a stat counts, a
// game he sat out gives way to the one before, and a game played before he
// was started never counts.
func TestAutoLastGame(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "auto_game_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'za-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'za-%'`)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Roster.Main = 4
	rules.Lineup.WeekStart = strings.ToLower(today.AddDate(0, 0, -2).Weekday().String())
	rules.Lineup.Slots = []settings.Slot{
		{Name: "G", Positions: []string{"G"}, Count: 1, GamesPerWeek: 1},
		{Name: "UTIL", Positions: []string{settings.AnyPosition}, Count: 1, GamesPerWeek: 1},
	}
	rules.Scoring = map[string]float64{"pts": 1, "reb": 1}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Auto Game Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, franchise := view.Leagues[0].ID, view.Franchises[0].ID

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nba", FullName: "Zqauto Guard", Positions: []string{"G"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zqauto Forward", Positions: []string{"F"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zqauto", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": p.ID, "list": "main", "force": true}, nil)
	}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	sql(`insert into pro_teams (competition, provider_id, abbrev, name) values ('nba', 'za-x', 'ZAX', 'X'), ('nba', 'za-y', 'ZAY', 'Y')`)
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'za-x') where id = $1`, id["Zqauto Guard"])
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'za-y') where id = $1`, id["Zqauto Forward"])
	game := func(providerID, home string, offset int, startsAt time.Time, status string) {
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
		     values ('nba', $1, $2, $3, $4, (select id from pro_teams where provider_id = $5))`, providerID, today.AddDate(0, 0, offset), startsAt, status, home)
	}
	line := func(gameID, player, stats string) {
		sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, $3::jsonb)
		     on conflict (game_id, player_id) do update set stats = excluded.stats`, gameID, id[player], stats)
	}
	at := func(offset int) time.Time { return sportsday.Start(today.AddDate(0, 0, offset)).Add(14 * time.Hour) }
	// The guard played yesterday and plays in two and three days; the forward in one, two and three days.
	game("za-x1", "za-x", -1, sportsday.Start(today).Add(-10*time.Hour), "final")
	game("za-x2", "za-x", 2, at(2), "scheduled")
	game("za-x3", "za-x", 3, at(3), "scheduled")
	game("za-y1", "za-y", 1, at(1), "scheduled")
	game("za-y2", "za-y", 2, at(2), "scheduled")
	game("za-y3", "za-y", 3, at(3), "scheduled")
	line("za-x1", "Zqauto Guard", `{"pts": 30}`)
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(), "starts_on": day(-7), "ends_on": day(30),
	}, nil)

	lineupURL := "/api/leagues/" + league + "/lineup"
	type seen struct {
		Slot       string
		CountsFrom string `json:"counts_from"`
		CountsLast bool   `json:"counts_last"`
		Locked     bool
		Points     float64
	}
	lineup := func() map[string]seen {
		t.Helper()
		var v struct {
			Players []struct {
				FullName string `json:"full_name"`
				seen
			}
		}
		ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise+"&day="+day(0), nil, &v)
		byName := map[string]seen{}
		for _, p := range v.Players {
			byName[p.FullName] = p.seen
		}
		return byName
	}
	total := func() float64 {
		t.Helper()
		assertCountedGameTotals(t, pool, league)
		var all []struct{ Rows []struct{ Points float64 } }
		ann.want(http.StatusOK, "GET", "/api/standings", nil, &all)
		return all[0].Rows[0].Points
	}

	// --- auto set starts both, on auto ---------------------------------------
	ann.want(http.StatusNoContent, "POST", lineupURL+"/auto", map[string]string{"day": day(0)}, nil)
	now := lineup()
	if g, f := now["Zqauto Guard"], now["Zqauto Forward"]; g.Slot != "G" || f.Slot != "UTIL" || !g.CountsLast || !f.CountsLast {
		t.Fatalf("after auto set: guard %+v, forward %+v; want both started on auto", g, f)
	}
	// Yesterday's game came before he started: from his next game only.
	if g := now["Zqauto Guard"]; g.CountsFrom != day(2) || g.Locked {
		t.Fatalf("guard = %+v, want counting from %s and free", g, day(2))
	}
	if got := total(); got != 0 {
		t.Fatalf("total = %v, want 0: yesterday's 30 came before he was started", got)
	}

	// --- the last game with a stat counts, once he has none left to play; ---
	// --- one he sat out gives way -------------------------------------------
	// Each game is played out today, in order, so the standings count it.
	finish := func(gameID string, hoursAgo int) {
		sql(`update games set status = 'final', day = $2, starts_at = now() - make_interval(hours => $3) where provider_id = $1`, gameID, today, hoursAgo)
	}
	finish("za-y1", 6)
	line("za-y1", "Zqauto Forward", `{"pts": 12}`)
	if got, p := total(), lineup()["Zqauto Forward"]; !p.Locked || p.Points != 0 || got != 0 {
		t.Fatalf("after his first game: %+v, total %v; want locked with nothing counted while games are to come", p, got)
	}
	// He has played, so he stays until the week is over.
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/leagues/"+league+"/roster/drop", map[string]any{"player_id": id["Zqauto Forward"]}, nil)
	finish("za-y2", 4)
	line("za-y2", "Zqauto Forward", `{"pts": 0, "reb": 0}`) // sat out
	if got := total(); got != 0 {
		t.Fatalf("total = %v after a game he sat out, want still 0 with a game to come", got)
	}
	finish("za-y3", 2)
	line("za-y3", "Zqauto Forward", `{"pts": 5, "reb": 2}`)
	if got, p := total(), lineup()["Zqauto Forward"]; math.Abs(got-7) > 0.001 || p.Points != 7 {
		t.Fatalf("total = %v, forward %+v; want 7 from his last game, in place of the 12", got, p)
	}
	// A locked player cannot be moved, by hand or by auto set.
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, map[string]any{"day": day(0), "entries": []map[string]any{
		{"slot": "G", "player_id": id["Zqauto Guard"], "auto": true},
	}}, nil)
	ann.want(http.StatusNoContent, "POST", lineupURL+"/auto", map[string]string{"day": day(0)}, nil)
	if p := lineup()["Zqauto Forward"]; p.Slot != "UTIL" {
		t.Fatalf("auto set moved a locked player: %+v", p)
	}

	// --- a doubled matchup counts his last two games with a stat -------------
	sql(`insert into periods (season_id, seq, starts_on, ends_on, is_playoff, double_games)
	     select id, 1, $2, $3, false, true from seasons where league_id = $1`, league, day(-2), day(11))
	var doubled struct {
		First   string `json:"day"`
		Last    string `json:"last_day"`
		Doubled *struct {
			StartsOn string `json:"starts_on"`
		}
	}
	ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise+"&day="+day(0), nil, &doubled)
	if doubled.Doubled == nil || doubled.First != day(-2) || doubled.Last != day(11) {
		t.Fatalf("lineup = %+v; want one lineup for the doubled matchup, %s to %s", doubled, day(-2), day(11))
	}
	if got, p := total(), lineup()["Zqauto Forward"]; math.Abs(got-19) > 0.001 || p.Points != 19 {
		t.Fatalf("total = %v, forward %+v in a doubled matchup; want 19, his last two games with a stat", got, p)
	}
}
