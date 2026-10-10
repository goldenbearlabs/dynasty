package players

import (
	"cmp"
	"context"
	"crossover/internal/cache"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResearchFilter affects displayed rows, never the population used to compute
// benchmarks. BenchmarkGames and ReplacementRank explicitly change the model.
type ResearchPool struct {
	Competition string `json:"competition"`
	Season      string `json:"season"`
}

type ResearchFilter struct {
	PlayerID                                                                 pgtype.UUID
	PitcherWorkloadPercent                                                   *int
	MinRateSet, MaxRateSet, MinIndexSet, MaxIndexSet, MinPARSet, MaxGamesSet bool
	Pools                                                                    []ResearchPool
	MaxGames                                                                 int
	MaxRate, MaxIndex, MinPAR                                                float64
	RawPerGame                                                               bool
	QualifiedOnly, MissingOnly, IncludeChart                                 bool
	StatKey, ChartX, ChartY, ChartGroup                                      string
	MinStat, MaxStat                                                         *float64
	Competition, Season, Status, Search, Position, Team, Owner, Sort         string
	MinGames, BenchmarkGames, ReplacementRank, Page, PerPage                 int
	MinRate, MinIndex                                                        float64
	AboveReplacement, Ascending                                              bool
}

type AnalyticsPlayer struct {
	QualificationNote       string  `json:"qualification_note"`
	BenchmarkMinimumGames   int     `json:"benchmark_minimum_games"`
	BenchmarkMinimumInnings float64 `json:"benchmark_minimum_innings"`
	db.ListResearchPlayersRow
	RowKey                 string   `json:"row_key"`
	ScoringSource          string   `json:"scoring_source"`
	NormalizationGroup     string   `json:"normalization_group"`
	HasScoringStats        bool     `json:"has_scoring_stats"`
	Qualified              bool     `json:"qualified"`
	MetricPosition         string   `json:"metric_position"`
	LeagueIndex            *float64 `json:"league_index"`
	PositionIndex          *float64 `json:"position_index"`
	Percentile             *float64 `json:"percentile"`
	ReplacementRate        *float64 `json:"replacement_rate"`
	ReplacementRank        *int     `json:"replacement_rank"`
	PointsAboveReplacement *float64 `json:"points_above_replacement"`
	PARPerGame             *float64 `json:"par_per_game"`
	WinShareAdded          *float64 `json:"win_share_added"`
	Availability           *float64 `json:"availability"`
	ProductionShare        *float64 `json:"production_share"`
	values                 map[string]float64
}

type Benchmark struct {
	MeanIndex       *float64 `json:"mean_index"`
	Season          string   `json:"season"`
	Competition     string   `json:"competition"`
	Position        string   `json:"position"`
	Players         int      `json:"players"`
	Mean            float64  `json:"mean"`
	SD              float64  `json:"sd"`
	StarterSlots    int      `json:"starter_slots"`
	ReplacementRank *int     `json:"replacement_rank"`
	ReplacementRate *float64 `json:"replacement_rate"`
}

type ChartPoint struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Group string   `json:"group"`
	X     *float64 `json:"x"`
	Y     *float64 `json:"y"`
}

type LeagueAnalysis struct {
	Competition   string             `json:"competition"`
	Season        string             `json:"season"`
	ScoringSource string             `json:"scoring_source"`
	Players       int                `json:"players"`
	Qualified     int                `json:"qualified"`
	Scored        int                `json:"scored"`
	Games         int                `json:"games"`
	Points        float64            `json:"points"`
	Median        float64            `json:"median"`
	P90           float64            `json:"p90"`
	TopTenShare   float64            `json:"top_ten_share"`
	Contributions map[string]float64 `json:"contributions"`
}

type AnalyticsPage struct {
	PitcherWorkloadPercent int                         `json:"pitcher_workload_percent"`
	StartingConferences    map[string][]string         `json:"starting_conferences"`
	RawStatKeys            []string                    `json:"raw_stat_keys"`
	Catalog                []db.ListResearchCatalogRow `json:"catalog"`
	Analysis               []LeagueAnalysis            `json:"analysis"`
	Chart                  []ChartPoint                `json:"chart"`
	ChartTotal             int                         `json:"chart_total"`
	Warnings               []string                    `json:"warnings"`
	Players                []AnalyticsPlayer           `json:"players"`
	Total                  int                         `json:"total"`
	Page                   int                         `json:"page"`
	PerPage                int                         `json:"per_page"`
	Seasons                []db.ListResearchSeasonsRow `json:"seasons"`
	Teams                  []string                    `json:"teams"`
	Benchmarks             []Benchmark                 `json:"benchmarks"`
	BenchmarkGames         int                         `json:"benchmark_games"`
}

type analyticRules struct {
	Rules     settings.League
	Managers  int
	Positions []string
	Source    string
}

