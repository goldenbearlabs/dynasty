package players_test

import (
	"context"
	"encoding/json"
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
