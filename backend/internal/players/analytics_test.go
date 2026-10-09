package players

import (
	"crossover/internal/db"
	"crossover/internal/settings"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"math"
	"testing"
)

func analyticFixture() ([]db.ListResearchPlayersRow, map[string]analyticRules) {
	rows := []db.ListResearchPlayersRow{}
	rules := map[string]analyticRules{}
	for league, scale := range map[string]float64{"small": 1, "large": 100} {
		rules[league] = analyticRules{Rules: settings.League{Scoring: map[string]float64{"pts": scale}, Lineup: settings.Lineup{Slots: []settings.Slot{{Name: "G", Positions: []string{"PG"}, Count: 1}}}}, Managers: 1, Positions: []string{"PG"}}
		for i, rate := range []float64{10, 20, 30, 40} {
			id := pgtype.UUID{Valid: true}
			id.Bytes[0] = byte(i + 1)
			if league == "large" {
				id.Bytes[1] = 1
			}
			stats, _ := json.Marshal(map[string]float64{"pts": rate * 10})
			owner := ""
			if i == 3 {
				owner = "champ"
			}
			rows = append(rows, db.ListResearchPlayersRow{ID: id, Competition: league, FullName: league + string(rune('A'+i)), Positions: []string{"PG"}, Status: "active", Team: "AAA / BBB", OwnerSlug: owner, Season: "2026", Games: 10, Points: rate * scale * 10, PointsPerGame: rate * scale, Stats: stats})
		}
	}
	return rows, rules
}

func TestAnalyticsScalingAndReplacement(t *testing.T) {
	rows, rules := analyticFixture()
	result := analyze(rows, rules, ResearchFilter{Sort: "league_index"})
	if len(result.Players) != 8 || len(result.Teams) != 2 {
		t.Fatalf("result: %+v", result)
	}
	var small, large AnalyticsPlayer
	for _, p := range result.Players {
		if p.FullName == "smallD" {
			small = p
		}
		if p.FullName == "largeD" {
			large = p
		}
	}
	if small.LeagueIndex == nil || large.LeagueIndex == nil || math.Abs(*small.LeagueIndex-*large.LeagueIndex) > 1e-9 {
		t.Fatal("normalization must be independent of scoring scale")
	}
	if *small.ReplacementRank != 2 || *small.ReplacementRate != 30 || *small.PointsAboveReplacement != 100 || *small.PARPerGame != 10 {
		t.Fatalf("replacement: %+v", small)
	}
	if *small.Percentile != 100 || *small.Availability != 100 || *small.ProductionShare != 40 {
		t.Fatalf("rank and shares: %+v", small)
	}
	if *small.WinShareAdded <= 0 || math.Abs(*small.WinShareAdded-*large.WinShareAdded) > 1e-9 {
		t.Fatal("WSA should be positive and scale invariant")
	}
}

func TestAnalyticsFiltersDoNotChangeBenchmarks(t *testing.T) {
	rows, rules := analyticFixture()
	full := analyze(rows, rules, ResearchFilter{Sort: "league_index"})
	filtered := analyze(rows, rules, ResearchFilter{Search: "smallD", Position: "PG", Team: "BBB", Owner: "champ", MinGames: 10, MinRate: 35, MinIndex: 115, AboveReplacement: true})
	if filtered.Total != 1 {
		t.Fatalf("filters: %+v", filtered)
	}
	for _, p := range full.Players {
		if p.FullName == "smallD" && *p.LeagueIndex != *filtered.Players[0].LeagueIndex {
			t.Fatal("search changed league benchmark")
		}
	}
	paged := analyze(rows, rules, ResearchFilter{Owner: "available", Sort: "points", Ascending: true, Page: 2, PerPage: 2})
	if paged.Total != 6 || paged.Page != 2 || len(paged.Players) != 2 || paged.Players[0].FullName != "smallC" {
		t.Fatalf("paged: %+v", paged)
	}
	missing := analyze(rows, rules, ResearchFilter{Position: "C"})
	if missing.Total != 0 || missing.Players == nil {
		t.Fatal("empty page must serialize as an array")
	}
}

