package web_test

import (
	"context"
	"crossover/internal/dbtest"
	"crossover/internal/players"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"strings"
	"testing"
)

func TestPlayerProfileCareerAndHistory(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "profile_api_fixture"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	uuid := func(sql string, args ...any) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	t.Cleanup(func() {
		dbtest.Reset(t, pool)
		exec(`delete from games where provider_id like 'profile-test-%'`)
		exec(`delete from players where note=$1`, marker)
	})
	dynasty := uuid(`insert into dynasties(name,settings) values('Profiles','{}') returning id`)
	nba := uuid(`insert into leagues(dynasty_id,competition,name,settings) values($1,'nba','NBA','{"scoring":{"pts":2},"roster":{"reserve_lock_days":7},"lineup":{"slots":[{"name":"G","positions":["G"],"count":1}]}}') returning id`, dynasty)
	exec(`insert into leagues(dynasty_id,competition,name,settings) values($1,'cbb','CBB','{"scoring":{"pts":1},"lineup":{"conferences":["2"],"slots":[{"name":"G","positions":["G"],"count":1}]}}')`, dynasty)
	owner := uuid(`insert into franchises(dynasty_id,name,manager_name,slug,invite_token) values($1,'Owner','Manager','profile-owner','profile-token') returning id`, dynasty)
	var target pgtype.UUID
	for i := 0; i < 4; i++ {
		p := uuid(`insert into players(competition,status,full_name,positions,note) values('nba','active','Same Name','{G}',$1) returning id`, marker)
		exec(`insert into player_seasons(player_id,competition,year,label,team,games,stats) values($1,'nba',9999,'profile-season','NBA',10,jsonb_build_object('pts',$2::int,'reb',3))`, p, (i+1)*100)
		if i == 3 {
			target = p
		}
	}
	exec(`insert into player_seasons(player_id,competition,year,label,team,conference,games,stats) values($1,'cbb',9998,'profile-college','COL','2',10,'{"pts":100}')`, target)
	exec(`insert into player_seasons(player_id,competition,year,label,team,conference,games,stats) values($1,'cbb',9997,'profile-small','SMALL','12',10,'{"pts":200}')`, target)
	exec(`insert into roster_entries(league_id,franchise_id,player_id,list,acquired_via,reserved_at) values($1,$2,$3,'reserve','draft',now())`, nba, owner, target)
	exec(`insert into waivers(league_id,player_id,clears_at) values($1,$2,now()+interval '1 day')`, nba, target)
	exec(`insert into waiver_claims(league_id,franchise_id,player_id,bid,reason) values($1,$2,$3,87,'private-claim-secret')`, nba, owner, target)
	exec(`insert into transactions(dynasty_id,league_id,franchise_id,player_id,kind,detail) select $1,$2,$3,$4,'move','{"from":"main","to":"reserve"}' from generate_series(1,31)`, dynasty, nba, owner, target)
	exec(`insert into games(competition,provider_id,day,starts_at,status) select 'nba','profile-test-'||n,current_date-n,now()-n*interval '1 day',case when n=32 then 'live' else 'final' end from generate_series(1,32) n`)
	exec(`insert into stat_lines(game_id,player_id,stats) select id,$1,'{"pts":15,"reb":2}' from games where provider_id like 'profile-test-%'`, target)
	draft := uuid(`insert into drafts(dynasty_id,name,kind,year,status) values($1,'Profile Draft','startup',9999,'complete') returning id`, dynasty)
	exec(`insert into draft_picks(draft_id,round,position,original_franchise_id,current_franchise_id,player_id,league_id,picked_at) values($1,2,4,$2,$2,$3,$4,now())`, draft, owner, target, nba)
	for _, status := range []string{"proposed", "executed", "reversed", "rejected"} {
		trade := uuid(`insert into trades(dynasty_id,proposed_by,status,note) values($1,$2,$3,$3) returning id`, dynasty, owner, status)
		exec(`insert into trade_items(trade_id,from_franchise,to_franchise,league_id,player_id) values($1,$2,$2,$3,$4)`, trade, owner, nba, target)
	}
	exec(`insert into seasons(league_id,year,starts_on,ends_on) values($1,2026,current_date-30,current_date+1)`, nba)
	exec(`insert into lineups(league_id,franchise_id,effective_on) values($1,$2,current_date-30)`, nba, owner)
	exec(`insert into lineup_entries(league_id,franchise_id,effective_on,slot,slot_index,player_id,counts_from) values($1,$2,current_date-30,'G',0,$3,current_date-3)`, nba, owner, target)
	server, _ := startServer(t, pool)
	browser := newBrowser(t, server.URL)
	path := "/api/players/" + target.String()
	var profile players.Profile
	browser.want(http.StatusOK, "GET", path+"/profile", nil, &profile)
	if profile.Player.ID != target || len(profile.Seasons) != 3 || profile.Seasons[0].Points != 800 || profile.Seasons[1].Points != 100 || profile.Seasons[2].Points != 200 {
		t.Fatalf("career competitions must use their own scoring rules: %+v", profile.Seasons)
	}
	if len(profile.FantasyProduction) != 1 || profile.FantasyProduction[0].Games != 3 || profile.FantasyProduction[0].Points != 90 {
		t.Fatalf("counted fantasy scoring must honor historical lineup activation: %+v", profile.FantasyProduction)
	}
	top := profile.Seasons[0].Research
	if top == nil || top.ID != target || top.LeagueIndex == nil || *top.Percentile != 100 || *top.PointsAboveReplacement != 200 {
		t.Fatalf("identity filter must preserve full same-name peer benchmarks: %+v", top)
	}
	if profile.Seasons[2].Research != nil || !strings.Contains(profile.Seasons[2].ResearchNote, "conference") {
		t.Fatal("excluded college season must keep raw stats without misleading research")
	}
	if len(profile.Ownership) != 1 || profile.Ownership[0].List != "reserve" || profile.Ownership[0].Slot != "" || len(profile.ReserveLockedUntil) != 1 || len(profile.Drafts) != 1 || len(profile.Waivers) != 1 || len(profile.Trades) != 2 {
		t.Fatal("fantasy context missing")
	}
	if profile.Games.Total != 31 || len(profile.Games.Games) != 25 || profile.Games.Games[0].Points != 30 || profile.Timeline.Total != 31 || len(profile.Timeline.Events) != 25 {
		t.Fatal("final-game scoring or initial pagination incorrect")
	}
	var log players.GameLogPage
	browser.want(http.StatusOK, "GET", path+"/games?page=2", nil, &log)
	if len(log.Games) != 6 || log.Games[0].ID == profile.Games.Games[0].ID {
		t.Fatal("game pagination duplicates or loses rows")
	}
	browser.want(http.StatusOK, "GET", path+"/games?competition=cbb", nil, &log)
	if log.Total != 0 || log.Games == nil {
		t.Fatal("cross-career sport filtering failed")
	}
	var history players.TimelinePage
	browser.want(http.StatusOK, "GET", path+"/transactions?page=2", nil, &history)
	if len(history.Events) != 6 || history.Events[0].ID >= profile.Timeline.Events[24].ID {
		t.Fatal("transaction pagination incorrect")
	}
	for _, trade := range profile.Trades {
		if trade.Status != "executed" && trade.Status != "reversed" {
			t.Fatal("private trade proposal leaked")
		}
	}
	raw, _ := json.Marshal(profile)
	if strings.Contains(string(raw), "private-claim-secret") || strings.Contains(string(raw), "\"bid\":87") {
		t.Fatal("public profile leaked a pending waiver claim")
	}
	browser.want(http.StatusUnprocessableEntity, "GET", path+"/games?page=0", nil, nil)
	browser.want(http.StatusUnprocessableEntity, "GET", path+"/transactions?page=100001", nil, nil)
	browser.want(http.StatusNotFound, "GET", "/api/players/00000000-0000-0000-0000-000000000001/profile", nil, nil)
}
