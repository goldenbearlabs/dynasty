package web_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/sportsday"
)

// TestScheduleFollowsTheLeague checks that nobody has to ask for a schedule:
// starting a season builds it, and it is rebuilt when the season's dates,
// the franchises or the format change.
func TestScheduleFollowsTheLeague(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	t.Cleanup(func() { dbtest.Reset(t, pool) })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Schedule Test", Settings: dynastySettings(10),
		Leagues: []dynasty.LeagueSetup{{Competition: "nhl", Settings: nhl.Defaults}}, // head to head by default
		Franchises: []dynasty.FranchiseSetup{
			{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}, {Name: "Cy", ManagerName: "Cy"}, {Name: "Di", ManagerName: "Di"},
		},
		Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues []struct {
			ID       string
			Settings map[string]any
		}
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := view.Leagues[0]

	count := func(what string) int {
		t.Helper()
		var n int
		query := map[string]string{
			"periods":  `select count(*) from periods p join seasons s on s.id = p.season_id where s.league_id = $1`,
			"matchups": `select count(*) from matchups m join periods p on p.id = m.period_id join seasons s on s.id = p.season_id where s.league_id = $1`,
			"playoffs": `select count(*) from periods p join seasons s on s.id = p.season_id where s.league_id = $1 and p.is_playoff`,
		}[what]
		if err := pool.QueryRow(ctx, query, league.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// A season starting next week and running ten weeks.
	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	var season struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league.ID, "year": today.Year(), "starts_on": day(7), "ends_on": day(7 + 69),
	}, &season)
	if p, m, k := count("periods"), count("matchups"), count("playoffs"); p != 10 || k != 2 || m != 8*2 {
		t.Fatalf("starting the season built %d periods (%d playoff) and %d matchups; want 10, 2 and 16", p, k, m)
	}

	// Four weeks longer: four more regular-season periods.
	ann.want(http.StatusNoContent, "PUT", "/api/admin/seasons/"+season.ID, map[string]any{"starts_on": day(7), "ends_on": day(7 + 97)}, nil)
	if p, m := count("periods"), count("matchups"); p != 14 || m != 12*2 {
		t.Fatalf("after lengthening the season: %d periods and %d matchups; want 14 and 24", p, m)
	}

	// A fifth franchise: three periods of playoffs for five teams, and one team sits out each week.
	ann.want(http.StatusCreated, "POST", "/api/admin/franchises", map[string]string{"name": "Ed", "manager_name": "Ed"}, nil)
	if k, m := count("playoffs"), count("matchups"); k != 3 || m != 11*3 {
		t.Fatalf("after adding a franchise: %d playoff periods and %d matchups; want 3 and 33", k, m)
	}

	// Switching to total points takes the matchups away again.
	league.Settings["format"].(map[string]any)["type"] = "total_points"
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league.ID+"/settings", league.Settings, nil)
	if p := count("periods"); p != 0 {
		t.Fatalf("%d periods left in a total-points league, want 0", p)
	}
	league.Settings["format"].(map[string]any)["type"] = "head_to_head"
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league.ID+"/settings", league.Settings, nil)
	if p := count("periods"); p != 14 {
		t.Fatalf("%d periods after switching back to head to head, want 14", p)
	}
}
