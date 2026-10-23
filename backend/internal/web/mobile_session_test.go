// Exercises mobile token lifecycle while proving browser-cookie login still works.

package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
)

func TestMobileSessions(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })
	server, registry := startServer(t, pool)
	browser := newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	browser.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name:    "Mobile Test",
		Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}},
		Franchises: []dynasty.FranchiseSetup{
			{Name: "First", ManagerName: "Ada"},
			{Name: "Second", ManagerName: "Bea"},
		},
		Account: dynasty.Account{Email: "ada@example.com", Password: "correct horse"},
	}, nil)

	var signed struct {
		Token   string `json:"token"`
		Session struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"session"`
	}
	browser.want(http.StatusOK, "POST", "/api/mobile/auth/login",
		map[string]string{"email": "ada@example.com", "password": "correct horse"}, &signed)
	if signed.Token == "" || signed.Session.Email != "ada@example.com" || signed.Session.Name != "First" {
		t.Fatalf("mobile login response missing manager: %+v", signed.Session)
	}

	call := func(token, method, path string, want int) {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("%s %s = %d, want %d", method, path, res.StatusCode, want)
		}
	}
	call(signed.Token, "GET", "/api/mobile/session", http.StatusOK)
	call(signed.Token, "GET", "/api/watchlist", http.StatusOK)
	call("bad-token", "GET", "/api/mobile/session", http.StatusUnauthorized)

	var renewed struct {
		Token string `json:"token"`
	}
	req, _ := http.NewRequest("POST", server.URL+"/api/mobile/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+signed.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("refresh = %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&renewed); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if renewed.Token == "" || renewed.Token == signed.Token {
		t.Fatal("refresh did not rotate token")
	}
	call(signed.Token, "GET", "/api/mobile/session", http.StatusUnauthorized)
	call(renewed.Token, "GET", "/api/mobile/session", http.StatusOK)
	call(renewed.Token, "POST", "/api/mobile/auth/logout", http.StatusNoContent)
	call(renewed.Token, "GET", "/api/mobile/session", http.StatusUnauthorized)
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/mobile/auth/login",
		map[string]string{"email": "ada@example.com", "password": "wrong password"}, nil)
	var expiring struct {
		Token string `json:"token"`
	}
	browser.want(http.StatusOK, "POST", "/api/mobile/auth/login",
		map[string]string{"email": "ada@example.com", "password": "correct horse"}, &expiring)
	if _, err := pool.Exec(context.Background(), `update mobile_sessions set expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	call(expiring.Token, "GET", "/api/mobile/session", http.StatusUnauthorized)

	var webSession struct {
		Email string `json:"email"`
	}
	browser.want(http.StatusOK, "GET", "/api/session", nil, &webSession)
	if webSession.Email != "ada@example.com" {
		t.Fatalf("browser cookie lost after mobile logout: %+v", webSession)
	}

	var invite string
	if err := pool.QueryRow(context.Background(), `select invite_token from franchises where name = 'Second'`).Scan(&invite); err != nil {
		t.Fatal(err)
	}
	var claimed struct {
		Token string `json:"token"`
	}
	browser.want(http.StatusOK, "POST", "/api/mobile/invites/"+invite+"/claim",
		map[string]string{"email": "bea@example.com", "password": "correct horse"}, &claimed)
	if claimed.Token == "" {
		t.Fatal("invite claim returned no token")
	}
	call(claimed.Token, "GET", "/api/mobile/session", http.StatusOK)
	browser.want(http.StatusUnprocessableEntity, "POST", "/api/mobile/invites/"+invite+"/claim",
		map[string]string{"email": "again@example.com", "password": "correct horse"}, nil)
}
