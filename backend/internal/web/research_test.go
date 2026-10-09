package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/players"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestResearchAnalyticsAPI(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "analytics_api_test"
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note=$1`, marker) })
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into dynasties (name,settings) values ('Analytics','{}')`)
	exec(`insert into leagues (dynasty_id,competition,name,settings) select id,'nba','NBA','{"scoring":{"pts":1},"lineup":{"slots":[{"name":"G","positions":["G"],"count":1}]}}' from dynasties`)
	exec(`insert into franchises (dynasty_id,name,manager_name,slug,invite_token) select id,'Test','Test','test','analytics-token' from dynasties`)
	for i, rate := range []int{10, 20, 30, 40} {
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `insert into players (competition,status,full_name,positions,note) values ('nba','active',$1,'{G}',$2) returning id`, "Za Analytics "+string(rune('A'+i)), marker).Scan(&id); err != nil {
			t.Fatal(err)
		}
		exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) values ($1,'nba',9999,'analytics-test','AAA',10,jsonb_build_object('pts',$2::int))`, id, rate*10)
		exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) values ($1,'nba',9998,'analytics-previous','BBB',10,jsonb_build_object('pts',$2::int))`, id, rate*100)
		if i == 3 {
			exec(`insert into roster_entries (league_id,franchise_id,player_id,list,acquired_via) select l.id,f.id,$1,'main','manual' from leagues l cross join franchises f`, id)
		}
	}
	for i, rate := range []int{10, 20, 30, 40} {
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `insert into players (competition,status,full_name,positions,note) values ('wnba','active',$1,'{G}',$2) returning id`, "Za Analytics W"+string(rune('A'+i)), marker).Scan(&id); err != nil {
			t.Fatal(err)
		}
		exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) values ($1,'wnba',9999,'analytics-women','WNA',10,jsonb_build_object('pts',$2::int))`, id, rate*10)
	}
	server, _ := startServer(t, pool)
	b := newBrowser(t, server.URL)
	base := "/api/research?competition=nba&season=analytics-test&q=Za+Analytics&sort=league_index"
	var all, filtered players.AnalyticsPage
	b.want(http.StatusOK, "GET", base, nil, &all)
	if all.Total != 4 || all.Players[0].LeagueIndex == nil || *all.Players[0].PointsAboveReplacement != 100 {
		t.Fatalf("analytics API: %+v", all)
	}
	b.want(http.StatusOK, "GET", base+"&position=G&owner=test&min_games=10&min_index=115&above_replacement=true", nil, &filtered)
	if filtered.Total != 1 || *filtered.Players[0].LeagueIndex != *all.Players[0].LeagueIndex {
		t.Fatal("HTTP filters changed comparative metrics")
	}
	b.want(http.StatusOK, "GET", base+"&owner=available", nil, &filtered)
	if filtered.Total != 3 {
		t.Fatal("available filter failed")
	}
	b.want(http.StatusOK, "GET", base+"&position=C", nil, &filtered)
	if filtered.Total != 0 || filtered.Players == nil {
		t.Fatal("empty research response failed")
	}
	selected, _ := json.Marshal([]players.ResearchPool{{Competition: "nba", Season: "analytics-test"}, {Competition: "nba", Season: "analytics-previous"}, {Competition: "wnba", Season: "analytics-women"}, {Competition: "nba", Season: "analytics-test"}})
	multi := "/api/research?pools=" + url.QueryEscape(string(selected)) + "&sort=league_index&include_chart=true&chart_x=games&chart_y=league_index"
	b.want(http.StatusOK, "GET", multi, nil, &all)
	if all.Total != 12 || len(all.Analysis) != 3 || len(all.Chart) != 12 || all.ChartTotal != 12 {
		t.Fatalf("multi-season pool: %+v", all)
	}
	keys := map[string]bool{}
	var nbaTop, previousTop, womenTop *float64
	for _, p := range all.Players {
		if keys[p.RowKey] {
			t.Fatal("duplicate snapshot key")
		}
		keys[p.RowKey] = true
		if p.FullName == "Za Analytics D" {
			if p.Season == "analytics-test" {
				nbaTop = p.LeagueIndex
			} else {
				previousTop = p.LeagueIndex
			}
		}
		if p.FullName == "Za Analytics WD" {
			womenTop = p.LeagueIndex
			var seasons []players.SeasonLine
			b.want(http.StatusOK, "GET", "/api/players/"+p.ID.String()+"/seasons", nil, &seasons)
			if len(seasons) != 1 || seasons[0].Points != 200 {
				t.Fatal("player season history must use the same default scoring as research")
			}
			var profile players.Research
			b.want(http.StatusOK, "GET", "/api/players/"+p.ID.String(), nil, &profile)
			if profile.ScoringSource != "defaults" {
				t.Fatal("player research must disclose default scoring")
			}
			if p.Points != 200 || p.ScoringSource != "defaults" {
				t.Fatalf("WNBA fallback scoring: %+v", p)
			}
		}
	}
	if nbaTop == nil || previousTop == nil || womenTop == nil || *nbaTop != *previousTop || *nbaTop != *womenTop {
		t.Fatal("league-season normalization blended seasons")
	}
	if len(all.Warnings) == 0 {
		t.Fatal("default scoring must be disclosed")
	}
	b.want(http.StatusOK, "GET", multi+"&stat_key=pts&min_stat=300&max_stat=400&position=G,F", nil, &filtered)
	if filtered.Total != 4 || len(filtered.Analysis) != 3 {
		t.Fatalf("raw filters or league profiles: %+v", filtered)
	}
	b.want(http.StatusOK, "GET", multi+"&max_games=9", nil, &filtered)
	if filtered.Total != 0 || len(filtered.Analysis) != 3 {
		t.Fatal("player filters must not change league-wide analysis")
	}
	b.want(http.StatusUnprocessableEntity, "GET", "/api/research?pools=oops", nil, nil)
	for _, query := range []string{"&min_rate=NaN", "&min_index=Inf", "&min_games=-1", "&replacement_rank=99999999"} {
		b.want(http.StatusUnprocessableEntity, "GET", base+query, nil, nil)
	}
}
