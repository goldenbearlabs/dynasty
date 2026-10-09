package ingest_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
	"crossover/internal/sportsday"
)

// fakeSource serves one team whose roster the test controls.
type fakeSource struct {
	roster    []ingest.Player
	rosterErr error
}

func (f *fakeSource) Teams(context.Context) ([]ingest.Team, error) {
	return []ingest.Team{{ProviderID: "1", Abbrev: "TST", Name: "Testers"}}, nil
}

func (f *fakeSource) Roster(context.Context, ingest.Team) ([]ingest.Player, error) {
	return f.roster, f.rosterErr
}

func TestSyncRosters(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()

	const college, pro, provider = "test_college", "test_pro", "test_provider"
	cleanup := func() {
		pool.Exec(ctx, `delete from players where competition in ($1, $2)`, college, pro)
		pool.Exec(ctx, `delete from pro_teams where competition in ($1, $2)`, college, pro)
		pool.Exec(ctx, `delete from ingest_runs where competition in ($1, $2)`, college, pro)
	}
	cleanup()
	defer cleanup()

	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	var moves []string
	syncer.OnMove = func(_ context.Context, _ pgtype.UUID, from, to string) { moves = append(moves, from+">"+to) }
	ann := ingest.Player{Provider: provider, ProviderID: "a", FullName: "Ann Able"}
	bob := ingest.Player{Provider: provider, ProviderID: "b", FullName: "Bob Baker"}

	type row struct {
		competition, status string
		movedPool           bool // eligible_since was reset after the row was created
	}
	player := func(providerID string) row {
		t.Helper()
		var r row
		err := pool.QueryRow(ctx, `
			select p.competition, p.status, p.eligible_since > (select min(started_at) from ingest_runs where competition = $3)
			from players p join player_external_ids x on x.player_id = p.id
			where x.provider = $1 and x.provider_id = $2`, provider, providerID, college).
			Scan(&r.competition, &r.status, &r.movedPool)
		if err != nil {
			t.Fatalf("player %s: %v", providerID, err)
		}
		return r
	}
	count := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `select count(*) from players where competition in ($1, $2)`, college, pro).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// A first sync creates both players; a second creates no duplicates.
	src := &fakeSource{roster: []ingest.Player{ann, bob}}
	for range 2 {
		if err := syncer.SyncRosters(ctx, college, src); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(); n != 2 {
		t.Fatalf("after two syncs there are %d players, want 2", n)
	}

	// A failed roster fetch must not mark anyone as gone.
	if err := syncer.SyncRosters(ctx, college, &fakeSource{rosterErr: errors.New("feed down")}); err == nil {
		t.Fatal("sync with a failing roster returned no error")
	}
	if got := player("a"); got.status != "active" {
		t.Errorf("after a failed sync Ann is %q, want active", got.status)
	}

	// A player missing from a complete sync becomes inactive.
	if err := syncer.SyncRosters(ctx, college, &fakeSource{roster: []ingest.Player{ann}}); err != nil {
		t.Fatal(err)
	}
	if got := player("b"); got.status != "inactive" {
		t.Errorf("Bob left the roster but is %q, want inactive", got.status)
	}

	// The same external id appearing in another competition moves the
	// player instead of creating a second row.
	if err := syncer.SyncRosters(ctx, pro, &fakeSource{roster: []ingest.Player{ann}}); err != nil {
		t.Fatal(err)
	}
	if got := player("a"); got.competition != pro || got.status != "active" || !got.movedPool {
		t.Errorf("Ann after turning pro = %+v, want competition %q, active, with eligible_since reset", got, pro)
	}
	if n := count(); n != 2 {
		t.Errorf("turning pro left %d players, want 2", n)
	}
	// The move, and only the move, is reported so her fantasy rights can follow.
	if len(moves) != 1 || moves[0] != college+">"+pro {
		t.Errorf("reported moves = %v, want one from %s to %s", moves, college, pro)
	}

	// A prospect feed adds prospects, and never overwrites someone who has arrived.
	cy := ingest.Player{Provider: provider, ProviderID: "c", FullName: "Cy Young", Note: "2026 draft"}
	stale := ann
	stale.FullName = "Stale Prospect Entry"
	if err := syncer.SyncProspects(ctx, pro, prospects{cy, stale}); err != nil {
		t.Fatal(err)
	}
	if got := player("c"); got.competition != pro || got.status != "prospect" {
		t.Errorf("new prospect = %+v, want a prospect in %q", got, pro)
	}
	var name string
	pool.QueryRow(ctx, `select full_name from players p join player_external_ids x on x.player_id = p.id
		where x.provider = $1 and x.provider_id = 'a'`, provider).Scan(&name)
	if got := player("a"); got.status != "active" || name != "Ann Able" {
		t.Errorf("the prospect feed changed an arrived player: %q %+v", name, got)
	}

	// When the prospect appears on a roster he becomes active, as the same row.
	if err := syncer.SyncRosters(ctx, pro, &fakeSource{roster: []ingest.Player{ann, cy}}); err != nil {
		t.Fatal(err)
	}
	if got := player("c"); got.status != "active" {
		t.Errorf("prospect on a roster is %q, want active", got.status)
	}
	if n := count(); n != 3 {
		t.Errorf("%d players, want 3", n)
	}
}