// Analytics reads all selected-season players once, computes league and
// position benchmarks, then filters, sorts and pages the result. No feeds are
// called. Loading only the visible page would make comparative metrics wrong.
func (s *Service) Analytics(ctx context.Context, f ResearchFilter) (AnalyticsPage, error) {
	dataset, err := s.researchData(ctx, f)
	if err != nil {
		return AnalyticsPage{}, err
	}
	valued, rules := dataset.Rows, dataset.Rules
	result := analyze(valued, rules, f)
	result.StartingConferences = map[string][]string{}
	for key, r := range rules {
		ids := r.Rules.StarterConferences(key)
		if key == "cbb" {
			if ids == nil {
				ids = []string{}
			}
			result.StartingConferences[key] = ids
		}
	}
	keys := map[string]bool{}
	for _, row := range valued {
		if f.Competition != "" && row.Competition != f.Competition {
			continue
		}
		for key := range statValues(row.Stats) {
			keys[key] = true
		}
	}
	result.RawStatKeys = []string{}
	for key := range keys {
		result.RawStatKeys = append(result.RawStatKeys, key)
	}
	slices.Sort(result.RawStatKeys)
	result.Seasons, err = db.New(s.pool).ListResearchSeasons(ctx, f.Competition)
	if err != nil {
		return AnalyticsPage{}, err
	}
	result.Catalog = dataset.Catalog
	f.BenchmarkGames = result.BenchmarkGames
	result.Analysis = leagueAnalysis(valued, rules, f)
	result.Warnings = []string{}
	for _, a := range result.Analysis {
		if a.ScoringSource == "defaults" {
			result.Warnings = append(result.Warnings, a.Competition+" "+a.Season+": using sport default scoring and lineup assumptions; no fantasy league is configured.")
		}
		if a.Scored < a.Players {
			result.Warnings = append(result.Warnings, a.Competition+" "+a.Season+": some imported rows have no matching scoring stats. They are excluded from benchmarks.")
		}
	}
	for _, pool := range f.Pools {
		found := false
		for _, a := range result.Analysis {
			if a.Competition == pool.Competition && (pool.Season == "" || a.Season == pool.Season) {
				found = true
			}
		}
		if !found {
			result.Warnings = append(result.Warnings, pool.Competition+" "+pool.Season+": no imported season data. Sync season history in Commissioner → Data feeds.")
		}
	}
	return result, nil
}

// Reuse the full eligible season pool across filters, pages and charts. Only
// selection changes require another database aggregation; display filters
// are applied after benchmarks and cannot alter their population.
type researchDataset struct {
	Rows    []db.ListResearchPlayersRow
	Rules   map[string]analyticRules
	Catalog []db.ListResearchCatalogRow
}

func (s *Service) researchData(ctx context.Context, f ResearchFilter) (researchDataset, error) {
	if len(f.Pools) > 24 {
		return researchDataset{}, problem.New("Choose up to 24 league-season pools.")
	}
	for _, pool := range f.Pools {
		if _, ok := s.registry.Get(pool.Competition); !ok {
			return researchDataset{}, problem.New("Unknown research league: %s.", pool.Competition)
		}
	}

	if s.Cache == nil {
		return s.loadResearchData(ctx, f)
	}
	pools := slices.Clone(f.Pools)
	slices.SortFunc(pools, func(a, b ResearchPool) int {
		return cmp.Or(cmp.Compare(a.Competition, b.Competition), cmp.Compare(a.Season, b.Season))
	})
	pools = slices.Compact(pools)
	selection, err := json.Marshal(struct {
		Season string
		Pools  []ResearchPool
	}{f.Season, pools})
	if err != nil {
		return researchDataset{}, err
	}
	revision, err := s.Cache.Revision(ctx, cache.Research)
	if err != nil {
		return s.loadResearchData(ctx, f) // no generation to hold it under: never stale
	}
	key := revision + "|" + string(selection)
	if dataset, ok := s.datasets.get(key); ok {
		return dataset, nil
	}
	// Simultaneous cold reads share one load, which outlives any one of them.
	result := s.loads.DoChan(key, func() (any, error) {
		if dataset, ok := s.datasets.get(key); ok {
			return dataset, nil
		}
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		dataset, err := s.loadResearchData(shared, f)
		if err == nil {
			s.datasets.put(revision, key, dataset)
		}
		return dataset, err
	})
	select {
	case <-ctx.Done():
		return researchDataset{}, ctx.Err()
	case loaded := <-result:
		if loaded.Err != nil {
			return researchDataset{}, loaded.Err
		}
		return loaded.Val.(researchDataset), nil
	}
}

// researchMemo holds the last few datasets in this process, as loaded, for
// one generation of the data. They are shared between requests and must
// only be read. A dataset is a few megabytes, so only a few are kept.
type researchMemo struct {
	mu       sync.Mutex
	revision string
	sets     map[string]researchDataset
}

