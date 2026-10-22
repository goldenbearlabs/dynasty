package web_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
)

type inboxItem struct {
	ID                      int64 `json:"id"`
	Kind, Title, Body, Href string
	ReadAt                  *string `json:"read_at"`
	WaiverUntil             *string `json:"waiver_until"`
	CurrentWaiverUntil      *string `json:"current_waiver_until"`
}
type inboxPage struct {
	Notifications []inboxItem
	Total         int
}
type inboxSummary struct {
	Unread   int
	LatestID int64 `json:"latest_id"`
}

func TestInboxAndTradePrivacy(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "inbox_test"
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note=$1`, marker) })
	server, registry := startServer(t, pool)
	ann, bob, cy, guest := newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	rules := nhl.Defaults
	rules.Roster.Main = 10
	rules.Lineup.Slots = nil
	rules.Trades.Approval = "commissioner"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Inbox", Settings: dynastySettings(10), Leagues: []dynasty.LeagueSetup{{Competition: "nhl", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}, {Name: "Cy", ManagerName: "Cy"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"}}, nil)
	var invites []struct {
		Name        string
		InviteToken string `json:"invite_token"`
	}
	ann.want(200, "GET", "/api/admin/franchises", nil, &invites)
	for _, i := range invites {
		if i.Name == "Bob" {
			bob.claim(i.InviteToken, "bob@example.com")
		}
		if i.Name == "Cy" {
			cy.claim(i.InviteToken, "cy@example.com")
		}
	}
	var dynastyView struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Name string }
	}
	ann.want(200, "GET", "/api/dynasty", nil, &dynastyView)
	league := dynastyView.Leagues[0].ID
	teams := map[string]string{}
	for _, f := range dynastyView.Franchises {
		teams[f.Name] = f.ID
	}
	ann.want(201, "POST", "/api/admin/players", []players.NewPlayer{{Competition: "nhl", FullName: "Inbox One", Status: "active", Note: marker}, {Competition: "nhl", FullName: "Inbox Two", Status: "active", Note: marker}}, nil)
	var playerPage struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(200, "GET", "/api/players?q=Inbox", nil, &playerPage)
	ids := map[string]string{}
	for _, p := range playerPage.Players {
		ids[p.FullName] = p.ID
	}
	for _, p := range []struct{ name, owner string }{{"Inbox One", "Bob"}, {"Inbox Two", "Cy"}} {
		ann.want(204, "POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": ids[p.name], "list": "main", "franchise_id": teams[p.owner], "force": true}, nil)
	}
	look := func(b *browser, query string) inboxPage {
		t.Helper()
		var page inboxPage
		b.want(200, "GET", "/api/notifications"+query, nil, &page)
		return page
	}
	summary := func(b *browser) inboxSummary {
		t.Helper()
		var s inboxSummary
		b.want(200, "GET", "/api/notifications/summary", nil, &s)
		return s
	}
	guest.want(401, "GET", "/api/notifications", nil, nil)
	guest.want(401, "GET", "/api/notifications/summary", nil, nil)
	for _, b := range []*browser{ann, bob, cy} {
		if page := look(b, ""); page.Total != 2 {
			t.Fatalf("expected two commissioner roster notifications: %+v", page)
		}
		b.want(204, "POST", "/api/notifications/read", map[string]any{"through_id": summary(b).LatestID}, nil)
	}
	target := map[string]any{"league_id": league, "player_id": ids["Inbox Two"], "on": true}
	bob.want(204, "PUT", "/api/trade-market/targets", target, nil)
	bob.want(204, "PUT", "/api/trade-market/targets", target, nil)
	targets := look(cy, "?kind=target&unread=true")
	if targets.Total != 1 || !strings.Contains(targets.Notifications[0].Title, "Bob") {
		t.Fatalf("target notification: %+v", targets)
	}
	if look(ann, "?kind=target").Total != 0 || look(bob, "?kind=target").Total != 0 {
		t.Fatal("target leaked to another manager or duplicated")
	}
	targetID := strconv.FormatInt(targets.Notifications[0].ID, 10)
	ann.want(404, "PATCH", "/api/notifications/"+targetID, map[string]bool{"read": true}, nil)
	cy.want(204, "PATCH", "/api/notifications/"+targetID, map[string]bool{"read": true}, nil)
	if look(cy, "?kind=target&unread=true").Total != 0 {
		t.Fatal("read notification still unread")
	}
	cy.want(204, "PATCH", "/api/notifications/"+targetID, map[string]bool{"read": false}, nil)
	if look(cy, "?kind=target&unread=true").Total != 1 {
		t.Fatal("mark unread failed")
	}
	offer := map[string]any{"note": "PRIVATE TERMS", "items": []map[string]string{
		{"from_franchise_id": teams["Bob"], "to_franchise_id": teams["Cy"], "league_id": league, "player_id": ids["Inbox One"]},
		{"from_franchise_id": teams["Cy"], "to_franchise_id": teams["Bob"], "league_id": league, "player_id": ids["Inbox Two"]}}}
	var trade struct{ ID string }
	bob.want(201, "POST", "/api/trades", offer, &trade)
	if look(cy, "?kind=trade").Total != 1 || look(bob, "?kind=trade").Total != 0 || look(ann, "?kind=trade").Total != 0 {
		t.Fatal("trade notification recipients incorrect")
	}
	countTrades := func(b *browser) int {
		t.Helper()
		var trades []tradeView
		b.want(200, "GET", "/api/trades", nil, &trades)
		return len(trades)
	}
	if countTrades(ann) != 0 || countTrades(guest) != 0 || countTrades(bob) != 1 || countTrades(cy) != 1 {
		t.Fatal("unaccepted offer leaked, including to commissioner")
	}
	cy.want(204, "POST", "/api/trades/"+trade.ID+"/accept", nil, nil)
	if countTrades(ann) != 1 || countTrades(guest) != 1 {
		t.Fatal("accepted trade should be public before commissioner approval")
	}
	ann.want(204, "POST", "/api/admin/trades/"+trade.ID+"/approve", nil, nil)
	// An unaccepted offer remains private after being declined.
	secondOffer := map[string]any{"note": "DECLINED PRIVATE TERMS", "items": []map[string]string{
		{"from_franchise_id": teams["Bob"], "to_franchise_id": teams["Cy"], "league_id": league, "player_id": ids["Inbox Two"]},
		{"from_franchise_id": teams["Cy"], "to_franchise_id": teams["Bob"], "league_id": league, "player_id": ids["Inbox One"]}}}
	var second struct{ ID string }
	bob.want(201, "POST", "/api/trades", secondOffer, &second)
	cy.want(204, "POST", "/api/trades/"+second.ID+"/reject", nil, nil)
	// The proposer hears who turned the offer down; the decliner is not told about themselves.
	if declined := look(bob, "?kind=trade"); declined.Total != 1 || !strings.Contains(declined.Notifications[0].Title, "Cy declined your trade") {
		t.Fatalf("decline notification: %+v", declined)
	}
	if look(cy, "?kind=trade").Total != 2 {
		t.Fatal("decliner was notified of their own decline")
	}
	if countTrades(ann) != 1 || countTrades(guest) != 1 || countTrades(bob) != 2 {
		t.Fatal("declined unaccepted trade leaked")
	}
	bob.want(204, "POST", "/api/leagues/"+league+"/roster/drop", map[string]string{"player_id": ids["Inbox Two"]}, nil)
	page := look(cy, "?kind=roster")
	dropped := page.Notifications[0]
	if !strings.Contains(dropped.Title, "dropped Inbox Two") || dropped.WaiverUntil == nil || dropped.CurrentWaiverUntil == nil || !strings.HasPrefix(dropped.Href, "/waivers") {
		t.Fatalf("missing waiver notification: %+v", dropped)
	}
	before := summary(cy)
	// An invalid add must neither modify a roster nor produce any notification.
	bob.want(422, "POST", "/api/leagues/"+league+"/roster/add", map[string]string{"player_id": ids["Inbox Two"], "list": "main"}, nil)
	if summary(cy) != before {
		t.Fatal("failed roster move produced notification")
	}
	// Even an already-inserted event is removed when its enclosing write rolls back.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `insert into transactions(dynasty_id,league_id,franchise_id,player_id,kind,detail) select dynasty_id,id,$2::uuid,$3::uuid,'move','{}' from leagues where id=$1`, league, teams["Bob"], ids["Inbox One"])
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if summary(cy) != before {
		t.Fatal("rolled-back move produced notification")
	}
	// Mark-all only includes entries already observed; a newly-arriving event stays unread.
	bob.want(204, "PUT", "/api/trade-market/targets", map[string]any{"league_id": league, "player_id": ids["Inbox One"], "on": true}, nil)
	cy.want(204, "POST", "/api/notifications/read", map[string]any{"through_id": before.LatestID}, nil)
	if summary(cy).Unread != 1 {
		t.Fatalf("mark-all swallowed new notification: %+v", summary(cy))
	}
	if _, err = pool.Exec(ctx, `delete from waivers where league_id=$1 and player_id=$2`, league, ids["Inbox Two"]); err != nil {
		t.Fatal(err)
	}
	if latest := look(cy, "?kind=roster").Notifications[0]; latest.CurrentWaiverUntil != nil || latest.WaiverUntil == nil {
		t.Fatal("historical waiver notification did not reflect current availability")
	}
}
