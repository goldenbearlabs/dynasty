package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/dynasty"
)

func TestScheduleGameCountsForEveryLeague(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "test_period_count_"
	cleanup := func() {
		dbtest.Reset(t, pool)
		if _, err := pool.Exec(ctx, `delete from games where provider_id like $1`, marker+"%"); err != nil {
			t.Fatal(err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	server, registry := startServer(t, pool)
	ann := newBrowser(t, server.URL)
	leagues := []dynasty.LeagueSetup{}
	for _, c := range registry {
		leagues = append(leagues, dynasty.LeagueSetup{Competition: c.Key, Settings: c.Defaults})
	}
	ann.want(http.StatusCreated, "POST", "/api/dynasty", dynasty.Setup{
		Name: "Game Counts", Settings: dynastySettings(10), Leagues: leagues,
		Franchises: []dynasty.FranchiseSetup{{Name: "Ann", ManagerName: "Ann"}, {Name: "Bob", ManagerName: "Bob"}},
		Account:    dynasty.Account{Email: "ann@example.com", Password: "correct horse"},
	}, nil)
	var view struct {
		Leagues []struct{ ID, Competition string }
	}
	ann.want(http.StatusOK, "GET", "/api/dynasty", nil, &view)
	first := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	for sport, c := range registry {
		for day := 0; day < 3; day++ {
			for game := 0; game < sport+day+1; game++ {
				date := first.AddDate(0, 0, day)
				if _, err := pool.Exec(ctx, `insert into games(competition,provider_id,day,starts_at,status) values($1,$2,$3,$4,$5)`, c.Key, fmt.Sprintf("%s%s_%d_%d", marker, c.Key, day, game), date, date.Add(18*time.Hour), []string{"scheduled", "live", "final"}[game%3]); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for sport, c := range registry {
		t.Run(c.Key, func(t *testing.T) {
			leagueID := ""
			for _, l := range view.Leagues {
				if l.Competition == c.Key {
					leagueID = l.ID
				}
			}
			if leagueID == "" {
				t.Fatal("league missing")
			}
			path := "/api/leagues/" + leagueID + "/game-counts"
			var counts []db.GameDayCount
			ann.want(http.StatusOK, "GET", path+"?starts_on=2031-01-01&ends_on=2031-01-02", nil, &counts)
			if len(counts) != 2 || counts[0].Day != "2031-01-01" || counts[1].Day != "2031-01-02" || counts[0].Games != int64(sport+1) || counts[1].Games != int64(sport+2) {
				t.Fatalf("game counts = %+v", counts)
			}
			ann.want(http.StatusOK, "GET", path+"?starts_on=2031-01-02&ends_on=2031-01-02", nil, &counts)
			if len(counts) != 1 || counts[0].Games != int64(sport+2) {
				t.Fatalf("single-day count = %+v", counts)
			}
			ann.want(http.StatusOK, "GET", path+"?starts_on=2031-01-04&ends_on=2031-01-05", nil, &counts)
			if counts == nil || len(counts) != 0 {
				t.Fatalf("empty calendar = %+v", counts)
			}
			for _, query := range []string{"", "?starts_on=invalid&ends_on=2031-01-02", "?starts_on=2031-01-02&ends_on=2031-01-01", "?starts_on=2031-01-01&ends_on=2035-01-01"} {
				ann.want(http.StatusUnprocessableEntity, "GET", path+query, nil, nil)
			}
		})
	}
}