const researchMemoSize = 3

func (m *researchMemo) get(key string) (researchDataset, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dataset, ok := m.sets[key]
	return dataset, ok
}

func (m *researchMemo) put(revision, key string, dataset researchDataset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.revision != revision || len(m.sets) >= researchMemoSize {
		m.revision, m.sets = revision, map[string]researchDataset{}
	}
	m.sets[key] = dataset
}

// statValues reads a stats object, remembering the result: the same season
// line is read several times in one analysis and again by every request
// after it. The map is shared and must only be read.
func statValues(raw json.RawMessage) map[string]float64 {
	parsedStats.RLock()
	stats, ok := parsedStats.byText[string(raw)]
	parsedStats.RUnlock()
	if ok {
		return stats
	}
	if json.Unmarshal(raw, &stats) != nil {
		stats = nil
	}
	parsedStats.Lock()
	if len(parsedStats.byText) >= parsedStatsLimit {
		clear(parsedStats.byText) // bounded: start again and refill from use
	}
	parsedStats.byText[string(raw)] = stats
	parsedStats.Unlock()
	return stats
}

const parsedStatsLimit = 12000

var parsedStats = struct {
	sync.RWMutex
	byText map[string]map[string]float64
}{byText: map[string]map[string]float64{}}

func (s *Service) loadResearchData(ctx context.Context, f ResearchFilter) (researchDataset, error) {
	select {
	case s.analyticsSlots <- struct{}{}:
	case <-ctx.Done():
		return researchDataset{}, ctx.Err()
	}
	defer func() { <-s.analyticsSlots }()
	q := db.New(s.pool)

	rules := map[string]analyticRules{}
	dynasty, err := q.GetDynasty(ctx)
	managers := 0
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return researchDataset{}, err
	}
	if err == nil {
		leagues, err := q.ListLeagues(ctx, dynasty.ID)
		if err != nil {
			return researchDataset{}, err
		}
		franchises, err := q.ListFranchises(ctx, dynasty.ID)
		if err != nil {
			return researchDataset{}, err
		}
		managers = len(franchises)
		for _, l := range leagues {
			parsed, err := settings.Parse[settings.League](l.Settings)
			if err != nil {
				return researchDataset{}, err
			}
			c, _ := s.registry.Get(l.Competition)
			rules[l.Competition] = analyticRules{Rules: parsed, Managers: managers, Positions: c.Positions, Source: "league"}
		}
	}

	for _, c := range s.registry {
		if _, exists := rules[c.Key]; !exists {
			rules[c.Key] = analyticRules{Rules: c.Defaults, Managers: managers, Positions: c.Positions, Source: "defaults"}
		}
	}
	var rows []db.ListResearchPlayersRow
	if len(f.Pools) == 0 {
		rows, err = q.ListResearchPlayers(ctx, db.ListResearchPlayersParams{Competition: "", Conferences: rules["cbb"].Rules.StarterConferences("cbb"), Season: f.Season, Sort: "name", PageSize: math.MaxInt32})
		if err != nil {
			return researchDataset{}, err
		}
	} else {
		if len(f.Pools) > 24 {
			return researchDataset{}, problem.New("Choose up to 24 league-season pools.")
		}
		seen := map[string]bool{}
		for _, pool := range f.Pools {
			if _, ok := s.registry.Get(pool.Competition); !ok {
				return researchDataset{}, problem.New("Unknown research league: %s.", pool.Competition)
			}
			key := pool.Competition + "|" + pool.Season
			if seen[key] {
				continue
			}
			seen[key] = true
			records, err := q.ListResearchSeasonPool(ctx, db.ListResearchSeasonPoolParams{Competition: pool.Competition, Conferences: rules[pool.Competition].Rules.StarterConferences(pool.Competition), Season: pool.Season})
			if err != nil {
				return researchDataset{}, err
			}
			for _, r := range records {
				rows = append(rows, db.ListResearchPlayersRow{ID: r.ID, Competition: r.Competition, FullName: r.FullName, Positions: r.Positions, Status: r.Status, InjuryDesignation: r.InjuryDesignation, HeadshotUrl: r.HeadshotUrl, Team: r.Team, OwnerName: r.OwnerName, OwnerSlug: r.OwnerSlug, Season: r.Season, Games: r.Games, Stats: r.Stats})
			}
		}
	}
	// Research also covers sports outside the dynasty. Their configured sport
	// defaults are a labelled scoring basis, rather than fictitious zero scores.
	unique := map[string]bool{}
	valued := []db.ListResearchPlayersRow{}
	for _, row := range rows {
		key := row.ID.String() + "|" + row.Competition + "|" + row.Season
		if unique[key] {
			continue
		}
		unique[key] = true
		row.Points, row.PointsPerGame = 0, 0
		for stat, value := range statValues(row.Stats) {
			row.Points += value * rules[row.Competition].Rules.Scoring[stat]
		}
		if row.Games > 0 {
			row.PointsPerGame = row.Points / float64(row.Games)
		}
		valued = append(valued, row)
	}
	catalog, err := q.ListResearchCatalog(ctx, rules["cbb"].Rules.StarterConferences("cbb"))
	if err != nil {
		return researchDataset{}, err
	}
	return researchDataset{Rows: valued, Rules: rules, Catalog: catalog}, nil
}

