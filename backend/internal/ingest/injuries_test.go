package ingest_test

import (
	"context"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"log/slog"
	"testing"
)

type injuryFeed struct {
	entries []ingest.Injury
	err     error
}

func (f injuryFeed) Injuries(context.Context) ([]ingest.Injury, error) { return f.entries, f.err }

func TestSyncInjuries(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	var id pgtype.UUID
	err := pool.QueryRow(ctx, `insert into players(competition,status,full_name,positions) values('nhl','active','Injury Test José',array['C']) returning id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `delete from players where id=$1`, id) })
	q := db.New(pool)
	syncer := ingest.NewSyncer(q, slog.New(slog.NewTextHandler(io.Discard, nil)))
	feed := injuryFeed{entries: []ingest.Injury{{FullName: "Injury Test Jose", Designation: "IR"}}}
	if err := syncer.SyncInjuries(ctx, "nhl", feed); err != nil {
		t.Fatal(err)
	}
	player, err := q.GetPlayer(ctx, id)
	if err != nil || player.InjuryDesignation != "IR" {
		t.Fatalf("player = %+v; %v", player, err)
	}
	if err := syncer.SyncInjuries(ctx, "nhl", injuryFeed{err: errors.New("unavailable")}); err == nil {
		t.Fatal("failed feed accepted")
	}
	player, _ = q.GetPlayer(ctx, id)
	if player.InjuryDesignation != "IR" {
		t.Fatal("failed feed cleared designation")
	}
	if err := syncer.SyncInjuries(ctx, "nhl", injuryFeed{}); err != nil {
		t.Fatal(err)
	}
	player, _ = q.GetPlayer(ctx, id)
	if player.InjuryDesignation != "" || !player.InjuryCheckedAt.Valid {
		t.Fatal("successful return did not clear designation")
	}
}
