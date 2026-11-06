package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// TestLineupLocks covers who may be moved in a league whose slots count one
// picked game a week: anyone may be moved or dropped until a game he started
// for has counted, whatever he played before or on the bench.
func TestLineupLocks(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "lineup_lock_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zl-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'zl-%'`)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Roster.Main, rules.Roster.Reserve = 10, 4
	// The week began three days ago and has three days still to come.
	rules.Lineup.WeekStart = strings.ToLower(today.AddDate(0, 0, -3).Weekday().String())
	rules.Lineup.Slots = []settings.Slot{
		{Name: "G", Positions: []string{"G"}, Count: 1, GamesPerWeek: 1},
		{Name: "C", Positions: []string{"C"}, Count: 1, GamesPerWeek: 1},
	}
	rules.Scoring = map[string]float64{"pts": 1}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Lineup Lock Test", Settings: dynastySettings(10),
		Leagues:    []dynasty.LeagueSetup{{Competition: "nba", Settings: rules}},
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league, franchise := view.Leagues[0].ID, view.Franchises[0].ID

	// One pro team each, so each has his own games.
	roster := map[string][]string{
		"Zqlock Starter": {"G"}, "Zqlock Benchguard": {"G"}, "Zqlock Doneguard": {"G"},
		"Zqlock Injured": {"C"}, "Zqlock Hayes": {"C"},
	}
	var newPlayers []players.NewPlayer
	for name, positions := range roster {
		newPlayers = append(newPlayers, players.NewPlayer{Competition: "nba", FullName: name, Positions: positions, Status: "active", Note: marker})
	}
	ann.want(http.StatusCreated, "POST", "/api/admin/players", newPlayers, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zqlock", nil, &page)
	id := map[string]string{}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range page.Players {
		id[p.FullName] = p.ID
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": p.ID, "list": "main", "force": true}, nil)
		team := "zl-" + string(rune('a'+i))
		sql(`insert into pro_teams (competition, provider_id, abbrev, name) values ('nba', $1, upper($1), $1)`, team)
		sql(`update players set pro_team_id = (select id from pro_teams where provider_id = $1) where id = $2`, team, p.ID)
	}
	teamOf := func(player string) string {
		var team string
		pool.QueryRow(ctx, `select t.provider_id from players p join pro_teams t on t.id = p.pro_team_id where p.id = $1`, id[player]).Scan(&team)
		return team
	}
	// Every game starts in the future, so the week's first lineup can be set
	// before any has begun; those on past days are then played out.
	game := func(player string, offset int) string {
		gameID := teamOf(player) + day(offset)
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
		     values ('nba', $1, $2, $3, 'scheduled', (select id from pro_teams where provider_id = $4))`,
			gameID, today.AddDate(0, 0, offset), time.Now().Add(time.Duration(30+24*offset)*time.Hour), teamOf(player))
		return gameID
	}
	finish := func(gameID, player, stats string, hoursAgo int) {
		sql(`update games set status = 'final', starts_at = now() - make_interval(hours => $2) where provider_id = $1`, gameID, hoursAgo)
		if stats != "" {
			sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, $3::jsonb)`, gameID, id[player], stats)
		}
	}
	starterEarly, _ := game("Zqlock Starter", -2), game("Zqlock Starter", 2)
	benchEarly, _ := game("Zqlock Benchguard", -2), game("Zqlock Benchguard", 1)
	doneEarly := game("Zqlock Doneguard", -2)
	injuredPick := game("Zqlock Injured", -1)
	hayesEarly, _ := game("Zqlock Hayes", -2), game("Zqlock Hayes", 3)
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(), "starts_on": day(-7), "ends_on": day(30),
	}, nil)

	lineupURL := "/api/leagues/" + league + "/lineup"
	entry := func(slot, player, countsFrom string) map[string]any {
		return map[string]any{"slot": slot, "player_id": id[player], "counts_from": countsFrom, "auto": countsFrom == ""}
	}
	set := func(status int, entries ...map[string]any) {
		ann.want(status, "PUT", lineupURL, map[string]any{"day": day(0), "entries": entries}, nil)
	}
	// Each case starts from the week's first lineup, its own failure its own.
	run := func(name string, steps func()) {
		t.Run(name, func(t *testing.T) {
			parent := ann.t
			ann.t = t
			defer func() { ann.t = parent }()
			ann.want(http.StatusNoContent, "PUT", lineupURL, map[string]any{"day": day(0), "force": true, "entries": []map[string]any{
				entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Injured", day(-1)),
			}}, nil)
			steps()
		})
	}

	// Before the week: the starter is picked for his later game, the injured
	// centre for his game yesterday.
	set(http.StatusNoContent, entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Injured", day(-1)))
	finish(starterEarly, "Zqlock Starter", `{"pts": 30}`, 60)
	finish(benchEarly, "Zqlock Benchguard", `{"pts": 25}`, 60)
	finish(doneEarly, "Zqlock Doneguard", `{"pts": 25}`, 60)
	finish(hayesEarly, "Zqlock Hayes", `{"pts": 12}`, 60)
	finish(injuredPick, "Zqlock Injured", "", 30) // hurt: no stat line

	run("swap a starter whose pick is to come for a bench player who has played", func() {
		set(http.StatusNoContent, entry("G", "Zqlock Benchguard", ""), entry("C", "Zqlock Injured", day(-1)))
		set(http.StatusNoContent, entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Injured", day(-1)))
	})
	run("swap a starter for a bench player with no game left", func() {
		set(http.StatusNoContent, entry("G", "Zqlock Doneguard", ""), entry("C", "Zqlock Injured", day(-1)))
		// Coming in, nothing he has already played counts.
		var v struct {
			Players []struct {
				FullName string `json:"full_name"`
				Points   float64
			}
		}
		ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise+"&day="+day(0), nil, &v)
		for _, p := range v.Players {
			if p.FullName == "Zqlock Doneguard" && p.Points != 0 {
				ann.t.Errorf("%s has %v points; want none from games played before he came in", p.FullName, p.Points)
			}
		}
		set(http.StatusNoContent, entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Injured", day(-1)))
	})
	run("an auto starter who has played a game is free until his last", func() {
		set(http.StatusNoContent, entry("G", "Zqlock Starter", ""), entry("C", "Zqlock Injured", day(-1)))
		set(http.StatusNoContent, entry("G", "Zqlock Benchguard", day(1)), entry("C", "Zqlock Injured", day(-1)))
	})
	run("sub in for a hurt starter, pick a later game, then drop him", func() {
		// As the editor saves: on auto as he comes in, then the game picked.
		set(http.StatusNoContent, entry("G", "Zqlock Benchguard", day(1)), entry("C", "Zqlock Hayes", ""))
		set(http.StatusNoContent, entry("G", "Zqlock Benchguard", day(1)), entry("C", "Zqlock Hayes", day(3)))
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/drop", map[string]any{"player_id": id["Zqlock Hayes"]}, nil)
	})

	// On auto from before the week: the editor's default for anyone put in a
	// slot, and what auto-set does.
	onAutoFromTheStart := func(player string) {
		sql(`update lineup_entries set counts_last = true, counts_from = null, joined_on = null where player_id = $1`, id[player])
	}
	run("an auto starter set before the week, who has played, is free while his last game is to come", func() {
		onAutoFromTheStart("Zqlock Starter")
		set(http.StatusNoContent, entry("G", "Zqlock Benchguard", day(1)), entry("C", "Zqlock Injured", day(-1)))
	})
	run("an auto sub who has played may be picked for a later game and dropped", func() {
		ann.call("POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": id["Zqlock Hayes"], "list": "main", "force": true}) // back, if dropped before
		set(http.StatusNoContent, entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Hayes", ""))
		onAutoFromTheStart("Zqlock Hayes")
		set(http.StatusNoContent, entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Hayes", day(3)))
		onAutoFromTheStart("Zqlock Hayes")
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/drop", map[string]any{"player_id": id["Zqlock Hayes"]}, nil)
	})
	run("an auto sub who has played may be dropped while his last game is to come", func() {
		ann.call("POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": id["Zqlock Hayes"], "list": "main", "force": true}) // back, if dropped before
		set(http.StatusNoContent, entry("G", "Zqlock Starter", day(2)), entry("C", "Zqlock Hayes", ""))
		onAutoFromTheStart("Zqlock Hayes")
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/drop", map[string]any{"player_id": id["Zqlock Hayes"]}, nil)
	})
}