type prospects []ingest.Player

func (p prospects) Prospects(context.Context) ([]ingest.Player, error) { return p, nil }

// fakeGames serves a fixed schedule and counts box score requests per game.
type fakeGames struct {
	games       []ingest.Game
	lines       []ingest.StatLine
	fetched     map[string]int
	scoreboards int
	byET        bool // file games under their Eastern date, as the real feeds do
}

func (f *fakeGames) Games(_ context.Context, day time.Time) ([]ingest.Game, error) {
	f.scoreboards++
	var onDay []ingest.Game
	for _, g := range f.games {
		filed := g.StartsAt.Format(time.DateOnly)
		if f.byET {
			filed = sportsday.Calendar(g.StartsAt).Format(time.DateOnly)
		}
		if filed == day.Format(time.DateOnly) {
			onDay = append(onDay, g)
		}
	}
	return onDay, nil
}

func (f *fakeGames) BoxScore(_ context.Context, game ingest.Game) ([]ingest.StatLine, error) {
	f.fetched[game.ProviderID]++
	return f.lines, nil
}

func TestSyncGames(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition, provider = "test_games", "test_games_provider"
	cleanup := func() {
		pool.Exec(ctx, `delete from games where competition = $1`, competition)
		pool.Exec(ctx, `delete from players where competition = $1`, competition)
		pool.Exec(ctx, `delete from pro_teams where competition = $1`, competition)
		pool.Exec(ctx, `delete from ingest_runs where competition = $1`, competition)
	}
	cleanup()
	defer cleanup()

	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	// One known player on one known team.
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{{Provider: provider, ProviderID: "known", FullName: "Known Player"}}}); err != nil {
		t.Fatal(err)
	}

	// Noon UTC, so the sports day matches the calendar day.
	noon := func(daysFromNow int) time.Time {
		return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, daysFromNow).Add(12 * time.Hour)
	}
	src := &fakeGames{
		games: []ingest.Game{
			{ProviderID: "final", StartsAt: noon(-1), Status: ingest.GameFinal, HomeTeam: "1", AwayTeam: "nobody"},
			{ProviderID: "live", StartsAt: time.Now().Add(-time.Hour), Status: ingest.GameLive, HomeTeam: "1"},
			{ProviderID: "later", StartsAt: noon(2), Status: ingest.GameScheduled, HomeTeam: "1"},
		},
		lines: []ingest.StatLine{
			{Provider: provider, ProviderID: "known", Stats: map[string]float64{"pts": 12, "reb": 3.5}},
			{Provider: provider, ProviderID: "stranger", Stats: map[string]float64{"pts": 99}},
		},
		fetched: map[string]int{},
	}
	from, to := noon(-2), noon(3)
	for range 2 {
		if err := syncer.SyncGames(ctx, competition, src, from, to); err != nil {
			t.Fatal(err)
		}
	}

	// A finished game is fetched once; a live one every time; a future one never.
	if src.fetched["final"] != 1 || src.fetched["live"] != 2 || src.fetched["later"] != 0 {
		t.Errorf("box score fetches = %v, want final 1, live 2, later 0", src.fetched)
	}

	var games, withHome, lines int
	var stats string
	pool.QueryRow(ctx, `select count(*), count(home_team_id) from games where competition = $1`, competition).Scan(&games, &withHome)
	pool.QueryRow(ctx, `select count(*), min(sl.stats::text) from stat_lines sl join games g on g.id = sl.game_id where g.competition = $1`, competition).Scan(&lines, &stats)
	if games != 3 || withHome != 3 {
		t.Errorf("%d games stored, %d with their home team; want 3 and 3", games, withHome)
	}
	// One line per fetched game for the known player; the stranger is skipped.
	if lines != 2 || stats != `{"pts": 12, "reb": 3.5}` {
		t.Errorf("%d stat lines stored, like %s; want 2 like {\"pts\": 12, \"reb\": 3.5}", lines, stats)
	}
}

