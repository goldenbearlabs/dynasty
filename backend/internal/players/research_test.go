package players_test

import (
	"context"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/competition"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
	"crossover/internal/players"
)

// career is a per-player feed that counts how often it is asked.
type career struct {
	asked   int
	seasons []ingest.Season
}

func (c *career) IDSpace() string { return "test_seasons_provider" }

func (c *career) Career(context.Context, string) ([]ingest.Season, error) {
	c.asked++
	return c.seasons, nil
}

func TestSeasons(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const pro, college = "test_pro_league", "test_college_league"
	cleanup := func() {
		dbtest.Reset(t, pool)
		pool.Exec(ctx, `delete from players where competition in ($1, $2)`, pro, college)
	}
	cleanup()
	defer cleanup()

	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// The pro league values a point at 1 and a rebound at 1.5; the college
	// league's rules are different and must not be used for a pro.
	sql(`insert into dynasties (name, settings) values ('Seasons Test', '{}')`)
	sql(`insert into leagues (dynasty_id, competition, name, settings)
	     select id, $1, 'Pro', '{"scoring": {"pts": 1, "reb": 1.5}}' from dynasties`, pro)
	sql(`insert into leagues (dynasty_id, competition, name, settings)
	     select id, $1, 'College', '{"scoring": {"pts": 100}}' from dynasties`, college)
	var vet, recruit pgtype.UUID
	pool.QueryRow(ctx, `insert into players (competition, status, full_name) values ($1, 'active', 'Known Vet') returning id`, pro).Scan(&vet)
	pool.QueryRow(ctx, `insert into players (competition, status, full_name) values ($1, 'prospect', 'Unknown Recruit') returning id`, college).Scan(&recruit)
	sql(`insert into player_external_ids (provider, provider_id, player_id) values ('test_seasons_provider', 'vet', $1)`, vet)

	// What the nightly job has stored: a pro season, and the college season before it.
	season := func(competition string, year int, label, team string, games int, stats string) {
		sql(`insert into player_seasons (player_id, competition, year, label, team, games, stats) values ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
			vet, competition, year, label, team, games, stats)
	}
	season(pro, 2026, "2025-26", "DEN", 50, `{"pts": 1000, "reb": 400, "ast": 300}`)
	season(college, 2025, "2024-25", "DUKE", 30, `{"pts": 600, "reb": 200}`)

	// Seasons in other leagues come one player at a time.
	juniors := &career{seasons: []ingest.Season{{Year: 2024, Label: "2023-24", Team: "Otters", League: "OHL", Stats: map[string]float64{"pts": 10}}}}
	service := players.NewService(pool, competition.Registry{
		{Key: pro, Name: "Pro League", Career: juniors},
		{Key: college, Name: "College League"},
	})

	seasons, err := service.Seasons(ctx, vet)
	if err != nil {
		t.Fatal(err)
	}
	if len(seasons) != 3 {
		t.Fatalf("got %d seasons, want the pro, college and junior ones: %+v", len(seasons), seasons)
	}
	// Newest first. 1000 points and 400 rebounds at 1.5; assists are not scored.
	latest := seasons[0]
	if latest.Season != "2025-26" || latest.Team != "DEN" || latest.League != "" || latest.Games != 50 || latest.Stats["ast"] != 300 ||
		math.Abs(latest.Points-1600) > 0.001 || math.Abs(latest.PointsPerGame-32) > 0.001 {
		t.Errorf("pro season = %+v, want 1600 points, 32 a game", latest)
	}
	// The college season is named for where it was played, and valued by the pro league's rules.
	if c := seasons[1]; c.Season != "2024-25" || c.Competition != college || c.League != "College League" || c.Points != 900 || c.PointsPerGame != 30 {
		t.Errorf("college season = %+v, want it labelled College League and worth 900 under the pro rules", c)
	}
	if j := seasons[2]; j.League != "OHL" || j.Points != 10 || j.PointsPerGame != 0 {
		t.Errorf("junior season = %+v, want 10 points and no per-game figure without games", j)
	}

	// Points follow the rules as they change, and the player's profile is not read again.
	sql(`update leagues set settings = '{"scoring": {"pts": 2}}' where competition = $1`, pro)
	seasons, err = service.Seasons(ctx, vet)
	if err != nil {
		t.Fatal(err)
	}
	if juniors.asked != 1 || seasons[0].Points != 2000 {
		t.Errorf("after a rule change: profile read %d times, %v points; want once, 2000", juniors.asked, seasons[0].Points)
	}

	// A player no feed knows has no seasons.
	seasons, err = service.Seasons(ctx, recruit)
	if err != nil || seasons == nil || len(seasons) != 0 {
		t.Errorf("recruit: %v seasons, err %v; want an empty list", seasons, err)
	}
}
