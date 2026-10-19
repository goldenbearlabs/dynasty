package players

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

type RosterStatSeason struct {
	Label string `json:"label"`
	Year  int    `json:"year"`
}
type RosterStatPlayer struct {
	PlayerID      pgtype.UUID        `json:"player_id"`
	Games         int64              `json:"games"`
	Stats         map[string]float64 `json:"stats"`
	Points        float64            `json:"points"`
	PointsPerGame float64            `json:"points_per_game"`
	OverallRank   *int               `json:"overall_rank"`
	PositionRanks map[string]int     `json:"position_ranks"`
	positions     []string
}
type RosterStats struct {
	CurrentSeason string             `json:"current_season"`
	Window        string             `json:"window"`
	Season        string             `json:"season"`
	FromDay       string             `json:"from_day"`
	ToDay         string             `json:"to_day"`
	Source        string             `json:"source"`
	RankedPlayers int                `json:"ranked_players"`
	Seasons       []RosterStatSeason `json:"seasons"`
	Players       []RosterStatPlayer `json:"players"`
}

// sportSeason follows official season numbering: basketball and hockey use
// the ending year, football the year in which its fall schedule begins.
func sportSeason(competition string, today time.Time) (RosterStatSeason, time.Time) {
	year := today.Year()
	switch competition {
	case "nba", "cbb", "nhl":
		if today.Month() >= time.July {
			year++
		}
		return RosterStatSeason{fmt.Sprintf("%d-%02d", year-1, year%100), year}, time.Date(year-1, time.July, 1, 0, 0, 0, 0, time.UTC)
	case "nfl":
		if today.Month() < time.July {
			year--
		}
		return RosterStatSeason{strconv.Itoa(year), year}, time.Date(year, time.July, 1, 0, 0, 0, 0, time.UTC)
	default:
		return RosterStatSeason{strconv.Itoa(year), year}, time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
}

// RosterPerformance returns current roster players with their real production
// and competition-wide ranks under this league's scoring rules.
func (s *Service) RosterPerformance(ctx context.Context, league db.League, franchise pgtype.UUID, window, seasonLabel string) (RosterStats, error) {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return RosterStats{}, err
	}
	if window == "" {
		window = "current"
	}
	if !slices.Contains([]string{"current", "7", "14", "30", "season"}, window) {
		return RosterStats{}, problem.New("Choose current season, a past season, or the last 7, 14 or 30 days.")
	}
	today := sportsday.Today()
	current, from := sportSeason(league.Competition, today)
	result := RosterStats{CurrentSeason: current.Label, Window: window, Season: current.Label, FromDay: from.Format(time.DateOnly), ToDay: today.Format(time.DateOnly), Source: "season", Seasons: []RosterStatSeason{}, Players: []RosterStatPlayer{}}
	q := db.New(s.pool)
	seasons, err := q.ListResearchSeasons(ctx, league.Competition)
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, season := range seasons {
		// Some providers temporarily attach the previous label to the upcoming
		// year. Never duplicate those totals or present them as current production.
		expected := int(season.Year)
		if len(season.Label) == 4 {
			if parsed, e := strconv.Atoi(season.Label); e == nil {
				expected = parsed
			}
		}
		if len(season.Label) == 7 && season.Label[4] == '-' {
			if parsed, e := strconv.Atoi(season.Label[:4]); e == nil {
				expected = parsed + 1
			}
		}
		if expected != int(season.Year) || seen[season.Label] {
			continue
		}
		seen[season.Label] = true
		result.Seasons = append(result.Seasons, RosterStatSeason{season.Label, int(season.Year)})
	}
	selected := current
	if window == "season" {
		i := slices.IndexFunc(result.Seasons, func(s RosterStatSeason) bool { return s.Label == seasonLabel })
		if i < 0 {
			return result, problem.New("Choose an available season.")
		}
		selected = result.Seasons[i]
		result.Season = selected.Label
		result.FromDay = ""
		result.ToDay = ""
	}
	useSeason := window == "season" || window == "current"
	if !useSeason {
		days, _ := strconv.Atoi(window)
		from = today.AddDate(0, 0, -days+1)
		result.FromDay = from.Format(time.DateOnly)
		result.Season = ""
		result.Source = "games"
	}
	params := db.ListRosterStatsPoolParams{UseSeason: useSeason, Competition: league.Competition, Year: int32(selected.Year), Season: selected.Label, FromDay: sportsday.Date(from), ToDay: sportsday.Date(today)}
	pool, err := q.ListRosterStatsPool(ctx, params)
	if err != nil {
		return result, err
	}
	if window == "current" && !slices.ContainsFunc(pool, func(p db.ListRosterStatsPoolRow) bool { return p.Games > 0 }) {
		params.UseSeason = false
		pool, err = q.ListRosterStatsPool(ctx, params)
		if err != nil {
			return result, err
		}
		result.Source = "games"
	}
	valued := make([]RosterStatPlayer, 0, len(pool))
	scoringKeys := make([]string, 0, len(rules.Scoring))
	for key := range rules.Scoring {
		scoringKeys = append(scoringKeys, key)
	}
	slices.Sort(scoringKeys)
	for _, p := range pool {
		row := RosterStatPlayer{PlayerID: p.PlayerID, Games: p.Games, Stats: map[string]float64{}, PositionRanks: map[string]int{}, positions: p.Positions}
		if err := json.Unmarshal(p.Stats, &row.Stats); err != nil {
			return result, err
		}
		for _, key := range scoringKeys {
			row.Points += row.Stats[key] * rules.Scoring[key]
		}
		row.Points = math.Round(row.Points*1e6) / 1e6
		if row.Games > 0 {
			row.PointsPerGame = row.Points / float64(row.Games)
		}
		valued = append(valued, row)
	}
	rankRosterStats(valued)
	for _, row := range valued {
		if row.OverallRank != nil {
			result.RankedPlayers++
		}
	}
	roster, err := q.ListRosterEntries(ctx, db.ListRosterEntriesParams{LeagueID: league.ID, FranchiseID: franchise})
	if err != nil {
		return result, err
	}
	for _, player := range roster {
		i := slices.IndexFunc(valued, func(p RosterStatPlayer) bool { return p.PlayerID == player.PlayerID })
		if i >= 0 {
			result.Players = append(result.Players, valued[i])
		} else {
			result.Players = append(result.Players, RosterStatPlayer{PlayerID: player.PlayerID, Stats: map[string]float64{}, PositionRanks: map[string]int{}})
		}
	}
	return result, nil
}

