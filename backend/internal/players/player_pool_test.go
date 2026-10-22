package players_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"crossover/internal/db"
	"crossover/internal/dbtest"
)

func TestPlayerPoolOwnershipAndStats(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const sport = "pool_test"
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where competition=$1`, sport) })
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into dynasties (name,settings) values ('Pool','{}')`)
	exec(`insert into leagues (dynasty_id,competition,name,settings) select id,$1,'Pool','{"scoring":{"pts":2},"roster":{"main":2}}' from dynasties`, sport)
	exec(`insert into franchises (dynasty_id,name,manager_name,slug,invite_token) select id,'Alpha','A','alpha','pool-a' from dynasties`)
	exec(`insert into franchises (dynasty_id,name,manager_name,slug,invite_token) select id,'Beta','B','beta','pool-b' from dynasties`)
	for _, name := range []string{"Available", "Rostered", "Reserve", "Unsigned"} {
		exec(`insert into players (competition,status,full_name) values ($1,'active',$2)`, sport, name)
	}
	for _, entry := range []struct{ name, list, owner string }{{"Rostered", "main", "alpha"}, {"Reserve", "reserve", "beta"}, {"Unsigned", "rights", "alpha"}} {
		exec(`insert into roster_entries (league_id,franchise_id,player_id,list,acquired_via) select l.id,f.id,p.id,$1,'test' from leagues l,franchises f,players p where l.competition=$2 and p.competition=$2 and p.full_name=$3 and f.slug=$4`, entry.list, sport, entry.name, entry.owner)
	}
	exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) select id,$1,2026,'2026','A',10,'{"pts":20,"ast":4}' from players where competition=$1 and full_name='Available'`, sport)
	exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) select id,$1,2026,'2026','B',5,'{"pts":10,"ast":2}' from players where competition=$1 and full_name='Available'`, sport)
	exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) select id,$1,2025,'2025','A',40,'{"pts":999}' from players where competition=$1 and full_name='Available'`, sport)
	// Career history in another sport must not leak into this sport's raw totals.
	exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) select id,'pool_other',2026,'2026','C',1,'{"pts":9999}' from players where competition=$1 and full_name='Available'`, sport)
	q := db.New(pool)
	for _, tc := range []struct {
		name              string
		ownership, owners []string
		count             int
	}{
		{"all", nil, nil, 4}, {"available", []string{"available"}, nil, 1}, {"taken", []string{"taken"}, nil, 3},
		{"reserve and rostered", []string{"reserve", "main"}, nil, 2}, {"available and reserve", []string{"available", "reserve"}, nil, 2},
		{"team", nil, []string{"alpha"}, 2}, {"team and main", []string{"main"}, []string{"alpha"}, 1},
		{"multiple teams", []string{"reserve", "main"}, []string{"alpha", "beta"}, 2}, {"unsigned", []string{"rights"}, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := q.ListPlayers(ctx, db.ListPlayersParams{Competition: sport, Ownership: tc.ownership, Owners: tc.owners, PageSize: 50})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.count {
				t.Fatalf("got %d rows, want %d", len(rows), tc.count)
			}
			count, err := q.CountPlayers(ctx, db.CountPlayersParams{Competition: sport, Ownership: tc.ownership, Owners: tc.owners})
			if err != nil || int(count) != tc.count {
				t.Fatalf("count=%d, err=%v", count, err)
			}
		})
	}
	rows, err := q.ListPlayers(ctx, db.ListPlayersParams{Competition: sport, Ownership: []string{"available"}, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected available player: %+v", rows)
	}
	allSports, err := q.ListPlayers(ctx, db.ListPlayersParams{Search: "Available", PageSize: 50})
	if err != nil || len(allSports) != 1 {
		t.Fatalf("all-sport player query: %+v, %v", allSports, err)
	}
	var allStats map[string]float64
	if err := json.Unmarshal(allSports[0].SeasonStats, &allStats); err != nil || allStats["pts"] != 30 {
		t.Fatalf("cross-sport contamination: %v, %v", allStats, err)
	}
	var stats map[string]float64
	if err := json.Unmarshal(rows[0].SeasonStats, &stats); err != nil {
		t.Fatal(err)
	}
	if rows[0].SeasonGames != 15 || rows[0].SeasonPoints != 60 || stats["pts"] != 30 || stats["ast"] != 6 {
		t.Fatalf("incorrect season split totals: %+v, %v", rows[0], stats)
	}
	rows, err = q.ListPlayers(ctx, db.ListPlayersParams{Competition: sport, Ownership: []string{"available"}, SeasonBack: 1, PageSize: 50})
	if err != nil || len(rows) != 1 || rows[0].SeasonPoints != 1998 {
		t.Fatalf("historical season: %+v, %v", rows, err)
	}
}

