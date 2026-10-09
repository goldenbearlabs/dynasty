package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/auth"
	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/draft"
	"crossover/internal/dynasty"
	"crossover/internal/hub"
	"crossover/internal/ingest"
	"crossover/internal/lineup"
	"crossover/internal/live"
	"crossover/internal/players"
	"crossover/internal/roster"
	"crossover/internal/scoring"
	"crossover/internal/settings"
	"crossover/internal/trade"
	"crossover/internal/waiver"
	"crossover/internal/web"
)

// browser is one person's session against the test server.
type browser struct {
	t      *testing.T
	base   string
	client *http.Client
}

func newBrowser(t *testing.T, base string) *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, base: base, client: &http.Client{Jar: jar}}
}

// call makes a request and returns the status and the raw response body.
func (b *browser) call(method, path string, body any) (int, []byte) {
	b.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, b.base+path, reader)
	res, err := b.client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw
}

// want makes a request and fails the test unless it gets the given status.
// A non-nil out receives the decoded response.
func (b *browser) want(status int, method, path string, body, out any) {
	b.t.Helper()
	got, raw := b.call(method, path, body)
	if got != status {
		b.t.Fatalf("%s %s = %d, want %d: %s", method, path, got, status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			b.t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
}

// claim opens an invite link and creates the manager's account, which
// signs the browser in.
func (b *browser) claim(invite, email string) {
	b.t.Helper()
	b.want(http.StatusNoContent, "POST", "/api/invites/"+invite+"/claim",
		map[string]string{"email": email, "password": "correct horse"}, nil)
}

// movedPlayer is the running server's hook for a player turning up in a new
// competition, as the roster sync would call it.
var movedPlayer func(ctx context.Context, playerID pgtype.UUID, from, to string)

// startServer runs the real HTTP handler against the test database, with a
// fast draft clock.
func startServer(t *testing.T, pool *pgxpool.Pool) (*httptest.Server, competition.Registry) {
	ctx, stop := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := competition.NewRegistry(ingest.NewClient(0))

	accounts, err := auth.NewService(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	events := hub.New()
	drafts := draft.NewService(pool, events, log)
	drafts.ClockInterval = 100 * time.Millisecond
	go drafts.RunClock(ctx)
	syncer := ingest.NewSyncer(db.New(pool), log)
	rosters := roster.NewService(pool)
	syncer.OnMove = func(ctx context.Context, playerID pgtype.UUID, from, to string) {
		if err := rosters.Graduate(ctx, playerID, from, to); err != nil {
			t.Errorf("graduate: %v", err)
		}
	}
	movedPlayer = syncer.OnMove

	server := httptest.NewServer((&web.Server{
		Queries:    db.New(pool),
		Auth:       accounts,
		Registry:   registry,
		Dynasty:    dynasty.NewService(pool, registry),
		Roster:     rosters,
		Players:    players.NewService(pool, registry),
		Drafts:     drafts,
		Trades:     trade.NewService(pool),
		Waivers:    waiver.NewService(pool, log),
		Lineups:    lineup.NewService(pool),
		Scoring:    scoring.NewService(pool),
		Live:       live.NewService(pool, syncer, registry, events, log),
		Log:        log,
		Background: ctx,
	}).Handler())
	t.Cleanup(func() { stop(); server.Close() })
	return server, registry
}

// TestLeagueFlow walks the life of a new league through the HTTP API:
// setup, sign-in, the commissioner's tools, and the roster rules.
func TestLeagueFlow(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "web_test"
	removePlayers := func() { pool.Exec(ctx, `delete from players where note = $1`, marker) }
	removePlayers()
	t.Cleanup(func() { dbtest.Reset(t, pool); removePlayers() })

	server, registry := startServer(t, pool)

	luke, sam := newBrowser(t, server.URL), newBrowser(t, server.URL)

	// --- setup ---------------------------------------------------------
	luke.want(http.StatusNotFound, "GET", "/api/dynasty", nil, nil)

	nba, _ := registry.Get("nba")
	cbb, _ := registry.Get("cbb")
	nba.Defaults.Roster.ReserveEligibility = "prospects_only" // this walk-through starts from the stricter rule and relaxes it
	setup := dynasty.Setup{
		Name: "Test Dynasty",
		Leagues: []dynasty.LeagueSetup{
			{Competition: "cbb", Settings: cbb.Defaults},
			{Competition: "nba", Settings: nba.Defaults},
		},
		Franchises: []dynasty.FranchiseSetup{
			{Name: "Wismer Dynasty", ManagerName: "Luke"},
			{Name: "Sam's Spoilers", ManagerName: "Sam"},
		},
		Account: dynasty.Account{Email: "Luke@Example.com", Password: "correct horse"},
	}
	weak := setup
	weak.Account.Password = "short"
	luke.want(http.StatusUnprocessableEntity, "POST", "/api/dynasty", weak, nil)
	luke.want(http.StatusCreated, "POST", "/api/dynasty", setup, nil)
	luke.want(http.StatusUnprocessableEntity, "POST", "/api/dynasty", setup, nil) // only one dynasty

	var view struct {
		Leagues []struct {
			ID          string
			Competition string
			Settings    json.RawMessage
		}
		Franchises []struct{ ID, Slug string }
	}
	status, raw := luke.call("GET", "/api/dynasty", nil)
	if status != http.StatusOK || strings.Contains(string(raw), "invite") {
		t.Fatalf("GET /api/dynasty = %d, and must not expose invite tokens: %s", status, raw)
	}
	json.Unmarshal(raw, &view)
	if len(view.Leagues) != 2 || view.Leagues[1].Competition != "nba" || len(view.Franchises) != 2 {
		t.Fatalf("dynasty = %s", raw)
	}
	league := view.Leagues[1]
	rosterURL := "/api/leagues/" + league.ID + "/roster/"

	// The creator is signed in as the commissioner, with the account they chose.
	var me struct {
		ID             string
		Email          string
		IsCommissioner bool `json:"is_commissioner"`
	}
	luke.want(http.StatusOK, "GET", "/api/session", nil, &me)
	if !me.IsCommissioner || me.Email != "luke@example.com" {
		t.Fatalf("the dynasty's creator = %+v, want a commissioner signed in as luke@example.com", me)
	}

	// Signing out and back in, on what could be another device.
	luke.want(http.StatusNoContent, "POST", "/api/auth/logout", nil, nil)
	if _, raw := luke.call("GET", "/api/session", nil); strings.TrimSpace(string(raw)) != "null" {
		t.Fatalf("still signed in after signing out: %s", raw)
	}
	login := map[string]string{"email": "luke@example.com", "password": "wrong horse"}
	luke.want(http.StatusUnprocessableEntity, "POST", "/api/auth/login", login, nil)
	login["password"] = "correct horse"
	luke.want(http.StatusNoContent, "POST", "/api/auth/login", login, nil)

	// --- invites and accounts ------------------------------------------
	var invites []struct {
		ID          string
		Slug        string
		Email       string
		InviteToken string `json:"invite_token"`
	}
	sam.want(http.StatusForbidden, "GET", "/api/admin/franchises", nil, nil)
	luke.want(http.StatusOK, "GET", "/api/admin/franchises", nil, &invites)
	samInvite := invites[0] // franchises are listed by name
	if samInvite.Slug != "sam-s-spoilers" || samInvite.InviteToken == "" || invites[1].InviteToken != "" {
		t.Fatalf("invites = %+v, want a link for Sam and none for the commissioner, who already has an account", invites)
	}

	// The link says whose it is, then creates the account and signs in.
	var greeting struct{ Franchise struct{ Name string } }
	sam.want(http.StatusNotFound, "GET", "/api/invites/wrong", nil, nil)
	sam.want(http.StatusOK, "GET", "/api/invites/"+samInvite.InviteToken, nil, &greeting)
	if greeting.Franchise.Name != "Sam's Spoilers" {
		t.Fatalf("invite greets %q", greeting.Franchise.Name)
	}
	sam.want(http.StatusUnprocessableEntity, "POST", "/api/invites/"+samInvite.InviteToken+"/claim",
		map[string]string{"email": "luke@example.com", "password": "correct horse"}, nil) // someone else's email
	sam.claim(samInvite.InviteToken, "sam@example.com")
	sam.want(http.StatusNotFound, "GET", "/api/invites/"+samInvite.InviteToken, nil, nil) // a link works once

	// --- the commissioner adds players the feeds do not have -----------
	newPlayers := []players.NewPlayer{
		{Competition: "nba", FullName: "Zz Veteran", Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zz Prospect", Note: marker}, // prospect by default
		{Competition: "nba", FullName: "Zz Twin", Note: marker},
		{Competition: "nba", FullName: "Zz Twin", Status: "active", Note: marker},
	}
	sam.want(http.StatusForbidden, "POST", "/api/admin/players", newPlayers, nil)
	luke.want(http.StatusCreated, "POST", "/api/admin/players", newPlayers, nil)
	luke.want(http.StatusUnprocessableEntity, "POST", "/api/admin/players",
		[]players.NewPlayer{{Competition: "curling", FullName: "Nobody"}}, nil)

	var page struct {
		Players []struct {
			ID        string
			FullName  string `json:"full_name"`
			Status    string
			OwnerSlug string `json:"owner_slug"`
		}
		Total int
	}
	id := map[string]string{} // "name/status" -> player id
	luke.want(http.StatusOK, "GET", "/api/players?q=Zz+&available_in="+league.ID, nil, &page)
	if page.Total != 4 {
		t.Fatalf("available players = %d, want 4", page.Total)
	}
	for _, p := range page.Players {
		id[p.FullName+"/"+p.Status] = p.ID
	}
	veteran, prospect := id["Zz Veteran/active"], id["Zz Prospect/prospect"]

	add := func(playerID, list string) map[string]any {
		return map[string]any{"player_id": playerID, "list": list}
	}

	// --- free agency ---------------------------------------------------
	// By default nobody can be added until the league has held a draft...
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"add", add(veteran, "main"), nil)
	// ...and only a commissioner can force a move.
	forced := add(veteran, "main")
	forced["force"] = true
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"add", forced, nil)
	luke.want(http.StatusNoContent, "POST", rosterURL+"add", forced, nil)

	// Once a draft has been held, players already in the pool are free agents.
	if _, err := pool.Exec(ctx, `update leagues set last_draft_at = now() where id = $1`, league.ID); err != nil {
		t.Fatal(err)
	}
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"add", add(veteran, "main"), nil) // already Luke's
	sam.want(http.StatusNoContent, "POST", rosterURL+"add", add(prospect, "reserve"), nil)
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"add", add(prospect, "main"), nil) // already Sam's

	// A player who arrives after the draft waits for the next one.
	luke.want(http.StatusCreated, "POST", "/api/admin/players",
		[]players.NewPlayer{{Competition: "nba", FullName: "Zz Rookie", Note: marker}}, nil)
	luke.want(http.StatusOK, "GET", "/api/players?q=Zz+Rookie", nil, &page)
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"add", add(page.Players[0].ID, "reserve"), nil)

	// Rostered players leave the available list and show their owner.
	luke.want(http.StatusOK, "GET", "/api/players?q=Zz+&available_in="+league.ID, nil, &page)
	if page.Total != 3 {
		t.Errorf("available players after two adds = %d, want 3", page.Total)
	}
	luke.want(http.StatusOK, "GET", "/api/players?q=Zz+Veteran", nil, &page)
	if page.Players[0].OwnerSlug != "wismer-dynasty" {
		t.Errorf("veteran's owner = %q, want wismer-dynasty", page.Players[0].OwnerSlug)
	}

	// --- list moves and ownership --------------------------------------
	sam.want(http.StatusNoContent, "POST", rosterURL+"move", add(prospect, "main"), nil)
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", add(veteran, "main"), nil) // not Sam's player
	lukes := add(veteran, "")
	lukes["franchise_id"] = me.ID
	sam.want(http.StatusForbidden, "POST", rosterURL+"drop", lukes, nil) // not Sam's franchise

	// --- settings ------------------------------------------------------
	// Reserve is prospects-only by default, so a veteran cannot go there.
	luke.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", add(veteran, "reserve"), nil)

	var rules map[string]any
	json.Unmarshal(league.Settings, &rules)
	settingsURL := "/api/leagues/" + league.ID + "/settings"
	sam.want(http.StatusForbidden, "PUT", settingsURL, rules, nil)

	rules["roster"].(map[string]any)["main"] = 0
	luke.want(http.StatusUnprocessableEntity, "PUT", settingsURL, rules, nil)
	rules["roster"].(map[string]any)["main"] = 14
	rules["mystery_rule"] = true
	luke.want(http.StatusBadRequest, "PUT", settingsURL, rules, nil) // a misspelled rule is an error
	delete(rules, "mystery_rule")

	rules["roster"].(map[string]any)["reserve_eligibility"] = "anyone"
	luke.want(http.StatusNoContent, "PUT", settingsURL, rules, nil)
	luke.want(http.StatusNoContent, "POST", rosterURL+"move", add(veteran, "reserve"), nil)

	// With a reserve lock he has to stay there; only the commissioner's
	// override, or dropping the rule, brings him back early.
	lockedCount := func() int {
		locked := 0
		for _, f := range view.Franchises {
			_, raw := luke.call("GET", "/api/franchises/"+f.Slug, nil)
			locked += strings.Count(string(raw), `"locked_until":"`)
		}
		return locked
	}
	rules["roster"].(map[string]any)["reserve_lock_days"] = 3
	luke.want(http.StatusNoContent, "PUT", settingsURL, rules, nil)
	luke.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", add(veteran, "main"), nil)
	if n := lockedCount(); n != 1 {
		t.Errorf("%d players shown as locked on reserve, want 1", n)
	}
	override := add(veteran, "main")
	override["force"] = true
	luke.want(http.StatusNoContent, "POST", rosterURL+"move", override, nil)
	override["list"] = "reserve"
	luke.want(http.StatusNoContent, "POST", rosterURL+"move", override, nil) // placed by the commissioner: no lock
	luke.want(http.StatusNoContent, "POST", rosterURL+"move", add(veteran, "main"), nil)
	luke.want(http.StatusNoContent, "POST", rosterURL+"move", add(veteran, "reserve"), nil)
	rules["roster"].(map[string]any)["reserve_lock_days"] = 0
	luke.want(http.StatusNoContent, "PUT", settingsURL, rules, nil)
	if n := lockedCount(); n != 0 {
		t.Errorf("%d players shown as locked with the rule off, want 0", n)
	}

	// --- merge ---------------------------------------------------------
	// Sam holds the hand-entered prospect; the real player then appears.
	twinProspect, twinReal := id["Zz Twin/prospect"], id["Zz Twin/active"]
	sam.want(http.StatusNoContent, "POST", rosterURL+"add", add(twinProspect, "reserve"), nil)

	var suggestions []struct {
		ProspectID string `json:"prospect_id"`
		PlayerID   string `json:"player_id"`
	}
	luke.want(http.StatusOK, "GET", "/api/admin/players/merge-suggestions", nil, &suggestions)
	suggested := false
	for _, s := range suggestions {
		suggested = suggested || (s.ProspectID == twinProspect && s.PlayerID == twinReal)
	}
	if !suggested {
		t.Fatalf("the twins are not among the merge suggestions: %+v", suggestions)
	}
	luke.want(http.StatusNoContent, "POST", "/api/admin/players/merge",
		map[string]string{"keep_id": twinReal, "duplicate_id": twinProspect}, nil)

	var franchise struct {
		Rosters []struct {
			Competition string
			Overage     int
			Players     []struct {
				PlayerID string `json:"player_id"`
				List     string
			}
		}
	}
	sam.want(http.StatusOK, "GET", "/api/franchises/"+samInvite.Slug, nil, &franchise)
	nbaRoster := franchise.Rosters[1]
	if nbaRoster.Competition != "nba" || len(nbaRoster.Players) != 2 {
		t.Fatalf("Sam's rosters = %+v", franchise.Rosters)
	}
	held := map[string]string{}
	for _, p := range nbaRoster.Players {
		held[p.PlayerID] = p.List
	}
	if held[twinReal] != "reserve" || held[prospect] != "main" {
		t.Errorf("Sam holds %v, want the merged twin on reserve and the prospect on main", held)
	}

	// --- drop, activity, sign-out by reissued link -----------------------
	sam.want(http.StatusNoContent, "POST", rosterURL+"drop", add(prospect, ""), nil)
	sam.want(http.StatusUnprocessableEntity, "POST", rosterURL+"drop", add(prospect, ""), nil)

	var activity []struct{ Kind string }
	luke.want(http.StatusOK, "GET", "/api/activity", nil, &activity)
	if len(activity) < 6 || activity[0].Kind != "drop" {
		t.Errorf("activity = %+v, want the drop first among at least six entries", activity)
	}

	// --- a reissued link resets the account ------------------------------
	var reissued struct {
		InviteToken string `json:"invite_token"`
	}
	luke.want(http.StatusOK, "POST", "/api/admin/franchises/"+samInvite.ID+"/invite", nil, &reissued)
	sam.want(http.StatusOK, "GET", "/api/session", nil, &me) // a new link alone signs nobody out
	if me.Email != "sam@example.com" {
		t.Fatalf("Sam's session = %+v", me)
	}
	time.Sleep(1100 * time.Millisecond) // token times are whole seconds
	samsNewPhone := newBrowser(t, server.URL)
	samsNewPhone.claim(reissued.InviteToken, "sam.new@example.com")
	if _, raw := sam.call("GET", "/api/session", nil); strings.TrimSpace(string(raw)) != "null" {
		t.Errorf("Sam's old sign-in survived a reset of his account: %s", raw)
	}
	samsNewPhone.want(http.StatusOK, "GET", "/api/session", nil, &me)
	if me.Email != "sam.new@example.com" || me.ID != samInvite.ID {
		t.Errorf("after the reset Sam's franchise is run by %+v", me)
	}
}

// dynastySettings is a dynasty with an overall title awarding these points
// for first place, second place, and so on.
func dynastySettings(pointsByFinish ...float64) settings.Dynasty {
	return settings.Dynasty{OverallTitle: settings.OverallTitle{Enabled: true, PointsByFinish: pointsByFinish}}
}
