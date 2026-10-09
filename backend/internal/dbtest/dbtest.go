// Package dbtest opens the database for tests that need a real one.
package dbtest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/migrations"
)

// Open connects to TEST_DATABASE_URL with the schema migrated, or skips the
// test when it is unset. Tests delete data freely, so it refuses any
// database whose name does not end in "_test".
func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; run `make test`")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if name := pool.Config().ConnConfig.Database; !strings.HasSuffix(name, "_test") {
		t.Fatalf("refusing to run tests against database %q: its name must end in _test", name)
	}
	if err := migrations.Up(pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// Reset empties every league table, leaving the player pool alone.
func Reset(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `truncate periods, lineups, seasons, trades, drafts, transactions, roster_entries, leagues, franchises, dynasties, users cascade`)
	if err != nil {
		t.Fatal(err)
	}
}
