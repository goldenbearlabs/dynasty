package scoring

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

// TestSeasonScoresFromSettledWeeks checks that a season added up from its
// stored weeks matches adding up every box score since it opened, and that
// whatever changes a stored week clears it: a stat correction, a lineup
// change, undoubling a matchup, the league's rules.
func TestSeasonScoresFromSettledWeeks(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Reset(t, pool)
	ctx := context.Background()
	const marker = "settled_weeks_test"
	clean := func() {
		pool.Exec(ctx, `delete from games where provider_id like 'zw-%'`)
		pool.Exec(ctx, `delete from players where note = $1`, marker)
		pool.Exec(ctx, `delete from pro_teams where provider_id like 'zw-%'`)
	}
	clean()
	t.Cleanup(func() { dbtest.Reset(t, pool); clean() })
	sql := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}

	// A picked slot, an auto one and one where every game counts.
	rules, _ := json.Marshal(settings.League{
		Lineup: settings.Lineup{Period: settings.PeriodWeek, WeekStart: "monday", Lock: settings.LockGameStart, Slots: []settings.Slot{
			{Name: "G", Positions: []string{"G"}, Count: 1, GamesPerWeek: 1},
			{Name: "F", Positions: []string{"F"}, Count: 1, GamesPerWeek: 1},
			{Name: "UTIL", Positions: []string{settings.AnyPosition}, Count: 1},
		}},
		Scoring: map[string]float64{"pts": 1, "reb": 0.5},
		Format:  settings.Format{Type: settings.FormatTotalPoints},
	})
	var dynasty, league pgtype.UUID
	if err := pool.QueryRow(ctx, `insert into dynasties (name, settings) values ('Weeks', '{}') returning id`).Scan(&dynasty); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `insert into leagues (dynasty_id, competition, name, settings) values ($1, 'nba', 'NBA', $2) returning id`, dynasty, rules).Scan(&league); err != nil {
		t.Fatal(err)
	}
	// Three franchises of three starters, each player on his own team.
	sql(`insert into franchises (dynasty_id, name, manager_name, slug) select $1, 'F'||n, 'M'||n, 'f'||n from generate_series(0, 2) n`, dynasty)
	sql(`insert into pro_teams (competition, provider_id, abbrev, name) select 'nba', 'zw-'||n, 'ZW'||n, 'ZW'||n from generate_series(0, 8) n`)
	sql(`insert into players (competition, status, full_name, positions, pro_team_id, note)
	     select 'nba', 'active', 'Zw '||f.slug||'-'||k, array[(array['G','F','G'])[k+1]],
	            (select id from pro_teams where provider_id = 'zw-'||(f.n*3+k)), $1
	     from (select slug, row_number() over (order by slug) - 1 as n from franchises) f cross join generate_series(0, 2) k`, marker)

	// Four weeks back to today; the season opens on the first Wednesday.
	today := sportsday.Today()
	monday := sportsday.WeekStart(today, "monday").AddDate(0, 0, -21)
	sql(`insert into seasons (league_id, year, starts_on, ends_on) values ($1, 2026, $2, $3)`, league, monday.AddDate(0, 0, 2), today.AddDate(0, 0, 30))
	var season db.Season
	q := db.New(pool)
	season, err := q.LatestSeason(ctx, league)
	if err != nil {
		t.Fatal(err)
	}
	// The second and third weeks are one doubled matchup.
	sql(`insert into periods (season_id, seq, starts_on, ends_on, double_games) values ($1, 1, $2, $3, true)`, season.ID, monday.AddDate(0, 0, 7), monday.AddDate(0, 0, 20))
	// Every team plays every other day, a player sits out now and then.
	sql(`insert into games (competition, provider_id, day, starts_at, status, home_team_id)
	     select 'nba', 'zw-'||t.n||'-'||d::date, d::date, d + interval '19 hours', 'final', (select id from pro_teams where provider_id = 'zw-'||t.n)
	     from generate_series($1::date, $2::date - 1, interval '1 day') d cross join generate_series(0, 8) t(n)
	     where (extract(doy from d)::int + t.n) % 2 = 0`, monday, today)
	sql(`insert into stat_lines (game_id, player_id, stats)
	     select g.id, p.id, case when (extract(doy from g.day)::int + length(p.full_name)) % 5 = 0 then '{"pts": 0}'::jsonb
	                             else jsonb_build_object('pts', 10 + extract(doy from g.day)::int % 17, 'reb', extract(doy from g.day)::int % 7) end
	     from games g join players p on p.pro_team_id = g.home_team_id where g.provider_id like 'zw-%'`)
	// A lineup each week: G picked from mid-week, F on auto, the rest in UTIL.
	sql(`insert into lineups (league_id, franchise_id, effective_on)
	     select $1, f.id, w::date from franchises f cross join generate_series($2::date, $3::date, interval '7 days') w`, league, monday, today)
	sql(`insert into lineup_entries (league_id, franchise_id, effective_on, slot, slot_index, player_id, counts_from, counts_last)
	     select $1, re.franchise_id, lu.effective_on, (array['G','F','UTIL'])[k+1], 0, re.player_id,
	            case when k = 0 then lu.effective_on + 2 end, k = 1
	     from (select f.id as franchise_id, p.id as player_id, right(p.full_name, 1)::int as k
	           from players p join franchises f on p.full_name like 'Zw '||f.slug||'-%' where p.note = $2) re
	     join lineups lu on lu.franchise_id = re.franchise_id`, league, marker)
	sql(`insert into roster_entries (league_id, franchise_id, player_id, list, acquired_via)
	     select $1, f.id, p.id, 'main', 'draft' from players p join franchises f on p.full_name like 'Zw '||f.slug||'-%' where p.note = $2`, league, marker)

	stored := func() (n int) {
		pool.QueryRow(ctx, `select count(*) from settled_weeks where league_id = $1`, league).Scan(&n)
		return n
	}
	matches := func(label string, wantStored int) {
		t.Helper()
		row, err := q.GetLeague(ctx, league)
		if err != nil {
			t.Fatal(err)
		}
		whole, err := scores(ctx, q, row, season.StartsOn.Time, season.EndsOn.Time)
		if err != nil {
			t.Fatal(err)
		}
		weekly, err := seasonScores(ctx, q, row, season)
		if err != nil {
			t.Fatal(err)
		}
		if len(whole) != 3 {
			t.Fatalf("%s: %d franchises scored, want 3", label, len(whole))
		}
		for id, w := range whole {
			got := weekly[id]
			if math.Abs(got.Points-w.Points) > 1e-6 || len(got.Players) != len(w.Players) {
				t.Errorf("%s: %v points from %d players by week, %v from %d whole", label, got.Points, len(got.Players), w.Points, len(w.Players))
			}
		}
		if n := stored(); n != wantStored {
			t.Errorf("%s: %d weeks stored, want %d", label, n, wantStored)
		}
	}
	// The first week (from Wednesday) and the doubled matchup are over.
	matches("settling", 2)
	matches("from the stored weeks", 2)

	sql(`update stat_lines set stats = '{"pts": 60}' where game_id in (select id from games where provider_id like 'zw-%' and day = $1)`, monday.AddDate(0, 0, 3))
	if n := stored(); n != 1 {
		t.Errorf("a stat correction in the first week left %d weeks stored, want 1", n)
	}
	matches("after a stat correction", 2)

	sql(`update lineup_entries set counts_last = not counts_last where league_id = $1 and effective_on = $2`, league, monday.AddDate(0, 0, 14))
	if n := stored(); n != 1 {
		t.Errorf("a lineup change in the doubled matchup left %d weeks stored, want 1", n)
	}
	matches("after a lineup change", 2)

	// Undoubled, the matchup's two weeks are counted on their own.
	sql(`update periods set double_games = false where season_id = $1`, season.ID)
	matches("after undoubling", 3)

	sql(`update leagues set settings = jsonb_set(settings, '{scoring,pts}', '2') where id = $1`, league)
	if n := stored(); n != 0 {
		t.Errorf("a scoring change left %d weeks stored, want none", n)
	}
	matches("after a scoring change", 3)

}
