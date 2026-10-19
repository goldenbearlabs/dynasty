package players

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRosterPerformanceWindowsAndRanks(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const sport = "roster_stats_fixture"
	cleanup := func() {
		pool.Exec(ctx, `delete from games where competition = $1`, sport)
		pool.Exec(ctx, `delete from players where competition = $1`, sport)
	}
	cleanup()
	t.Cleanup(func() { dbtest.Reset(t, pool); cleanup() })
	run := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(pool)
	dynasty, err := q.CreateDynasty(ctx, db.CreateDynastyParams{Name: "Stats", Settings: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	league, err := q.CreateLeague(ctx, db.CreateLeagueParams{DynastyID: dynasty.ID, Competition: sport, Name: "Stats", Settings: json.RawMessage(`{"scoring":{"pts":2,"reb":1,"tov":-1}}`)})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := q.CreateFranchise(ctx, db.CreateFranchiseParams{DynastyID: dynasty.ID, Name: "Owner", ManagerName: "Owner", Slug: "owner", InviteToken: pgtype.Text{String: "roster-stats-test", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	player := func(name string, positions []string) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `insert into players (competition,status,full_name,positions) values ($1,'active',$2,$3) returning id`, sport, name, positions).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a := player("Roster A", []string{"PG", "SG"})
	b := player("Free agent B", []string{"PG"})
	c := player("Free agent C", []string{"SG"})
	d := player("Reserve with no games", []string{"C"})
	f := player("Free agent tied", []string{"PG"})
	run(`insert into roster_entries (league_id,franchise_id,player_id,list,acquired_via) values ($1,$2,$3,'main','commissioner'),($1,$2,$4,'reserve','commissioner')`, league.ID, owner.ID, a, d)
	today := sportsday.Today()
	game := func(offset int, values map[pgtype.UUID]string, status string) {
		t.Helper()
		var id pgtype.UUID
		day := today.AddDate(0, 0, offset)
		if err := pool.QueryRow(ctx, `insert into games (competition,provider_id,day,starts_at,status) values ($1,$2,$3,$4,$5) returning id`, sport, "roster-stats-"+strconv.Itoa(offset), day, sportsday.Start(day).Add(12*time.Hour), status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for p, stats := range values {
			run(`insert into stat_lines (game_id,player_id,stats) values ($1,$2,$3::jsonb)`, id, p, stats)
		}
	}
	game(0, map[pgtype.UUID]string{a: `{"pts":10,"reb":4,"tov":1}`, b: `{"pts":20}`, c: `{"pts":5}`, f: `{"pts":16.5}`}, "live")
	for _, g := range []struct {
		offset int
		stats  string
	}{{-6, `{"pts":5}`}, {-7, `{"pts":20}`}, {-13, `{"pts":1}`}, {-14, `{"pts":2}`}, {-29, `{"pts":1}`}, {-30, `{"pts":1000}`}, {1, `{"pts":999}`}, {-1, `{}`}, {-2, `{"pts":0,"reb":0}`}} {
		game(g.offset, map[pgtype.UUID]string{a: g.stats}, "final")
	}
	service := NewService(pool, nil)
	row := func(result RosterStats, id pgtype.UUID) RosterStatPlayer {
		t.Helper()
		for _, p := range result.Players {
			if p.PlayerID == id {
				return p
			}
		}
		t.Fatalf("missing roster player %v", id)
		return RosterStatPlayer{}
	}
	for _, tc := range []struct {
		window string
		gp     int64
		fp     float64
	}{{"today", 1, 23}, {"7", 2, 33}, {"14", 4, 75}, {"30", 6, 81}} {
		result, err := service.RosterPerformance(ctx, league, owner.ID, tc.window, "")
		if err != nil {
			t.Fatal(err)
		}
		got := row(result, a)
		if got.Games != tc.gp || got.Points != tc.fp || math.Abs(got.PointsPerGame-tc.fp/float64(tc.gp)) > 1e-9 {
			t.Fatalf("window %s: %+v", tc.window, got)
		}
		if len(result.Players) != 2 || result.RankedPlayers != 4 {
			t.Fatalf("must return only the roster but rank all competitors: %+v", result)
		}
		missing := row(result, d)
		if missing.OverallRank != nil || len(missing.PositionRanks) != 0 || missing.Games != 0 {
			t.Fatalf("unplayed reserve must remain unranked: %+v", missing)
		}
		if tc.window == "7" && (*got.OverallRank != 2 || got.PositionRanks["PG"] != 2 || got.PositionRanks["SG"] != 1) {
			t.Fatalf("global and multi-position tied ranks: %+v", got)
		}
	}
	season := func(id pgtype.UUID, year int, label, team string, games int, stats string) {
		run(`insert into player_seasons (player_id,competition,year,label,team,games,stats) values ($1,$2,$3,$4,$5,$6,$7::jsonb)`, id, sport, year, label, team, games, stats)
	}
	year := today.Year()
	current := strconv.Itoa(year)
	past := strconv.Itoa(year - 1)
	season(d, year, current, "EMPTY", 0, `{}`)
	fallback, err := service.RosterPerformance(ctx, league, owner.ID, "current", "")
	if err != nil || fallback.Source != "games" || row(fallback, a).Games == 0 {
		t.Fatalf("empty season placeholders must fall back to recorded games: %+v, %v", fallback, err)
	}
	season(a, year, current, "AAA", 10, `{"pts":100,"reb":20,"tov":5}`)
	season(a, year, current, "BBB", 5, `{"pts":50,"reb":10,"tov":2}`)
	season(b, year, current, "AAA", 20, `{"pts":200}`)
	season(c, year, current, "AAA", 10, `{"pts":5}`)
	season(a, year-1, past, "AAA", 10, `{"pts":90}`)
	season(b, year-1, past, "AAA", 10, `{"pts":80}`)
	season(a, year+2, past, "AAA", 10, `{"pts":9999}`) // mislabeled provider row must never double the past season
	result, err := service.RosterPerformance(ctx, league, owner.ID, "current", "")
	if err != nil {
		t.Fatal(err)
	}
	got := row(result, a)
	if got.Games != 15 || got.Points != 323 || got.Stats["pts"] != 150 || got.Stats["reb"] != 30 || *got.OverallRank != 2 || result.Source != "season" {
		t.Fatalf("current season split totals: %+v, %+v", result, got)
	}
	result, err = service.RosterPerformance(ctx, league, owner.ID, "season", past)
	if err != nil {
		t.Fatal(err)
	}
	got = row(result, a)
	if got.Points != 180 || got.Games != 10 || *got.OverallRank != 1 || got.PositionRanks["PG"] != 1 || len(result.Seasons) != 2 {
		t.Fatalf("historical totals and rank: %+v, %+v", result, got)
	}
	if _, err := service.RosterPerformance(ctx, league, owner.ID, "season", "not-a-season"); err == nil {
		t.Fatal("unknown season accepted")
	}
	if _, err := service.RosterPerformance(ctx, league, owner.ID, "8", ""); err == nil {
		t.Fatal("invalid rolling window accepted")
	}
	// Changing roster ownership does not change the comparison pool.
	run(`update roster_entries set list='reserve' where league_id=$1 and player_id=$2`, league.ID, a)
	again, err := service.RosterPerformance(ctx, league, owner.ID, "season", past)
	if err != nil || *row(again, a).OverallRank != 1 {
		t.Fatalf("reserve placement changed rank: %+v, %v", again, err)
	}
}

func TestSportSeasonNumbering(t *testing.T) {
	for _, tc := range []struct {
		sport, date, label string
		year               int
	}{{"nba", "2026-10-19", "2026-27", 2027}, {"nhl", "2026-02-01", "2025-26", 2026}, {"cbb", "2026-11-01", "2026-27", 2027}, {"nfl", "2027-01-15", "2026", 2026}, {"nfl", "2026-09-01", "2026", 2026}, {"mlb", "2026-06-01", "2026", 2026}, {"wnba", "2026-06-01", "2026", 2026}} {
		day, _ := time.Parse(time.DateOnly, tc.date)
		got, _ := sportSeason(tc.sport, day)
		if got.Label != tc.label || got.Year != tc.year {
			t.Errorf("%s %s: %+v", tc.sport, tc.date, got)
		}
	}
}
