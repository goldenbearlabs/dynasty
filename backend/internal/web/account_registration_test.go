// Checks that new accounts can verify email but cannot read a league they have not joined.

package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
)

func TestAccountRegistrationAndPrivateLeague(t *testing.T) {
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
		Name:       "Private League",
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Founders", ManagerName: "Ada"}, {Name: "New GM", ManagerName: "New Manager"}},
		Account:    dynasty.Account{Email: "ada@example.com", Password: "correct horse"},
	}, nil)
	admin.want(http.StatusOK, "GET", "/api/dynasty", nil, nil)

	guest := newBrowser(t, server.URL)
	guest.want(http.StatusUnauthorized, "GET", "/api/dynasty", nil, nil)
	guest.want(http.StatusUnauthorized, "GET", "/api/drafts", nil, nil)
	var created struct {
		EmailSent bool `json:"email_sent"`
	}
	guest.want(http.StatusCreated, "POST", "/api/accounts", map[string]string{
		"email": "new@example.com", "password": "correct horse",
	}, &created)
	if !created.EmailSent || !strings.Contains(mailBody, "/verify?token=") {
		t.Fatalf("verification email missing: %+v, %q", created, mailBody)
	}
	guest.want(http.StatusNoContent, "POST", "/api/auth/login", map[string]string{
		"email": "new@example.com", "password": "correct horse",
	}, nil)
	var pending struct {
		Email    string  `json:"email"`
		Verified bool    `json:"verified"`
		ID       *string `json:"id"`
	}
	guest.want(http.StatusOK, "GET", "/api/session", nil, &pending)
	if pending.Email != "new@example.com" || pending.Verified || pending.ID != nil {
		t.Fatalf("new account unexpectedly owns a league: %+v", pending)
	}
	guest.want(http.StatusUnauthorized, "GET", "/api/dynasty", nil, nil)
	token := strings.TrimSpace(strings.Split(strings.Split(mailBody, "/verify?token=")[1], "\n")[0])
	guest.want(http.StatusNoContent, "POST", "/api/accounts/verify", map[string]string{"token": token}, nil)
	guest.want(http.StatusUnprocessableEntity, "POST", "/api/accounts/verify", map[string]string{"token": token}, nil)
	guest.want(http.StatusOK, "GET", "/api/session", nil, &pending)
	if !pending.Verified {
		t.Fatal("verification did not update the account")
	}
	guest.want(http.StatusUnauthorized, "GET", "/api/dynasty", nil, nil)
	guest.want(http.StatusNoContent, "POST", "/api/accounts/password-reset", map[string]string{"email": "new@example.com"}, nil)
	if !strings.Contains(mailBody, "/reset-password?token=") {
		t.Fatalf("password reset email missing: %q", mailBody)
	}
	resetToken := strings.TrimSpace(strings.Split(strings.Split(mailBody, "/reset-password?token=")[1], "\n")[0])
	guest.want(http.StatusNoContent, "POST", "/api/accounts/password-reset/confirm", map[string]string{"token": resetToken, "password": "new correct horse"}, nil)
	guest.want(http.StatusUnprocessableEntity, "POST", "/api/accounts/password-reset/confirm", map[string]string{"token": resetToken, "password": "another password"}, nil)
	guest.want(http.StatusOK, "GET", "/api/session", nil, nil)
	guest.want(http.StatusUnprocessableEntity, "POST", "/api/auth/login", map[string]string{"email": "new@example.com", "password": "correct horse"}, nil)
	guest.want(http.StatusNoContent, "POST", "/api/auth/login", map[string]string{"email": "new@example.com", "password": "new correct horse"}, nil)
	var invite string
	if err := pool.QueryRow(context.Background(), `select invite_token from franchises where name = 'New GM'`).Scan(&invite); err != nil {
		t.Fatal(err)
	}
	guest.want(http.StatusNoContent, "POST", "/api/invites/"+invite+"/accept", nil, nil)
	guest.want(http.StatusOK, "GET", "/api/dynasty", nil, nil)
	guest.want(http.StatusUnprocessableEntity, "POST", "/api/invites/"+invite+"/accept", nil, nil)
}
