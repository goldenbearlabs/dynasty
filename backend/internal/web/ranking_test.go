package web_test

import (
	"context"
	"net/http"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
)

// TestRankings covers a manager's pre-draft lists: making and ordering one,
// attaching it to a draft, keeping it private, and loading it into a queue.
func TestRankings(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "ranking_test"
	removePlayers := func() { pool.Exec(ctx, `delete from players where note = $1`, marker) }
	removePlayers()
	t.Cleanup(func() { dbtest.Reset(t, pool); removePlayers() })

	server, registry := startServer(t, pool)
	ann, bob := newBrowser(t, server.URL), newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	nhl, _ := registry.Get("nhl")
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Ranking Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}, {Competition: "nhl", Settings: nhl.Defaults}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID, Competition string }
		Franchises []struct{ ID, Name string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := map[string]string{}
	for _, l := range view.Leagues {
		league[l.Competition] = l.ID
	}
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
		if invite.Name == "Bob" {
			bob.claim(invite.InviteToken, "bob@example.com")
		}
	}

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nba", FullName: "Zr One", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zr Two", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zr Three", Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Zr Skater", Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zr+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	draft := func(name, kind, competition string) string {
		t.Helper()
		var d struct{ ID string }
		ann.want(http.StatusCreated, "POST", "/api/admin/drafts", map[string]any{
			"name": name, "kind": kind, "year": 2027, "league_ids": []string{league[competition]},
			"rounds": 2, "order": "snake", "franchise_order": []string{}, "pick_clock_seconds": 0,
		}, &d)
		return d.ID
	}
	startup, hockey := draft("Startup", "startup", "nba"), draft("2027 NHL Draft", "seasonal", "nhl")

	type ranking struct {
		ID      string
		Name    string
		DraftID *string `json:"draft_id"`
		Players []struct {
			FullName  string `json:"full_name"`
			OwnerName string `json:"owner_name"`
		}
	}
	names := func(r ranking) string {
		out := ""
		for _, p := range r.Players {
			out += p.FullName + ", "
		}
		return out
	}

	// --- making a list ------------------------------------------------------
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/rankings", map[string]any{"name": " ", "league_id": league["nba"]}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/rankings", map[string]any{"name": "Board"}, nil) // no league
	var mine ranking
	ann.want(http.StatusCreated, "POST", "/api/rankings", map[string]any{"name": "My board", "league_id": league["nba"]}, &mine)
	url := "/api/rankings/" + mine.ID
	if mine.DraftID != nil || len(mine.Players) != 0 {
		t.Fatalf("new ranking = %+v", mine)
	}

	// --- ordering it, and attaching it to a draft -----------------------------
	order := func(players ...string) map[string]any {
		ids := []string{}
		for _, p := range players {
			ids = append(ids, id[p])
		}
		return map[string]any{"name": "Startup board", "draft_id": startup, "player_ids": ids}
	}
	ann.want(http.StatusOK, "PUT", url, order("Zr Two", "Zr One", "Zr Three"), &mine)
	if mine.Name != "Startup board" || mine.DraftID == nil || *mine.DraftID != startup || names(mine) != "Zr Two, Zr One, Zr Three, " {
		t.Fatalf("after ordering: %+v", mine)
	}
	ann.want(http.StatusUnprocessableEntity, "PUT", url, order("Zr One", "Zr One"), nil)    // the same player twice
	ann.want(http.StatusUnprocessableEntity, "PUT", url, order("Zr One", "Zr Skater"), nil) // another sport's player
	ann.want(http.StatusUnprocessableEntity, "PUT", url, map[string]any{"name": "Startup board", "draft_id": hockey}, nil)
	// A refused change leaves the list as it was; renaming alone leaves the players alone.
	ann.want(http.StatusOK, "PUT", url, map[string]any{"name": "Renamed", "draft_id": startup}, &mine)
	if mine.Name != "Renamed" || names(mine) != "Zr Two, Zr One, Zr Three, " {
		t.Fatalf("after renaming: %+v", mine)
	}

	// --- nobody else sees it --------------------------------------------------
	var listed []struct {
		ID, Competition string
		DraftName       string `json:"draft_name"`
		Players         int
	}
	ann.want(http.StatusOK, "GET", "/api/rankings", nil, &listed)
	if len(listed) != 1 || listed[0].Competition != "nba" || listed[0].DraftName != "Startup" || listed[0].Players != 3 {
		t.Fatalf("Ann's rankings = %+v", listed)
	}
	bob.want(http.StatusOK, "GET", "/api/rankings", nil, &listed)
	if len(listed) != 0 {
		t.Fatalf("Bob sees %d rankings, want none", len(listed))
	}
	bob.want(http.StatusNotFound, "GET", url, nil, nil)
	bob.want(http.StatusNotFound, "PUT", url, order("Zr One"), nil)
	bob.want(http.StatusUnprocessableEntity, "DELETE", url, nil, nil)
	newBrowser(t, server.URL).want(http.StatusUnauthorized, "GET", "/api/rankings", nil, nil)

	// --- loading it into a queue ----------------------------------------------
	// Someone takes the top player first.
	ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league["nba"]+"/roster/add",
		map[string]any{"player_id": id["Zr Two"], "list": "main", "franchise_id": team["Bob"], "force": true}, nil)
	ann.want(http.StatusOK, "GET", url, nil, &mine)
	if mine.Players[0].OwnerName != "Bob" {
		t.Errorf("the ranking does not show that %s has been taken: %+v", mine.Players[0].FullName, mine.Players[0])
	}
	ann.want(http.StatusNoContent, "PUT", "/api/drafts/"+startup+"/queue", map[string]any{"player_ids": []string{id["Zr Three"]}}, nil)
	var result struct{ Added int }
	importing := map[string]string{"ranking_id": mine.ID}
	ann.want(http.StatusUnprocessableEntity, "POST", "/api/drafts/"+hockey+"/queue/import", importing, nil) // not this draft's league
	bob.want(http.StatusNotFound, "POST", "/api/drafts/"+startup+"/queue/import", importing, nil)           // not Bob's ranking
	ann.want(http.StatusOK, "POST", "/api/drafts/"+startup+"/queue/import", importing, &result)
	var queue []struct {
		FullName string `json:"full_name"`
	}
	ann.want(http.StatusOK, "GET", "/api/drafts/"+startup+"/queue", nil, &queue)
	if result.Added != 1 || len(queue) != 2 || queue[0].FullName != "Zr Three" || queue[1].FullName != "Zr One" {
		t.Fatalf("import added %d and the queue is %+v; want Zr One added after Zr Three, the taken player left out", result.Added, queue)
	}
	ann.want(http.StatusOK, "POST", "/api/drafts/"+startup+"/queue/import", importing, &result)
	if result.Added != 0 {
		t.Errorf("a second import added %d players, want none", result.Added)
	}

	ann.want(http.StatusNoContent, "DELETE", url, nil, nil)
	ann.want(http.StatusNotFound, "GET", url, nil, nil)
}