func TestPollLive(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition, provider = "test_live", "test_live_provider"
	cleanup := func() {
		pool.Exec(ctx, `delete from games where competition = $1`, competition)
		pool.Exec(ctx, `delete from players where competition = $1`, competition)
		pool.Exec(ctx, `delete from pro_teams where competition = $1`, competition)
		pool.Exec(ctx, `delete from ingest_runs where competition = $1`, competition)
	}
	cleanup()
	defer cleanup()

	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{{Provider: provider, ProviderID: "known", FullName: "Known Player"}}}); err != nil {
		t.Fatal(err)
	}

	// The schedule knows of a game due to start a minute ago, and one tomorrow.
	tipoff := time.Now().Add(-time.Minute)
	src := &fakeGames{
		games: []ingest.Game{
			{ProviderID: "tonight", StartsAt: tipoff, Status: ingest.GameScheduled, HomeTeam: "1"},
			{ProviderID: "tomorrow", StartsAt: tipoff.Add(24 * time.Hour), Status: ingest.GameScheduled, HomeTeam: "1"},
		},
		lines:   []ingest.StatLine{{Provider: provider, ProviderID: "known", Stats: map[string]float64{"pts": 2}}},
		fetched: map[string]int{},
		byET:    true,
	}
	if err := syncer.SyncGames(ctx, competition, src, tipoff.AddDate(0, 0, -1), tipoff.AddDate(0, 0, 2)); err != nil {
		t.Fatal(err)
	}
	poll := func() ingest.LiveUpdate {
		t.Helper()
		update, err := syncer.PollLive(ctx, competition, src)
		if err != nil {
			t.Fatal(err)
		}
		return update
	}

	// Not started yet according to the feed: the scoreboard is checked, nothing has changed.
	if update := poll(); update.Scoreboard || len(update.Games) != 0 || src.fetched["tonight"] != 0 {
		t.Fatalf("before tip-off: %+v, box fetches %v", update, src.fetched)
	}

	// The game goes live with a score.
	src.games[0].Status, src.games[0].HomeScore, src.games[0].Detail = ingest.GameLive, 7, "9:41 - 1st"
	if update := poll(); !update.Scoreboard || len(update.Games) != 1 || src.fetched["tonight"] != 1 {
		t.Fatalf("at tip-off: %+v, box fetches %v", update, src.fetched)
	}
	var score int
	var detail string
	pool.QueryRow(ctx, `select home_score, detail from games where competition = $1 and provider_id = 'tonight'`, competition).Scan(&score, &detail)
	if score != 7 || detail != "9:41 - 1st" {
		t.Errorf("stored score %d and detail %q, want 7 and 9:41 - 1st", score, detail)
	}

	// Nothing moved on the scoreboard, but the box score is still refreshed.
	if update := poll(); update.Scoreboard || len(update.Games) != 1 {
		t.Errorf("mid-game with no change: %+v", update)
	}

	// The final whistle: one last box score, then the game is left alone.
	src.games[0].Status, src.games[0].Detail = ingest.GameFinal, "Final"
	if update := poll(); !update.Scoreboard || len(update.Games) != 1 || src.fetched["tonight"] != 3 {
		t.Fatalf("at the end: %+v, box fetches %v", update, src.fetched)
	}
	before := src.scoreboards
	if update := poll(); update.Scoreboard || len(update.Games) != 0 || src.scoreboards != before {
		t.Errorf("after the end: %+v with %d more scoreboard requests; want no requests at all", update, src.scoreboards-before)
	}
	if src.fetched["tomorrow"] != 0 {
		t.Error("tomorrow's game was polled")
	}

	var runs int
	pool.QueryRow(ctx, `select count(*) from ingest_runs where competition = $1 and job = 'games'`, competition).Scan(&runs)
	if runs != 1 {
		t.Errorf("%d recorded runs, want only the schedule sync: live polls are not logged", runs)
	}
}

