package ingest_test

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"testing"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
)

type discoveringSeasons struct{ *seasonFeed }

func (*discoveringSeasons) DiscoverySeasons() int { return 3 }

func TestSeasonDiscoveryRecoversMissingVeteransAndHistory(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition, provider = "test_discovery", "test_discovery_provider"
	t.Cleanup(func() {
		pool.Exec(ctx, `delete from players where competition=$1`, competition)
		pool.Exec(ctx, `delete from pro_teams where competition=$1`, competition)
		pool.Exec(ctx, `delete from ingest_runs where competition=$1`, competition)
	})
	q := db.New(pool)
	syncer := ingest.NewSyncer(q, slog.New(slog.NewTextHandler(io.Discard, nil)))
	current := ingest.Player{Provider: provider, ProviderID: "current", FullName: "Current Player", Positions: []string{"D"}}
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{current}}); err != nil {
		t.Fatal(err)
	}
	id, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: provider, ProviderID: "current"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := q.GetPlayer(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	line := func(id string, year int) ingest.SeasonLine {
		return ingest.SeasonLine{Provider: provider, ProviderID: id, Season: ingest.Season{Year: year, Label: "history", Team: "OLD", Games: 20, Stats: map[string]float64{"goals": 10}}}
	}
	feed := &seasonFeed{latest: 2027, byYear: map[int][]ingest.SeasonLine{}}
	for year := 2027; year >= 2023; year-- {
		feed.byYear[year] = []ingest.SeasonLine{line("current", year), line("injured", year)}
	}
	// The old importer already stored these years but skipped unknown players.
	if err := syncer.SyncSeasons(ctx, competition, feed, 5); err != nil {
		t.Fatal(err)
	}
	feed.byYear[2027] = []ingest.SeasonLine{line("current", 2027)}
	injured := ingest.Player{Provider: provider, ProviderID: "injured", FullName: "Injured Veteran", Positions: []string{"R"}, BirthDate: ingest.ParseDate("2002-02-01")}
	goalie := ingest.Player{Provider: provider, ProviderID: "goalie", FullName: "Missing Goalie", Positions: []string{"G"}}
	unsigned := ingest.Player{Provider: provider, ProviderID: "unsigned", FullName: "Unsigned Veteran", Positions: []string{"D"}}
	for _, p := range []ingest.Player{injured, goalie} {
		l := line(p.ProviderID, 2026)
		l.Player = &p
		feed.byYear[2026] = append(feed.byYear[2026], l)
	}
	l := line(unsigned.ProviderID, 2025)
	l.Player = &unsigned
	feed.byYear[2025] = append(feed.byYear[2025], l)
	// Historical names/positions must not overwrite the current roster.
	stale := current
	stale.FullName = "Old Name"
	stale.Positions = []string{"C"}
	feed.byYear[2027][0].Player = &stale
	feed.byYear[2023] = append(feed.byYear[2023], line("long-retired", 2023))
	feed.asked = nil
	if err := syncer.SyncSeasons(ctx, competition, &discoveringSeasons{feed}, 5); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(feed.asked, []int{2027, 2026, 2025, 2024, 2023}) {
		t.Fatalf("recovered players' older seasons not backfilled: %v", feed.asked)
	}
	for _, p := range []ingest.Player{injured, goalie, unsigned} {
		pid, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: provider, ProviderID: p.ProviderID})
		if err != nil {
			t.Fatal(err)
		}
		stored, err := q.GetPlayer(ctx, pid)
		if err != nil || !stored.PoolEligible || stored.Status != "inactive" || stored.ProTeamID.Valid || stored.FullName != p.FullName {
			t.Fatalf("missing veteran not safely admitted: %+v, err=%v", stored, err)
		}
		if p.ProviderID == "injured" {
			seasons, err := q.ListPlayerSeasons(ctx, pid)
			if err != nil || len(seasons) != 4 || stored.BirthDate.Time != p.BirthDate {
				t.Fatalf("veteran history or birth date missing: %+v, err=%v", seasons, err)
			}
		}
	}
	after, err := q.GetPlayer(ctx, id)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("season discovery overwrote the current player: before=%+v after=%+v err=%v", before, after, err)
	}
	feed.asked = nil
	if err := syncer.SyncSeasons(ctx, competition, &discoveringSeasons{feed}, 5); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(feed.asked, []int{2027, 2026, 2025}) {
		t.Fatalf("repeat run needlessly reimported older history: %v", feed.asked)
	}
	var count int
	if err := pool.QueryRow(ctx, `select count(*) from players where competition=$1`, competition).Scan(&count); err != nil || count != 4 {
		t.Fatalf("duplicates or long-retired players admitted: count=%d err=%v", count, err)
	}
	// A return to the roster updates the same identity, rather than duplicating it.
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{current, injured}}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from players where competition=$1 and status='active'`, competition).Scan(&count); err != nil || count != 2 {
		t.Fatalf("returning veteran did not activate: count=%d err=%v", count, err)
	}
}
