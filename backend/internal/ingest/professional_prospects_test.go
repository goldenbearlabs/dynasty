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
)

func TestProfessionalProspectAdmission(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition = "test_professional_pool"
	t.Cleanup(func() {
		pool.Exec(ctx, `delete from players where competition=$1`, competition)
		pool.Exec(ctx, `delete from pro_teams where competition=$1`, competition)
		pool.Exec(ctx, `delete from ingest_runs where competition=$1`, competition)
	})
	q := db.New(pool)
	syncer := ingest.NewSyncer(q, slog.New(slog.NewTextHandler(io.Discard, nil)))
	college := ingest.Player{Provider: "test_professional", ProviderID: "college", FullName: "Still In College"}
	pro := ingest.Player{Provider: "test_professional", ProviderID: "pro", FullName: "Signed Professional"}
	rookie := ingest.Player{Provider: "test_professional", ProviderID: "rookie", FullName: "College Player Joining Roster"}
	// Reproduce records imported by the former unrestricted college board.
	if err := syncer.SyncProspects(ctx, competition, prospects{college, pro, rookie}); err != nil {
		t.Fatal(err)
	}
	collegeID, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: college.Provider, ProviderID: college.ProviderID})
	if err != nil {
		t.Fatal(err)
	}
	// Model the migration's exclusion of previously imported college rows.
	if _, err := pool.Exec(ctx, `update players set pool_eligible=false, eligible_since=now()-interval '1 year' where competition=$1 and full_name<>$2`, competition, pro.FullName); err != nil {
		t.Fatal(err)
	}
	if err := syncer.SyncProspects(ctx, competition, prospects{pro}); err != nil {
		t.Fatal(err)
	}
	before, err := q.GetPlayer(ctx, collegeID)
	if err != nil || before.PoolEligible {
		t.Fatalf("college prospect remains eligible: %+v, err=%v", before, err)
	}
	// He becomes a professional prospect: retain his id, and reset admission
	// time so an old college import cannot bypass future rookie-draft rules.
	if err := syncer.SyncProspects(ctx, competition, prospects{college, pro}); err != nil {
		t.Fatal(err)
	}
	after, err := q.GetPlayer(ctx, collegeID)
	if err != nil || !after.PoolEligible || !after.EligibleSince.Time.After(before.EligibleSince.Time) {
		t.Fatalf("professional admission failed: %+v, err=%v", after, err)
	}
	// The roster route also admits an excluded player. Professionals remain
	// eligible when they age beyond the prospect feed's draft-year window.
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{rookie}}); err != nil {
		t.Fatal(err)
	}
	if err := syncer.SyncProspects(ctx, competition, prospects{}); err != nil {
		t.Fatal(err)
	}
	var status string
	var eligible bool
	var admitted time.Time
	if err := pool.QueryRow(ctx, `select status,pool_eligible,eligible_since from players where competition=$1 and full_name=$2`, competition, rookie.FullName).Scan(&status, &eligible, &admitted); err != nil || !eligible || status != "active" || time.Since(admitted) > time.Minute {
		t.Fatalf("roster did not admit the professional: status=%s, eligible=%v, err=%v", status, eligible, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `select count(*) from players where id=$1 and pool_eligible`, collegeID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("identity lost: count=%d, err=%v", count, err)
	}
}
