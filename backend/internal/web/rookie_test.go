package web_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/roster"
)

// TestRookieDraft walks a rookie draft: who can be drafted, passing a pick,
// holding the picks as rights, and signing or losing them afterwards.
func TestRookieDraft(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "rookie_test"
	removePlayers := func() { pool.Exec(ctx, `delete from players where note = $1`, marker) }
	removePlayers()
	t.Cleanup(func() { dbtest.Reset(t, pool); removePlayers() })

	server, registry := startServer(t, pool)
	ann, bob := newBrowser(t, server.URL), newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Roster.Reserve = 5 // so three rounds: half the reserve list, rounded up
	rules.Roster.ReserveEligibility = "prospects_only" // to show that a signed rookie may sit there all the same
	rules.Waivers.Mode = "rolling"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Rookie Test", Settings: dynastySettings(10),
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
	slug := map[string]string{}
	for _, f := range view.Franchises {
		slug[f.Name] = f.Slug
	}
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

	// --- the coming rookie drafts exist from the start ------------------------
	var drafts []struct {
		ID, Kind string
		Year     int
		Picks    int
	}
	ann.want(http.StatusOK, "GET", "/api/drafts", nil, &drafts)
	if len(drafts) != 3 {
		t.Fatalf("a new league has %d drafts on the books, want its next three", len(drafts))
	}
	next := drafts[0]
	for _, d := range drafts {
		if d.Kind != "seasonal" || d.Picks != 6 { // three rounds for two franchises
			t.Fatalf("draft = %+v, want a rookie draft with 6 picks", d)
		}
		if d.Year < next.Year {
			next = d
		}
	}

	// --- one startup draft per league ------------------------------------------
	startup := map[string]any{"name": "Startup", "kind": "startup", "year": 2026, "league_ids": []string{league},
		"rounds": 1, "order": "snake", "franchise_order": []string{}, "pick_clock_seconds": 0}
	ann.want(http.StatusCreated, "POST", "/api/admin/drafts", startup, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/admin/drafts", startup, nil)

	// --- anyone unrostered can be drafted: new to the pool, or a free agent ------
	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nba", FullName: "Zk Veteran", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zk Rookie One", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zk Rookie Two", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zk Rookie Three", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zk Rookie Four", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zk Rookie Five", Status: "active", Note: marker},
	}, nil)
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	sql(`update leagues set last_draft_at = now() - interval '1 day' where id = $1`, league)
	sql(`update players set eligible_since = now() - interval '2 days' where full_name = 'Zk Veteran' and note = $1`, marker)
	type listed struct {
		ID          string
		FullName    string  `json:"full_name"`
		OwnerName   string  `json:"owner_name"`
		WaiverUntil *string `json:"waiver_until"`
	}
	find := func(query string) map[string]listed {
		t.Helper()
		var page struct{ Players []listed }
		ann.want(http.StatusOK, "GET", "/api/players?q=Zk+"+query, nil, &page)
		byName := map[string]listed{}
		for _, p := range page.Players {
			byName[p.FullName] = p
		}
		return byName
	}
	id := map[string]string{}
	for name, p := range find("") {
		id[name] = p.ID
	}
	draftURL := "/api/drafts/" + next.ID
	if pool := find("&draft_id=" + next.ID); len(pool) != 6 || pool["Zk Veteran"].ID == "" {
		t.Fatalf("the rookie pool = %v; want all six, the long-standing free agent included", pool)
	}

	// Ann's roster is over its limits (a veteran on a prospects-only reserve list). That stops
	// ordinary moves, but not a rookie pick, which is held on no list.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add",
		map[string]any{"player_id": id["Zk Veteran"], "list": "reserve", "force": true}, nil)

	// --- picking, and passing ---------------------------------------------------
	ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+next.ID+"/start", nil, nil)
	pick := func(b *browser, want int, player string) {
		t.Helper()
		b.want(want, "POST", draftURL+"/pick", map[string]string{"player_id": id[player], "list": "main"}, nil)
	}
	bob.want(http.StatusUnprocessableEntity, "POST", draftURL+"/pass", nil, nil) // not Bob's pick
	pick(ann, http.StatusUnprocessableEntity, "Zk Veteran")                      // on a roster already
	pick(ann, http.StatusNoContent, "Zk Rookie One")
	bob.want(http.StatusNoContent, "POST", draftURL+"/pass", nil, nil)
	pick(ann, http.StatusNoContent, "Zk Rookie Two")
	var state struct {
		Draft struct{ Status string }
		Picks []struct {
			ID       string
			PassedAt *string `json:"passed_at"`
		}
	}
	ann.want(http.StatusOK, "GET", draftURL, nil, &state)
	if state.Draft.Status != "live" || state.Picks[1].PassedAt == nil {
		t.Fatalf("after a pass: %+v; want the draft still live and the second pick passed", state)
	}
	// A passed pick is gone for good.
	bob.want(http.StatusUnprocessableEntity, "POST", draftURL+"/pick", map[string]string{"player_id": id["Zk Rookie Three"], "pick_id": state.Picks[1].ID}, nil)
	pick(bob, http.StatusNoContent, "Zk Rookie Three")
	pick(ann, http.StatusNoContent, "Zk Rookie Four")
	pick(bob, http.StatusNoContent, "Zk Rookie Five")
	ann.want(http.StatusOK, "GET", draftURL, nil, &state)
	if state.Draft.Status != "complete" {
		t.Fatalf("the draft is %q with every pick made or passed, want complete", state.Draft.Status)
	}

	// --- the picks are held as rights, with a week to sign them ------------------
	type held struct {
		Rosters []struct {
			Overage int
			Players []struct {
				PlayerID    string `json:"player_id"`
				FullName    string `json:"full_name"`
				List        string
				RightsUntil *string `json:"rights_until"`
			}
		}
	}
	lists := func(b *browser, team string, over int) map[string]string {
		t.Helper()
		var h held
		b.want(http.StatusOK, "GET", "/api/franchises/"+slug[team], nil, &h)
		out := map[string]string{}
		for _, p := range h.Rosters[0].Players {
			out[p.FullName] = p.List
			if p.List == "rights" && p.RightsUntil == nil {
				t.Errorf("%s is held without a deadline", p.FullName)
			}
		}
		if h.Rosters[0].Overage != over {
			t.Errorf("%s is over its limits by %d, want %d; unsigned picks count against nothing", team, h.Rosters[0].Overage, over)
		}
		return out
	}
	if l := lists(ann, "Ann", 1); l["Zk Rookie One"] != "rights" || l["Zk Rookie Two"] != "rights" { // over by the veteran alone
		t.Fatalf("Ann's picks are held as %v, want rights", l)
	}
	move := func(b *browser, want int, action, player, list string) {
		t.Helper()
		b.want(want, "POST", "/api/leagues/"+league+"/roster/"+action, map[string]string{"player_id": id[player], "list": list}, nil)
	}
	move(ann, http.StatusNoContent, "drop", "Zk Veteran", "") // back under the limits
	// He is not a prospect, and this league's reserve list is for prospects: a signed rookie may sit there all the same.
	move(ann, http.StatusNoContent, "move", "Zk Rookie One", "reserve")
	// A pick can go straight to the main roster too.
	move(ann, http.StatusNoContent, "move", "Zk Rookie Two", "main")
	if l := lists(ann, "Ann", 0); l["Zk Rookie One"] != "reserve" || l["Zk Rookie Two"] != "main" || l["Zk Rookie Four"] != "rights" {
		t.Fatalf("after signing two of three: %v", l)
	}

	// --- released: by choice, or by the deadline ---------------------------------
	move(bob, http.StatusNoContent, "drop", "Zk Rookie Three", "")
	if p := find("")["Zk Rookie Three"]; p.OwnerName != "" || p.WaiverUntil == nil {
		t.Fatalf("a released pick = %+v; want him unowned and on waivers", p)
	}
	var clears time.Duration
	if err := pool.QueryRow(ctx, `select clears_at - now() from waivers where player_id = $1`, id["Zk Rookie Three"]).Scan(&clears); err != nil ||
		clears < 23*time.Hour || clears > 24*time.Hour {
		t.Fatalf("a released pick clears waivers in %v (err %v), want a day", clears, err)
	}
	move(ann, http.StatusUnprocessableEntity, "add", "Zk Rookie Three", "main") // claimed, not added, while he is on them
	rosters := roster.NewService(pool)
	if n, err := rosters.ReleaseUnsigned(ctx); err != nil || n != 0 {
		t.Fatalf("released %d before any deadline (err %v), want none", n, err)
	}
	sql(`update roster_entries set rights_until = now() - interval '1 minute' where player_id = $1`, id["Zk Rookie Four"])
	if n, err := rosters.ReleaseUnsigned(ctx); err != nil || n != 1 {
		t.Fatalf("released %d at the deadline (err %v), want the one unsigned pick", n, err)
	}
	after := find("")
	if p := after["Zk Rookie Four"]; p.OwnerName != "" || p.WaiverUntil == nil || after["Zk Rookie One"].OwnerName != "Ann" || after["Zk Rookie Five"].OwnerName != "Bob" {
		t.Fatalf("after the deadline: %+v; want the unsigned pick on waivers, and the signed and the still-held kept", after)
	}
	// Once he goes up to the main roster he is an ordinary player: he cannot go back to a prospects-only reserve list.
	move(ann, http.StatusNoContent, "move", "Zk Rookie One", "main")
	move(ann, http.StatusUnprocessableEntity, "move", "Zk Rookie One", "reserve")
}
