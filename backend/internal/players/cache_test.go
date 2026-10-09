package players

import (
	"context"
	"os"
	"testing"
	"time"

	"crossover/internal/cache"
	"crossover/internal/competition"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
)

func TestResearchDatasetCacheAcrossFilters(t *testing.T) {
	pool := dbtest.Open(t)
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	client, err := cache.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool, competition.NewRegistry(ingest.NewClient(0)))
	service.Cache = cache.New(client, pool, nil)
	// Force a fresh generation without altering any player data.
	if _, err := pool.Exec(ctx, "update player_seasons set games=games where false"); err != nil {
		t.Fatal(err)
	}
	f := ResearchFilter{Pools: []ResearchPool{{Competition: "nba", Season: ""}}}
	start := time.Now()
	first, err := service.researchData(ctx, f)
	cold := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rows) == 0 {
		t.Skip("no imported NBA season in test database")
	}
	f.Position = "C"
	f.Search = "Jokic"
	f.Page = 2
	f.IncludeChart = true
	f.ChartY = "league_index"
	start = time.Now()
	second, err := service.researchData(ctx, f)
	warm := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rows) != len(second.Rows) || first.Rows[0].Points != second.Rows[0].Points {
		t.Fatal("display filters changed shared dataset")
	}
	if &first.Rows[0] != &second.Rows[0] {
		t.Fatal("filter change did not reuse the dataset held in memory")
	}
	// A committed change to season data starts a new generation.
	if _, err := pool.Exec(ctx, "update player_seasons set games=games where false"); err != nil {
		t.Fatal(err)
	}
	third, err := service.researchData(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if &first.Rows[0] == &third.Rows[0] {
		t.Fatal("season import did not replace the dataset held in memory")
	}
	// Selection validation must run even if the canonical dataset is cached.
	f.Pools = make([]ResearchPool, 25)
	for i := range f.Pools {
		f.Pools[i] = ResearchPool{Competition: "nba", Season: ""}
	}
	if _, err := service.researchData(ctx, f); err == nil {
		t.Fatal("cached selection bypassed pool limit")
	}
	t.Logf("%d NBA players: cold dataset %s, cached filter change %s", len(first.Rows), cold, warm)
}