func TestAnalyticsSmallSamplesTiesAndRank(t *testing.T) {
	rows, rules := analyticFixture()
	for i := range rows {
		rows[i].PointsPerGame = 20
		rows[i].Points = 200
	}
	rows[0].Games = 1
	rows = append(rows, db.ListResearchPlayersRow{FullName: "No stats", Competition: "small", Positions: []string{"PG"}, Stats: json.RawMessage(`{}`)})
	result := analyze(rows, rules, ResearchFilter{ReplacementRank: 100, Sort: "league_index"})
	for _, p := range result.Players {
		if p.LeagueIndex != nil || p.PositionIndex != nil || p.ReplacementRate != nil || p.WinShareAdded != nil {
			t.Fatalf("constant cohort or missing replacement must not invent metrics: %+v", p)
		}
		if p.Qualified && (p.Percentile == nil || *p.Percentile != 50) {
			t.Fatal("ties should have percentile 50")
		}
		if (p.FullName == rows[0].FullName || p.FullName == "No stats") && p.Percentile != nil {
			t.Fatal("small samples must not get qualified metrics")
		}
	}
	empty := analyze(rows, rules, ResearchFilter{BenchmarkGames: 100})
	if len(empty.Benchmarks) != 0 {
		t.Fatal("no players meet benchmark threshold")
	}
	override := analyze(rows, rules, ResearchFilter{BenchmarkGames: 1, ReplacementRank: 1})
	if override.Players[0].ReplacementRank == nil || *override.Players[0].ReplacementRank != 1 {
		t.Fatal("manual rank must override lineup demand")
	}
}

func TestAnalyticsMultiPositionComparison(t *testing.T) {
	rows, rules := analyticFixture()
	for i := range rows {
		if rows[i].Competition == "small" && rows[i].FullName != "smallA" {
			rows[i].Positions = append(rows[i].Positions, "SG")
		}
	}
	r := rules["small"]
	r.Rules.Lineup.Slots = append(r.Rules.Lineup.Slots, settings.Slot{Name: "S", Positions: []string{"SG"}, Count: 2})
	r.Positions = append(r.Positions, "SG")
	rules["small"] = r
	auto := analyze(rows, rules, ResearchFilter{Search: "smallD"})
	selected := analyze(rows, rules, ResearchFilter{Search: "smallD", Position: "PG"})
	if auto.Players[0].MetricPosition != "SG" || selected.Players[0].MetricPosition != "PG" {
		t.Fatal("comparison position should be explicit, or best eligible advantage")
	}
	multiple := analyze(rows, rules, ResearchFilter{Search: "smallD", Position: "PG,C"})
	if multiple.Players[0].MetricPosition != "PG" {
		t.Fatal("multi-position filters must restrict the comparison positions")
	}
	if *auto.Players[0].PARPerGame != 20 || *selected.Players[0].PARPerGame != 10 {
		t.Fatal("incorrect multi-position replacement")
	}
}

func TestAnalyticsSeasonIsolationAndRawSorting(t *testing.T) {
	rows, rules := analyticFixture()
	copies := []db.ListResearchPlayersRow{}
	for _, p := range rows {
		if p.Competition != "small" {
			continue
		}
		p.Season = "2025"
		p.Points *= 10
		p.PointsPerGame *= 10
		copies = append(copies, p)
	}
	rows = append(rows, copies...)
	result := analyze(rows, rules, ResearchFilter{Competition: "small", Sort: "league_index", IncludeChart: true, ChartX: "games", ChartY: "league_index"})
	if result.Total != 8 || len(result.Chart) != 8 {
		t.Fatal("snapshots and charts should include both seasons")
	}
	var current, previous *float64
	keys := map[string]bool{}
	for _, p := range result.Players {
		if keys[p.RowKey] {
			t.Fatal("season snapshots require distinct keys")
		}
		keys[p.RowKey] = true
		if p.FullName == "smallD" {
			if p.Season == "2026" {
				current = p.LeagueIndex
			} else {
				previous = p.LeagueIndex
			}
		}
	}
	if current == nil || previous == nil || *current != *previous {
		t.Fatal("seasons must use independent benchmarks")
	}
	rows, rules = analyticFixture()
	for i := range rows {
		if rows[i].FullName == "smallD" {
			rows[i].Games = 40
			rows[i].PointsPerGame = 10
		}
	}
	result = analyze(rows, rules, ResearchFilter{Competition: "small", Sort: "pts", RawPerGame: true})
	if result.Players[0].FullName != "smallC" {
		t.Fatal("raw per-game columns must sort by rates, not totals")
	}
	result = analyze(rows, rules, ResearchFilter{Competition: "small", Sort: "name", Ascending: true})
	if result.Players[0].FullName != "smallA" {
		t.Fatal("text column ascending sort failed")
	}
}

func TestAnalyticsMissingScoringStatsAreNotZeroProduction(t *testing.T) {
	rows, rules := analyticFixture()
	row := rows[0]
	row.FullName = "Unmatched stats"
	row.ID.Bytes[3] = 1
	row.Points = 0
	row.PointsPerGame = 0
	row.Stats = json.RawMessage(`{"fga":100}`)
	rows = append(rows, row)
	result := analyze(rows, rules, ResearchFilter{MissingOnly: true, IncludeChart: true, ChartX: "games", ChartY: "points"})
	if result.Total != 1 || result.Players[0].Qualified || result.Players[0].HasScoringStats || result.Players[0].LeagueIndex != nil {
		t.Fatal("unmatched stats must not enter the benchmark")
	}
	if result.Chart[0].Y != nil {
		t.Fatal("missing fantasy scoring cannot become a zero chart observation")
	}
}

