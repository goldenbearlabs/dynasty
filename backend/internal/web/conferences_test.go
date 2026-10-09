package web_test

import (
	"context"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/lineup"
	"crossover/internal/players"
	"crossover/internal/sportsday"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestCollegeConferenceEligibility(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	cleanup := func() {
		dbtest.Reset(t, pool)
		exec(`delete from games where provider_id='conference-test'`)
		exec(`delete from players where note='conference-test'`)
		exec(`delete from pro_teams where provider_id like 'conference-test-%'`)
	}
	cleanup()
	t.Cleanup(cleanup)
	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	c, _ := registry.Get("cbb")
	rules := c.Defaults
	rules.Continuity = nil
	rules.FreeAgency.NewEntrantsDraftOnly = false
	if len(rules.Lineup.Conferences) != 8 {
		t.Fatal("missing default conferences")
	}
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{Name: "Conference test", Leagues: []dynasty.LeagueSetup{{Competition: "cbb", Settings: rules}}, Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}}, Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"}}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, franchise := view.Leagues[0].ID, view.Franchises[0].ID
	ids := map[string]string{}
	for _, x := range []struct{ name, conference string }{{"ACC", "2"}, {"Ivy", "12"}} {
		exec(`insert into pro_teams (competition,provider_id,abbrev,name,conference) values ('cbb',$1,$2,$2,$3)`, "conference-test-"+x.name, x.name, x.conference)
		var id string
		if err := pool.QueryRow(ctx, `insert into players (competition,status,full_name,positions,note,pro_team_id) select 'cbb','active',$1,'{G}','conference-test',id from pro_teams where provider_id=$2 returning id`, x.name, "conference-test-"+x.name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[x.name] = id
		exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats,conference) values ($1,'cbb',9999,'conference-test',$2,10,'{"pts":200}',$3)`, id, x.name, x.conference)
	}
	// Conference history follows the season, including a player now outside the starter pool.
	exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats,conference) values ($1,'cbb',9998,'conference-old','Old',10,'{"pts":100}','2')`, ids["Ivy"])
	research := func(season string, explicit bool) players.AnalyticsPage {
		t.Helper()
		path := "/api/research?competition=cbb&season=" + season
		if explicit {
			raw, _ := json.Marshal([]players.ResearchPool{{Competition: "cbb", Season: season}})
			path = "/api/research?pools=" + url.QueryEscape(string(raw))
		}
		var result players.AnalyticsPage
		ann.want(http.StatusOK, "GET", path, nil, &result)
		return result
	}
	for _, explicit := range []bool{false, true} {
		r := research("conference-test", explicit)
		if r.Total != 1 || r.Players[0].FullName != "ACC" || len(r.Analysis) != 1 || r.Analysis[0].Players != 1 {
			t.Fatalf("research eligibility: %+v", r)
		}
	}
	if r := research("conference-old", true); r.Total != 1 || r.Players[0].FullName != "Ivy" {
		t.Fatal("historical membership used today's conference")
	}
	// The smaller-conference player stays visible in the draft pool and may land on reserve.
	var drafted struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/drafts", map[string]any{"name": "Rookie", "kind": "seasonal", "year": 2026, "league_ids": []string{league}, "rounds": 1, "order": "linear", "franchise_order": []string{view.Franchises[0].ID, view.Franchises[1].ID}, "pick_clock_seconds": 0}, &drafted)
	var draftPool struct{ Players []struct{ ID string } }
	ann.want(http.StatusOK, "GET", "/api/players?draft_id="+drafted.ID+"&q=Ivy", nil, &draftPool)
	if len(draftPool.Players) != 1 || draftPool.Players[0].ID != ids["Ivy"] {
		t.Fatal("smaller conference missing from rookie pool")
	}
	ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+drafted.ID+"/start", nil, nil)
	ann.want(http.StatusNoContent, "POST", "/api/drafts/"+drafted.ID+"/pick", map[string]string{"player_id": ids["Ivy"], "list": "reserve"}, nil)
	// Add to main through commissioner override to exercise server-side starter enforcement.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/move", map[string]any{"player_id": ids["Ivy"], "list": "main", "force": true}, nil)
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": ids["ACC"], "list": "main", "force": true}, nil)
	tomorrow := sportsday.Today().AddDate(0, 0, 7)
	lineupURL := "/api/leagues/" + league + "/lineup"
	put := func(player string) map[string]any {
		return map[string]any{"day": tomorrow.Format(time.DateOnly), "force": true, "entries": []map[string]string{{"slot": "G", "player_id": ids[player]}}}
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, put("Ivy"), nil)
	ann.want(http.StatusNoContent, "PUT", lineupURL, put("ACC"), nil)
	var lineupView lineup.View
	ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise+"&day="+tomorrow.Format(time.DateOnly), nil, &lineupView)
	for _, p := range lineupView.Players {
		if p.FullName == "Ivy" && (p.StarterEligible || p.EligibilityNote == "") {
			t.Fatal("missing eligibility explanation")
		}
	}
	// Changing the rules is commissioner-only and updates research and lineups immediately.
	rules.Lineup.Conferences = append(rules.Lineup.Conferences, "12")
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league+"/settings", rules, nil)
	if r := research("conference-test", true); r.Total != 2 {
		t.Fatal("research ignored commissioner rule")
	}
	ann.want(http.StatusNoContent, "PUT", lineupURL, put("Ivy"), nil)
	// Existing lineups cannot score through a conference restriction introduced later.
	day := sportsday.Today()
	exec(`insert into games (competition,provider_id,day,starts_at,status,home_team_id) select 'cbb','conference-test',$1,$2,'final',id from pro_teams where provider_id='conference-test-Ivy'`, day, time.Now().Add(-time.Hour))
	exec(`insert into stat_lines (game_id,player_id,stats) select id,$1,'{"pts":10}' from games where provider_id='conference-test'`, ids["Ivy"])
	exec(`insert into lineups (league_id,franchise_id,effective_on) values ($1,$2,$3)`, league, franchise, day)
	exec(`insert into lineup_entries (league_id,franchise_id,effective_on,slot,slot_index,player_id) values ($1,$2,$3,'G',0,$4)`, league, franchise, day, ids["Ivy"])
	q := db.New(pool)
	var leagueID db.League
	if err := pool.QueryRow(ctx, `select id from leagues where id=$1`, league).Scan(&leagueID.ID); err != nil {
		t.Fatal(err)
	}
	points := func() []db.ListLineupPointsRow {
		r, err := q.ListLineupPoints(ctx, db.ListLineupPointsParams{LeagueID: leagueID.ID, FromDay: sportsday.Date(day), ToDay: sportsday.Date(day), WeekStart: sportsday.Date(sportsday.WeekStart(day, "monday"))})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if len(points()) != 1 {
		t.Fatal("allowed conference did not score")
	}
	rules.Lineup.Conferences = c.Defaults.Lineup.Conferences
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league+"/settings", rules, nil)
	if len(points()) != 0 {
		t.Fatal("ineligible existing starter still scored")
	}
	rules.Lineup.Conferences = []string{}
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+league+"/settings", rules, nil)
	if r := research("conference-test", true); r.Total != 2 {
		t.Fatal("all-conference setting ignored")
	}
	rules.Lineup.Conferences = []string{"unknown"}
	ann.want(http.StatusUnprocessableEntity, "PUT", "/api/leagues/"+league+"/settings", rules, nil)
}
