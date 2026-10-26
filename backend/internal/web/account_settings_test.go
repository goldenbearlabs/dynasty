// Verifies self-service account changes, session revocation, and safe commissioner succession.
package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
)

func TestAccountSecurityWithoutLeague(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })
	var emailBody string
	server, _ := startServer(t, pool, func(_ context.Context, _, _, body string) error { emailBody = body; return nil })
	browser := newBrowser(t, server.URL)
	browser.want(http.StatusCreated, "POST", "/api/accounts", map[string]string{"email": "first@example.com", "password": "correct horse"}, nil)
	browser.want(http.StatusNoContent, "POST", "/api/auth/login", map[string]string{"email": "first@example.com", "password": "correct horse"}, nil)
	var mobile struct {
		Token string `json:"token"`
	}
	browser.want(http.StatusOK, "POST", "/api/mobile/auth/login", map[string]string{"email": "first@example.com", "password": "correct horse"}, &mobile)
	mobileStatus := func() int {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL+"/api/mobile/session", nil)
		req.Header.Set("Authorization", "Bearer "+mobile.Token)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if mobileStatus() != http.StatusOK {
		t.Fatal("new mobile session did not work")
	}
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/account/password", map[string]string{"current_password": "wrong", "new_password": "changed horse"}, nil)
	browser.want(http.StatusNoContent, "POST", "/api/account/email-change", map[string]string{"current_password": "correct horse", "new_email": "second@example.com"}, nil)
	if !strings.Contains(emailBody, "Enter this code") {
		t.Fatalf("change code missing: %q", emailBody)
	}
	code := strings.TrimSpace(strings.Split(strings.Split(emailBody, "app to confirm your new email:\n\n")[1], "\n")[0])
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/account/email-change/confirm", map[string]string{"code": "00000000"}, nil)
	browser.want(http.StatusNoContent, "POST", "/api/account/email-change/confirm", map[string]string{"code": code}, nil)
	if mobileStatus() != http.StatusUnauthorized {
		t.Fatal("email change did not revoke mobile token")
	}
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/auth/login", map[string]string{"email": "first@example.com", "password": "correct horse"}, nil)
	browser.want(http.StatusNoContent, "POST", "/api/auth/login", map[string]string{"email": "second@example.com", "password": "correct horse"}, nil)
	browser.want(http.StatusNoContent, "POST", "/api/account/password", map[string]string{"current_password": "correct horse", "new_password": "changed horse"}, nil)
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/auth/login", map[string]string{"email": "second@example.com", "password": "correct horse"}, nil)
	browser.want(http.StatusNoContent, "POST", "/api/auth/login", map[string]string{"email": "second@example.com", "password": "changed horse"}, nil)
	browser.want(http.StatusNoContent, "DELETE", "/api/account", map[string]string{"password": "changed horse"}, nil)
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/auth/login", map[string]string{"email": "second@example.com", "password": "changed horse"}, nil)
}

func TestAccountDeletionTransfersCommissionerAndKeepsHistory(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })
	server, registry := startServer(t, pool)
	admin, successor := newBrowser(t, server.URL), newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	admin.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Keep History", Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Admin Team", ManagerName: "Ada"}, {Name: "Next Team", ManagerName: "Bea"}},
		Account:    dynasty.Account{Email: "admin@example.com", Password: "correct horse"},
	}, nil)
	var invites []inviteRecord
	admin.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	var successorID, adminID string
	for _, invite := range invites {
		if invite.Name == "Admin Team" {
			adminID = invite.ID
		}
		if invite.Name == "Next Team" {
			successorID = invite.ID
			successor.claim(invite.InviteToken, "next@example.com")
		}
	}
	var leagueID string
	if err := pool.QueryRow(context.Background(), `select id from leagues limit 1`).Scan(&leagueID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `insert into franchise_teams(franchise_id, league_id, name, image_url) values($1,$2,'Private image','https://example.com/logo.png')`, adminID, leagueID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `insert into notifications(franchise_id, kind, title, href) values($1,'trade','Private notification','/trades')`, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `insert into transactions(dynasty_id, league_id, franchise_id, kind) select dynasty_id, $2, id, 'add' from franchises where id = $1`, adminID, leagueID); err != nil {
		t.Fatal(err)
	}
	var options []struct {
		DynastyID  string `json:"dynasty_id"`
		Successors []struct {
			FranchiseID string `json:"franchise_id"`
		} `json:"successors"`
	}
	admin.want(http.StatusOK, "GET", "/api/account/deletion-options", nil, &options)
	if len(options) != 1 || len(options[0].Successors) != 1 || options[0].Successors[0].FranchiseID != successorID {
		t.Fatalf("succession options: %+v", options)
	}
	admin.want(http.StatusUnprocessableEntity, "DELETE", "/api/account", map[string]any{"password": "correct horse"}, nil)
	admin.want(http.StatusNoContent, "DELETE", "/api/account", map[string]any{"password": "correct horse", "commissioner_transfers": map[string]string{options[0].DynastyID: successorID}}, nil)
	var formerName, formerManager, formerSlug string
	var ownerGone, commissioner bool
	if err := pool.QueryRow(context.Background(), `select name, manager_name, slug, user_id is null, is_commissioner from franchises where name = 'Former manager'`).Scan(&formerName, &formerManager, &formerSlug, &ownerGone, &commissioner); err != nil {
		t.Fatal(err)
	}
	if !ownerGone || commissioner || formerManager != "Former manager" || !strings.HasPrefix(formerSlug, "former-") {
		t.Fatalf("former franchise: %q %q %q %t %t", formerName, formerManager, formerSlug, ownerGone, commissioner)
	}
	var nextCommissioner bool
	if err := pool.QueryRow(context.Background(), `select is_commissioner from franchises where id = $1`, successorID).Scan(&nextCommissioner); err != nil {
		t.Fatal(err)
	}
	if !nextCommissioner {
		t.Fatal("successor did not become commissioner")
	}
	for _, table := range []string{"franchise_teams", "notifications"} {
		var count int
		if err := pool.QueryRow(context.Background(), "select count(*) from "+table+" where franchise_id = $1", adminID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s private data survived deletion", table)
		}
	}
	var history int
	if err := pool.QueryRow(context.Background(), `select count(*) from transactions where franchise_id = $1`, adminID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 1 {
		t.Fatal("historical transaction was lost")
	}
	successor.want(http.StatusOK, "GET", "/api/dynasty", nil, nil)
}
