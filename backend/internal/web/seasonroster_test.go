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
	"crossover/internal/sportsday"
)

// TestSeasonRoster covers setting a roster for the season: it waits for
// the commissioner to open the season, is set once, and then holds every
// player on his list, while drops still go through.
func TestSeasonRoster(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "season_roster_test"
	pool.Exec(ctx, `delete from players where note = $1`, marker)
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note = $1`, marker) })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	rules := nhl.Defaults
	rules.Roster.Main, rules.Roster.Reserve = 2, 2
	rules.Lineup.Slots = []settings.Slot{{Name: "C", Positions: []string{"C"}, Count: 1}}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Season Roster Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nhl", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Slug string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, slug := view.Leagues[0].ID, view.Franchises[0].Slug

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nhl", FullName: "Zsr One", Positions: []string{"C"}, Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Zsr Two", Positions: []string{"C"}, Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Zsr Three", Positions: []string{"D"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zsr+", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
	}
	rosterURL := "/api/leagues/" + league + "/roster/"
	ann.want(http.StatusNoContent, "POST", rosterURL+"add", map[string]any{"player_id": id["Zsr One"], "list": "main", "force": true}, nil)
	ann.want(http.StatusNoContent, "POST", rosterURL+"add", map[string]any{"player_id": id["Zsr Two"], "list": "main", "force": true}, nil)
	ann.want(http.StatusNoContent, "POST", rosterURL+"add", map[string]any{"player_id": id["Zsr Three"], "list": "reserve", "force": true}, nil)

	type state struct {
		Rosters []struct {
			Season *struct {
				SetAt  *string `json:"set_at"`
				Locked bool
			}
			Players []struct {
				FullName    string  `json:"full_name"`
				List        string  `json:"list"`
				LockedUntil *string `json:"locked_until"`
			}
		}
	}
	mine := func() state {
		t.Helper()
		var s state
		ann.want(http.StatusOK, "GET", "/api/franchises/"+slug, nil, &s)
		return s
	}
	season := map[string]any{"reserve": []string{id["Zsr One"]}}

	// --- nothing to set until the commissioner opens the season ------------
	if s := mine(); s.Rosters[0].Season != nil {
		t.Fatalf("season before one is opened: %+v", s.Rosters[0].Season)
	}
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"season", season, nil)
	today := sportsday.Today()
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(),
		"starts_on": today.AddDate(0, 0, 7).Format(time.DateOnly), "ends_on": today.AddDate(0, 3, 0).Format(time.DateOnly),
	}, nil)
	if s := mine(); s.Rosters[0].Season == nil || s.Rosters[0].Season.SetAt != nil || s.Rosters[0].Season.Locked {
		t.Fatalf("an opened season = %+v, want one waiting for its roster", s.Rosters[0].Season)
	}

	// --- until it is set, players move freely -------------------------------
	ann.want(http.StatusNoContent, "POST", rosterURL+"move", map[string]any{"player_id": id["Zsr Two"], "list": "reserve"}, nil)
	ann.want(http.StatusNoContent, "POST", rosterURL+"move", map[string]any{"player_id": id["Zsr Two"], "list": "main"}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"season", map[string]any{"reserve": []string{}}, nil) // three on a main roster of two

	// --- set once, then held ------------------------------------------------
	ann.want(http.StatusNoContent, "POST", rosterURL+"season", season, nil)
	s := mine()
	if r := s.Rosters[0]; r.Season == nil || r.Season.SetAt == nil || !r.Season.Locked {
		t.Fatalf("after setting it: %+v, want set and locked", r.Season)
	}
	for _, p := range s.Rosters[0].Players {
		if want := p.FullName == "Zsr One"; (p.List == "reserve") != want || (p.LockedUntil != nil) != want {
			t.Fatalf("%s is on %s, locked until %v; want only Zsr One, on reserve and locked", p.FullName, p.List, p.LockedUntil)
		}
	}
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"season", season, nil)                                                      // set once
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", map[string]any{"player_id": id["Zsr One"], "list": "main"}, nil)    // up from reserve
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", map[string]any{"player_id": id["Zsr Two"], "list": "reserve"}, nil) // down to reserve
	ann.want(http.StatusNoContent, "POST", rosterURL+"drop", map[string]any{"player_id": id["Zsr One"]}, nil)                              // drops go through
	// The commissioner can still override the lock.
	ann.want(http.StatusNoContent, "POST", rosterURL+"move", map[string]any{"player_id": id["Zsr Two"], "list": "reserve", "force": true}, nil)
}

// TestRosterDeadline covers the commissioner's deadline for season rosters:
// until it passes nothing changes, and once it has, a roster not set holds
// as it stood.
func TestRosterDeadline(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "roster_deadline_test"
	pool.Exec(ctx, `delete from players where note = $1`, marker)
	t.Cleanup(func() { dbtest.Reset(t, pool); pool.Exec(ctx, `delete from players where note = $1`, marker) })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nhl, _ := registry.Get("nhl")
	rules := nhl.Defaults
	rules.Roster.Main, rules.Roster.Reserve = 2, 2
	rules.Lineup.Slots = []settings.Slot{{Name: "C", Positions: []string{"C"}, Count: 1}}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Roster Deadline Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nhl", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Slug string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, slug, me := view.Leagues[0].ID, view.Franchises[0].Slug, view.Franchises[0].ID
	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nhl", FullName: "Zrd One", Positions: []string{"C"}, Status: "active", Note: marker},
		{Competition: "nhl", FullName: "Zrd Two", Positions: []string{"C"}, Status: "active", Note: marker},
	}, nil)
	var page struct{ Players []struct{ ID string } }
	ann.want(http.StatusOK, "GET", "/api/players?q=Zrd+", nil, &page)
	rosterURL := "/api/leagues/" + league + "/roster/"
	for _, p := range page.Players {
		ann.want(http.StatusNoContent, "POST", rosterURL+"add", map[string]any{"player_id": p.ID, "list": "main", "force": true}, nil)
	}
	one := page.Players[0].ID
	today := sportsday.Today()
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(),
		"starts_on": today.AddDate(0, 0, 7).Format(time.DateOnly), "ends_on": today.AddDate(0, 3, 0).Format(time.DateOnly),
	}, nil)
	type season struct {
		ID       string
		Deadline *string
		Expired  bool
		Locked   bool
	}
	current := func() season {
		t.Helper()
		var v struct{ Rosters []struct{ Season *season } }
		ann.want(http.StatusOK, "GET", "/api/franchises/"+slug, nil, &v)
		return *v.Rosters[0].Season
	}
	deadline := func(at *time.Time) {
		t.Helper()
		ann.want(http.StatusNoContent, "PUT", "/api/admin/seasons/"+current().ID+"/roster-deadline", map[string]any{"deadline": at}, nil)
	}

	// A deadline still to come changes nothing yet.
	soon := time.Now().Add(48 * time.Hour)
	deadline(&soon)
	if s := current(); s.Deadline == nil || s.Expired || s.Locked {
		t.Fatalf("with a deadline ahead: %+v", s)
	}
	ann.want(http.StatusNoContent, "POST", rosterURL+"move", map[string]any{"player_id": one, "list": "reserve"}, nil)

	// Once it passes, the lists stand as they were.
	past := time.Now().Add(-time.Minute)
	deadline(&past)
	if s := current(); !s.Expired || !s.Locked {
		t.Fatalf("after the deadline: %+v, want expired and locked", s)
	}
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", map[string]any{"player_id": one, "list": "main"}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"season", map[string]any{"reserve": []string{}}, nil)
	ann.want(http.StatusNoContent, "POST", rosterURL+"season", map[string]any{"reserve": []string{one}, "franchise_id": me, "force": true}, nil) // the commissioner still can

	// Clearing the deadline lifts it for anyone yet to set a roster.
	deadline(nil)
	if s := current(); s.Deadline != nil || s.Expired {
		t.Fatalf("after clearing the deadline: %+v", s)
	}
}