func TestAnalyticsZeroUpperBound(t *testing.T) {
	rows, rules := analyticFixture()
	result := analyze(rows, rules, ResearchFilter{MaxRate: 0, MaxRateSet: true})
	if result.Total != 0 {
		t.Fatal("an explicit zero maximum is a filter, not an unset value")
	}
}

func TestBasketballAliasesShareReplacementDemand(t *testing.T) {
	rows := []db.ListResearchPlayersRow{}
	for _, group := range []string{"G", "F", "C"} {
		for i := 0; i < 10; i++ {
			pos := group
			if group == "G" {
				pos = []string{"G", "PG", "SG"}[i%3]
			}
			stats, _ := json.Marshal(map[string]float64{"pts": float64(1000 - i*50)})
			rows = append(rows, db.ListResearchPlayersRow{Competition: "nba", Season: "2026", FullName: pos + string(rune('A'+i)), Positions: []string{pos}, Games: 10, Stats: stats, Points: float64(1000 - i*50), PointsPerGame: float64(100 - i*5)})
		}
	}
	rules := map[string]analyticRules{"nba": {Rules: settings.League{Scoring: map[string]float64{"pts": 1}, Lineup: settings.Lineup{Slots: []settings.Slot{{Name: "G", Count: 2, Positions: []string{"G", "PG", "SG"}}, {Name: "F", Count: 2, Positions: []string{"F", "SF", "PF"}}, {Name: "C", Count: 1, Positions: []string{"C"}}, {Name: "UTIL", Count: 2, Positions: []string{"*"}}}}}, Managers: 2}}
	result := analyze(rows, rules, ResearchFilter{})
	for _, b := range result.Benchmarks {
		if b.Position == "" {
			continue
		}
		want := 7
		if b.Position == "C" {
			want = 5
		}
		if b.Players != 10 || b.ReplacementRank == nil || *b.ReplacementRank != want {
			t.Fatalf("aliases diluted demand: %+v", b)
		}
	}
	selected := analyze(rows, rules, ResearchFilter{Position: "PG"})
	if selected.Total != 3 || selected.Players[0].MetricPosition != "G" || *selected.Players[0].ReplacementRank != 7 {
		t.Fatal("alias filters must keep shared comparison pool")
	}
}

func TestMLBLeagueIndexUsesComparableRoles(t *testing.T) {
	rows := []db.ListResearchPlayersRow{}
	for _, group := range []string{"hitter", "starter", "reliever", "mixed"} {
		scale := 1.0
		pos := "1B"
		if group == "starter" {
			scale = 10
			pos = "P"
		}
		if group == "mixed" {
			scale = 6
			pos = "P"
		}
		if group == "reliever" {
			scale = 3
			pos = "P"
		}
		for i := 1; i <= 3; i++ {
			stats := map[string]float64{"pts": float64(i) * scale * 10}
			if group != "hitter" {
				stats["pit_games"] = 10
				stats["pit_gs"] = 0
				if group == "starter" {
					stats["pit_gs"] = 10
				}
				if group == "mixed" {
					stats["pit_gs"] = 4
				}
			}
			raw, _ := json.Marshal(stats)
			rows = append(rows, db.ListResearchPlayersRow{Competition: "mlb", Season: "2026", FullName: group + string(rune('A'+i)), Positions: []string{pos}, Games: 10, Stats: raw, Points: stats["pts"], PointsPerGame: stats["pts"] / 10})
		}
	}
	rules := map[string]analyticRules{"mlb": {Rules: settings.League{Scoring: map[string]float64{"pts": 1}, Lineup: settings.Lineup{Slots: []settings.Slot{{Name: "1B", Count: 1, Positions: []string{"1B"}}, {Name: "P", Count: 2, Positions: []string{"P"}}}}}, Managers: 1}}
	result := analyze(rows, rules, ResearchFilter{})
	for _, p := range result.Players {
		want := 100.0
		if p.FullName[len(p.FullName)-1] == 'B' {
			want -= 15 * math.Sqrt(1.5)
		}
		if p.FullName[len(p.FullName)-1] == 'D' {
			want += 15 * math.Sqrt(1.5)
		}
		if p.LeagueIndex == nil || math.Abs(*p.LeagueIndex-want) > 1e-9 {
			t.Fatalf("role scale biased League+: %+v", p)
		}
		if p.NormalizationGroup == "League" || p.NormalizationGroup == "Pitchers (role unknown)" {
			t.Fatal("historical pitching workload not used")
		}
	}
	selected := analyze(rows, rules, ResearchFilter{Position: "SP"})
	if selected.Total != 3 {
		t.Fatal("generic P labels should be filterable by season pitching role")
	}
	// A two-way cohort with no peers has no invented comparable index.
	row := rows[0]
	row.Positions = []string{"TWP"}
	row.FullName = "Two-way"
	rows = append(rows, row)
	result = analyze(rows, rules, ResearchFilter{Search: "Two-way"})
	if result.Players[0].LeagueIndex != nil {
		t.Fatal("singleton two-way role must not get an index")
	}
}

