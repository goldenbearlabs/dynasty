package web_test

import (
	"context"
	"net/http"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/settings"
)

// TestTradeMarket covers the trade block, wants and targets: who may set
// each, who sees each, and that a player who leaves a roster leaves the
// market with him.
func TestTradeMarket(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "trade_market_test"
	pool.Exec(ctx, `delete from players where note = $1`, marker)
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note = $1`, marker) })

	server, registry := startServer(t, pool)
	ann, bob := newBrowser(t, server.URL), newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	rules := nhl.Defaults
	rules.Roster.Main, rules.Roster.Reserve = 3, 1
	rules.Lineup.Slots = []settings.Slot{{Name: "C", Positions: []string{"C"}, Count: 1}}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Trade Market Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nhl", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var invites []struct {
		Name        string
		InviteToken string `json:"invite_token"`
	}
	ann.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	for _, invite := range invites {
		if invite.Name == "Bob" {
			bob.claim(invite.InviteToken, "bob@example.com")
		}
	}
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Name string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := view.Leagues[0].ID
	franchise := map[string]string{}
	for _, f := range view.Franchises {
		franchise[f.Name] = f.ID
	}

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nhl", FullName: "Ztm Anns", Positions: []string{"C"}, Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Ztm Bobs", Positions: []string{"D"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Ztm+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	roster := "/api/leagues/" + league + "/roster/"
	ann.want(http.StatusNoContent, "POST", roster+"add", map[string]any{"player_id": id["Ztm Anns"], "list": "main", "force": true}, nil)
	ann.want(http.StatusNoContent, "POST", roster+"add", map[string]any{"player_id": id["Ztm Bobs"], "list": "main", "force": true, "franchise_id": franchise["Bob"]}, nil)

	type entry struct {
		ID             string `json:"id"`
		PlayerID       string `json:"player_id"`
		FranchiseID    string `json:"franchise_id"`
		ByFranchiseID  string `json:"by_franchise_id"`
		Note           string
		Positions      []string
		OwnerFranchise string `json:"owner_franchise_id"`
	}
	type market struct{ Block, Wants, Targets, Interest []entry }
	look := func(b *browser) market {
		t.Helper()
		var m market
		b.want(http.StatusOK, "GET", "/api/trade-market", nil, &m)
		return m
	}
	block := func(player string, on bool, note string) map[string]any {
		return map[string]any{"league_id": league, "player_id": id[player], "on": on, "note": note}
	}

	// --- the block: your own players only, seen by everyone -----------------
	ann.want(http.StatusNoContent, "PUT", "/api/trade-market/block", block("Ztm Anns", true, "Looking for a goalie"), nil)
	ann.want(http.StatusUnprocessableEntity, "PUT", "/api/trade-market/block", block("Ztm Bobs", true, ""), nil)
	if m := look(bob); len(m.Block) != 1 || m.Block[0].PlayerID != id["Ztm Anns"] || m.Block[0].Note != "Looking for a goalie" {
		t.Fatalf("Bob sees the block as %+v", m.Block)
	}

	// --- wants: real positions, several to a league, each removed alone ----
	wants := func(positions []string, note string) map[string]any {
		return map[string]any{"league_id": league, "positions": positions, "note": note}
	}
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trade-market/wants", wants([]string{"QB"}, ""), nil)
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trade-market/wants", wants(nil, " "), nil)
	bob.want(http.StatusNoContent, "POST", "/api/trade-market/wants", wants([]string{"C", "C"}, "A top-six centre"), nil)
	bob.want(http.StatusNoContent, "POST", "/api/trade-market/wants", wants([]string{"G"}, "A young goalie"), nil)
	m := look(ann)
	if len(m.Wants) != 2 || m.Wants[0].Note != "A young goalie" || len(m.Wants[1].Positions) != 1 || m.Wants[1].Note != "A top-six centre" {
		t.Fatalf("Ann sees Bob's wants as %+v", m.Wants)
	}
	ann.want(http.StatusUnprocessableEntity, "DELETE", "/api/trade-market/wants/"+m.Wants[0].ID, nil, nil)
	bob.want(http.StatusNoContent, "DELETE", "/api/trade-market/wants/"+m.Wants[0].ID, nil, nil)
	if m := look(ann); len(m.Wants) != 1 || m.Wants[0].Note != "A top-six centre" {
		t.Fatalf("after removing one want: %+v", m.Wants)
	}

	// --- targets: someone else's player, seen by the two of them ------------
	target := func(player string, on bool) map[string]any {
		return map[string]any{"league_id": league, "player_id": id[player], "on": on}
	}
	bob.want(http.StatusUnprocessableEntity, "PUT", "/api/trade-market/targets", target("Ztm Bobs", true), nil)
	bob.want(http.StatusNoContent, "PUT", "/api/trade-market/targets", target("Ztm Anns", true), nil)
	if m := look(bob); len(m.Targets) != 1 || len(m.Interest) != 0 {
		t.Fatalf("Bob's targets = %+v, interest = %+v", m.Targets, m.Interest)
	}
	if m := look(ann); len(m.Interest) != 1 || m.Interest[0].ByFranchiseID != franchise["Bob"] || len(m.Targets) != 0 {
		t.Fatalf("Ann's interest = %+v, targets = %+v", m.Interest, m.Targets)
	}

	// --- a player who leaves the roster leaves the market --------------------
	ann.want(http.StatusNoContent, "POST", roster+"drop", map[string]any{"player_id": id["Ztm Anns"]}, nil)
	if m := look(ann); len(m.Block) != 0 || len(m.Interest) != 0 {
		t.Fatalf("after the drop, block = %+v, interest = %+v", m.Block, m.Interest)
	}
}
