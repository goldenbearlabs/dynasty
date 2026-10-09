package players_test

import (
	"context"
	"encoding/json"
	"testing"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestResearchList(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const pro, college = "research_pro", "research_college"
	t.Cleanup(func() {
		dbtest.Reset(t, pool)
		pool.Exec(ctx, `delete from players where competition in ($1, $2)`, pro, college)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into dynasties (name, settings) values ('Research', '{}')`)
	exec(`insert into leagues (dynasty_id, competition, name, settings) select id, $1, 'Pro', '{"scoring":{"pts":2,"reb":1.5,"tov":-1}}' from dynasties`, pro)
	exec(`insert into leagues (dynasty_id, competition, name, settings) select id, $1, 'College', '{"scoring":{"pts":3}}' from dynasties`, college)
	player := func(name, competition string) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `insert into players (competition,status,full_name) values ($1,'active',$2) returning id`, competition, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	vet := player("Research Vet", pro)
	rookie := player("Research Rookie", pro)
	student := player("Research Student", college)
	season := func(id pgtype.UUID, competition string, year, games int, label, team, stats string) {
		exec(`insert into player_seasons (player_id,competition,year,label,team,games,stats) values ($1,$2,$3,$4,$5,$6,$7::jsonb)`, id, competition, year, label, team, games, stats)
	}
	season(vet, pro, 2026, 10, "2025-26", "AAA", `{"pts":100,"reb":20,"tov":5,"ast":50}`)
	season(vet, pro, 2026, 5, "2025-26", "BBB", `{"pts":50,"reb":10,"tov":2}`)
	season(vet, pro, 2025, 20, "2024-25", "AAA", `{"pts":9999}`)
	// A prior college season belongs in player history, not the current pro totals.
	season(vet, college, 2026, 30, "2025-26", "COL", `{"pts":5000}`)
	season(student, college, 2026, 10, "2025-26", "COL", `{"pts":200}`)
	q := db.New(pool)
	filter := db.ListResearchPlayersParams{Competition: pro, Sort: "points", PageSize: 50}
	rows, err := q.ListResearchPlayers(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != vet || rows[0].Points != 338 || rows[0].Games != 15 || rows[0].PointsPerGame != 338.0/15 {
		t.Fatalf("incorrect team-split totals or sorting: %+v", rows)
	}
	var stats map[string]float64
	if err := json.Unmarshal(rows[0].Stats, &stats); err != nil {
		t.Fatal(err)
	}
	if stats["pts"] != 150 || stats["reb"] != 30 || stats["ast"] != 50 {
		t.Fatalf("stats = %v", stats)
	}
	if rows[1].ID != rookie || rows[1].Season != "" || rows[1].PointsPerGame != 0 {
		t.Fatalf("missing stats = %+v", rows[1])
	}
	filter.Competition = ""
	filter.Search = "Research"
	rows, err = q.ListResearchPlayers(ctx, filter)
	if err != nil || len(rows) != 3 || rows[0].ID != student || rows[0].Points != 600 {
		t.Fatalf("cross-league scoring: %+v, %v", rows, err)
	}
	filter.Competition, filter.Season = pro, "2024-25"
	rows, err = q.ListResearchPlayers(ctx, filter)
	if err != nil || rows[0].Points != 19998 {
		t.Fatalf("historical season: %+v, %v", rows, err)
	}
	filter.Season, filter.Sort, filter.PageSize, filter.PageOffset = "", "reb", 1, 1
	rows, err = q.ListResearchPlayers(ctx, filter)
	if err != nil || len(rows) != 1 || rows[0].ID != rookie {
		t.Fatalf("stat sort and pagination: %+v, %v", rows, err)
	}
	exec(`update leagues set settings = '{"scoring":{"pts":1}}' where competition = $1`, pro)
	filter.Sort, filter.PageOffset = "points", 0
	rows, err = q.ListResearchPlayers(ctx, filter)
	if err != nil || rows[0].Points != 150 {
		t.Fatalf("current scoring: %+v, %v", rows, err)
	}
}
