package ingest_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
	"crossover/internal/sportsday"
)

func TestScheduleHorizon(t *testing.T) {
	today := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name            string
		days            int
		seasonEnd, want time.Time
	}{
		{"old 60-day configuration still covers the year", 60, time.Time{}, today.AddDate(0, 0, 365)},
		{"unconfigured server", 0, time.Time{}, today.AddDate(0, 0, 365)},
		{"longer configuration", 500, time.Time{}, today.AddDate(0, 0, 500)},
		{"long fantasy season", 365, today.AddDate(0, 0, 600), today.AddDate(0, 0, 600)},
		{"season crosses New Year", 60, time.Date(2027, 4, 16, 0, 0, 0, 0, time.UTC), today.AddDate(0, 0, 365)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ingest.ScheduleHorizon(today, tt.days, tt.seasonEnd); !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFullScheduleBackfillsBeyondSixtyDays(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition = "test_full_schedule"
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	syncer := ingest.NewSyncer(db.New(tx), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{}); err != nil {
		t.Fatal(err)
	}
	today := sportsday.Today()
	future := today.AddDate(0, 0, 200).Add(18 * time.Hour)
	source := &fakeGames{games: []ingest.Game{{ProviderID: "late-season", StartsAt: future, Status: ingest.GameScheduled, HomeTeam: "1"}}}
	// Even a server still configured with the old 60-day setting imports this game.
	if err := syncer.SyncFullSchedule(ctx, competition, source, today, 60); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `select count(*) from games where competition=$1 and provider_id='late-season'`, competition).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("late-season game was not imported")
	}
	if len(source.fetched) != 0 {
		t.Fatal("schedule backfill fetched box scores")
	}
	// An unusually long active season extends importing past the one-year floor.
	late := today.AddDate(0, 0, 500)
	if _, err := tx.Exec(ctx, `with d as (
		insert into dynasties(name,settings) values('Full Schedule Test','{}') returning id
	), l as (
		insert into leagues(dynasty_id,competition,name,settings) select id,$1,'Full Schedule Test','{}' from d returning id
	) insert into seasons(league_id,year,starts_on,ends_on) select id,$2,$3,$4 from l`, competition, today.Year(), today, late); err != nil {
		t.Fatal(err)
	}
	source.games = append(source.games, ingest.Game{ProviderID: "extended-season", StartsAt: late.Add(18 * time.Hour), Status: ingest.GameScheduled, HomeTeam: "1"})
	if err := syncer.SyncFullSchedule(ctx, competition, source, late, 60); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `select count(*) from games where competition=$1 and provider_id='extended-season'`, competition).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("active season beyond the year horizon was not imported")
	}
}
