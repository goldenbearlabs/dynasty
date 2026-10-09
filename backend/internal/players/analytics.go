package players

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sort"
	"strings"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"github.com/jackc/pgx/v5"
)

// ResearchFilter affects displayed rows, never the population used to compute
// benchmarks. BenchmarkGames and ReplacementRank explicitly change the model.
type ResearchPool struct {
	Competition string `json:"competition"`
	Season      string `json:"season"`
}

type ResearchFilter struct {
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
	db.ListResearchPlayersRow
	RowKey                 string   `json:"row_key"`
	ScoringSource          string   `json:"scoring_source"`
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
	StartingConferences map[string][]string         `json:"starting_conferences"`
	RawStatKeys         []string                    `json:"raw_stat_keys"`
	Catalog             []db.ListResearchCatalogRow `json:"catalog"`
	Analysis            []LeagueAnalysis            `json:"analysis"`
	Chart               []ChartPoint                `json:"chart"`
	ChartTotal          int                         `json:"chart_total"`
	Warnings            []string                    `json:"warnings"`
	Players             []AnalyticsPlayer           `json:"players"`
	Total               int                         `json:"total"`
	Page                int                         `json:"page"`
	PerPage             int                         `json:"per_page"`
	Seasons             []db.ListResearchSeasonsRow `json:"seasons"`
	Teams               []string                    `json:"teams"`
	Benchmarks          []Benchmark                 `json:"benchmarks"`
	BenchmarkGames      int                         `json:"benchmark_games"`
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
	q := db.New(s.pool)

	rules := map[string]analyticRules{}
	dynasty, err := q.GetDynasty(ctx)
	managers := 0
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return AnalyticsPage{}, err
	}
	if err == nil {
		leagues, err := q.ListLeagues(ctx, dynasty.ID)
		if err != nil {
			return AnalyticsPage{}, err
		}
		franchises, err := q.ListFranchises(ctx, dynasty.ID)
		if err != nil {
			return AnalyticsPage{}, err
		}
		managers = len(franchises)
		for _, l := range leagues {
			parsed, err := settings.Parse[settings.League](l.Settings)
			if err != nil {
				return AnalyticsPage{}, err
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
			return AnalyticsPage{}, err
		}
	} else {
		if len(f.Pools) > 24 {
			return AnalyticsPage{}, problem.New("Choose up to 24 league-season pools.")
		}
		seen := map[string]bool{}
		for _, pool := range f.Pools {
			if _, ok := s.registry.Get(pool.Competition); !ok {
				return AnalyticsPage{}, problem.New("Unknown research league: %s.", pool.Competition)
			}
			key := pool.Competition + "|" + pool.Season
			if seen[key] {
				continue
			}
			seen[key] = true
			records, err := q.ListResearchSeasonPool(ctx, db.ListResearchSeasonPoolParams{Competition: pool.Competition, Conferences: rules[pool.Competition].Rules.StarterConferences(pool.Competition), Season: pool.Season})
			if err != nil {
				return AnalyticsPage{}, err
			}
			for _, r := range records {
				rows = append(rows, db.ListResearchPlayersRow{ID: r.ID, Competition: r.Competition, FullName: r.FullName, Positions: r.Positions, Status: r.Status, HeadshotUrl: r.HeadshotUrl, Team: r.Team, OwnerName: r.OwnerName, OwnerSlug: r.OwnerSlug, Season: r.Season, Games: r.Games, Stats: r.Stats})
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
		var stats map[string]float64
		if err := json.Unmarshal(row.Stats, &stats); err != nil {
			return AnalyticsPage{}, err
		}
		row.Points, row.PointsPerGame = 0, 0
		for stat, value := range stats {
			row.Points += value * rules[row.Competition].Rules.Scoring[stat]
		}
		if row.Games > 0 {
			row.PointsPerGame = row.Points / float64(row.Games)
		}
		valued = append(valued, row)
	}
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
		var stats map[string]float64
		json.Unmarshal(row.Stats, &stats)
		for key := range stats {
			keys[key] = true
		}
	}
	result.RawStatKeys = []string{}
	for key := range keys {
		result.RawStatKeys = append(result.RawStatKeys, key)
	}
	slices.Sort(result.RawStatKeys)
	result.Seasons, err = q.ListResearchSeasons(ctx, f.Competition)
	if err != nil {
		return AnalyticsPage{}, err
	}
	result.Catalog, err = q.ListResearchCatalog(ctx, rules["cbb"].Rules.StarterConferences("cbb"))
	if err != nil {
		return AnalyticsPage{}, err
	}
	result.Analysis = leagueAnalysis(valued, rules, result.BenchmarkGames)
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

func index(rate float64, d distribution) *float64 {
	if len(d.rates) < 2 || d.sd < 1e-9 {
		return nil
	}
	return ptr(100 + 15*(rate-d.mean)/d.sd)
}
func ptr[T any](v T) *T { return &v }

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
	cohorts := map[string][]db.ListResearchPlayersRow{}
	positions := map[string]map[string][]db.ListResearchPlayersRow{}
	teams := map[string]bool{}
	for _, p := range rows {
		for _, team := range strings.Split(p.Team, " / ") {
			if team != "" {
				teams[team] = true
			}
		}
		if p.Season == "" || int(p.Games) < f.BenchmarkGames || !hasScoringStats(p.Stats, rules[p.Competition].Rules.Scoring) {
			continue
		}
		key := cohortKey(p.Competition, p.Season)
		cohorts[key] = append(cohorts[key], p)
		if positions[key] == nil {
			positions[key] = map[string][]db.ListResearchPlayersRow{}
		}
		for _, pos := range slices.Compact(slices.Sorted(slices.Values(p.Positions))) {
			positions[key][pos] = append(positions[key][pos], p)
		}
	}
	result := AnalyticsPage{Players: []AnalyticsPlayer{}, Teams: []string{}, Benchmarks: []Benchmark{}, BenchmarkGames: f.BenchmarkGames, Page: f.Page, PerPage: f.PerPage}
	for team := range teams {
		result.Teams = append(result.Teams, team)
	}
	slices.Sort(result.Teams)
	distributions := map[string]distribution{}
	positional := map[string]map[string]distribution{}
	baselines := map[string]map[string]Benchmark{}
	for comp, cohort := range cohorts {
		d := summarize(cohort)
		distributions[comp] = d
		actualComp := cohort[0].Competition
		season := cohort[0].Season
		r := rules[actualComp]
		demand := map[string]float64{}
		slots := 0
		for _, slot := range r.Rules.Lineup.Slots {
			slots += slot.Count
			eligible := slot.Positions
			if slices.Contains(eligible, settings.AnyPosition) {
				eligible = r.Positions
			}
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
		p := AnalyticsPlayer{ListResearchPlayersRow: row, RowKey: row.ID.String() + "|" + row.Competition + "|" + row.Season, ScoringSource: rules[row.Competition].Source, HasScoringStats: hasScoringStats(row.Stats, rules[row.Competition].Rules.Scoring)}
		key := cohortKey(p.Competition, p.Season)
		if err := json.Unmarshal(row.Stats, &p.values); err != nil {
			p.values = map[string]float64{}
		}
		d := distributions[key]
		p.Qualified = p.Season != "" && int(p.Games) >= f.BenchmarkGames && p.HasScoringStats && len(d.rates) > 0
		if p.Qualified {
			p.LeagueIndex = index(p.PointsPerGame, d)
			if len(d.rates) >= 2 {
				lo, _ := slices.BinarySearch(d.rates, p.PointsPerGame)
				hi := sort.Search(len(d.rates), func(i int) bool { return d.rates[i] > p.PointsPerGame })
				p.Percentile = ptr(100 * float64(lo+hi-1) / 2 / float64(len(d.rates)-1))
			}
			if d.maxGames > 0 {
				p.Availability = ptr(100 * float64(p.Games) / float64(d.maxGames))
			}
			if d.positivePoints > 0 {
				p.ProductionShare = ptr(100 * max(0, p.Points) / d.positivePoints)
			}
			// For multi-position players, use their most valuable eligible position.
			// A position filter explicitly selects the comparison position.
			candidates := slices.Clone(p.Positions)
			slices.Sort(candidates)
			if f.Position != "" {
				selected := strings.Split(f.Position, ",")
				candidates = slices.DeleteFunc(candidates, func(pos string) bool { return !slices.Contains(selected, pos) })
			}
			for _, pos := range candidates {
				if !slices.Contains(p.Positions, pos) {
					continue
				}
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
		if f.Position != "" && !matchesPosition(p.Positions, f.Position) {
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
	var stats map[string]float64
	if json.Unmarshal(raw, &stats) != nil {
		return false
	}
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
func leagueAnalysis(rows []db.ListResearchPlayersRow, rules map[string]analyticRules, minimum int) []LeagueAnalysis {
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
			var stats map[string]float64
			json.Unmarshal(p.Stats, &stats)
			for stat, value := range stats {
				if weight := r.Rules.Scoring[stat]; weight != 0 {
					a.Contributions[stat] += value * weight
				}
			}
			if int(p.Games) >= minimum {
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
