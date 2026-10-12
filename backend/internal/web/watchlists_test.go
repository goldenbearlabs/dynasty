package web_test

import (
	"context"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"strings"
	"testing"
)

func TestPrivateWatchlist(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "watchlist_fixture"
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note=$1`, marker) })
	server, registry := startServer(t, pool)
	admin, other, guest := newBrowser(t, server.URL), newBrowser(t, server.URL), newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	admin.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{Name: "Watchlist test", Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}}, Franchises: []dynasty.FranchiseSetup{{Name: "Admin", ManagerName: "Admin"}, {Name: "Other", ManagerName: "Other"}}, Account: dynasty.Account{Email: "watchlist@example.com", Password: "correct horse"}}, nil)
	var invites []inviteRecord
	admin.want(200, "GET", "/api/admin/franchises", nil, &invites)
	var otherID string
	for _, i := range invites {
		if i.Name == "Other" {
			other.claim(i.InviteToken, "watchlist-other@example.com")
			otherID = i.ID
		}
	}
	admin.want(201, "POST", "/api/admin/players", []players.NewPlayer{{Competition: "nba", FullName: "Watch Free", Note: marker}, {Competition: "nba", FullName: "Watch Own", Note: marker}, {Competition: "nba", FullName: "Watch Other", Note: marker}, {Competition: "nba", FullName: "Watch Duplicate", Note: marker}, {Competition: "nhl", FullName: "Watch Hockey", Note: marker}}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	admin.want(200, "GET", "/api/players?q=Watch", nil, &page)
	ids := map[string]string{}
	for _, p := range page.Players {
		ids[p.FullName] = p.ID
	}
	var view struct{ Leagues []struct{ ID string } }
	admin.want(200, "GET", "/api/dynasty", nil, &view)
	rosterURL := "/api/leagues/" + view.Leagues[0].ID + "/roster/add"
	admin.want(204, "POST", rosterURL, map[string]any{"player_id": ids["Watch Own"], "list": "reserve", "force": true}, nil)
	admin.want(204, "POST", rosterURL, map[string]any{"player_id": ids["Watch Other"], "franchise_id": otherID, "list": "reserve", "force": true}, nil)
	endpoint := func(id string) string { return "/api/watchlist/players/" + id }
	guest.want(401, "GET", "/api/watchlist", nil, nil)
	guest.want(401, "PUT", endpoint(ids["Watch Free"]), nil, nil)
	admin.want(404, "PUT", endpoint("bad-id"), nil, nil)
	for _, name := range []string{"Watch Free", "Watch Own", "Watch Other", "Watch Hockey"} {
		admin.want(204, "PUT", endpoint(ids[name]), nil, nil)
	}
	admin.want(204, "PUT", endpoint(ids["Watch Free"]), nil, nil)
	var entries []db.WatchlistPlayer
	admin.want(200, "GET", "/api/watchlist", nil, &entries)
	if len(entries) != 4 {
		t.Fatalf("watchlist: %+v", entries)
	}
	if entries[0].OwnerSlug != "" || entries[1].OwnerSlug == "" || entries[2].OwnerSlug == "" {
		t.Fatalf("ownership not shown: %+v", entries)
	}
	other.want(200, "GET", "/api/watchlist", nil, &entries)
	if len(entries) != 0 {
		t.Fatal("another manager can read private watchlist")
	}
	other.want(404, "PATCH", endpoint(ids["Watch Own"]), map[string]string{"category": "Hijack", "notes": "No"}, nil)
	other.want(204, "DELETE", endpoint(ids["Watch Own"]), nil, nil)
	other.want(204, "PUT", endpoint(ids["Watch Free"]), nil, nil)
	other.want(204, "PATCH", endpoint(ids["Watch Free"]), map[string]string{"category": "Other group", "notes": "Private"}, nil)
	admin.want(422, "PATCH", endpoint(ids["Watch Free"]), map[string]string{"notes": strings.Repeat("x", 2001)}, nil)
	admin.want(204, "PATCH", endpoint(ids["Watch Free"]), map[string]string{"category": " Trade targets ", "notes": " Scout next week "}, nil)
	admin.want(200, "GET", "/api/watchlist", nil, &entries)
	if entries[0].Category != "Trade targets" || entries[0].Notes != "Scout next week" {
		t.Fatalf("saved notes: %+v", entries[0])
	}
	ordered := []string{ids["Watch Hockey"], ids["Watch Other"], ids["Watch Own"], ids["Watch Free"]}
	admin.want(204, "PUT", "/api/watchlist/order", map[string]any{"player_ids": ordered}, nil)
	admin.want(200, "GET", "/api/watchlist", nil, &entries)
	if entries[0].FullName != "Watch Hockey" {
		t.Fatalf("order not persisted: %+v", entries)
	}
	admin.want(409, "PUT", "/api/watchlist/order", map[string]any{"player_ids": []string{ids["Watch Free"], ids["Watch Free"]}}, nil)
	admin.want(409, "PUT", "/api/watchlist/order", map[string]any{"player_ids": ordered[:3]}, nil)
	other.want(204, "PUT", endpoint(ids["Watch Duplicate"]), nil, nil)
	other.want(204, "PATCH", endpoint(ids["Watch Duplicate"]), map[string]string{"notes": "Merged scouting notes"}, nil)
	admin.want(204, "PUT", endpoint(ids["Watch Duplicate"]), nil, nil)
	admin.want(204, "PATCH", endpoint(ids["Watch Duplicate"]), map[string]string{"category": "Prospect", "notes": "Keep through merge"}, nil)
	admin.want(204, "POST", "/api/admin/players/merge", map[string]string{"keep_id": ids["Watch Own"], "duplicate_id": ids["Watch Duplicate"]}, nil)
	admin.want(200, "GET", "/api/watchlist", nil, &entries)
	if len(entries) != 4 {
		t.Fatal("merge duplicated watchlist membership")
	}
	var duplicate pgtype.UUID
	duplicate.Scan(ids["Watch Duplicate"])
	for _, e := range entries {
		if e.PlayerID == duplicate {
			t.Fatal("merge lost canonical player")
		}
	}
	admin.want(204, "DELETE", endpoint(ids["Watch Own"]), nil, nil)
	admin.want(200, "GET", "/api/watchlist", nil, &entries)
	if len(entries) != 3 {
		t.Fatal("remove did not persist")
	}
	other.want(200, "GET", "/api/watchlist", nil, &entries)
	if len(entries) != 2 || entries[0].Notes != "Private" || entries[1].Notes != "Merged scouting notes" || entries[1].FullName != "Watch Own" {
		t.Fatal("other manager's list changed")
	}
}
