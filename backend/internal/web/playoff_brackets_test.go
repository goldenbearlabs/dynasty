package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
	"crossover/internal/scoring"
	"crossover/internal/sportsday"
)

func TestFourteenTeamPlayoffBrackets(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	t.Cleanup(func() { dbtest.Reset(t, pool) })
	ctx := context.Background()
	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	nba, _ := registry.Get("nba")
	rules := nba.Defaults
	rules.Format.MatchupDays = 1
	if rules.Format.PlayoffTeams != 8 || rules.Format.Reseed || !rules.Format.ConsolationEnabled() {
		t.Fatal("wrong default playoff rules")
	}
	franchises := []dynasty.FranchiseSetup{}
	for i := 1; i <= 14; i++ {
		name := fmt.Sprintf("Team %02d", i)
		franchises = append(franchises, dynasty.FranchiseSetup{Name: name, ManagerName: name})
	}
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{Name: "Fixed Brackets", Settings: dynastySettings(10), Leagues: []dynasty.LeagueSetup{{Competition: "nba", Settings: rules}}, Franchises: franchises, Account: dynasty.Account{Email: "ann@example.com", Password: "correct horse"}}, nil)
	var view struct {
		Leagues    []struct{ ID string }
		Franchises []struct{ ID, Name string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	league := view.Leagues[0].ID
	team := map[int]string{}
	seed := map[string]int{}
	for _, f := range view.Franchises {
		var n int
		fmt.Sscanf(f.Name, "Team %d", &n)
		team[n] = f.ID
		seed[f.ID] = n
	}
	today := sportsday.Today()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	var season struct{ ID string }
	ann.want(http.StatusCreated, "POST", "/api/admin/seasons", map[string]any{"league_id": league, "year": today.Year(), "starts_on": day(7), "ends_on": day(10)}, &season)
	var periods []scoring.SchedulePeriod
	read := func() { t.Helper(); ann.want(http.StatusOK, "GET", "/api/leagues/"+league+"/schedule", nil, &periods) }
	read()
	if len(periods) != 4 || periods[0].IsPlayoff || !periods[1].IsPlayoff {
		t.Fatalf("schedule = %+v", periods)
	}
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	sql(`update seasons set starts_on=$2 where id=$1`, season.ID, day(-2))
	sql(`update periods set starts_on=$2,ends_on=$3 where id=$1`, periods[0].ID, day(-2), day(-1))
	// One settled regular-season matchup per team: seeds 1–7 won, 8–14 lost,
	// and descending points for breaks the ties within those record groups.
	for i := 1; i <= 14; i++ {
		sql(`insert into period_scores(period_id,franchise_id,points) values($1,$2,$3)`, periods[0].ID, team[i], float64(1500-i*100))
	}
	advance := func() {
		t.Helper()
		ann.want(http.StatusNoContent, "POST", "/api/admin/playoffs/advance", nil, nil)
		read()
	}
	pairs := func(index int, bracket string) [][2]int {
		found := [][2]int{}
		for _, m := range periods[index].Matchups {
			if m.Bracket == bracket {
				found = append(found, [2]int{seed[m.Home.String()], seed[m.Away.String()]})
			}
		}
		return found
	}
	same := func(got, want [][2]int) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("pairs=%v, want %v", got, want)
		}
	}
	advance()
	same(pairs(1, "championship"), [][2]int{{1, 8}, {4, 5}, {2, 7}, {3, 6}})
	same(pairs(1, "consolation"), [][2]int{{14, 0}, {11, 10}, {13, 0}, {12, 9}})
	if len(periods[1].Matchups) != 8 {
		t.Fatal("expected four slots in each bracket")
	}
	advance() // Idempotent, and later rounds wait for the previous round.
	if len(periods[1].Matchups) != 8 || len(periods[2].Matchups) != 0 {
		t.Fatal("advanced too early or duplicated a round")
	}
	sql(`update periods set starts_on=$2,ends_on=$3 where id=$1`, periods[1].ID, day(-2), day(-1))
	advance()
	same(pairs(2, "championship"), [][2]int{{1, 4}, {2, 3}})
	same(pairs(2, "consolation"), [][2]int{{14, 11}, {13, 12}})
	sql(`update periods set starts_on=$2,ends_on=$3 where id=$1`, periods[2].ID, day(-2), day(-1))
	advance()
	same(pairs(3, "championship"), [][2]int{{1, 2}})
	same(pairs(3, "consolation"), [][2]int{{14, 13}})
	sql(`update periods set starts_on=$2,ends_on=$3 where id=$1`, periods[3].ID, day(-2), day(-1))
	var tables []struct {
		ConsolationChampion string `json:"consolation_champion_id"`
		Rows                []struct {
			PointsFor     float64 `json:"points_for"`
			PointsAgainst float64 `json:"points_against"`
		}
	}
	ann.want(http.StatusOK, "GET", "/api/standings", nil, &tables)
	if tables[0].ConsolationChampion != team[14] || tables[0].Rows[0].PointsFor != 1400 || tables[0].Rows[0].PointsAgainst != 100 {
		t.Fatalf("standings = %+v", tables)
	}
	ann.want(http.StatusNoContent, "POST", "/api/admin/seasons/"+season.ID+"/close", map[string]any{}, nil)
	var champion string
	if err := pool.QueryRow(ctx, `select champion_franchise_id::text from seasons where id=$1`, season.ID).Scan(&champion); err != nil {
		t.Fatal(err)
	}
	if champion != team[1] {
		t.Fatalf("champion = %s, want championship winner", champion)
	}
}
