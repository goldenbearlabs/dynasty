package web_test

import (
	"context"
	"net/http"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
)

func TestFuturePicksForAddedFranchises(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })
	ctx := context.Background()
	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Future picks", Leagues: []dynasty.LeagueSetup{{Competition: "nhl", Settings: nhl.Defaults}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var bob struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/franchises", dynasty.FranchiseSetup{Name: "Bob", ManagerName: "Bob"}, &bob)
	var annID, dynastyID, draftID, pickID string
	if err := pool.QueryRow(ctx, `select f.id,f.dynasty_id,d.id,k.id from franchises f
		join drafts d on d.dynasty_id=f.dynasty_id join draft_picks k on k.draft_id=d.id
		where f.name='Ann' and k.original_franchise_id=f.id order by d.year,k.position limit 1`).Scan(&annID, &dynastyID, &draftID, &pickID); err != nil {
		t.Fatal(err)
	}
	assertPicks := func() {
		t.Helper()
		var missing int
		err := pool.QueryRow(ctx, `select count(*) from drafts d
			cross join franchises f cross join generate_series(1,$1::int) r
			where d.dynasty_id=$2 and f.dynasty_id=d.dynasty_id and not exists
			(select 1 from draft_picks k where k.draft_id=d.id and k.round=r and k.original_franchise_id=f.id)`, nhl.Defaults.RookieRounds(), dynastyID).Scan(&missing)
		if err != nil || missing != 0 {
			t.Fatalf("missing entitlement slots=%d err=%v", missing, err)
		}
	}
	assertPicks()
	// Simulate the old incomplete drafts, with an existing pick already traded.
	if _, err := pool.Exec(ctx, `update draft_picks set current_franchise_id=$1 where id=$2`, bob.ID, pickID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `delete from draft_picks where original_franchise_id=$1`, bob.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := pool.Exec(ctx, `select reconcile_placeholder_draft_picks($1)`, dynastyID); err != nil {
			t.Fatal(err)
		}
	}
	assertPicks()
	var owner string
	if err := pool.QueryRow(ctx, `select current_franchise_id from draft_picks where id=$1`, pickID).Scan(&owner); err != nil || owner != bob.ID {
		t.Fatalf("traded pick owner=%s err=%v", owner, err)
	}
	var duplicates int
	if err := pool.QueryRow(ctx, `select count(*) from (select draft_id,round,original_franchise_id from draft_picks group by 1,2,3 having count(*)>1) x`).Scan(&duplicates); err != nil || duplicates != 0 {
		t.Fatalf("duplicate entitlements=%d err=%v", duplicates, err)
	}
	// Once announced, an existing board must not acquire automatic new slots.
	if _, err := pool.Exec(ctx, `update drafts set is_placeholder=false where id=$1`, draftID); err != nil {
		t.Fatal(err)
	}
	var cy struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/franchises", dynasty.FranchiseSetup{Name: "Cy", ManagerName: "Cy"}, &cy)
	var added int
	if err := pool.QueryRow(ctx, `select count(*) from draft_picks where draft_id=$1 and original_franchise_id=$2`, draftID, cy.ID).Scan(&added); err != nil || added != 0 {
		t.Fatalf("announced board gained %d picks: %v", added, err)
	}
}
