package web_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/settings"
)

// TestAddDrop covers adding a free agent to a full roster by dropping
// someone in the same move: the drop goes on waivers for the league's
// hours, and a refused add leaves the roster as it was.
func TestAddDrop(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "add_drop_test"
	pool.Exec(ctx, `delete from players where note = $1`, marker)
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note = $1`, marker) })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	rules := nhl.Defaults
	rules.Roster.Main, rules.Roster.Reserve = 1, 0
	rules.Lineup.Slots = []settings.Slot{{Name: "C", Positions: []string{"C"}, Count: 1}}
	rules.Format.Type = "total_points"
	rules.FreeAgency.NewEntrantsDraftOnly = false
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Add Drop Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nhl", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct{ Leagues []struct{ ID string } }
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := "/api/leagues/" + view.Leagues[0].ID

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nhl", FullName: "Zad Kept", Positions: []string{"C"}, Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Zad Free", Positions: []string{"C"}, Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Zad Other", Positions: []string{"C"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zad+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	ann.want(http.StatusNoContent, "POST", league+"/roster/add", map[string]any{"player_id": id["Zad Kept"], "list": "main"}, nil)

	// A full roster turns a plain add away.
	ann.want(http.StatusUnprocessableEntity, "POST", league+"/roster/add", map[string]any{"player_id": id["Zad Free"], "list": "main"}, nil)
	// An add that cannot go through leaves the drop undone: the reserve list holds nobody.
	ann.want(http.StatusUnprocessableEntity, "POST", league+"/roster/add", map[string]any{"player_id": id["Zad Free"], "list": "reserve", "drop_player_id": id["Zad Kept"]}, nil)
	var waived struct {
		Players []struct {
			FullName string    `json:"full_name"`
			ClearsAt time.Time `json:"clears_at"`
		}
	}
	ann.want(http.StatusOK, "GET", league+"/waivers", nil, &waived)
	if len(waived.Players) != 0 {
		t.Fatalf("a refused add still dropped someone: %+v", waived.Players)
	}

	// Adding and dropping together: the drop waits on waivers for a day.
	ann.want(http.StatusNoContent, "POST", league+"/roster/add", map[string]any{"player_id": id["Zad Free"], "list": "main", "drop_player_id": id["Zad Kept"]}, nil)
	ann.want(http.StatusOK, "GET", league+"/waivers", nil, &waived)
	if len(waived.Players) != 1 || waived.Players[0].FullName != "Zad Kept" {
		t.Fatalf("on waivers: %+v, want the dropped player", waived.Players)
	}
	if left := time.Until(waived.Players[0].ClearsAt); left < 23*time.Hour || left > 25*time.Hour {
		t.Fatalf("the drop clears waivers in %v, want about 24 hours", left)
	}
	// He cannot be added straight back as a free agent while he waits.
	ann.want(http.StatusUnprocessableEntity, "POST", league+"/roster/add", map[string]any{"player_id": id["Zad Kept"], "list": "main", "drop_player_id": id["Zad Free"]}, nil)
}
