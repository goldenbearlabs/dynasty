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

// TestPitcherStartsCap covers the weekly limit on a team's pitcher starts:
// what counts as a start, and that one beyond the limit scores nothing.
func TestPitcherStartsCap(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "starts_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zp-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'zp-%'`)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	today := sportsday.Today()
	mlb, _ := registry.Get("mlb")
	rules := mlb.Defaults
	rules.Lineup.WeekStart = strings.ToLower(today.AddDate(0, 0, -3).Weekday().String()) // so the days before today are in this week
	rules.Lineup.Slots = []settings.Slot{
		{Name: "SP", Positions: []string{"SP"}, Count: 3},
		{Name: "RP", Positions: []string{"RP"}, Count: 1},
	}
	rules.Lineup.PitcherStartsPerWeek = 2
	rules.Scoring = map[string]float64{"pit_so": 1}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Starts Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "mlb", Settings: rules}},
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
		{Competition: "mlb", FullName: "Zp Ace", Positions: []string{"SP"}, Status: "active", Note: marker},
		{Competition: "mlb", FullName: "Zp Second", Positions: []string{"SP"}, Status: "active", Note: marker},
		{Competition: "mlb", FullName: "Zp Swingman", Positions: []string{"SP"}, Status: "active", Note: marker},
		{Competition: "mlb", FullName: "Zp Opener", Positions: []string{"RP"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zp+", nil, &page)
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
	sql(`insert into pro_teams (competition, provider_id, abbrev, name) values ('mlb', 'zp-x', 'ZPX', 'X')`)
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zp-x') where note = $1`, marker)
	// Four games, on each of the last three days and one tomorrow.
	for offset, provider := range map[int]string{-3: "zp-1", -2: "zp-2", -1: "zp-3", 1: "zp-4"} {
		day := today.AddDate(0, 0, offset)
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
		     values ('mlb', $1, $2, $3, 'final', (select id from pro_teams where provider_id = 'zp-x'))`, provider, day, sportsday.Start(day).Add(14*time.Hour))
	}
	line := func(game, player, stats string) {
		sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, $3::jsonb)`, game, id[player], stats)
	}
	line("zp-1", "Zp Ace", `{"pit_gs": 1, "pit_ip": 6, "pit_so": 5}`)      // start 1
	line("zp-1", "Zp Swingman", `{"pit_gs": 0, "pit_ip": 2, "pit_so": 2}`) // relief: not a start
	line("zp-2", "Zp Opener", `{"pit_gs": 1, "pit_ip": 1, "pit_so": 1}`)   // a reliever opening for an inning: not a start
	line("zp-2", "Zp Second", `{"pit_gs": 0, "pit_ip": 5, "pit_so": 4}`)   // the bulk man behind him: not a start either
	line("zp-3", "Zp Opener", `{"pit_gs": 1, "pit_ip": 2.3, "pit_so": 3}`) // a reliever starting and going on: start 2

	// A lineup in force since before the first game.
	lineupURL := "/api/leagues/" + league + "/lineup"
	ann.want(http.StatusNoContent, "PUT", lineupURL, map[string]any{"day": today.Format(time.DateOnly), "entries": []map[string]string{
		{"slot": "SP", "player_id": id["Zp Ace"]}, {"slot": "SP", "player_id": id["Zp Second"]}, {"slot": "SP", "player_id": id["Zp Swingman"]},
		{"slot": "RP", "player_id": id["Zp Opener"]},
	}}, nil)
	earlier := today.AddDate(0, 0, -5)
	sql(`insert into lineups (league_id, franchise_id, effective_on) select league_id, franchise_id, $1 from lineups where franchise_id = $2`, earlier, franchise)
	sql(`update lineup_entries set effective_on = $1 where franchise_id = $2`, earlier, franchise)
	sql(`delete from lineups where franchise_id = $1 and effective_on > $2`, franchise, earlier)
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(), "starts_on": today.AddDate(0, 0, -10).Format(time.DateOnly), "ends_on": today.AddDate(0, 0, 30).Format(time.DateOnly),
	}, nil)

	type starts struct {
		Limit int
		Made  []struct {
			FullName string `json:"full_name"`
			Innings  float64
			Counts   bool
		}
	}
	seen := func() starts {
		t.Helper()
		var v struct{ Starts *starts }
		ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise, nil, &v)
		if v.Starts == nil {
			t.Fatal("the lineup does not report the week's starts")
		}
		return *v.Starts
	}
	total := func() float64 {
		t.Helper()
		var all []struct{ Rows []struct{ Points float64 } }
		ann.want(http.StatusOK, "GET", "/api/standings", nil, &all)
		return all[0].Rows[0].Points
	}

	// Two starts so far: the ace's, and the reliever who went past an inning.
	if s := seen(); s.Limit != 2 || len(s.Made) != 2 || s.Made[0].FullName != "Zp Ace" || s.Made[1].FullName != "Zp Opener" || !s.Made[1].Counts {
		t.Fatalf("starts = %+v; want the ace's and the reliever's longer one, both counting", s)
	}
	if got := total(); math.Abs(got-15) > 0.001 { // 5 + 2 + 1 + 4 + 3: everything pitched so far
		t.Fatalf("total = %v, want 15", got)
	}

	// Tomorrow's start is the third of the week: it is listed, and scores nothing.
	sql(`update games set day = $1, starts_at = now() - interval '1 minute' where provider_id = 'zp-4'`, today)
	line("zp-4", "Zp Swingman", `{"pit_gs": 1, "pit_ip": 5, "pit_so": 7}`)
	if s := seen(); len(s.Made) != 3 || s.Made[2].FullName != "Zp Swingman" || s.Made[2].Counts {
		t.Fatalf("starts = %+v; want a third that does not count", s)
	}
	if got := total(); math.Abs(got-15) > 0.001 {
		t.Fatalf("total = %v after a start over the limit, want still 15", got)
	}

	// Without a limit it counts like any other game.
	var current struct {
		Leagues []struct{ Settings map[string]any }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &current)
	free := current.Leagues[0].Settings
	free["lineup"].(map[string]any)["pitcher_starts_per_week"] = 0
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league+"/settings", free, nil)
	if got := total(); math.Abs(got-22) > 0.001 {
		t.Fatalf("total = %v with no limit, want 22", got)
	}
}
