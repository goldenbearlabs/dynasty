package web_test

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/players"
	"crossover/internal/scoring"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// TestPickedGameMissed covers a starter whose picked game, in a slot that
// counts one game a week, ends without him recording a stat: a later game
// of his counts in its place and he may be picked again; with none left, his
// latest earlier game counts. And once a game counts for him, he cannot be
// dropped or sent to reserve until the week is over.
func TestPickedGameMissed(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "missed_game_test"
	removeData := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zm-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'zm-%'`)
	}
	removeData()
	t.Cleanup(func() { dbtest.Reset(t, pool); removeData() })

	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Roster.Main, rules.Roster.Reserve = 4, 4
	rules.Lineup.WeekStart = strings.ToLower(today.AddDate(0, 0, -2).Weekday().String())
	rules.Lineup.Slots = []settings.Slot{
		{Name: "G", Positions: []string{"G"}, Count: 1, GamesPerWeek: 1},
		{Name: "UTIL", Positions: []string{settings.AnyPosition}, Count: 1, GamesPerWeek: 1},
	}
	rules.Scoring = map[string]float64{"pts": 1}
	rules.Format.Type = "total_points"
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Missed Game Test", Settings: dynastySettings(10),
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

	ann.want(http.StatusCreated, "POST", "/api/admin/players", []players.NewPlayer{
		{Competition: "nba", FullName: "Zqmiss Guard", Positions: []string{"G"}, Status: "active", Note: marker},
		{Competition: "nba", FullName: "Zqmiss Wing", Positions: []string{"F"}, Status: "active", Note: marker},
	}, nil)
	var page struct {
		Players []struct {
			ID       string
			FullName string `json:"full_name"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/players?q=Zqmiss", nil, &page)
	id := map[string]string{}
	for _, p := range page.Players {
		id[p.FullName] = p.ID
		ann.want(http.StatusNoContent, "POST", "/api/leagues/"+league+"/roster/add", map[string]any{"player_id": p.ID, "list": "main", "force": true}, nil)
	}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	sql(`insert into pro_teams (competition, provider_id, abbrev, name) values ('nba', 'zm-x', 'ZMX', 'X'), ('nba', 'zm-y', 'ZMY', 'Y')`)
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zm-x') where id = $1`, id["Zqmiss Guard"])
	sql(`update players set pro_team_id = (select id from pro_teams where provider_id = 'zm-y') where id = $1`, id["Zqmiss Wing"])
	// Every game is on a day of this week up to today, but starts later, so
	// the lineup can be set before any has begun; each is then played out.
	game := func(providerID, home string, offset int) {
		sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
		     values ('nba', $1, $2, $3, 'scheduled', (select id from pro_teams where provider_id = $4))`,
			providerID, today.AddDate(0, 0, offset), time.Now().Add(time.Duration(10+offset)*time.Hour), home)
	}
	line := func(gameID, player, stats string) {
		sql(`insert into stat_lines (game_id, player_id, stats) values ((select id from games where provider_id = $1), $2, $3::jsonb)
		     on conflict (game_id, player_id) do update set stats = excluded.stats`, gameID, id[player], stats)
	}
	finish := func(gameID string, hoursAgo int) {
		sql(`update games set status = 'final', starts_at = now() - make_interval(hours => $2) where provider_id = $1`, gameID, hoursAgo)
	}
	// The guard's team plays on each of the week's three days so far; the wing's on the last two.
	game("zm-x1", "zm-x", -2)
	game("zm-x2", "zm-x", -1)
	game("zm-x3", "zm-x", 0)
	game("zm-y1", "zm-y", -1)
	game("zm-y2", "zm-y", 0)
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{
		"league_id": league, "year": today.Year(), "starts_on": day(-7), "ends_on": day(30),
	}, nil)

	lineupURL := "/api/leagues/" + league + "/lineup"
	entry := func(slot, player, countsFrom string) map[string]string {
		return map[string]string{"slot": slot, "player_id": id[player], "counts_from": countsFrom}
	}
	type seen struct {
		CountsFrom string `json:"counts_from"`
		Locked     bool
		Points     float64
	}
	lineup := func() map[string]seen {
		t.Helper()
		var v struct {
			Players []struct {
				FullName string `json:"full_name"`
				seen
			}
		}
		ann.want(http.StatusOK, "GET", lineupURL+"?franchise="+franchise+"&day="+day(0), nil, &v)
		byName := map[string]seen{}
		for _, p := range v.Players {
			byName[p.FullName] = p.seen
		}
		return byName
	}
	total := func() float64 {
		t.Helper()
		assertCountedGameTotals(t, pool, league)
		var all []struct{ Rows []struct{ Points float64 } }
		ann.want(http.StatusOK, "GET", "/api/standings", nil, &all)
		return all[0].Rows[0].Points
	}
	rosterURL := "/api/leagues/" + league + "/roster/"

	// Before any game: the guard is picked for his last game, the wing for his first.
	ann.want(http.StatusNoContent, "PUT", lineupURL, map[string]any{"day": day(0), "entries": []map[string]string{
		entry("G", "Zqmiss Guard", day(0)), entry("UTIL", "Zqmiss Wing", day(-1)),
	}}, nil)

	// --- games before the picked one count nothing while it is to come -------
	finish("zm-x1", 8)
	line("zm-x1", "Zqmiss Guard", `{"pts": 20}`)
	finish("zm-x2", 6)
	line("zm-x2", "Zqmiss Guard", `{"pts": 8}`)
	if got, p := total(), lineup()["Zqmiss Guard"]; got != 0 || p.Points != 0 || p.Locked {
		t.Fatalf("total = %v, guard %+v; want nothing counted and him free while his picked game is to come", got, p)
	}

	// --- the wing misses his picked game: his next one takes its place -------
	finish("zm-y1", 7) // no stat line: he did not play
	if got, p := total(), lineup()["Zqmiss Wing"]; got != 0 || p.Locked {
		t.Fatalf("total = %v, wing %+v; want nothing counted and him free to be picked again", got, p)
	}
	// So he may be picked again, as if his game had not begun.
	ann.want(http.StatusNoContent, "PUT", lineupURL, map[string]any{"day": day(0), "entries": []map[string]string{
		entry("G", "Zqmiss Guard", ""), entry("UTIL", "Zqmiss Wing", day(0)),
	}}, nil)
	if p := lineup()["Zqmiss Wing"]; p.CountsFrom != day(0) {
		t.Fatalf("wing = %+v; want picked for %s", p, day(0))
	}

	// --- the guard misses his last game: the one before it counts ------------
	finish("zm-x3", 4)
	line("zm-x3", "Zqmiss Guard", `{"pts": 0}`) // on the floor, but nothing recorded
	if got, p := total(), lineup()["Zqmiss Guard"]; math.Abs(got-8) > 0.001 || p.Points != 8 || !p.Locked {
		t.Fatalf("total = %v, guard %+v; want 8 from his game before the last, and him locked", got, p)
	}
	// Never a game from before he came into the slot.
	sql(`update lineup_entries set joined_on = $2 where player_id = $1`, id["Zqmiss Guard"], today)
	if got := total(); got != 0 {
		t.Fatalf("total = %v, want 0: his earlier games came before he joined the slot", got)
	}
	sql(`update lineup_entries set joined_on = null where player_id = $1`, id["Zqmiss Guard"])

	// --- a player whose game counts stays until the week is over -------------
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"drop", map[string]any{"player_id": id["Zqmiss Guard"]}, nil)
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"move", map[string]any{"player_id": id["Zqmiss Guard"], "list": "reserve"}, nil)
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, map[string]any{"day": day(0), "entries": []map[string]string{
		entry("UTIL", "Zqmiss Wing", ""),
	}}, nil)
	if got := total(); math.Abs(got-8) > 0.001 {
		t.Fatalf("total = %v, want still 8", got)
	}

	// --- the wing's new pick is played and counts -----------------------------
	finish("zm-y2", 2)
	line("zm-y2", "Zqmiss Wing", `{"pts": 15}`)
	if got, p := total(), lineup()["Zqmiss Wing"]; math.Abs(got-23) > 0.001 || p.Points != 15 || !p.Locked {
		t.Fatalf("total = %v, wing %+v; want 23 in all and him locked with 15", got, p)
	}
	ann.want(http.StatusUnprocessableEntity, "POST", rosterURL+"drop", map[string]any{"player_id": id["Zqmiss Wing"]}, nil)

	// --- a period settled under the old count is added up again -------------
	sql(`insert into periods (season_id, seq, starts_on, ends_on, is_playoff)
	     select id, 1, $2, $3, false from seasons where league_id = $1`, league, day(-2), day(0))
	sql(`insert into period_scores (period_id, franchise_id, points)
	     select p.id, $2, 0 from periods p join seasons s on s.id = p.season_id where s.league_id = $1`, league, franchise)
	stored := func() (points float64) {
		t.Helper()
		pool.QueryRow(ctx, `select points from period_scores where franchise_id = $1`, franchise).Scan(&points)
		return points
	}
	rescorer := scoring.NewService(pool)
	changed, err := rescorer.Rescore(ctx, "nba", today.AddDate(0, 0, -2), false)
	if err != nil || len(changed) != 1 || changed[0].Before != 0 || math.Abs(changed[0].After-23) > 0.001 || stored() != 0 {
		t.Fatalf("dry run = %+v, %v, stored %v; want one change from 0 to 23 and nothing written", changed, err, stored())
	}
	if _, err := rescorer.Rescore(ctx, "nba", today.AddDate(0, 0, -2), true); err != nil || math.Abs(stored()-23) > 0.001 {
		t.Fatalf("after rescoring, stored %v, %v; want 23", stored(), err)
	}
	if changed, err := rescorer.Rescore(ctx, "nba", today.AddDate(0, 0, -2), false); err != nil || len(changed) != 0 {
		t.Fatalf("second run = %+v, %v; want no changes", changed, err)
	}

	// --- the commissioner may count any game of the week, even one played ----
	ann.want(http.StatusUnprocessableEntity, "PUT", lineupURL, map[string]any{"day": day(0), "entries": []map[string]string{
		entry("G", "Zqmiss Guard", day(-2)), entry("UTIL", "Zqmiss Wing", ""),
	}}, nil)
	ann.want(http.StatusNoContent, "PUT", lineupURL, map[string]any{"day": day(0), "force": true, "entries": []map[string]string{
		entry("G", "Zqmiss Guard", day(-2)), entry("UTIL", "Zqmiss Wing", ""),
	}}, nil)
	if got, p := total(), lineup()["Zqmiss Guard"]; math.Abs(got-35) > 0.001 || p.Points != 20 || p.CountsFrom != day(-2) {
		t.Fatalf("total = %v, guard %+v; want his 20 from %s counted, 35 in all", got, p, day(-2))
	}

	// The commissioner may still overrule the hold.
	ann.want(http.StatusNoContent, "POST", rosterURL+"drop", map[string]any{"player_id": id["Zqmiss Wing"], "force": true}, nil)
}