type distribution struct {
	rates          []float64
	mean, sd       float64
	maxGames       int32
	positivePoints float64
}

func summarize(rows []db.ListResearchPlayersRow) distribution {
	d := distribution{}
	// Welford's algorithm avoids subtracting nearly equal large numbers.
	var m2 float64
	for _, p := range rows {
		d.rates = append(d.rates, p.PointsPerGame)
		delta := p.PointsPerGame - d.mean
		d.mean += delta / float64(len(d.rates))
		m2 += delta * (p.PointsPerGame - d.mean)
		d.maxGames = max(d.maxGames, p.Games)
		d.positivePoints += max(0, p.Points)
	}
	if len(d.rates) > 0 {
		d.sd = math.Sqrt(max(0, m2/float64(len(d.rates))))
	}
	slices.Sort(d.rates)
	return d
}

// Season production shares one league-wide scale, including hitters and pitchers.
// Rate distributions remain separate for Position+, replacement and win share.
func summarizeTotals(rows []db.ListResearchPlayersRow) distribution {
	totals := slices.Clone(rows)
	for i := range totals {
		totals[i].PointsPerGame = totals[i].Points
	}
	return summarize(totals)
}

func index(rate float64, d distribution) *float64 {
	if len(d.rates) < 2 || d.sd < 1e-9 {
		return nil
	}
	return ptr(100 + 15*(rate-d.mean)/d.sd)
}
func ptr[T any](v T) *T { return &v }

func metricPosition(comp, pos string) string {
	if comp == "nba" || comp == "wnba" || comp == "cbb" {
		switch pos {
		case "PG", "SG":
			return "G"
		case "SF", "PF":
			return "F"
		}
	}
	if comp == "mlb" {
		switch pos {
		case "LF", "CF", "RF":
			return "OF"
		}
	}
	return pos
}

func normalizationGroup(p db.ListResearchPlayersRow) string {
	if p.Competition != "mlb" {
		return "League"
	}
	if slices.Contains(p.Positions, "TWP") {
		return "Two-way players"
	}
	pitcher := slices.ContainsFunc(p.Positions, func(pos string) bool { return pos == "P" || pos == "SP" || pos == "RP" })
	if !pitcher {
		return "Hitters"
	}
	stats := statValues(p.Stats)
	games, knownGames := stats["pit_games"]
	starts, knownStarts := stats["pit_gs"]
	if knownGames && knownStarts && games > 0 {
		if starts >= games/2 {
			return "Starting pitchers"
		}
		if starts > 0 {
			return "Mixed-role pitchers"
		}
		return "Relievers"
	}
	if slices.Contains(p.Positions, "SP") {
		return "Starting pitchers"
	}
	if slices.Contains(p.Positions, "RP") {
		return "Relievers"
	}
	return "Pitchers (role unknown)"
}

func comparisonPositions(p db.ListResearchPlayersRow) []string {
	positions := []string{}
	for _, pos := range p.Positions {
		if p.Competition == "mlb" && pos == "P" {
			switch normalizationGroup(p) {
			case "Starting pitchers":
				pos = "SP"
			case "Relievers", "Mixed-role pitchers":
				pos = "RP"
			}
		}
		positions = append(positions, metricPosition(p.Competition, pos))
	}
	return slices.Compact(slices.Sorted(slices.Values(positions)))
}

// Split flexible demand over actual, distinct eligible pools: aliases such as
// G/PG/SG describe one basketball pool, not three separate positions.
func replacementPositions(comp string, positions, available []string) []string {
	result := []string{}
	for _, pos := range positions {
		if pos == settings.AnyPosition {
			result = append(result, available...)
			continue
		}
		if comp == "mlb" && pos == "P" {
			for _, pitcher := range []string{"SP", "RP", "P"} {
				if slices.Contains(available, pitcher) {
					result = append(result, pitcher)
				}
			}
			continue
		}
		result = append(result, metricPosition(comp, pos))
	}
	return slices.Compact(slices.Sorted(slices.Values(result)))
}

type workloadThreshold struct {
	games   int
	innings float64
}

func pitcherWorkloadPercent(f ResearchFilter) int {
	if f.PitcherWorkloadPercent == nil {
		return 25
	}
	return max(0, min(100, *f.PitcherWorkloadPercent))
}