// seasonFeed serves season lines by year and records which years were asked for.
type seasonFeed struct {
	latest int
	byYear map[int][]ingest.SeasonLine
	asked  []int
}

func (f *seasonFeed) LatestSeason(time.Time) int { return f.latest }

func (f *seasonFeed) SeasonStats(_ context.Context, year int) ([]ingest.SeasonLine, error) {
	f.asked = append(f.asked, year)
	return f.byYear[year], nil
}

func TestSyncSeasons(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition, provider = "test_history", "test_history_provider"
	cleanup := func() {
		pool.Exec(ctx, `delete from players where competition = $1`, competition)
		pool.Exec(ctx, `delete from pro_teams where competition = $1`, competition)
		pool.Exec(ctx, `delete from ingest_runs where competition = $1`, competition)
	}
	cleanup()
	defer cleanup()

	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{{Provider: provider, ProviderID: "known", FullName: "Known Player"}}}); err != nil {
		t.Fatal(err)
	}
	line := func(id string, year int, team string, points float64) ingest.SeasonLine {
		return ingest.SeasonLine{Provider: provider, ProviderID: id, Season: ingest.Season{
			Year: year, Label: "season", Team: team, Games: 10, Stats: map[string]float64{"pts": points},
		}}
	}
	src := &seasonFeed{latest: 2026, byYear: map[int][]ingest.SeasonLine{
		2026: {line("known", 2026, "AAA", 100), line("stranger", 2026, "AAA", 999)},
		2025: {line("known", 2025, "AAA", 80)},
		2024: {line("known", 2024, "AAA", 60)},
	}}
	stored := func() map[int]string { // year -> "team points"
		t.Helper()
		rows, err := pool.Query(ctx, `select ps.year, ps.team || ' ' || (ps.stats->>'pts') from player_seasons ps
			join players p on p.id = ps.player_id where p.competition = $1`, competition)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		seasons := map[int]string{}
		for rows.Next() {
			var year int
			var text string
			rows.Scan(&year, &text)
			seasons[year] = text
		}
		return seasons
	}

	// The first sync fetches the whole backfill, for players we know.
	if err := syncer.SyncSeasons(ctx, competition, src, 4); err != nil {
		t.Fatal(err)
	}
	if got := stored(); len(got) != 3 || got[2026] != "AAA 100" || got[2024] != "AAA 60" {
		t.Fatalf("after the first sync: %v", got)
	}
	if len(src.asked) != 4 { // 2026 down to 2023, which has nothing
		t.Errorf("first sync asked for %v, want four seasons", src.asked)
	}

	// Later syncs refresh only the two newest seasons. The player was
	// traded, so his old line for this season must go.
	src.asked = nil
	src.byYear[2026] = []ingest.SeasonLine{line("known", 2026, "BBB", 130)}
	src.byYear[2024] = []ingest.SeasonLine{line("known", 2024, "AAA", 61)}
	if err := syncer.SyncSeasons(ctx, competition, src, 4); err != nil {
		t.Fatal(err)
	}
	if got := stored(); len(got) != 3 || got[2026] != "BBB 130" || got[2024] != "AAA 60" {
		t.Errorf("after the second sync: %v, want 2026 replaced and 2024 untouched", got)
	}
	// 2023 is asked for again because nothing was ever stored for it.
	if !slices.Equal(src.asked, []int{2026, 2025, 2023}) {
		t.Errorf("second sync asked for %v, want 2026, 2025 and the still-empty 2023", src.asked)
	}
}

