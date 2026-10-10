package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
)

// room is a draft room connection, as a browser tab would hold one.
type room struct {
	t    *testing.T
	conn *websocket.Conn
}

type roomMessage struct {
	Type  string
	Draft struct {
		Status         string
		ClockExpiresAt *time.Time `json:"clock_expires_at"`
	}
	Picks []struct {
		ID                 string
		Position           int
		CurrentFranchiseID string `json:"current_franchise_id"`
		PlayerName         string `json:"player_name"`
		Competition        string
		AutoPicked         bool    `json:"auto_picked"`
		SkippedAt          *string `json:"skipped_at"`
	}
	OnClock  *string  `json:"on_clock_pick_id"`
	AutoPick []string `json:"auto_pick_franchise_ids"`
}

// next waits for the next message from the room.
func (r *room) next() roomMessage {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, raw, err := r.conn.Read(ctx)
	if err != nil {
		r.t.Fatalf("draft room: %v", err)
	}
	var m roomMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		r.t.Fatal(err)
	}
	return m
}

// TestDraftFlow runs a combined startup draft from creation to completion:
// the commissioner's setup, live picks over the websocket, the roster
// limits, the clock with a queue and without one, make-up picks and undo,
// and then each franchise setting its reserve list before the season.
func TestDraftFlow(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "draft_test"
	removePlayers := func() { pool.Exec(ctx, `delete from players where note = $1`, marker) }
	removePlayers()
	t.Cleanup(func() { dbtest.Reset(t, pool); removePlayers() })

	server, registry := startServer(t, pool)
	ann, bob, cy := newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL)

	// --- a small dynasty: NBA rosters hold one main and one reserve, and
	// only a prospect, or a startup pick, may be on reserve ---------------
	nba, _ := registry.Get("nba")
	nhl, _ := registry.Get("nhl")
	tight := nba.Defaults
	tight.Roster.Main, tight.Roster.Reserve, tight.Roster.ReserveEligibility = 1, 1, "prospects_only"
	tight.Lineup.Slots = nil
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Draft Test",
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
		Leagues    []struct{ ID, Competition string }
		Franchises []struct{ ID, Name string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	nbaLeague, nhlLeague := view.Leagues[0].ID, view.Leagues[1].ID
	franchise := map[string]string{}
	for _, f := range view.Franchises {
		franchise[f.Name] = f.ID
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

	// --- a player pool of the test's own ---------------------------------
	var pool5 []players.NewPlayer
	for _, name := range []string{"Zd Guard One", "Zd Guard Two", "Zd Guard Three", "Zd Guard Four", "Zd Guard Five"} {
		pool5 = append(pool5, players.NewPlayer{Competition: "nba", FullName: name, Status: "active", Note: marker})
	}
	for _, name := range []string{"Zd Skater One", "Zd Skater Two", "Zd Skater Three", "Zd Skater Four"} {
		pool5 = append(pool5, players.NewPlayer{Competition: "nhl", FullName: name, Status: "active", Note: marker})
	}
	pool5 = append(pool5, players.NewPlayer{Competition: "nfl", FullName: "Zd Kicker", Status: "active", Note: marker})
	ann.want(http.StatusCreated, "POST", "/api/admin/players", pool5, nil)

	// --- creating the draft ----------------------------------------------
	newDraft := map[string]any{
		"name": "Startup", "kind": "seasonal", "year": 2026,
		"league_ids": []string{nbaLeague, nhlLeague},
		"rounds":     2, "order": "snake",
		"franchise_order":    []string{franchise["Ann"], franchise["Bob"], franchise["Cy"]},
		"pick_clock_seconds": 0,
	}
	bob.want(http.StatusForbidden, "POST", "/api/admin/drafts", newDraft, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/drafts", newDraft, nil) // seasonal cannot combine leagues
	newDraft["kind"], newDraft["rounds"] = "startup", 99
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/drafts", newDraft, nil) // more rounds than roster spots
	newDraft["rounds"] = 2
	var created struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/drafts", newDraft, &created)
	draftURL, adminURL := "/api/drafts/"+created.ID, "/api/admin/drafts/"+created.ID

	// Snake order: Ann, Bob, Cy, then Cy, Bob, Ann.
	var state roomMessage
	ann.want(http.StatusOK, "GET", draftURL, nil, &state)
	var order []string
	for _, p := range state.Picks {
		order = append(order, p.CurrentFranchiseID)
	}
	wantOrder := []string{franchise["Ann"], franchise["Bob"], franchise["Cy"], franchise["Cy"], franchise["Bob"], franchise["Ann"]}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("pick order = %v, want snake %v", order, wantOrder)
	}

	// The commissioner hands Ann's first pick to Bob, as a trade would.
	type slot struct {
		ID       string `json:"id"`
		Round    int    `json:"round"`
		Original string `json:"original_franchise_id"`
		Current  string `json:"current_franchise_id"`
	}
	var slots []slot
	for i, p := range state.Picks {
		slots = append(slots, slot{ID: p.ID, Round: i/3 + 1, Original: p.CurrentFranchiseID, Current: p.CurrentFranchiseID})
	}
	slots[0].Current = franchise["Bob"]
	bob.want(http.StatusForbidden, "PUT", adminURL+"/picks", slots, nil)
	ann.want(http.StatusNoContent, "PUT", adminURL+"/picks", slots, nil)

	// --- the room --------------------------------------------------------
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+draftURL+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	live := &room{t: t, conn: conn}
	if m := live.next(); m.Type != "state" || m.Draft.Status != "scheduled" || len(m.Picks) != 6 || m.OnClock != nil {
		t.Fatalf("first room message = %+v", m)
	}

	bob.want(http.StatusForbidden, "POST", adminURL+"/start", nil, nil)
	ann.want(http.StatusNoContent, "POST", adminURL+"/start", nil, nil)
	if m := live.next(); m.Draft.Status != "live" || m.OnClock == nil || *m.OnClock != state.Picks[0].ID {
		t.Fatalf("after start = %+v", m)
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", adminURL+"/picks", slots, nil) // too late to edit

	// The pool for this draft is every unrostered player in its sports.
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
		Total int
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zd+&draft_id="+created.ID, nil, &page)
	if page.Total != 9 {
		t.Fatalf("draft pool has %d of the test's players, want 9 (the kicker's sport is not in the draft)", page.Total)
	}
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	var kicker struct{ Players []struct{ ID string } }
	ann.want(http.StatusOK, "GET", "/api/players?q=Zd+Kicker", nil, &kicker)

	pick := func(player, list string) map[string]string {
		return map[string]string{"player_id": id[player], "list": list}
	}
	drafted := func(player string) map[string]string { return map[string]string{"player_id": id[player]} }
	pickURL := draftURL + "/pick"

	// Free agency waits for the draft.
	bob.want(http.StatusUnprocessableEntity, "POST", "/api/leagues/"+nbaLeague+"/roster/add", pick("Zd Guard One", "main"), nil)

	// Pick 1 is Bob's now, not Cy's.
	cy.want(http.StatusUnprocessableEntity, "POST", pickURL, drafted("Zd Guard One"), nil)
	bob.want(http.StatusUnprocessableEntity, "POST", pickURL,
		map[string]string{"player_id": kicker.Players[0].ID}, nil) // not a sport in this draft
	bob.want(http.StatusNoContent, "POST", pickURL, drafted("Zd Guard One"), nil)
	if m := live.next(); m.Type != "update" || len(m.Picks) != 1 || m.Picks[0].PlayerName != "Zd Guard One" ||
		m.Picks[0].Competition != "nba" || *m.OnClock != state.Picks[1].ID {
		t.Fatalf("after pick 1 = %+v", m)
	}

	// Pick 2 is Bob's own. A drafted player cannot be drafted again.
	bob.want(http.StatusUnprocessableEntity, "POST", pickURL, drafted("Zd Guard One"), nil)
	bob.want(http.StatusNoContent, "POST", pickURL, drafted("Zd Skater One"), nil) // another sport, same draft
	if m := live.next(); m.Picks[0].Competition != "nhl" {
		t.Fatalf("after pick 2 = %+v", m)
	}

	// --- the clock -------------------------------------------------------
	// Cy is up twice. With a queue the clock picks for him; without, it skips.
	ann.want(http.StatusNoContent, "PUT", adminURL, map[string]int{"pick_clock_seconds": 1}, nil)
	live.next() // the clock change
	cy.want(http.StatusNoContent, "PUT", draftURL+"/queue",
		map[string][]string{"player_ids": {id["Zd Guard One"], id["Zd Guard Two"]}}, nil) // the first is already gone
	ann.want(http.StatusNoContent, "POST", adminURL+"/pause", nil, nil)
	live.next()
	ann.want(http.StatusNoContent, "POST", adminURL+"/resume", nil, nil) // restarts the clock, now one second
	if m := live.next(); m.Draft.ClockExpiresAt == nil {
		t.Fatalf("resumed without a clock: %+v", m)
	}

	if m := live.next(); len(m.Picks) != 1 || m.Picks[0].PlayerName != "Zd Guard Two" || !m.Picks[0].AutoPicked {
		t.Fatalf("clock with a queue = %+v, want an automatic pick of the first available queued player", m)
	}
	var queue []struct{ PlayerID string }
	cy.want(http.StatusOK, "GET", draftURL+"/queue", nil, &queue)
	if len(queue) != 1 {
		t.Errorf("Cy's queue has %d players, want 1: drafted players leave every queue", len(queue))
	}
	cy.want(http.StatusNoContent, "PUT", draftURL+"/queue", map[string][]string{"player_ids": {}}, nil)

	if m := live.next(); len(m.Picks) != 1 || m.Picks[0].SkippedAt == nil || *m.OnClock != state.Picks[4].ID {
		t.Fatalf("clock without a queue = %+v, want pick 4 skipped and pick 5 up", m)
	}
	ann.want(http.StatusNoContent, "PUT", adminURL, map[string]int{"pick_clock_seconds": 0}, nil)
	live.next()

	// --- make-up picks and undo ------------------------------------------
	// Cy makes up his skipped pick while Bob is on the clock.
	cy.want(http.StatusNoContent, "POST", pickURL, drafted("Zd Skater Three"), nil)
	if m := live.next(); m.Picks[0].Position != 4 || *m.OnClock != state.Picks[4].ID {
		t.Fatalf("make-up pick = %+v", m)
	}
	cy.want(http.StatusUnprocessableEntity, "POST", pickURL, drafted("Zd Guard Four"), nil) // nothing left to make up

	bob.want(http.StatusForbidden, "POST", adminURL+"/undo", nil, nil)
	ann.want(http.StatusNoContent, "POST", adminURL+"/undo", nil, nil)
	if m := live.next(); m.Type != "state" || m.Picks[3].PlayerName != "" {
		t.Fatalf("after undo = %+v, want pick 4 open again", m.Picks[3])
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zd+Skater+Three&draft_id="+created.ID, nil, &page)
	if page.Total != 1 {
		t.Error("an undone pick's player did not return to the pool")
	}

	// --- finishing -------------------------------------------------------
	// Undo put pick 4 back on the clock. Cy turns auto pick on with nobody
	// queued: the room sees it, and a few seconds later he is given one of
	// the best players left.
	autoURL := draftURL + "/autopick"
	bob.want(http.StatusForbidden, "PUT", autoURL, map[string]any{"on": true, "franchise_id": franchise["Cy"]}, nil)
	cy.want(http.StatusNoContent, "PUT", autoURL, map[string]any{"on": true}, nil)
	if m := live.next(); len(m.AutoPick) != 1 || m.AutoPick[0] != franchise["Cy"] || m.Draft.ClockExpiresAt == nil {
		t.Fatalf("auto pick turned on = %+v, want Cy listed and a short clock", m)
	}
	if m := live.next(); len(m.Picks) != 1 || m.Picks[0].Position != 4 || !m.Picks[0].AutoPicked || m.Picks[0].PlayerName == "" {
		t.Fatalf("auto pick with an empty queue = %+v, want pick 4 made from the pool", m)
	}
	// Bob's main NBA spot is taken, so his next guard lands on the reserve
	// list. With both lists full the roster rules refuse a third.
	bob.want(http.StatusNoContent, "POST", pickURL, drafted("Zd Guard Three"), nil)
	live.next()
	listed := func() map[string]string {
		var bobs struct {
			Rosters []struct {
				Players []struct {
					FullName string `json:"full_name"`
					List     string
				}
			}
		}
		ann.want(http.StatusOK, "GET", "/api/franchises/bob", nil, &bobs)
		lists := map[string]string{}
		for _, r := range bobs.Rosters {
			for _, p := range r.Players {
				lists[p.FullName] = p.List
			}
		}
		return lists
	}
	if l := listed(); l["Zd Guard One"] != "main" || l["Zd Guard Three"] != "reserve" {
		t.Fatalf("Bob's guards are on %v, want the first on main and the overflow on reserve", l)
	}
	ann.want(http.StatusNoContent, "POST", adminURL+"/undo", nil, nil)
	live.next()
	// Ann, up last, queues a player and turns auto pick on before her turn.
	ann.want(http.StatusNoContent, "PUT", draftURL+"/queue", map[string][]string{"player_ids": {id["Zd Skater Two"]}}, nil)
	ann.want(http.StatusNoContent, "PUT", autoURL, map[string]any{"on": true}, nil)
	if m := live.next(); len(m.AutoPick) != 2 || *m.OnClock != state.Picks[4].ID || m.Draft.ClockExpiresAt != nil {
		t.Fatalf("auto pick turned on while waiting = %+v, want Bob's clock left alone", m)
	}
	bob.want(http.StatusNoContent, "POST", pickURL, drafted("Zd Guard Three"), nil)
	live.next()
	if m := live.next(); m.Draft.Status != "complete" || m.OnClock != nil || m.Picks[0].PlayerName != "Zd Skater Two" || !m.Picks[0].AutoPicked {
		t.Fatalf("after the last pick = %+v, want Ann's queued player taken for her and the draft complete", m)
	}

	// Everyone the draft passed over is now a free agent.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+nbaLeague+"/roster/add", pick("Zd Guard Five", "main"), nil)

	var activity []struct{ Kind string }
	ann.want(http.StatusOK, "GET", "/api/activity", nil, &activity)
	kinds := map[string]int{}
	for _, a := range activity {
		kinds[a.Kind]++
	}
	if kinds["pick"] != 8 || kinds["undo_pick"] != 2 {
		t.Errorf("activity kinds = %v, want 8 picks and 2 undos", kinds)
	}

	// --- setting the reserve list ----------------------------------------
	// Both of Bob's NBA lists are full, so swapping them takes one save.
	// The veteran may sit on reserve here only because he was a startup pick.
	nbaRoster := "/api/leagues/" + nbaLeague + "/roster/"
	reserve := func(players ...string) map[string]any {
		ids := []string{}
		for _, p := range players {
			ids = append(ids, id[p])
		}
		return map[string]any{"reserve": ids}
	}
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"move", pick("Zd Guard One", "reserve"), nil)
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"set", reserve("Zd Guard One", "Zd Guard Three"), nil) // more than it holds
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"set", reserve("Zd Guard Five"), nil)                  // Ann's
	bob.want(http.StatusNoContent, "POST", nbaRoster+"set", reserve("Zd Guard One"), nil)
	if l := listed(); l["Zd Guard One"] != "reserve" || l["Zd Guard Three"] != "main" {
		t.Fatalf("after the swap Bob's guards are on %v", l)
	}
	ann.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"move", pick("Zd Guard Five", "reserve"), nil) // a free agent has no exemption

	// Once the season is under way nobody comes up from reserve, though a
	// player can still go down; the commissioner can override.
	today := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": nbaLeague, "year": 2026, "starts_on": today, "ends_on": time.Now().AddDate(0, 1, 0).Format(time.DateOnly),
	}, nil)
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"set", reserve("Zd Guard Three"), nil)
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"move", pick("Zd Guard One", "main"), nil)
	bob.want(http.StatusNoContent, "POST", nbaRoster+"drop", pick("Zd Guard Three", ""), nil)
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"move", pick("Zd Guard One", "main"), nil)
	forced := reserve()
	forced["franchise_id"], forced["force"] = franchise["Bob"], true
	ann.want(http.StatusNoContent, "POST", nbaRoster+"set", forced, nil)
	if l := listed(); l["Zd Guard One"] != "main" {
		t.Fatalf("after the commissioner's override Bob's guard is on %q", l["Zd Guard One"])
	}
	bob.want(http.StatusNoContent, "POST", nbaRoster+"move", pick("Zd Guard One", "reserve"), nil) // down is still allowed
	bob.want(http.StatusUnprocessableEntity, "POST", nbaRoster+"move", pick("Zd Guard One", "main"), nil)
}
