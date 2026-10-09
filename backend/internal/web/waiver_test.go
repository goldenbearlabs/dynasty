package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/waiver"
)

// TestWaiverFlow covers a dropped player's time on waivers under both kinds
// of claim, and the weekly limit on acquisitions.
func TestWaiverFlow(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "waiver_test"
	removePlayers := func() { pool.Exec(ctx, `delete from players where note = $1`, marker) }
	removePlayers()
	t.Cleanup(func() { dbtest.Reset(t, pool); removePlayers() })

	server, registry := startServer(t, pool)
	waivers := waiver.NewService(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ann, bob, cy := newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL)

	// --- a league with rolling waivers and two acquisitions a week --------
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Lineup.Slots = nil
	rules.FreeAgency.NewEntrantsDraftOnly = false
	rules.FreeAgency.WeeklyLimit = 2
	rules.Waivers.Mode, rules.Waivers.Days = "rolling", 2
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name:    "Waiver Test",
		Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{
			{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}, {Name: "Cy", ManagerName: "Cy"},
		},
		Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)

	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Name, Slug string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := "/api/leagues/" + view.Leagues[0].ID
	team, slug := map[string]string{}, map[string]string{}
	for _, f := range view.Franchises {
		team[f.Name], slug[f.Name] = f.ID, f.Slug
	}
	var invites []struct {
		Name        string
		InviteToken string `json:"invite_token"`
	}
	ann.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	for _, invite := range invites {
		switch invite.Name {
		case "Bob":
			bob.claim(invite.InviteToken, "bob@example.com")
		case "Cy":
			cy.claim(invite.InviteToken, "cy@example.com")
		}
	}

	var fresh []players.NewPlayer
	for _, name := range []string{"Zw One", "Zw Two", "Zw Three", "Zw Four", "Zw Five"} {
		fresh = append(fresh, players.NewPlayer{Competition: "nba", FullName: name, Status: "active", Note: marker})
	}
	ann.want(http.StatusCreated, "POST", "/api/admin/players", fresh, nil)
	type listed struct {
		ID          string
		FullName    string  `json:"full_name"`
		WaiverUntil *string `json:"waiver_until"`
		OwnerName   string  `json:"owner_name"`
	}
	pool5 := func() map[string]listed {
		t.Helper()
		var page struct{ Players []listed }
		ann.want(http.StatusOK, "GET", "/api/players?q=Zw+", nil, &page)
		byName := map[string]listed{}
		for _, p := range page.Players {
			byName[p.FullName] = p
		}
		return byName
	}
	id := map[string]string{}
	for name, p := range pool5() {
		id[name] = p.ID
	}
	move := func(b *browser, want int, action, player string, extra map[string]any) {
		t.Helper()
		body := map[string]any{"player_id": id[player], "list": "main"}
		for k, v := range extra {
			body[k] = v
		}
		b.want(want, "POST", league+"/roster/"+action, body, nil)
	}
	place := func(player, franchise string) {
		t.Helper()
		move(ann, http.StatusNoContent, "add", player, map[string]any{"franchise_id": team[franchise], "force": true})
	}
	claim := func(b *browser, want int, player string, bid int, drop string) {
		t.Helper()
		body := map[string]any{"player_id": id[player], "bid": bid}
		if drop != "" {
			body["drop_player_id"] = id[drop]
		}
		b.want(want, "POST", league+"/waivers/claims", body, nil)
	}
	type waiverView struct {
		Acquisitions int
		Players      []struct {
			FullName string `json:"full_name"`
		}
		Standings []struct {
			Name       string
			BudgetLeft int `json:"budget_left"`
		}
		Claims []struct {
			ID, Status, Reason string
			PlayerName         string `json:"player_name"`
		}
	}
	see := func(b *browser) waiverView {
		t.Helper()
		var v waiverView
		b.want(http.StatusOK, "GET", league+"/waivers", nil, &v)
		return v
	}
	order := func() string {
		names := ""
		for _, s := range see(ann).Standings {
			names += s.Name + " "
		}
		return names
	}
	// clear ends a player's time on waivers and settles the claims for him.
	clear := func(player string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `update waivers set clears_at = now() - interval '1 second' where player_id = $1`, id[player]); err != nil {
			t.Fatal(err)
		}
		if err := waivers.Process(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// --- a dropped player can only be claimed ------------------------------
	claim(bob, http.StatusUnprocessableEntity, "Zw One", 0, "") // a free agent: nothing to claim
	place("Zw One", "Ann")
	move(ann, http.StatusNoContent, "drop", "Zw One", nil)
	if p := pool5()["Zw One"]; p.WaiverUntil == nil || p.OwnerName != "" {
		t.Fatalf("dropped player = %+v, want him on waivers and unowned", p)
	}
	move(bob, http.StatusUnprocessableEntity, "add", "Zw One", nil)
	claim(bob, http.StatusNoContent, "Zw One", 0, "")
	claim(cy, http.StatusNoContent, "Zw One", 0, "")
	claim(cy, http.StatusNoContent, "Zw One", 0, "") // claiming again replaces the claim
	if v := see(cy); len(v.Claims) != 1 || len(v.Players) != 1 || len(see(ann).Claims) != 0 {
		t.Fatalf("Cy sees %d claims and %d players on waivers, want 1 and 1, and Ann must see no claims", len(v.Claims), len(v.Players))
	}

	if err := waivers.Process(ctx); err != nil { // not due yet: nothing happens
		t.Fatal(err)
	}
	if pool5()["Zw One"].WaiverUntil == nil {
		t.Fatal("player left waivers before his time was up")
	}
	if got := order(); got != "Ann Bob Cy " {
		t.Fatalf("waiver order = %q before any claim is won", got)
	}

	clear("Zw One")
	if p := pool5()["Zw One"]; p.OwnerName != "Bob" || p.WaiverUntil != nil {
		t.Fatalf("after waivers = %+v, want him on Bob's roster", p)
	}
	if c := see(cy).Claims[0]; c.Status != "lost" || c.Reason == "" {
		t.Errorf("Cy's claim = %+v, want lost with a reason", c)
	}
	if got := order(); got != "Ann Cy Bob " {
		t.Errorf("waiver order = %q, want the winner moved to the back", got)
	}

	// --- the weekly limit counts claims and free agent adds ----------------
	move(bob, http.StatusNoContent, "add", "Zw Two", nil)
	if n := see(bob).Acquisitions; n != 2 {
		t.Errorf("Bob has made %d acquisitions, want 2", n)
	}
	move(bob, http.StatusUnprocessableEntity, "add", "Zw Three", nil)
	move(cy, http.StatusNoContent, "add", "Zw Three", nil)

	// --- FAAB: the highest bid that can be carried out wins ----------------
	var current struct {
		Leagues []struct{ Settings map[string]any }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &current)
	faab := current.Leagues[0].Settings
	faab["waivers"] = map[string]any{"mode": "faab", "days": 1, "budget": 100}
	ann.want(http.StatusNoContent, "PUT", league+"/settings", faab, nil)

	place("Zw Four", "Cy")
	place("Zw Five", "Ann")
	move(cy, http.StatusNoContent, "drop", "Zw Four", nil)
	claim(cy, http.StatusUnprocessableEntity, "Zw Four", 150, "")       // more than the budget
	claim(ann, http.StatusUnprocessableEntity, "Zw Four", 20, "Zw Two") // Zw Two is Bob's to drop
	claim(bob, http.StatusNoContent, "Zw Four", 60, "")                 // highest, but Bob is at his weekly limit
	claim(cy, http.StatusNoContent, "Zw Four", 20, "")
	claim(ann, http.StatusNoContent, "Zw Four", 20, "Zw Five") // ties Cy, and is ahead of him in the order
	cy.want(http.StatusNoContent, "DELETE", "/api/waivers/claims/"+see(cy).Claims[0].ID, nil, nil)
	claim(cy, http.StatusNoContent, "Zw Four", 20, "")

	clear("Zw Four")
	after := pool5()
	if after["Zw Four"].OwnerName != "Ann" {
		t.Fatalf("Zw Four went to %q, want Ann", after["Zw Four"].OwnerName)
	}
	if p := after["Zw Five"]; p.OwnerName != "" || p.WaiverUntil == nil {
		t.Errorf("the player dropped for the claim = %+v, want him on waivers", p)
	}
	if c := see(bob).Claims[0]; c.Status != "lost" {
		t.Errorf("Bob's claim over the weekly limit = %+v, want lost", c)
	}
	for _, s := range see(ann).Standings {
		if want := map[string]int{"Ann": 80, "Bob": 100, "Cy": 100}[s.Name]; s.BudgetLeft != want {
			t.Errorf("%s has %d left, want %d", s.Name, s.BudgetLeft, want)
		}
	}
	if got := order(); got != "Cy Bob Ann " {
		t.Errorf("waiver order = %q after Ann's win", got)
	}
}