// A player ranked before his draft has no id. When the draft record, and
// later a roster, describe the same person, he must stay one row.
func TestProspectKeepsOneRow(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const competition = "test_alias"
	cleanup := func() {
		pool.Exec(ctx, `delete from players where competition = $1`, competition)
		pool.Exec(ctx, `delete from pro_teams where competition = $1`, competition)
		pool.Exec(ctx, `delete from ingest_runs where competition = $1`, competition)
	}
	cleanup()
	defer cleanup()

	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	rankedAs := ingest.ExternalID{Provider: "test_alias_ranking", ProviderID: "future star|2009-01-15"}
	ranked := ingest.Player{Provider: rankedAs.Provider, ProviderID: rankedAs.ProviderID, FullName: "Future Star", Note: "ranked #2"}
	drafted := ingest.Player{Provider: "test_alias_league", ProviderID: "8480001", FullName: "Future Star", Note: "round 1, #3 overall", Aliases: []ingest.ExternalID{rankedAs}}
	other := ingest.Player{Provider: "test_alias_league", ProviderID: "8480002", FullName: "Someone Else", Aliases: []ingest.ExternalID{{Provider: rankedAs.Provider, ProviderID: "someone else|2008-03-03"}}}

	state := func() (players, ids int, note, status string) {
		t.Helper()
		pool.QueryRow(ctx, `select count(*) from players where competition = $1`, competition).Scan(&players)
		pool.QueryRow(ctx, `select count(*), min(p.note), min(p.status) from player_external_ids x join players p on p.id = x.player_id
			where p.competition = $1 and p.full_name = 'Future Star'`, competition).Scan(&ids, &note, &status)
		return
	}

	if err := syncer.SyncProspects(ctx, competition, prospects{ranked}); err != nil {
		t.Fatal(err)
	}
	// Drafted: the same row, now also known by his league id.
	if err := syncer.SyncProspects(ctx, competition, prospects{drafted, other}); err != nil {
		t.Fatal(err)
	}
	if players, ids, note, _ := state(); players != 2 || ids != 2 || note != "round 1, #3 overall" {
		t.Fatalf("after the draft: %d players, %d ids for the ranked player, note %q; want 2 players, 2 ids, the draft note", players, ids, note)
	}
	// He makes the team.
	onRoster := drafted
	onRoster.Note = ""
	if err := syncer.SyncRosters(ctx, competition, &fakeSource{roster: []ingest.Player{onRoster}}); err != nil {
		t.Fatal(err)
	}
	if players, ids, _, status := state(); players != 2 || ids != 2 || status != "active" {
		t.Errorf("after making the team: %d players, %d ids, status %q; want the same 2 players, 2 ids, active", players, ids, status)
	}
}

// leagueFeed serves the teams and rosters it is given, and as a PoolSource
// says how the league has been narrowed.
type leagueFeed struct {
	rosters map[string][]ingest.Player // by team id
	pool    ingest.Pool
}

func (f *leagueFeed) Teams(context.Context) ([]ingest.Team, error) {
	var teams []ingest.Team
	for id := range f.rosters {
		teams = append(teams, ingest.Team{ProviderID: id, Abbrev: id, Name: id})
	}
	return teams, nil
}