// Equal total FP share a rank (1, 1, 3). Position comparisons include every
// eligible position; roster ownership and list never change the ranking pool.
func rankRosterStats(players []RosterStatPlayer) {
	indices := []int{}
	for i, p := range players {
		if p.Games > 0 {
			indices = append(indices, i)
		}
	}
	slices.SortFunc(indices, func(a, b int) int { return cmp.Compare(players[b].Points, players[a].Points) })
	positionCounts := map[string]int{}
	positionRanks := map[string]int{}
	positionPoints := map[string]float64{}
	rank := 0
	for n, i := range indices {
		p := &players[i]
		if p.PositionRanks == nil {
			p.PositionRanks = map[string]int{}
		}
		if n == 0 || p.Points != players[indices[n-1]].Points {
			rank = n + 1
		}
		p.OverallRank = new(int)
		*p.OverallRank = rank
		seenPositions := map[string]bool{}
		for _, position := range p.positions {
			position = strings.TrimSpace(position)
			if position == "" || seenPositions[position] {
				continue
			}
			seenPositions[position] = true
			positionCounts[position]++
			last, exists := positionPoints[position]
			if !exists || last != p.Points {
				positionRanks[position] = positionCounts[position]
				positionPoints[position] = p.Points
			}
			p.PositionRanks[position] = positionRanks[position]
		}
	}
}
