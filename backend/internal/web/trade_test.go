package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
)

type tradeView struct {
	ID     string
	Status string
	Items  []struct {
		PlayerName string `json:"player_name"`
		DraftName  string `json:"draft_name"`
	}
}

// TestTradeFlow covers a cross-league trade with a draft pick from proposal
// to reversal: who may see and answer an offer, the roster limits at
// acceptance, commissioner approval, and the cases that must be refused.
func TestTradeFlow(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "trade_test"
	removePlayers := func() { pool.Exec(ctx, `delete from players where note = $1`, marker) }
	removePlayers()
	t.Cleanup(func() { dbtest.Reset(t, pool); removePlayers() })

	server, registry := startServer(t, pool)
	ann, bob, cy, stranger := newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL)

	// --- a dynasty where an NBA main roster holds two ---------------------
	nba, _ := registry.Get("nba")
	nhl, _ := registry.Get("nhl")
	tight := nba.Defaults
	tight.Roster.Main, tight.Roster.Reserve = 2, 1
	tight.Lineup.Slots = nil
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Trade Test",
		Leagues: []dynasty.LeagueSetup{
			{Competition: "nba", Settings: tight},
			{Competition: "nhl", Settings: nhl.Defaults},
		},
		Franchises: []dynasty.FranchiseSetup{
			{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}, {Name: "Cy", ManagerName: "Cy"},
		},
		Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)

	var view struct {
		Leagues []struct {
			ID, Competition string
			Settings        json.RawMessage
		}
		Franchises []struct{ ID, Name, Slug string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	nbaLeague, nhlLeague := view.Leagues[0], view.Leagues[1]
	team := map[string]string{}
	for _, f := range view.Franchises {
		team[f.Name] = f.ID
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

	// --- players, placed on rosters by the commissioner -------------------
	var pool7 []players.NewPlayer
	for _, name := range []string{"Zt Guard One", "Zt Guard Two", "Zt Guard Three"} {
		pool7 = append(pool7, players.NewPlayer{Competition: "nba", FullName: name, Status: "active", Note: marker})
	}
	for _, name := range []string{"Zt Skater One", "Zt Skater Two"} {
		pool7 = append(pool7, players.NewPlayer{Competition: "nhl", FullName: name, Status: "active", Note: marker})
	}
	ann.want(http.StatusCreated, "POST", "/api/admin/players", pool7, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zt+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	place := func(league, player, franchise string) {
		t.Helper()
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add",
			map[string]any{"player_id": id[player], "list": "main", "franchise_id": team[franchise], "force": true}, nil)
	}
	place(nbaLeague.ID, "Zt Guard One", "Ann")
	place(nbaLeague.ID, "Zt Guard Two", "Ann") // Ann's NBA main roster is now full
	place(nbaLeague.ID, "Zt Guard Three", "Bob")
	place(nhlLeague.ID, "Zt Skater One", "Ann")
	place(nhlLeague.ID, "Zt Skater Two", "Bob")

	// --- future drafts give everyone picks to trade ------------------------
	var made struct{ Created int }
	bob.want(http.StatusForbidden, "POST", "/api/admin/drafts/future", nil, nil)
	// They were created with the dynasty: three years ahead for each of two leagues.
	var listed []struct{ Kind string }
	ann.want(http.StatusOK, "GET", "/api/drafts", nil, &listed)
	if len(listed) != 6 {
		t.Fatalf("a new dynasty has %d drafts on the books, want 6", len(listed))
	}
	ann.want(http.StatusCreated, "POST", "/api/admin/drafts/future", nil, &made)
	if made.Created != 0 {
		t.Fatalf("asking again created %d more drafts, want none", made.Created)
	}

	type heldPick struct {
		ID                  string
		Round               int
		Year                int
		DraftID             string   `json:"draft_id"`
		DraftName           string   `json:"draft_name"`
		OriginalFranchiseID string   `json:"original_franchise_id"`
		Competitions        []string `json:"competitions"`
	}
	holdings := func(b *browser, slug string) (onRoster map[string]bool, picks []heldPick) {
		t.Helper()
		var detail struct {
			Rosters []struct {
				Players []struct {
					FullName string `json:"full_name"`
				}
			}
			Picks []heldPick
		}
		b.want(http.StatusOK, "GET", "/api/franchises/"+slug, nil, &detail)
		onRoster = map[string]bool{}
		for _, r := range detail.Rosters {
			for _, p := range r.Players {
				onRoster[p.FullName] = true
			}
		}
		return onRoster, detail.Picks
	}
	nextYear := time.Now().Year() + 1
	pickOf := func(picks []heldPick, sport string, year int) heldPick {
		t.Helper()
		for _, p := range picks {
			if p.Round == 1 && p.Year == year && p.Competitions[0] == sport {
				return p
			}
		}
		t.Fatalf("no round 1 %s pick for %d among %+v", sport, year, picks)
		return heldPick{}
	}
	_, bobPicks := holdings(bob, "bob")
	bobFirst := pickOf(bobPicks, "nba", nextYear)

	// --- proposing ---------------------------------------------------------
	player := func(from, to, league, name string) map[string]any {
		return map[string]any{"from_franchise_id": team[from], "to_franchise_id": team[to], "league_id": league, "player_id": id[name]}
	}
	pick := func(from, to, pickID string) map[string]any {
		return map[string]any{"from_franchise_id": team[from], "to_franchise_id": team[to], "draft_pick_id": pickID}
	}
	offer := func(items ...map[string]any) map[string]any {
		return map[string]any{"note": "", "items": items}
	}

	// Bob sends an NBA player and an NBA first for Ann's NHL player.
	blockbuster := offer(
		player("Bob", "Ann", nbaLeague.ID, "Zt Guard Three"),
		pick("Bob", "Ann", bobFirst.ID),
		player("Ann", "Bob", nhlLeague.ID, "Zt Skater One"),
	)
	stranger.want(http.StatusUnauthorized, "POST", "/api/trades", blockbuster, nil)
	cy.want(http.StatusUnprocessableEntity, "POST", "/api/trades", blockbuster, nil) // not his trade to propose
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trades", offer(), nil)
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trades",
		offer(player("Bob", "Ann", nbaLeague.ID, "Zt Guard One")), nil) // not his player to give
	var first tradeView
	bob.want(http.StatusCreated, "POST", "/api/trades", blockbuster, &first)

	// An unaccepted offer is private to its parties.
	count := func(b *browser) int {
		t.Helper()
		var trades []tradeView
		b.want(http.StatusOK, "GET", "/api/trades", nil, &trades)
		return len(trades)
	}
	if count(bob) != 1 || count(cy) != 0 || count(stranger) != 0 {
		t.Fatalf("the open offer is visible to bob=%d cy=%d stranger=%d, want 1, 0, 0", count(bob), count(cy), count(stranger))
	}

	// --- accepting ---------------------------------------------------------
	answer := func(tradeID, action string) string { return "/api/trades/" + tradeID + "/" + action }
	cy.want(http.StatusUnprocessableEntity, "POST", answer(first.ID, "accept"), nil, nil) // not a party

	// Ann's NBA roster is full, so she cannot take a third player.
	ann.want(http.StatusUnprocessableEntity, "POST", answer(first.ID, "accept"), nil, nil)
	if onRoster, _ := holdings(ann, "ann"); onRoster["Zt Guard Three"] {
		t.Fatal("a refused trade still moved a player")
	}
	// The recipient can release a player as part of acceptance, atomically.
	drop := func(owner, league, name, list string) map[string]any {
		return map[string]any{"franchise_id": team[owner], "league_id": league, "player_id": id[name], "list": list}
	}
	ann.want(http.StatusUnprocessableEntity, "POST", answer(first.ID, "accept"), map[string]any{"drops": []any{drop("Bob", nbaLeague.ID, "Zt Guard Three", "main")}}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", answer(first.ID, "accept"), map[string]any{"drops": []any{drop("Ann", nbaLeague.ID, "Zt Guard Two", "reserve")}}, nil)
	ann.want(http.StatusNoContent, "POST", answer(first.ID, "accept"), map[string]any{"drops": []any{drop("Ann", nbaLeague.ID, "Zt Guard Two", "main")}}, nil)
	if onRoster, _ := holdings(ann, "ann"); onRoster["Zt Guard Two"] {
		t.Fatal("the conditional release did not execute")
	}
	var waived bool
	if err := pool.QueryRow(ctx, `select exists (select 1 from waivers where league_id = $1 and player_id = $2)`, nbaLeague.ID, id["Zt Guard Two"]).Scan(&waived); err != nil || !waived {
		t.Fatalf("conditional release must follow normal waivers: waived=%v err=%v", waived, err)
	}
	ann.want(http.StatusUnprocessableEntity, "POST", answer(first.ID, "accept"), nil, nil) // already done

	annHas, annPicks := holdings(ann, "ann")
	bobHas, _ := holdings(bob, "bob")
	if !annHas["Zt Guard Three"] || annHas["Zt Skater One"] || !bobHas["Zt Skater One"] || bobHas["Zt Guard Three"] {
		t.Fatalf("after the trade Ann holds %v and Bob holds %v", annHas, bobHas)
	}
	// The pick is Ann's now, and still remembers it was Bob's.
	traded := false
	for _, p := range annPicks {
		traded = traded || (p.ID == bobFirst.ID && p.OriginalFranchiseID == team["Bob"])
	}
	if !traded {
		t.Fatalf("Ann's picks %+v do not include Bob's first", annPicks)
	}
	if count(cy) != 1 || count(stranger) != 1 {
		t.Error("an executed trade should be public")
	}

	// A player who has moved cannot be offered by his old franchise.
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trades",
		offer(player("Bob", "Cy", nbaLeague.ID, "Zt Guard Three")), nil)

	// --- rejecting and cancelling ------------------------------------------
	swap := offer(player("Ann", "Bob", nbaLeague.ID, "Zt Guard One"), player("Bob", "Ann", nhlLeague.ID, "Zt Skater Two"))
	var second, third tradeView
	ann.want(http.StatusCreated, "POST", "/api/trades", swap, &second)
	bob.want(http.StatusUnprocessableEntity, "POST", answer(second.ID, "cancel"), nil, nil) // only the proposer cancels
	bob.want(http.StatusNoContent, "POST", answer(second.ID, "reject"), nil, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", answer(second.ID, "cancel"), nil, nil) // already closed
	ann.want(http.StatusCreated, "POST", "/api/trades", swap, &third)
	ann.want(http.StatusNoContent, "POST", answer(third.ID, "cancel"), nil, nil)
	bob.want(http.StatusUnprocessableEntity, "POST", answer(third.ID, "accept"), nil, nil)

	// --- commissioner approval ---------------------------------------------
	var nhlRules map[string]any
	json.Unmarshal(nhlLeague.Settings, &nhlRules)
	nhlRules["trades"].(map[string]any)["approval"] = "commissioner"
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+nhlLeague.ID+"/settings", nhlRules, nil)

	rule := func(tradeID, action string) string { return "/api/admin/trades/" + tradeID + "/" + action }
	var fourth, fifth tradeView
	bob.want(http.StatusCreated, "POST", "/api/trades", offer(player("Bob", "Cy", nhlLeague.ID, "Zt Skater One")), &fourth)
	ann.want(http.StatusUnprocessableEntity, "POST", rule(fourth.ID, "approve"), nil, nil) // Cy has not accepted yet
	cy.want(http.StatusNoContent, "POST", answer(fourth.ID, "accept"), nil, nil)
	if onRoster, _ := holdings(cy, "cy"); onRoster["Zt Skater One"] {
		t.Fatal("a trade needing approval executed without it")
	}
	cy.want(http.StatusForbidden, "POST", rule(fourth.ID, "approve"), nil, nil)
	ann.want(http.StatusNoContent, "POST", rule(fourth.ID, "approve"), nil, nil)
	if onRoster, _ := holdings(cy, "cy"); !onRoster["Zt Skater One"] {
		t.Fatal("an approved trade did not execute")
	}
	bob.want(http.StatusCreated, "POST", "/api/trades", offer(player("Bob", "Cy", nhlLeague.ID, "Zt Skater Two")), &fifth)
	ann.want(http.StatusNoContent, "POST", rule(fifth.ID, "veto"), nil, nil)
	cy.want(http.StatusUnprocessableEntity, "POST", answer(fifth.ID, "accept"), nil, nil)

	// --- reversing ---------------------------------------------------------
	// The first trade cannot be reversed while one of its players has moved on.
	cy.want(http.StatusForbidden, "POST", rule(first.ID, "reverse"), nil, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", rule(first.ID, "reverse"), nil, nil)
	ann.want(http.StatusNoContent, "POST", rule(fourth.ID, "reverse"), nil, nil) // the skater returns to Bob
	ann.want(http.StatusNoContent, "POST", rule(first.ID, "reverse"), nil, nil)
	annHas, annPicks = holdings(ann, "ann")
	bobHas, bobPicks = holdings(bob, "bob")
	if !annHas["Zt Skater One"] || !bobHas["Zt Guard Three"] || pickOf(bobPicks, "nba", nextYear).ID != bobFirst.ID {
		t.Fatalf("after reversing, Ann holds %v and Bob holds %v with picks %+v", annHas, bobHas, bobPicks)
	}

	// --- picks and drafts --------------------------------------------------
	// A pick in an open offer protects its draft from deletion.
	later := pickOf(bobPicks, "nba", nextYear+1)
	var sixth tradeView
	bob.want(http.StatusCreated, "POST", "/api/trades", offer(pick("Bob", "Cy", later.ID)), &sixth)
	ann.want(http.StatusUnprocessableEntity, "DELETE", "/api/admin/drafts/"+later.DraftID, nil, nil)
	bob.want(http.StatusNoContent, "POST", answer(sixth.ID, "cancel"), nil, nil)

	// Once a draft starts, its picks can no longer be traded.
	ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+bobFirst.DraftID+"/start", nil, nil)
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trades", offer(pick("Bob", "Cy", bobFirst.ID)), nil)

	// --- the deadline ------------------------------------------------------
	var nbaRules map[string]any
	json.Unmarshal(nbaLeague.Settings, &nbaRules)
	nbaRules["trades"].(map[string]any)["deadline"] = time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+nbaLeague.ID+"/settings", nbaRules, nil)
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/trades",
		offer(player("Bob", "Cy", nbaLeague.ID, "Zt Guard Three")), nil)
	bob.want(http.StatusCreated, "POST", "/api/trades",
		offer(player("Bob", "Cy", nhlLeague.ID, "Zt Skater Two")), nil) // other leagues are unaffected

	// --- the feed ----------------------------------------------------------
	var activity []struct {
		Kind      string
		DraftName string `json:"draft_name"`
	}
	ann.want(http.StatusOK, "GET", "/api/activity", nil, &activity)
	trades, withPick := 0, 0
	for _, a := range activity {
		if a.Kind == "trade" {
			trades++
			if a.DraftName != "" {
				withPick++
			}
		}
	}
	// Three assets moved and came back, one moved and came back: 8 entries, 2 of them the pick.
	if trades != 8 || withPick != 2 {
		t.Errorf("activity has %d trade entries, %d for picks; want 8 and 2", trades, withPick)
	}

	// Reserve capacity is independent of main capacity, including a 2-for-1 offer.
	nbaRules["trades"].(map[string]any)["deadline"] = ""
	nbaRules["trades"].(map[string]any)["approval"] = "commissioner"
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+nbaLeague.ID+"/settings", nbaRules, nil)
	var prospects []players.NewPlayer
	for _, name := range []string{"Zt Reserve A", "Zt Reserve B", "Zt Reserve C", "Zt Reserve D"} {
		prospects = append(prospects, players.NewPlayer{Competition: "nba", FullName: name, Status: "prospect", Note: marker})
	}
	ann.want(http.StatusCreated, "POST", "/api/admin/players", prospects, nil)
	ann.want(http.StatusOK, "GET", "/api/players?q=Zt+Reserve", nil, &page)
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	reserve := func(name, owner string) {
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+nbaLeague.ID+"/roster/add", map[string]any{"player_id": id[name], "franchise_id": team[owner], "list": "reserve", "force": true}, nil)
	}
	reserve("Zt Reserve A", "Ann")
	reserve("Zt Reserve B", "Bob")
	reserve("Zt Reserve C", "Bob") // commissioner may place a roster over its limit
	// Ann gives a main player for two reserves. Her existing reserve must be released;
	// even then two incoming reserves exceed a size-one list, so enlarge it to two.
	nbaRules["roster"].(map[string]any)["reserve"] = 2
	ann.want(http.StatusNoContent, "PUT", "/api/leagues/"+nbaLeague.ID+"/settings", nbaRules, nil)
	unequal := offer(player("Ann", "Bob", nbaLeague.ID, "Zt Guard One"), player("Bob", "Ann", nbaLeague.ID, "Zt Reserve B"), player("Bob", "Ann", nbaLeague.ID, "Zt Reserve C"))
	unequal["drops"] = []any{drop("Ann", nbaLeague.ID, "Zt Reserve A", "reserve")}
	var pending tradeView
	ann.want(http.StatusCreated, "POST", "/api/trades", unequal, &pending)
	if held, _ := holdings(ann, "ann"); !held["Zt Reserve A"] {
		t.Fatal("proposing released a player")
	}
	bob.want(http.StatusNoContent, "POST", answer(pending.ID, "accept"), map[string]any{"drops": []any{}}, nil)
	if held, _ := holdings(ann, "ann"); !held["Zt Reserve A"] || held["Zt Reserve B"] {
		t.Fatal("players moved before commissioner approval")
	}
	ann.want(http.StatusNoContent, "POST", rule(pending.ID, "veto"), nil, nil)
	if held, _ := holdings(ann, "ann"); !held["Zt Reserve A"] {
		t.Fatal("vetoed trade released a player")
	}

	ann.want(http.StatusCreated, "POST", "/api/trades", unequal, &pending)
	bob.want(http.StatusNoContent, "POST", answer(pending.ID, "accept"), nil, nil)
	// A stale drop aborts approval, without moving any traded assets.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+nbaLeague.ID+"/roster/drop", map[string]any{"player_id": id["Zt Reserve A"]}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", rule(pending.ID, "approve"), nil, nil)
	if held, _ := holdings(bob, "bob"); !held["Zt Reserve B"] || held["Zt Guard One"] {
		t.Fatal("failed approval partially executed")
	}
	// Removing the stale selection is allowed even after all parties accepted.
	ann.want(http.StatusNoContent, "POST", answer(pending.ID, "drops"), map[string]any{"drops": []any{}}, nil)
	ann.want(http.StatusNoContent, "POST", rule(pending.ID, "approve"), nil, nil)
	if held, _ := holdings(ann, "ann"); !held["Zt Reserve B"] || !held["Zt Reserve C"] {
		t.Fatal("the unequal reserve trade did not execute")
	}

	// Full reserve list: a main drop cannot make space for an incoming reserve.
	reserve("Zt Reserve D", "Bob")
	place(nbaLeague.ID, "Zt Guard Two", "Ann")
	wrongList := offer(player("Bob", "Ann", nbaLeague.ID, "Zt Reserve D"))
	bob.want(http.StatusCreated, "POST", "/api/trades", wrongList, &pending)
	ann.want(http.StatusUnprocessableEntity, "POST", answer(pending.ID, "accept"), map[string]any{"drops": []any{drop("Ann", nbaLeague.ID, "Zt Guard Two", "main")}}, nil)
	ann.want(http.StatusNoContent, "POST", answer(pending.ID, "accept"), map[string]any{"drops": []any{drop("Ann", nbaLeague.ID, "Zt Reserve B", "reserve")}}, nil)
	ann.want(http.StatusNoContent, "POST", rule(pending.ID, "approve"), nil, nil)
	if held, _ := holdings(ann, "ann"); held["Zt Reserve B"] || !held["Zt Reserve D"] {
		t.Fatal("reserve drop did not execute with trade")
	}
}
