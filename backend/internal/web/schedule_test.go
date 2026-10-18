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

// TestCommissionerSetsMatchups checks that the commissioner can replace a
// period's matchups, that a rebuilt schedule keeps them, and that they can
// be handed back to the schedule.
func TestCommissionerSetsMatchups(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Override Test", Settings: dynastySettings(10),
		Leagues: []dynasty.LeagueSetup{{Competition: "nhl", Settings: nhl.Defaults}},
		Franchises: []dynasty.FranchiseSetup{
			{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}, {Name: "Cy", ManagerName: "Cy"}, {Name: "Di", ManagerName: "Di"},
		},
		Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct{ Leagues []struct{ ID string } }
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := view.Leagues[0].ID

	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	var season struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(), "starts_on": day(7), "ends_on": day(7 + 69),
	}, &season)

	type pairing struct {
		Home string  `json:"home_franchise_id"`
		Away *string `json:"away_franchise_id"`
	}
	type period struct {
		Period struct {
			ID     string `json:"id"`
			ByHand bool   `json:"by_hand"`
		} `json:"period"`
		Matchups []pairing `json:"matchups"`
	}
	second := func() period {
		t.Helper()
		var p period
		ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/matchups?period=2", nil, &p)
		return p
	}
	sides := func(p period) [2][2]string {
		t.Helper()
		if len(p.Matchups) != 2 {
			t.Fatalf("period 2 has %d matchups, want 2", len(p.Matchups))
		}
		return [2][2]string{{p.Matchups[0].Home, *p.Matchups[0].Away}, {p.Matchups[1].Home, *p.Matchups[1].Away}}
	}

	// The season's schedule lists every period: eight weeks of two matchups, then the playoffs still to be set.
	var schedule []struct {
		Seq       int       `json:"seq"`
		IsPlayoff bool      `json:"is_playoff"`
		Matchups  []pairing `json:"matchups"`
	}
	ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &schedule)
	if len(schedule) != 10 || len(schedule[0].Matchups) != 2 || !schedule[9].IsPlayoff || len(schedule[9].Matchups) != 0 {
		t.Fatalf("schedule: %+v, want 10 periods, the first with 2 matchups and the last an unset playoff round", schedule)
	}

	built := second()
	was := sides(built)
	path := "/api/admin/periods/" + built.Period.ID + "/matchups"
	// The two home sides play each other, and so do the two away sides.
	mine := [2][2]string{{was[0][0], was[1][0]}, {was[0][1], was[1][1]}}
	body := func(pairs [2][2]string) map[string]any {
		return map[string]any{"matchups": []pairing{{pairs[0][0], &pairs[0][1]}, {pairs[1][0], &pairs[1][1]}}}
	}
	same := func(a, b [2][2]string) bool { return a == b || a == [2][2]string{b[1], b[0]} }

	ann.want(http.StatusUnprocessableEntity, "PUT", path, body([2][2]string{{was[0][0], was[0][1]}, {was[0][0], was[1][1]}}), nil)
	ann.want(http.StatusNoContent, "PUT", path, body(mine), nil)
	if p := second(); !p.Period.ByHand || !same(sides(p), mine) {
		t.Fatalf("after setting them by hand: %+v, want %v", p, mine)
	}

	// Lengthening the season rebuilds the schedule around them.
	ann.want(http.StatusNoContent, "PUT", "/api/admin/seasons/"+season.ID, map[string]any{"starts_on": day(7), "ends_on": day(7 + 97)}, nil)
	rebuilt := second()
	if !rebuilt.Period.ByHand || !same(sides(rebuilt), mine) {
		t.Fatalf("after a rebuild: %+v, want %v kept", rebuilt, mine)
	}

	ann.want(http.StatusNoContent, "DELETE", "/api/admin/periods/"+rebuilt.Period.ID+"/matchups", nil, nil)
	if p := second(); p.Period.ByHand || !same(sides(p), was) {
		t.Fatalf("after handing them back: %+v, want %v", p, was)
	}
	// A custom schedule has individually chosen lengths and playoff dates.
	customPath := "/api/admin/seasons/" + season.ID + "/schedule"
	custom := []map[string]any{
		{"starts_on": day(7), "ends_on": day(9), "is_playoff": false},
		{"starts_on": day(10), "ends_on": day(17), "is_playoff": false, "matchups": body(mine)["matchups"]},
		{"starts_on": day(18), "ends_on": day(22), "is_playoff": true, "matchups": body(mine)["matchups"]},
		{"starts_on": day(23), "ends_on": day(30), "is_playoff": true},
	}
	visitor := newBrowser(t, server.URL)
	visitor.want(http.StatusForbidden, "PUT", customPath, map[string]any{"periods": custom}, nil)
	ann.want(http.StatusNoContent, "PUT", customPath, map[string]any{"periods": custom}, nil)
	var saved []struct {
		ID        string
		StartsOn  string    `json:"starts_on"`
		EndsOn    string    `json:"ends_on"`
		IsPlayoff bool      `json:"is_playoff"`
		ByHand    bool      `json:"by_hand"`
		Matchups  []pairing `json:"matchups"`
	}
	readCustom := func() {
		t.Helper()
		ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &saved)
		if len(saved) != 4 || saved[0].StartsOn != day(7) || saved[0].EndsOn != day(9) || saved[2].StartsOn != day(18) || !saved[2].IsPlayoff || !saved[1].ByHand || len(saved[2].Matchups) != 2 || len(saved[3].Matchups) != 0 {
			t.Fatalf("custom schedule = %+v", saved)
		}
	}
	readCustom()
	originalID := saved[0].ID
	// Date changes preserve the commissioner's schedule.
	ann.want(http.StatusNoContent, "PUT", "/api/admin/seasons/"+season.ID, map[string]any{"starts_on": day(7), "ends_on": day(7 + 104)}, nil)
	readCustom()
	if saved[0].ID != originalID {
		t.Fatal("reschedule replaced custom periods")
	}
	// Reject overlapping periods and duplicate opponents without losing any saved periods.
	custom[1]["starts_on"] = day(9)
	ann.want(http.StatusUnprocessableEntity, "PUT", customPath, map[string]any{"periods": custom}, nil)
	custom[1]["starts_on"] = day(10)
	custom[1]["matchups"] = body([2][2]string{{was[0][0], was[0][1]}, {was[0][0], was[1][1]}})["matchups"]
	ann.want(http.StatusUnprocessableEntity, "PUT", customPath, map[string]any{"periods": custom}, nil)
	readCustom()
	if saved[0].ID != originalID {
		t.Fatal("failed save did not roll back")
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", "/api/admin/seasons/"+season.ID, map[string]any{"starts_on": day(7), "ends_on": day(25)}, nil)
	// An explicit rebuild restores automatic dates.
	ann.want(http.StatusNoContent, "POST", customPath, nil, nil)
	ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &saved)
	if len(saved) <= 4 {
		t.Fatalf("automatic schedule was not restored: %+v", saved)
	}

	// A period in progress keeps its identity and scores when future dates change.
	ctx := context.Background()
	startedID := saved[0].ID
	if _, err := pool.Exec(ctx, `update seasons set starts_on=$2 where id=$1`, season.ID, day(-1)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `update periods set starts_on=$2, ends_on=$3 where id=$1`, startedID, day(-1), day(1)); err != nil {
		t.Fatal(err)
	}
	custom[1]["matchups"] = body(mine)["matchups"]
	ann.want(http.StatusNoContent, "PUT", customPath, map[string]any{"periods": custom}, nil)
	ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &saved)
	if len(saved) != 5 || saved[0].ID != startedID || saved[0].StartsOn != day(-1) || saved[0].EndsOn != day(1) {
		t.Fatalf("started period changed: %+v", saved)
	}
	// Once a playoff round starts, changing dates is refused.
	if _, err := pool.Exec(ctx, `update periods set is_playoff=true where id=$1`, startedID); err != nil {
		t.Fatal(err)
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", customPath, map[string]any{"periods": custom}, nil)

	// Explicitly moving the season into the future clears even started periods.
	seasonPath := "/api/admin/seasons/" + season.ID
	reset := map[string]any{"starts_on": day(0), "ends_on": day(111), "reset_schedule": true}
	ann.want(http.StatusUnprocessableEntity, "PUT", seasonPath, reset, nil)
	monday := today.AddDate(0, 0, 1)
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, 1)
	}
	reset["starts_on"] = monday.Format(time.DateOnly)
	visitor.want(http.StatusForbidden, "PUT", seasonPath, reset, nil)
	// An impossibly short season must roll back deletion of the old schedule.
	reset["ends_on"] = monday.Format(time.DateOnly)
	ann.want(http.StatusUnprocessableEntity, "PUT", seasonPath, reset, nil)
	ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &saved)
	if saved[0].ID != startedID {
		t.Fatal("failed restart deleted the schedule")
	}
	reset["ends_on"] = day(111)
	ann.want(http.StatusNoContent, "PUT", seasonPath, reset, nil)
	ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &saved)
	if len(saved) < 4 || saved[0].ID == startedID || saved[0].StartsOn != monday.Format(time.DateOnly) {
		t.Fatalf("restart schedule = %+v", saved)
	}
	for i, p := range saved {
		start, _ := time.Parse(time.DateOnly, p.StartsOn)
		end, _ := time.Parse(time.DateOnly, p.EndsOn)
		if start.Weekday() != time.Monday || (i < len(saved)-1 && end.Weekday() != time.Sunday) {
			t.Fatalf("NHL period not Monday–Sunday: %+v", p)
		}
	}
	var isCustom bool
	if err := pool.QueryRow(ctx, `select exists(select 1 from custom_schedules where season_id=$1)`, season.ID).Scan(&isCustom); err != nil {
		t.Fatal(err)
	}
	if isCustom {
		t.Fatal("restart retained the custom schedule flag")
	}

}
