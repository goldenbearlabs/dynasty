package web_test

import (
	"context"
	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/settings"
	"net/http"
	"testing"
)

func TestInjurySwap(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	cleanup := func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note='injury_swap_test'`) }
	t.Cleanup(cleanup)
	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Lineup.Slots = []settings.Slot{{Name: "UTIL", Count: 1, Positions: []string{"PG"}}}
	rules.Roster.Main, rules.Roster.Reserve = 1, 1
	rules.Roster.ReserveEligibility = "prospects_only"
	rules.Roster.ReserveLockDays = 30
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Injury Swap Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Name, Slug string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := view.Leagues[0].ID
	var owner, slug string
	for _, f := range view.Franchises {
		if f.Name == "Ann" {
			owner, slug = f.ID, f.Slug
		}
	}
	player := func(name, status string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `insert into players(competition,status,full_name,positions,note) values('nba',$1,$2,array['PG'],'injury_swap_test') returning id`, status, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	injured, replacement := player("Injury Swap Injured", "active"), player("Injury Swap Replacement", "prospect")
	for _, entry := range []struct{ id, list string }{{injured, "main"}, {replacement, "reserve"}} {
		if _, err := pool.Exec(ctx, `insert into roster_entries(league_id,franchise_id,player_id,list,acquired_via,reserved_at) values($1,$2,$3,$4,'commissioner',now())`, league, owner, entry.id, entry.list); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `update players set injury_designation='OUT' where id=$1`, injured); err != nil {
		t.Fatal(err)
	}
	route := "/api/leagues/" + league + "/roster/"
	swap := map[string]string{"player_id": injured, "replacement_id": replacement}
	// No season: rejected without changing either list.
	ann.want(http.StatusUnprocessableEntity, "POST", route+"injury-swap", swap, nil)
	var season string
	if err := pool.QueryRow(ctx, `insert into seasons(league_id,year,starts_on,ends_on) values($1,2026,current_date-1,current_date+20) returning id`, league).Scan(&season); err != nil {
		t.Fatal(err)
	}
	// Invalid status and wrong replacement are rejected.
	pool.Exec(ctx, `update players set injury_designation='Q' where id=$1`, injured)
	ann.want(http.StatusUnprocessableEntity, "POST", route+"injury-swap", swap, nil)
	pool.Exec(ctx, `update players set injury_designation='OUT' where id=$1`, injured)
	ann.want(http.StatusUnprocessableEntity, "POST", route+"injury-swap", map[string]string{"player_id": injured, "replacement_id": injured}, nil)
	// The normal call-up is locked and the main roster is full; the atomic swap succeeds.
	ann.want(http.StatusUnprocessableEntity, "POST", route+"move", map[string]string{"player_id": replacement, "list": "main"}, nil)
	ann.want(http.StatusNoContent, "POST", route+"injury-swap", swap, nil)
	var franchise struct {
		Rosters []struct {
			Overage int
			Players []struct {
				PlayerID string `json:"player_id"`
				List     string
				Locked   bool `json:"injury_reserve_locked"`
			}
		}
	}
	ann.want(http.StatusOK, "GET", "/api/franchises/"+slug, nil, &franchise)
	if len(franchise.Rosters) != 1 || franchise.Rosters[0].Overage != 0 {
		t.Fatalf("roster = %+v", franchise)
	}
	for _, p := range franchise.Rosters[0].Players {
		if p.PlayerID == injured && (p.List != "reserve" || !p.Locked) {
			t.Fatalf("injured = %+v", p)
		}
		if p.PlayerID == replacement && p.List != "main" {
			t.Fatalf("replacement = %+v", p)
		}
	}
	// Recovery and force do not lift the injury-season lock.
	pool.Exec(ctx, `update players set injury_designation='' where id=$1`, injured)
	ann.want(http.StatusUnprocessableEntity, "POST", route+"move", map[string]any{"player_id": injured, "list": "main", "force": true}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", route+"set", map[string]any{"reserve": []string{replacement}}, nil)
	// Dropping/re-adding cannot reset the lock.
	ann.want(http.StatusNoContent, "POST", route+"drop", map[string]string{"player_id": injured}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", route+"add", map[string]any{"player_id": injured, "list": "main", "force": true}, nil)
	ann.want(http.StatusNoContent, "POST", route+"add", map[string]any{"player_id": injured, "list": "reserve", "force": true}, nil)
	// Ownership transfers preserve the same season restriction.
	var bob string
	for _, f := range view.Franchises {
		if f.Name == "Bob" {
			bob = f.ID
		}
	}
	if _, err := pool.Exec(ctx, `update roster_entries set franchise_id=$1, acquired_via='trade' where league_id=$2 and player_id=$3`, bob, league, injured); err != nil {
		t.Fatal(err)
	}
	ann.want(http.StatusUnprocessableEntity, "POST", route+"move", map[string]any{"player_id": injured, "list": "main", "force": true, "franchise_id": bob}, nil)
	pool.Exec(ctx, `update roster_entries set franchise_id=$1 where league_id=$2 and player_id=$3`, owner, league, injured)
	// Once the season ends, the player may return normally (here using force for capacity).
	pool.Exec(ctx, `update seasons set status='complete' where id=$1`, season)
	ann.want(http.StatusNoContent, "POST", route+"move", map[string]any{"player_id": injured, "list": "main", "force": true}, nil)
}