func TestPitcherWorkloadQualification(t *testing.T) {
	rows := []db.ListResearchPlayersRow{}
	add := func(name string, games, starts int, innings, points float64) {
		stats, _ := json.Marshal(map[string]float64{"pts": points, "pit_games": float64(games), "pit_gs": float64(starts), "pit_ip": innings})
		rows = append(rows, db.ListResearchPlayersRow{Competition: "mlb", Season: "2026", FullName: name, Positions: []string{"P"}, Games: int32(games), Stats: stats, Points: points, PointsPerGame: points / float64(games)})
	}
	add("Full reliever", 81, 0, 96, 324)
	add("Other reliever", 60, 0, 90, 180)
	add("Stallings", 6, 0, 14, 59.5)
	add("Full mixed", 83, 30, 141, 400)
	add("Other mixed", 50, 10, 120, 250)
	add("Dollander", 10, 3, 44, 124.5)
	add("Low innings", 60, 0, 10, 100)
	rules := map[string]analyticRules{"mlb": {Rules: settings.League{Scoring: map[string]float64{"pts": 1}, Lineup: settings.Lineup{Slots: []settings.Slot{{Name: "P", Count: 1, Positions: []string{"P"}}}}}, Managers: 1}}
	result := analyze(rows, rules, ResearchFilter{IncludeChart: true, ChartX: "games", ChartY: "league_index"})
	for _, p := range result.Players {
		low := p.FullName == "Stallings" || p.FullName == "Dollander" || p.FullName == "Low innings"
		if low {
			if p.Qualified || p.LeagueIndex != nil || p.Percentile != nil || p.PositionIndex != nil || p.PointsAboveReplacement != nil || p.QualificationNote == "" {
				t.Fatalf("small workload got ranked: %+v", p)
			}
			if p.BenchmarkMinimumGames != 21 {
				t.Fatal("workload appearances not scaled by role")
			}
			if p.FullName == "Stallings" && p.BenchmarkMinimumInnings != 24 {
				t.Fatal("incorrect innings threshold")
			}
			if !p.HasScoringStats || p.Points <= 0 {
				t.Fatal("raw stats were removed")
			}
		} else if !p.Qualified || p.LeagueIndex == nil {
			t.Fatalf("sustained workload was excluded: %+v", p)
		}
	}
	filtered := analyze(rows, rules, ResearchFilter{Search: "Stallings"})
	if filtered.Total != 1 || filtered.Players[0].BenchmarkMinimumGames != 21 {
		t.Fatal("display filters changed qualification")
	}
	qualified := analyze(rows, rules, ResearchFilter{QualifiedOnly: true})
	if qualified.Total != 4 {
		t.Fatal("qualified-only filter retained small samples")
	}
	for _, p := range result.Chart {
		if (p.Label == "Stallings" || p.Label == "Dollander") && p.Y != nil {
			t.Fatal("unqualified chart value invented")
		}
	}
	disabled := analyze(rows, rules, ResearchFilter{Search: "Stallings", PitcherWorkloadPercent: ptr(0)})
	if disabled.Players[0].LeagueIndex == nil {
		t.Fatal("explicit workload override was ignored")
	}
	analysis := leagueAnalysis(rows, rules, ResearchFilter{BenchmarkGames: 5})
	if len(analysis) != 1 || analysis[0].Qualified != 4 {
		t.Fatal("league analysis uses different qualification")
	}
	// Workload gates scale with an in-progress season and isolate season pools.
	early := rows[0]
	early.Season = "2027"
	early.Games = 6
	early.FullName = "Early reliever"
	early.Stats = []byte(`{"pts":20,"pit_games":6,"pit_gs":0,"pit_ip":8}`)
	rows = append(rows, early)
	earlyResult := analyze(rows, rules, ResearchFilter{Search: "Early reliever"})
	if !earlyResult.Players[0].Qualified || earlyResult.Players[0].BenchmarkMinimumGames != 5 {
		t.Fatal("previous season workload contaminated new season")
	}
}
