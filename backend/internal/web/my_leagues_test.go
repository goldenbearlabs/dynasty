// Verifies that preset league creation copies rules without opening the original league.

package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
)

func TestPresetLeagueCreationKeepsOriginalPrivate(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })
	var mailBody string
	server, registry := startServer(t, pool, func(_ context.Context, _, _, body string) error {
		mailBody = body
		return nil
	})

	admin := newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	admin.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name:       "Original",
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Original franchise", ManagerName: "Original manager"}},
		Account:    dynasty.Account{Email: "original@example.com", Password: "correct horse"},
	}, nil)
	var original struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	admin.want(http.StatusOK, "GET", "/api/dynasty", nil, &original)

	creator := newBrowser(t, server.URL)
	creator.want(http.StatusUnauthorized, "GET", "/api/my-leagues", nil, nil)
	creator.want(http.StatusUnauthorized, "POST", "/api/my-leagues", map[string]string{"name": "Blocked"}, nil)
	creator.want(http.StatusCreated, "POST", "/api/accounts", map[string]string{
		"email": "creator@example.com", "password": "correct horse",
	}, nil)
	creator.want(http.StatusNoContent, "POST", "/api/auth/login", map[string]string{
		"email": "creator@example.com", "password": "correct horse",
	}, nil)
	input := map[string]string{"name": "New league", "franchise_name": "New franchise", "manager_name": "New manager"}
	creator.want(http.StatusForbidden, "POST", "/api/my-leagues", input, nil)
	token := strings.TrimSpace(strings.Split(strings.Split(mailBody, "/verify?token=")[1], "\n")[0])
	creator.want(http.StatusNoContent, "POST", "/api/accounts/verify", map[string]string{"token": token}, nil)
	var created struct {
		ID string `json:"id"`
	}
	creator.want(http.StatusCreated, "POST", "/api/my-leagues", input, &created)
	if created.ID == "" || created.ID == original.ID {
		t.Fatalf("new league ID was not independent: %+v", created)
	}
	var memberships []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Sports []string `json:"sports"`
	}
	creator.want(http.StatusOK, "GET", "/api/my-leagues", nil, &memberships)
	if len(memberships) != 1 || memberships[0].ID != created.ID || len(memberships[0].Sports) != 1 || memberships[0].Sports[0] != "nba" {
		t.Fatalf("wrong memberships: %+v", memberships)
	}
	creator.want(http.StatusOK, "GET", "/api/my-leagues/"+created.ID, nil, nil)
	creator.want(http.StatusNotFound, "GET", "/api/my-leagues/"+original.ID, nil, nil)
	creator.want(http.StatusUnauthorized, "GET", "/api/dynasty", nil, nil)
	creator.want(http.StatusUnauthorized, "GET", "/api/drafts", nil, nil)

	var originalRules, newRules json.RawMessage
	if err := pool.QueryRow(context.Background(), `select settings from leagues where dynasty_id = $1`, original.ID).Scan(&originalRules); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `select leagues_snapshot->0->'settings' from league_workspaces where id = $1`, created.ID).Scan(&newRules); err != nil {
		t.Fatal(err)
	}
	if string(originalRules) != string(newRules) {
		t.Fatalf("settings were not copied: original %s, new %s", originalRules, newRules)
	}
	var liveLeagueCount int
	if err := pool.QueryRow(context.Background(), `select count(*) from dynasties`).Scan(&liveLeagueCount); err != nil {
		t.Fatal(err)
	}
	if liveLeagueCount != 1 {
		t.Fatalf("creating a workspace changed live dynasties: %d", liveLeagueCount)
	}
	admin.want(http.StatusOK, "GET", "/api/dynasty", nil, &original)
	if original.Name != "Original" {
		t.Fatalf("original league changed: %+v", original)
	}
}
