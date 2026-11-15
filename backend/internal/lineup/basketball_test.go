package lineup

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestBasketballLockInCutover(t *testing.T) {
	for _, sport := range []string{"nba", "cbb", "wnba", "nhl", "mlb", "nfl"} {
		for _, day := range []string{"2026-11-09", "2026-11-16", "2026-11-23"} {
			first, _ := sportsday.Parse(day)
			got := basketballLockIn(sport, settings.Lineup{Period: settings.PeriodWeek}, first)
			want := (sport == "nba" || sport == "cbb" || sport == "wnba") && day >= "2026-11-16"
			if got != want {
				t.Errorf("%s %s: %v, want %v", sport, day, got, want)
			}
		}
	}
}

// Exercise the actual SQL shared by the editor and the official scorer,
// with fixed dates and an explicit clock so deadline tests never age out.
func TestBasketballLockInChoices(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	t.Cleanup(func() {
		dbtest.Reset(t, pool)
		pool.Exec(ctx, `delete from games where provider_id like 'lockin-test-%'`)
		pool.Exec(ctx, `delete from players where note = 'lockin-test'`)
		pool.Exec(ctx, `delete from pro_teams where provider_id = 'lockin-test'`)
	})
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	uuid := func(query string, args ...any) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	date := func(s string) time.Time {
		t.Helper()
		d, err := sportsday.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	clock := func(s string) time.Time {
		t.Helper()
		d, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	first, last := date("2026-11-16"), date("2026-11-22")
	dynasty := uuid(`insert into dynasties (name, settings) values ('Lockin Test', '{}') returning id`)
	franchise := uuid(`insert into franchises (dynasty_id,name,manager_name,slug,invite_token) values ($1,'Test','Test','test','lockin-test') returning id`, dynasty)
	rules := settings.League{Lineup: settings.Lineup{Period: settings.PeriodWeek, WeekStart: "monday", Lock: settings.LockGameStart,
		Slots: []settings.Slot{{Name: "G", Count: 1, Positions: []string{"G"}, GamesPerWeek: 1}}}, Scoring: map[string]float64{"pts": 1}}
	raw, _ := json.Marshal(rules)
	league := uuid(`insert into leagues (dynasty_id,competition,name,settings) values ($1,'nba','NBA',$2) returning id`, dynasty, raw)
	team := uuid(`insert into pro_teams (competition,provider_id,name,abbrev) values ('nba','lockin-test','Test','LKT') returning id`)
	player := uuid(`insert into players (competition,status,full_name,positions,pro_team_id,note) values ('nba','active','Lockin Player','{G}',$1,'lockin-test') returning id`, team)
	bench := uuid(`insert into players (competition,status,full_name,positions,pro_team_id,note) values ('nba','active','Lockin Bench','{G}',$1,'lockin-test') returning id`, team)
	for _, pid := range []pgtype.UUID{player, bench} {
		sql(`insert into roster_entries (league_id,franchise_id,player_id,list,acquired_via) values ($1,$2,$3,'main','test')`, league, franchise, pid)
	}
	sql(`insert into lineups (league_id,franchise_id,effective_on) values ($1,$2,$3)`, league, franchise, first)
	sql(`insert into lineup_entries (league_id,franchise_id,effective_on,slot,slot_index,player_id) values ($1,$2,$3,'G',0,$4)`, league, franchise, first, player)
	game := func(day, status string, min, pts int) pgtype.UUID {
		t.Helper()
		gid := uuid(`insert into games (competition,provider_id,day,starts_at,status,home_team_id) values ('nba',$1,$2,$3,$4,$5) returning id`,
			"lockin-test-"+day, date(day), clock(day+"T23:00:00Z"), status, team)
		for _, pid := range []pgtype.UUID{player, bench} {
			sql(`insert into stat_lines (game_id,player_id,stats) values ($1,$2,$3)`, gid, pid, fmt.Sprintf(`{"min":%d,"pts":%d}`, min, pts))
		}
		return gid
	}
	monday := game("2026-11-16", "final", 30, 20)
	wed := game("2026-11-18", "scheduled", 0, 0)
	friday := game("2026-11-20", "scheduled", 0, 0)
	sunday := game("2026-11-22", "scheduled", 0, 0)
	choices := func(at time.Time) map[pgtype.UUID]map[pgtype.UUID]db.ListBasketballGameChoicesRow {
		t.Helper()
		rows, err := q.ListBasketballGameChoices(ctx, db.ListBasketballGameChoicesParams{LeagueID: league, FranchiseID: franchise, FromDay: sportsday.Date(first), ToDay: sportsday.Date(last), AsOf: pgtype.Timestamptz{Time: at, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		out := map[pgtype.UUID]map[pgtype.UUID]db.ListBasketballGameChoicesRow{}
		for _, row := range rows {
			if out[row.PlayerID] == nil {
				out[row.PlayerID] = map[pgtype.UUID]db.ListBasketballGameChoicesRow{}
			}
			out[row.PlayerID][row.GameID] = row
		}
		return out
	}
	at := clock("2026-11-17T12:00:00Z")
	initial := choices(at)
	if !initial[player][monday].CanLockIn || initial[player][monday].Counts || initial[bench][monday].Eligible {
		t.Fatalf("initial choices: %+v", initial)
	}
	service := NewService(pool, nil)
	leagueRow := db.League{ID: league, Competition: "nba", Settings: raw}
	set := func(at time.Time, entries ...Entry) error {
		return service.setWithClock(ctx, leagueRow, franchise, first, entries, false, func() time.Time { return at })
	}
	// Bench after the game; its offer survives, including after a move back.
	if err := set(at, Entry{PlayerID: bench, Slot: "G"}); err != nil {
		t.Fatal(err)
	}
	if row := choices(at)[player][monday]; !row.CanLockIn || !row.Eligible {
		t.Fatalf("benching erased active offer: %+v", row)
	}
	if row := choices(at)[bench][monday]; row.Eligible || row.CanLockIn {
		t.Fatalf("bench game became eligible after promotion: %+v", row)
	}
	if err := set(at, Entry{PlayerID: bench, Slot: "G", LockGameID: monday}); err == nil {
		t.Fatal("API accepted a game played on the bench")
	}
	if err := set(at, Entry{PlayerID: player, Slot: "G", LockGameID: monday}); err != nil {
		t.Fatalf("move back and lock active offer: %v", err)
	}
	if err := set(at.Add(time.Minute), Entry{PlayerID: bench, Slot: "G"}); err == nil {
		t.Fatal("API moved a locked-in player")
	}
	sql(`delete from basketball_game_locks where league_id=$1`, league)
	at = clock("2026-11-18T22:00:00Z")
	if err := recordBasketballMove(ctx, q, league, franchise, first, player, "G", at); err != nil {
		t.Fatal(err)
	}
	if err := recordBasketballMove(ctx, q, league, franchise, first, bench, "", at); err != nil {
		t.Fatal(err)
	}
	if !choices(at)[player][monday].CanLockIn {
		t.Fatal("offer missing after moving back before deadline")
	}
	if choices(clock("2026-11-18T23:00:00Z"))[player][monday].CanLockIn {
		t.Fatal("offer survived next tipoff")
	}
	if err := set(clock("2026-11-18T23:00:00Z"), Entry{PlayerID: bench, Slot: "G"}); err == nil {
		t.Fatal("API allowed a swap at tipoff")
	}
	if err := set(clock("2026-11-18T23:00:00Z"), Entry{PlayerID: player, Slot: "G", LockGameID: monday}); err == nil {
		t.Fatal("API accepted an expired offer")
	}
	// A start while benched stays ineligible after a later promotion.
	if err := recordBasketballMove(ctx, q, league, franchise, first, player, "", clock("2026-11-19T12:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if err := recordBasketballMove(ctx, q, league, franchise, first, player, "G", clock("2026-11-21T12:00:00Z")); err != nil {
		t.Fatal(err)
	}
	sql(`update games set status='final' where id = any($1::uuid[])`, []pgtype.UUID{wed, friday, sunday})
	sql(`update stat_lines set stats='{"min":25,"pts":-5}' where game_id=$1`, wed)
	sql(`update stat_lines set stats='{"min":30,"pts":99}' where game_id=$1`, friday)
	sql(`update stat_lines set stats='{"min":0,"pts":100}' where game_id=$1`, sunday)
	end := clock("2026-11-23T09:00:00Z")
	rows := choices(end)[player]
	if !rows[wed].Counts || rows[friday].Eligible || rows[sunday].Counts || rows[monday].Counts {
		t.Fatalf("fallback must skip bench games and zero minutes: %+v", rows)
	}
	// Positive minutes qualify even when every scoring stat is zero.
	sql(`update stat_lines set stats='{"min":1,"pts":0}' where game_id=$1`, sunday)
	if rows := choices(end)[player]; !rows[sunday].Counts || rows[wed].Counts {
		t.Fatalf("zero-point appearance did not count: %+v", rows)
	}
	// The official score uses the same choices, with the configured scoring.
	sql(`update stat_lines set stats='{"min":0,"pts":0}' where game_id=$1`, sunday)
	scores, err := q.ListLineupPoints(ctx, db.ListLineupPointsParams{LeagueID: league, WeekStart: sportsday.Date(first), FromDay: sportsday.Date(first), ToDay: sportsday.Date(last), IncludeGames: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 || scores[0].Points != -5 {
		t.Fatalf("official fallback: %+v", scores)
	}
	// Doubled matchups allow two identities, never count a locked game twice,
	// and automatically fill only the remaining lock with an eligible game.
	season := uuid(`insert into seasons (league_id,year,starts_on,ends_on) values ($1,2026,$2,$3) returning id`, league, first, last)
	sql(`insert into periods (season_id,seq,starts_on,ends_on,double_games) values ($1,1,$2,$3,true)`, season, first, last)
	lock := func(gid pgtype.UUID) {
		t.Helper()
		if err := q.LockBasketballGame(ctx, db.LockBasketballGameParams{LeagueID: league, FranchiseID: franchise, EffectiveOn: sportsday.Date(first), PlayerID: player, GameID: gid, Slot: "G"}); err != nil {
			t.Fatal(err)
		}
	}
	lock(monday)
	if rows := choices(end)[player]; !rows[monday].Counts || !rows[wed].Counts || rows[friday].Counts {
		t.Fatalf("one lock plus fallback: %+v", rows)
	}
	// At Wednesday's completion the second manual choice is available despite
	// the movement lock already imposed by Monday's choice.
	if !choices(clock("2026-11-19T01:00:00Z"))[player][wed].CanLockIn {
		t.Fatal("second doubled lock was not offered")
	}
	if err := set(clock("2026-11-19T01:00:00Z"), Entry{PlayerID: player, Slot: "G", LockGameID: wed}); err != nil {
		t.Fatalf("second doubled lock through API: %v", err)
	}
	sql(`update stat_lines set stats='{"min":2,"pts":9}' where game_id=$1`, sunday)
	if rows := choices(end)[player]; !rows[monday].LockedIn || !rows[wed].LockedIn || rows[sunday].Counts || rows[sunday].CanLockIn {
		t.Fatalf("double lock cap: %+v", rows)
	}
	nextGame := game("2026-11-23", "final", 20, 4)
	next, err := q.ListBasketballGameChoices(ctx, db.ListBasketballGameChoicesParams{LeagueID: league, FranchiseID: franchise, FromDay: sportsday.Date(first.AddDate(0, 0, 7)), ToDay: sportsday.Date(last.AddDate(0, 0, 7)), AsOf: pgtype.Timestamptz{Time: end, Valid: true}})
	if err != nil || len(next) != 2 {
		t.Fatalf("locks leaked into the next week: %+v, %v", next, err)
	}
	for _, row := range next {
		if row.GameID != nextGame || row.LockedIn {
			t.Fatalf("next week's game retained a lock: %+v", row)
		}
	}
	nextView, err := loadAt(ctx, q, leagueRow, franchise, first.AddDate(0, 0, 7), clock("2026-11-24T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range nextView.Players {
		if p.LocksUsed != 0 || p.Locked {
			t.Fatalf("Monday did not reset movement locks: %+v", p)
		}
	}
	// Recomputing the prior week must still honor its advance selection,
	// including its old recorded-stat rule rather than the new minutes rule.
	oldFirst, oldLast := date("2026-11-09"), date("2026-11-15")
	game("2026-11-09", "final", 30, 100)
	game("2026-11-10", "final", 0, 7)
	sql(`insert into lineups (league_id,franchise_id,effective_on) values ($1,$2,$3)`, league, franchise, oldFirst)
	sql(`insert into lineup_entries (league_id,franchise_id,effective_on,slot,slot_index,player_id,counts_from)
        values ($1,$2,$3,'G',0,$4,'2026-11-10')`, league, franchise, oldFirst, player)
	history, err := q.ListLineupPoints(ctx, db.ListLineupPointsParams{LeagueID: league, WeekStart: sportsday.Date(oldFirst), FromDay: sportsday.Date(oldFirst), ToDay: sportsday.Date(oldLast), IncludeGames: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Points != 7 {
		t.Fatalf("legacy scoring changed: %+v", history)
	}
	// Roster exits before the activation Monday also clear already-prepared
	// future lineups, while leaving their past scoring windows alone.
	futureFirst := first.AddDate(0, 0, 7)
	if err := initializeBasketballWindow(ctx, q, league, franchise, View{Day: futureFirst.Format(time.DateOnly)}); err != nil {
		t.Fatal(err)
	}
	if err := basketballBench(ctx, q, league, franchise, player, date("2026-11-15")); err != nil {
		t.Fatal(err)
	}
	futureChoices, err := q.ListBasketballGameChoices(ctx, db.ListBasketballGameChoicesParams{
		LeagueID: league, FranchiseID: franchise, FromDay: sportsday.Date(futureFirst), ToDay: sportsday.Date(last.AddDate(0, 0, 7)), AsOf: pgtype.Timestamptz{Time: clock("2026-11-24T12:00:00Z"), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range futureChoices {
		if row.PlayerID == player && (row.Eligible || row.Counts) {
			t.Fatalf("pre-activation roster exit left a future starter: %+v", row)
		}
	}
	// The API also rejects expired/stale lock requests at the final write.
	p := Player{FullName: "Test", Games: []Game{{GameID: monday, CanLockIn: true, LockDeadline: clock("2026-11-18T23:00:00Z")}}}
	if validateBasketballLock(p, Entry{LockGameID: monday}, clock("2026-11-18T23:00:00Z")) == nil {
		t.Fatal("expired manual lock accepted")
	}
}