func pitcherWorkload(p db.ListResearchPlayersRow) (games, innings float64, pitcher bool) {
	pitcher = p.Competition == "mlb" && !slices.Contains(p.Positions, "TWP") && slices.ContainsFunc(p.Positions, func(pos string) bool { return pos == "P" || pos == "SP" || pos == "RP" })
	if !pitcher {
		return
	}
	stats := statValues(p.Stats)
	games, known := stats["pit_games"]
	if !known {
		games = float64(p.Games)
	}
	return games, stats["pit_ip"], true
}

// Workload qualification scales with each season's progress and pitching role.
// A display filter cannot change the threshold or the benchmark population.
func workloadThresholds(rows []db.ListResearchPlayersRow, rules map[string]analyticRules, f ResearchFilter) map[string]workloadThreshold {
	maximums := map[string]workloadThreshold{}
	for _, row := range rows {
		if row.Season == "" || !hasScoringStats(row.Stats, rules[row.Competition].Rules.Scoring) {
			continue
		}
		games, innings, pitcher := pitcherWorkload(row)
		if !pitcher {
			continue
		}
		key := cohortKey(row.Competition, row.Season) + "|" + normalizationGroup(row)
		m := maximums[key]
		m.games = max(m.games, int(games))
		m.innings = max(m.innings, innings)
		maximums[key] = m
	}
	fraction := float64(pitcherWorkloadPercent(f)) / 100
	for key, maximum := range maximums {
		maximums[key] = workloadThreshold{games: max(max(1, f.BenchmarkGames), int(math.Ceil(float64(maximum.games)*fraction))), innings: maximum.innings * fraction}
	}
	return maximums
}

func qualification(row db.ListResearchPlayersRow, minimum int, limits map[string]workloadThreshold) (bool, workloadThreshold, string) {
	limit := workloadThreshold{games: max(1, minimum)}
	games, innings, pitcher := pitcherWorkload(row)
	if pitcher {
		if l, ok := limits[cohortKey(row.Competition, row.Season)+"|"+normalizationGroup(row)]; ok {
			limit = l
		}
		if games < float64(limit.games) || innings+1e-9 < limit.innings {
			return false, limit, fmt.Sprintf("Small pitching workload · %.0f/%d appearances · %.1f/%.1f innings", games, limit.games, innings, limit.innings)
		}
	} else if int(row.Games) < limit.games {
		return false, limit, fmt.Sprintf("Small sample · %d/%d games", row.Games, limit.games)
	}
	return true, limit, ""
}

