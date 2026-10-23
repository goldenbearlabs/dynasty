package ingest_test

import (
	"context"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"
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

type previewFeed map[string][]ingest.Preview

func (f previewFeed) Previews(_ context.Context, day time.Time) ([]ingest.Preview, error) {
	return f[day.Format(time.DateOnly)], nil
}

func TestSyncPreviews(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	var player, team, game pgtype.UUID
	err := pool.QueryRow(ctx, `insert into pro_teams(competition,provider_id,abbrev,name) values('nhl','preview-test','PVT','Preview Test Red Sox') returning id`).Scan(&team)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `delete from pro_teams where id=$1`, team) })
	if err := pool.QueryRow(ctx, `insert into players(competition,status,full_name,positions,pro_team_id) values('nhl','active','Starter Test Goalie',array['G'],$1) returning id`, team).Scan(&player); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `delete from players where id=$1`, player) })
	if err := pool.QueryRow(ctx, `insert into games(competition,provider_id,day,starts_at,status,home_team_id) values('nhl','preview-game','2026-10-20','2026-10-20 23:00Z','scheduled',$1) returning id`, team).Scan(&game); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `delete from games where id=$1`, game) })

	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	day := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	starting := func() []bool {
		rows, _ := pool.Query(ctx, `select confirmed from probable_starters where player_id=$1 order by day`, player)
		confirmed, err := pgx.CollectRows(rows, pgx.RowTo[bool])
		if err != nil {
			t.Fatal(err)
		}
		return confirmed
	}
	homeImplied := func() (v pgtype.Float8) {
		pool.QueryRow(ctx, `select home_implied from games where id=$1`, game).Scan(&v)
		return v
	}
	// Another feed's game, found by the home side's nickname.
	feed := previewFeed{"2026-10-20": {{
		GameID: "espn-9", HomeTeam: "Red Sox", Odds: &ingest.Odds{HomeScore: 3.5, AwayScore: 2.5, HomeWin: 0.6},
		Starters: []ingest.Starter{{FullName: "Starter Test Goalie", Confirmed: true}},
	}}}
	if err := syncer.SyncPreviews(ctx, "nhl", feed, day, day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if got := starting(); !slices.Equal(got, []bool{true}) {
		t.Fatalf("after first sync = %v", got)
	}
	if got := homeImplied(); got.Float64 != 3.5 {
		t.Fatalf("home implied = %+v", got)
	}
	// A day the feed no longer names him, or prices the game, is cleared.
	if err := syncer.SyncPreviews(ctx, "nhl", previewFeed{}, day, day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if got := starting(); len(got) != 0 {
		t.Fatalf("after second sync = %v", got)
	}
	if got := homeImplied(); got.Valid {
		t.Fatalf("home implied kept = %+v", got)
	}
}