func (f *leagueFeed) Roster(_ context.Context, team ingest.Team) ([]ingest.Player, error) {
	return f.rosters[team.ProviderID], nil
}

func (f *leagueFeed) Pool() ingest.Pool { return f.pool }

// When a competition is narrowed, the players and teams it stored before
// and no longer covers are removed, not left behind as inactive.
func TestSyncRostersTrimsToPool(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const key, provider = "test_pool", "test_pool_provider"
	cleanup := func() {
		pool.Exec(ctx, `delete from players where competition = $1`, key)
		pool.Exec(ctx, `delete from pro_teams where competition = $1`, key)
		pool.Exec(ctx, `delete from ingest_runs where competition = $1`, key)
	}
	cleanup()
	defer cleanup()

	at := func(id, position string) ingest.Player {
		return ingest.Player{Provider: provider, ProviderID: id, FullName: "Player " + id, Positions: []string{position}}
	}
	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	feed := &leagueFeed{rosters: map[string][]ingest.Player{
		"in":  {at("passer", "QB"), at("kicker", "PK")},
		"out": {at("elsewhere", "QB")},
	}}
	if err := syncer.SyncRosters(ctx, key, feed); err != nil {
		t.Fatal(err)
	}

	// The league is narrowed to one team and one position.
	feed.rosters = map[string][]ingest.Player{"in": {at("passer", "QB")}}
	feed.pool = ingest.Pool{Positions: []string{"QB"}, ListedTeamsOnly: true}
	if err := syncer.SyncRosters(ctx, key, feed); err != nil {
		t.Fatal(err)
	}

	var players, teams string
	pool.QueryRow(ctx, `select coalesce(string_agg(full_name || '/' || status, ', ' order by full_name), '') from players where competition = $1`, key).Scan(&players)
	pool.QueryRow(ctx, `select coalesce(string_agg(provider_id, ', '), '') from pro_teams where competition = $1`, key).Scan(&teams)
	if players != "Player passer/active" || teams != "in" {
		t.Errorf("after narrowing: players = %q, teams = %q; want only the passer and his team", players, teams)
	}
}

// Conference metadata introduced after imports is filled even for seasons
// outside the regular history window, then left alone on subsequent syncs.
func TestSyncBackfillsOldConferenceMetadata(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const provider = "test_conference_backfill"
	cleanup := func() { pool.Exec(ctx, `delete from players where note = $1`, provider) }
	cleanup()
	defer cleanup()
	var id pgtype.UUID
	if err := pool.QueryRow(ctx, `insert into players (competition,status,full_name,note) values ('cbb','inactive','Conference backfill',$1) returning id`, provider).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into player_external_ids (provider,provider_id,player_id) values ($1,'known',$2)`, provider, id); err != nil {
		t.Fatal(err)
	}
	syncer := ingest.NewSyncer(db.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	old := ingest.Season{Year: 1800, Label: "1799-1800", Team: "TEST", Games: 10, Stats: map[string]float64{"pts": 100}}
	if err := syncer.StoreSeason(ctx, id, "cbb", old); err != nil {
		t.Fatal(err)
	}
	old.Conference = "2"
	src := &seasonFeed{latest: 2026, byYear: map[int][]ingest.SeasonLine{1800: {{Provider: provider, ProviderID: "known", Season: old}}}}
	if err := syncer.SyncSeasons(ctx, "cbb", src, 2); err != nil {
		t.Fatal(err)
	}
	var conference string
	if err := pool.QueryRow(ctx, `select conference from player_seasons where player_id=$1 and year=1800`, id).Scan(&conference); err != nil {
		t.Fatal(err)
	}
	if conference != "2" {
		t.Fatal("older season metadata was not filled")
	}
	src.asked = nil
	if err := syncer.SyncSeasons(ctx, "cbb", src, 2); err != nil {
		t.Fatal(err)
	}
	for _, year := range src.asked {
		if year == 1800 {
			t.Fatal("complete historical metadata was fetched again")
		}
	}
}