func analyze(rows []db.ListResearchPlayersRow, rules map[string]analyticRules, f ResearchFilter) AnalyticsPage {
	if f.BenchmarkGames < 1 {
		f.BenchmarkGames = 5
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 {
		f.PerPage = 50
	}
	if f.Sort == "" {
		f.Sort = "points"
	}
	limits := workloadThresholds(rows, rules, f)
	cohorts := map[string][]db.ListResearchPlayersRow{}
	positions := map[string]map[string][]db.ListResearchPlayersRow{}
	teams := map[string]bool{}
	for _, p := range rows {
		for _, team := range strings.Split(p.Team, " / ") {
			if team != "" {
				teams[team] = true
			}
		}
		qualified, _, _ := qualification(p, f.BenchmarkGames, limits)
		if p.Season == "" || !qualified || !hasScoringStats(p.Stats, rules[p.Competition].Rules.Scoring) {
			continue
		}
		key := cohortKey(p.Competition, p.Season)
		cohorts[key] = append(cohorts[key], p)
		if positions[key] == nil {
			positions[key] = map[string][]db.ListResearchPlayersRow{}
		}
		for _, pos := range comparisonPositions(p) {
			positions[key][pos] = append(positions[key][pos], p)
		}
	}
	result := AnalyticsPage{PitcherWorkloadPercent: pitcherWorkloadPercent(f), Players: []AnalyticsPlayer{}, Teams: []string{}, Benchmarks: []Benchmark{}, BenchmarkGames: f.BenchmarkGames, Page: f.Page, PerPage: f.PerPage}
	for team := range teams {
		result.Teams = append(result.Teams, team)
	}
	slices.Sort(result.Teams)
	positional := map[string]map[string]distribution{}
	baselines := map[string]map[string]Benchmark{}
	leagueDistributions := map[string]distribution{}
	seasonDistributions := map[string]distribution{}
	roleDistributions := map[string]distribution{}
	roleRows := map[string][]db.ListResearchPlayersRow{}
	for key, cohort := range cohorts {
		for _, row := range cohort {
			role := key + "|" + normalizationGroup(row)
			roleRows[role] = append(roleRows[role], row)
		}
	}
	for key, rows := range roleRows {
		roleDistributions[key] = summarize(rows)
	}
	for comp, cohort := range cohorts {
		d := summarize(cohort)
		leagueDistributions[comp] = d
		seasonDistributions[comp] = summarizeTotals(cohort)
		actualComp := cohort[0].Competition
		season := cohort[0].Season
		r := rules[actualComp]
		demand := map[string]float64{}
		available := []string{}
		for pos := range positions[comp] {
			available = append(available, pos)
		}
		slots := 0
		for _, slot := range r.Rules.Lineup.Slots {
			slots += slot.Count
			eligible := replacementPositions(actualComp, slot.Positions, available)
			if len(eligible) == 0 {
				continue
			}
			for _, pos := range eligible {
				demand[pos] += float64(slot.Count*r.Managers) / float64(len(eligible))
			}
		}
		result.Benchmarks = append(result.Benchmarks, Benchmark{Competition: actualComp, Season: season, Players: len(cohort), Mean: d.mean, SD: d.sd, StarterSlots: slots})
		positional[comp], baselines[comp] = map[string]distribution{}, map[string]Benchmark{}
		for pos, members := range positions[comp] {
			pd := summarize(members)
			positional[comp][pos] = pd
			b := Benchmark{Competition: actualComp, Season: season, Position: pos, Players: len(members), Mean: pd.mean, SD: pd.sd, StarterSlots: slots}
			meanIndex, count := 0.0, 0
			for _, member := range members {
				if v := index(member.Points, seasonDistributions[comp]); v != nil {
					meanIndex += *v
					count++
				}
			}
			if count > 0 {
				b.MeanIndex = ptr(meanIndex / float64(count))
			}
			// The first rate below estimated starting demand is replacement.
			// Flexible slots share demand equally across their eligible positions.
			rank := int(math.Ceil(demand[pos])) + 1
			if f.ReplacementRank > 0 {
				rank = f.ReplacementRank
			}
			if (demand[pos] > 0 || f.ReplacementRank > 0) && rank <= len(pd.rates) {
				b.ReplacementRank = ptr(rank)
				b.ReplacementRate = ptr(pd.rates[len(pd.rates)-rank])
			}
			baselines[comp][pos] = b
			result.Benchmarks = append(result.Benchmarks, b)
		}
	}
	slices.SortFunc(result.Benchmarks, func(a, b Benchmark) int {
		return cmp.Or(cmp.Compare(a.Competition, b.Competition), cmp.Compare(a.Season, b.Season), cmp.Compare(a.Position, b.Position))
	})
	var filtered []AnalyticsPlayer
	result.Chart = []ChartPoint{}
	for _, row := range rows {
		p := AnalyticsPlayer{ListResearchPlayersRow: row, RowKey: row.ID.String() + "|" + row.Competition + "|" + row.Season, NormalizationGroup: "League", ScoringSource: rules[row.Competition].Source, HasScoringStats: hasScoringStats(row.Stats, rules[row.Competition].Rules.Scoring)}
		key := cohortKey(p.Competition, p.Season)
		p.values = statValues(row.Stats)
		d := roleDistributions[key+"|"+normalizationGroup(row)]
		seasonD := seasonDistributions[key]
		leagueD := leagueDistributions[key]
		qualified, threshold, note := qualification(row, f.BenchmarkGames, limits)
		p.BenchmarkMinimumGames, p.BenchmarkMinimumInnings, p.QualificationNote = threshold.games, threshold.innings, note
		p.Qualified = p.Season != "" && qualified && p.HasScoringStats && len(seasonD.rates) > 0
		if p.Qualified {
			p.LeagueIndex = index(p.Points, seasonD)
			if len(seasonD.rates) >= 2 {
				lo, _ := slices.BinarySearch(seasonD.rates, p.Points)
				hi := sort.Search(len(seasonD.rates), func(i int) bool { return seasonD.rates[i] > p.Points })
				p.Percentile = ptr(100 * float64(lo+hi-1) / 2 / float64(len(seasonD.rates)-1))
			}
			if leagueD.maxGames > 0 {
				p.Availability = ptr(100 * float64(p.Games) / float64(leagueD.maxGames))
			}
			if leagueD.positivePoints > 0 {
				p.ProductionShare = ptr(100 * max(0, p.Points) / leagueD.positivePoints)
			}
			// For multi-position players, use their most valuable eligible position.
			// A position filter explicitly selects the comparison position.
			candidates := comparisonPositions(row)
			slices.Sort(candidates)
			if f.Position != "" {
				selected := []string{}
				for _, pos := range strings.Split(f.Position, ",") {
					selected = append(selected, replacementPositions(p.Competition, []string{pos}, candidates)...)
				}
				candidates = slices.DeleteFunc(candidates, func(pos string) bool { return !slices.Contains(selected, pos) })
			}
			for _, pos := range candidates {
				b := baselines[key][pos]
				if p.MetricPosition == "" {
					p.MetricPosition = pos
					p.PositionIndex = index(p.PointsPerGame, positional[key][pos])
				}
				if b.ReplacementRate == nil {
					continue
				}
				par := p.PointsPerGame - *b.ReplacementRate
				if p.PARPerGame != nil && par <= *p.PARPerGame {
					continue
				}
				p.MetricPosition, p.ReplacementRate, p.ReplacementRank = pos, b.ReplacementRate, b.ReplacementRank
				p.PositionIndex = index(p.PointsPerGame, positional[key][pos])
				p.PARPerGame, p.PointsAboveReplacement = ptr(par), ptr(par*float64(p.Games))
				if d.sd > 1e-9 && len(d.rates) >= 2 && b.StarterSlots > 0 {
					// Illustrative one-game-per-starter matchup: independent normal scores,
					// using observed season-rate spread as the variance proxy. Signed delta
					// from a replacement lineup's 50% win probability, times appearances.
					z := par / (d.sd * math.Sqrt(2*float64(b.StarterSlots)))
					p.WinShareAdded = ptr(0.5 * math.Erf(z/math.Sqrt2) * float64(p.Games))
				}
			}
		}
		if f.PlayerID.Valid && p.ID != f.PlayerID {
			continue
		}
		if f.Competition != "" && p.Competition != f.Competition {
			continue
		}
		if f.QualifiedOnly && !p.Qualified || f.MissingOnly && p.HasScoringStats {
			continue
		}
		if (f.MaxGamesSet || f.MaxGames > 0) && int(p.Games) > f.MaxGames || (f.MaxRateSet || f.MaxRate != 0) && (!p.HasScoringStats || p.Games <= 0 || p.PointsPerGame > f.MaxRate) {
			continue
		}
		if (f.MaxIndexSet || f.MaxIndex != 0) && (p.LeagueIndex == nil || *p.LeagueIndex > f.MaxIndex) {
			continue
		}
		if (f.MinPARSet || f.MinPAR != 0) && (p.PARPerGame == nil || *p.PARPerGame < f.MinPAR) {
			continue
		}
		if f.StatKey != "" {
			value, ok := p.values[f.StatKey]
			if !ok {
				continue
			}
			if f.MinStat != nil && value < *f.MinStat || f.MaxStat != nil && value > *f.MaxStat {
				continue
			}
		}
		if f.Status != "" && p.Status != f.Status || f.Search != "" && !strings.Contains(strings.ToLower(p.FullName), strings.ToLower(f.Search)) {
			continue
		}
		if f.Position != "" && !matchesPosition(p.Positions, f.Position) && !matchesPosition(comparisonPositions(row), f.Position) {
			continue
		}
		if f.Team != "" && !slices.Contains(strings.Split(p.Team, " / "), f.Team) && p.Team != f.Team {
			continue
		}
		if f.Owner == "available" && p.OwnerSlug != "" || f.Owner == "rostered" && p.OwnerSlug == "" {
			continue
		}
		if f.Owner != "" && f.Owner != "available" && f.Owner != "rostered" && p.OwnerSlug != f.Owner {
			continue
		}
		if int(p.Games) < f.MinGames || (f.MinRateSet || f.MinRate != 0) && (!p.HasScoringStats || p.Games <= 0 || p.PointsPerGame < f.MinRate) {
			continue
		}
		if (f.MinIndexSet || f.MinIndex != 0) && (p.LeagueIndex == nil || *p.LeagueIndex < f.MinIndex) {
			continue
		}
		if f.AboveReplacement && (p.PARPerGame == nil || *p.PARPerGame <= 0) {
			continue
		}
		filtered = append(filtered, p)
	}
	slices.SortFunc(filtered, func(a, b AnalyticsPlayer) int {
		if slices.Contains([]string{"name", "competition", "season", "team", "owner"}, f.Sort) {
			text := func(p AnalyticsPlayer) string {
				switch f.Sort {
				case "competition":
					return p.Competition + " " + p.Season
				case "season":
					return p.Season
				case "team":
					return p.Team
				case "owner":
					return p.OwnerName
				default:
					return p.FullName
				}
			}
			c := cmp.Compare(text(a), text(b))
			if !f.Ascending {
				c = -c
			}
			return cmp.Or(c, cmp.Compare(a.RowKey, b.RowKey))
		}
		av, bv := metricValue(a, f.Sort), metricValue(b, f.Sort)
		if f.RawPerGame {
			if value, ok := a.values[f.Sort]; ok {
				if a.Games > 0 {
					av = ptr(value / float64(a.Games))
				} else {
					av = nil
				}
			}
			if value, ok := b.values[f.Sort]; ok {
				if b.Games > 0 {
					bv = ptr(value / float64(b.Games))
				} else {
					bv = nil
				}
			}
		}
		if av == nil && bv != nil {
			return 1
		}
		if av != nil && bv == nil {
			return -1
		}
		c := 0
		if av != nil && bv != nil {
			c = cmp.Compare(*bv, *av)
			if f.Ascending {
				c = -c
			}
		}
		return cmp.Or(c, cmp.Compare(a.FullName, b.FullName), cmp.Compare(a.RowKey, b.RowKey))
	})
	result.Total = len(filtered)
	result.ChartTotal = len(filtered)
	if f.IncludeChart {
		for _, p := range filtered[:min(len(filtered), 20000)] {
			group := strings.ToUpper(p.Competition) + " " + p.Season
			switch f.ChartGroup {
			case "position":
				group = p.MetricPosition
				if group == "" {
					group = strings.Join(p.Positions, "/")
				}
			case "team":
				group = p.Team
			case "owner":
				group = p.OwnerName
				if group == "" {
					group = "Available"
				}
			}
			result.Chart = append(result.Chart, ChartPoint{Key: p.RowKey, Label: p.FullName + " · " + strings.ToUpper(p.Competition) + " " + p.Season, Group: group, X: metricValue(p, f.ChartX), Y: metricValue(p, f.ChartY)})
		}
	}
	lastPage := max(1, (result.Total+f.PerPage-1)/f.PerPage)
	result.Page = min(f.Page, lastPage)
	start := (result.Page - 1) * f.PerPage
	result.Players = append(result.Players, filtered[start:min(start+f.PerPage, len(filtered))]...)
	return result
}

func metricValue(p AnalyticsPlayer, key string) *float64 {
	switch key {
	case "league_index":
		return p.LeagueIndex
	case "position_index":
		return p.PositionIndex
	case "percentile":
		return p.Percentile
	case "points_above_replacement":
		return p.PointsAboveReplacement
	case "par_per_game":
		return p.PARPerGame
	case "win_share_added":
		return p.WinShareAdded
	case "availability":
		return p.Availability
	case "production_share":
		return p.ProductionShare
	case "points":
		if p.Season != "" && p.HasScoringStats {
			return ptr(p.Points)
		}
	case "points_per_game":
		if p.Games > 0 && p.HasScoringStats {
			return ptr(p.PointsPerGame)
		}
	case "games":
		if p.Season != "" {
			return ptr(float64(p.Games))
		}
	default:
		if v, ok := p.values[key]; ok {
			return ptr(v)
		}
	}
	return nil
}

func cohortKey(competition, season string) string { return competition + "|" + season }
func hasScoringStats(raw json.RawMessage, scoring map[string]float64) bool {
	stats := statValues(raw)
	for key, weight := range scoring {
		if _, ok := stats[key]; ok && weight != 0 {
			return true
		}
	}
	return false
}
func matchesPosition(positions []string, query string) bool {
	for _, pos := range strings.Split(query, ",") {
		if slices.Contains(positions, pos) {
			return true
		}
	}
	return false
}
func leagueAnalysis(rows []db.ListResearchPlayersRow, rules map[string]analyticRules, f ResearchFilter) []LeagueAnalysis {
	limits := workloadThresholds(rows, rules, f)
	groups := map[string][]db.ListResearchPlayersRow{}
	for _, row := range rows {
		if row.Season != "" {
			key := cohortKey(row.Competition, row.Season)
			groups[key] = append(groups[key], row)
		}
	}
	result := []LeagueAnalysis{}
	for _, members := range groups {
		first := members[0]
		r := rules[first.Competition]
		a := LeagueAnalysis{Competition: first.Competition, Season: first.Season, ScoringSource: r.Source, Players: len(members), Contributions: map[string]float64{}}
		rates, totals := []float64{}, []float64{}
		positive := 0.0
		for _, p := range members {
			a.Games += int(p.Games)
			if !hasScoringStats(p.Stats, r.Rules.Scoring) {
				continue
			}
			a.Scored++
			a.Points += p.Points
			for stat, value := range statValues(p.Stats) {
				if weight := r.Rules.Scoring[stat]; weight != 0 {
					a.Contributions[stat] += value * weight
				}
			}
			if qualified, _, _ := qualification(p, f.BenchmarkGames, limits); qualified {
				a.Qualified++
				rates = append(rates, p.PointsPerGame)
				totals = append(totals, max(0, p.Points))
				positive += max(0, p.Points)
			}
		}
		slices.Sort(rates)
		slices.Sort(totals)
		if len(rates) > 0 {
			a.Median = quantile(rates, 0.5)
			a.P90 = quantile(rates, 0.9)
		}
		if positive > 0 {
			for _, v := range totals[max(0, len(totals)-10):] {
				a.TopTenShare += 100 * v / positive
			}
		}
		result = append(result, a)
	}
	slices.SortFunc(result, func(a, b LeagueAnalysis) int {
		return cmp.Or(cmp.Compare(a.Competition, b.Competition), cmp.Compare(a.Season, b.Season))
	})
	return result
}
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := q * float64(len(sorted)-1)
	lo := int(math.Floor(index))
	hi := int(math.Ceil(index))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(index-float64(lo))
}
