package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"github.com/coder/websocket"
)

func TestDraftPresencePreservesLiveStateAndLegacyConnections(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "draft_presence_test"
	t.Cleanup(func() {
		dbtest.Reset(t, pool)
		pool.Exec(ctx, `delete from players where note=$1`, marker)
	})
	server, registry := startServer(t, pool)
	ann, bob := newBrowser(t, server.URL), newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Presence Test", Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: nba.Defaults}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Name string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	ids := map[string]string{}
	for _, f := range view.Franchises {
		ids[f.Name] = f.ID
	}
	var invites []struct {
		Name  string
		Token string `json:"invite_token"`
	}
	ann.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	for _, invite := range invites {
		if invite.Name == "Bob" {
			bob.claim(invite.Token, "bob@example.com")
		}
	}
	var draft struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/drafts", map[string]any{
		"name": "Live presence", "kind": "startup", "year": 2026, "rounds": 1, "order": "snake",
		"league_ids": []string{view.Leagues[0].ID}, "pick_clock_seconds": 120,
	}, &draft)
	url := "/api/drafts/" + draft.ID
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + url
	legacy, _, err := websocket.Dial(ctx, wsURL+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.CloseNow()
	oldRoom := &room{t: t, conn: legacy}
	if oldRoom.next().Type != "state" {
		t.Fatal("legacy client did not receive its draft snapshot")
	}
	ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+draft.ID+"/start", nil, nil)
	if m := oldRoom.next(); (m.Type != "update" && m.Type != "state") || m.Draft.Status != "live" {
		t.Fatalf("legacy draft start: %+v", m)
	}
	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{{Competition: "nba", FullName: "Presence Queued Player", Note: marker}}, nil)
	var playerID string
	if err := pool.QueryRow(ctx, `select id::text from players where note=$1`, marker).Scan(&playerID); err != nil {
		t.Fatal(err)
	}
	ann.want(http.StatusNoContent, "PUT", url+"/queue", map[string]any{"player_ids": []string{playerID}}, nil)
	_, before := ann.call("GET", url, nil)
	_, queueBefore := ann.call("GET", url+"/queue", nil)
	newBrowser(t, server.URL).want(http.StatusUnauthorized, "GET", url+"/presence/ws", nil, nil)
	// An authenticated request that fails its websocket upgrade must not
	// register a phantom manager.
	bob.want(http.StatusUpgradeRequired, "GET", url+"/presence/ws", nil, nil)
	dial := func(b *browser) *websocket.Conn {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		conn, _, err := websocket.Dial(ctx, wsURL+"/presence/ws", &websocket.DialOptions{HTTPClient: b.client})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		return conn
	}
	readUntil := func(conn *websocket.Conn, want ...string) {
		t.Helper()
		want = slices.Clone(want)
		slices.Sort(want)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var message struct {
				Online []string `json:"online_franchise_ids"`
			}
			if err := json.Unmarshal(raw, &message); err != nil {
				t.Fatal(err)
			}
			if slices.Equal(message.Online, want) {
				return
			}
		}
	}
	first := dial(ann)
	readUntil(first, ids["Ann"])
	second := dial(ann)
	readUntil(second, ids["Ann"])
	other := dial(bob)
	readUntil(other, ids["Ann"], ids["Bob"])
	readUntil(second, ids["Ann"], ids["Bob"])
	first.CloseNow()
	other.CloseNow()
	readUntil(second, ids["Ann"])
	reconnected := dial(bob)
	readUntil(reconnected, ids["Ann"], ids["Bob"])
	_, after := ann.call("GET", url, nil)
	_, queueAfter := ann.call("GET", url+"/queue", nil)
	if !bytes.Equal(before, after) || !bytes.Equal(queueBefore, queueAfter) {
		t.Fatal("presence changed the draft status, pick clock, picks, or saved queue")
	}
	// Existing open tabs receive only the unchanged draft protocol; no
	// presence message may leak into their next draft update.
	ann.want(http.StatusNoContent, "POST", "/api/admin/drafts/"+draft.ID+"/pause", nil, nil)
	if m := oldRoom.next(); (m.Type != "update" && m.Type != "state") || m.Draft.Status != "paused" {
		t.Fatalf("presence broke legacy draft messages: %+v", m)
	}
}