func TestPlayerPoolSortingAndCanonicalSeasons(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const sport = "pool_sort_test"
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where competition=$1`, sport) })
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into dynasties(name,settings) values('Sort','{}')`)
	exec(`insert into leagues(dynasty_id,competition,name,settings) select id,$1,'Sort','{"scoring":{"pts":2},"roster":{"main":2}}' from dynasties`, sport)
	for _, p := range []struct {
		name  string
		games int
		stats string
	}{{"Available", 15, `{"pts":30,"ast":6}`}, {"Rostered", 5, `{"pts":25,"ast":20}`}, {"Reserve", 50, `{"pts":50,"ast":10}`}, {"Unsigned", 0, `{}`}} {
		exec(`insert into players(competition,status,full_name) values($1,'active',$2)`, sport, p.name)
		if p.games > 0 {
			exec(`insert into player_seasons(player_id,competition,year,label,games,stats) select id,$1,2026,'2025-26',$3,$4 from players where competition=$1 and full_name=$2`, sport, p.name, p.games, p.stats)
		}
	}
	// A provider fallback incorrectly saved the latest season under a future year.
	exec(`insert into player_seasons(player_id,competition,year,label,games,stats) select player_id,competition,2027,label,games,stats from player_seasons where competition=$1`, sport)
	exec(`insert into player_seasons(player_id,competition,year,label,games,stats) select id,$1,2025,'2024-25',1,'{"pts":999}' from players where competition=$1 and full_name='Available'`, sport)
	q := db.New(pool)
	seasons, err := q.ListStatSeasons(ctx)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, s := range seasons {
		if s.Competition == sport {
			labels = append(labels, s.Label)
		}
	}
	if len(labels) != 2 || labels[0] != "2025-26" || labels[1] != "2024-25" {
		t.Fatalf("duplicate season choices: %v", labels)
	}
	for _, tc := range []struct {
		name, sort, key string
		asc, perGame    bool
		want            []string
	}{
		{"totals", "points", "", false, false, []string{"Reserve", "Available", "Rostered", "Unsigned"}},
		{"totals ascending", "points", "", true, false, []string{"Rostered", "Available", "Reserve", "Unsigned"}},
		{"rate", "points", "", false, true, []string{"Rostered", "Available", "Reserve", "Unsigned"}},
		{"rate ascending", "points", "", true, true, []string{"Reserve", "Available", "Rostered", "Unsigned"}},
		{"raw", "stat", "ast", false, false, []string{"Rostered", "Reserve", "Available", "Unsigned"}},
		{"raw ascending", "stat", "ast", true, false, []string{"Available", "Reserve", "Rostered", "Unsigned"}},
		{"raw rate", "stat", "ast", false, true, []string{"Rostered", "Available", "Reserve", "Unsigned"}},
		{"games", "games", "", false, false, []string{"Reserve", "Available", "Rostered", "Unsigned"}},
		{"name", "name", "", true, false, []string{"Available", "Reserve", "Rostered", "Unsigned"}},
		{"name descending", "name", "", false, false, []string{"Unsigned", "Rostered", "Reserve", "Available"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := db.ListPlayersParams{Competition: sport, SortBy: tc.sort, StatKey: tc.key, Ascending: tc.asc, PerGame: tc.perGame, PageSize: 50}
			rows, err := q.ListPlayers(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(tc.want) {
				t.Fatalf("rows=%d", len(rows))
			}
			for i, row := range rows {
				if row.FullName != tc.want[i] {
					t.Fatalf("row %d=%s want %s", i, row.FullName, tc.want[i])
				}
			}
			f.PageSize = 1
			f.PageOffset = 1
			rows, err = q.ListPlayers(ctx, f)
			if err != nil || len(rows) != 1 || rows[0].FullName != tc.want[1] {
				t.Fatalf("global pagination sort failed: %+v, %v", rows, err)
			}
		})
	}
	rows, err := q.ListPlayers(ctx, db.ListPlayersParams{Competition: sport, SeasonBack: 1, Search: "Available", PageSize: 50})
	if err != nil || len(rows) != 1 || rows[0].SeasonPoints != 1998 {
		t.Fatalf("season offset includes duplicate: %+v %v", rows, err)
	}
}

func TestCopiedSeasonRepair(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into players(competition,status,full_name,note) values('nba','active','Copied season test','copied_season_repair')`)
	exec(`insert into player_seasons(player_id,competition,year,label,team,games,stats) select id,'nba',2026,'2025-26','EXACT',10,'{"pts":20}' from players where note='copied_season_repair'`)
	exec(`insert into player_seasons(player_id,competition,year,label,team,games,stats) select player_id,competition,2027,label,team,games,stats from player_seasons where player_id in(select id from players where note='copied_season_repair')`)
	exec(`insert into player_seasons(player_id,competition,year,label,team,games,stats) select id,'nba',2027,'2025-26','DIFFERENT',10,'{"pts":21}' from players where note='copied_season_repair'`)
	raw, err := os.ReadFile("../../migrations/0044_copied_seasons.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(strings.Split(string(raw), "-- +goose Down")[0])
	var count int
	if err := tx.QueryRow(ctx, `select count(*) from player_seasons where player_id in(select id from players where note='copied_season_repair')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("repair removed unique data or kept exact copy: count=%d err=%v", count, err)
	}
	if err := tx.QueryRow(ctx, `select count(*) from player_seasons where player_id in(select id from players where note='copied_season_repair') and year=2027 and team='EXACT'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("exact duplicate remains: count=%d err=%v", count, err)
	}
}
