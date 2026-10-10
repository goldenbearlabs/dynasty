package players

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"crossover/internal/db"
	"crossover/internal/roster"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const profilePageSize = 25

type TimelinePage struct {
	Events  []db.ListPlayerTimelineRow `json:"events"`
	Total   int64                      `json:"total"`
	Page    int                        `json:"page"`
	PerPage int                        `json:"per_page"`
}
type ProfileGame struct {
	db.ListPlayerGameLogRow
	Points        float64 `json:"points"`
	ScoringSource string  `json:"scoring_source"`
}
type GameLogPage struct {
	Games   []ProfileGame `json:"games"`
	Total   int64         `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
}
type ProfileSeason struct {
	SeasonLine
	Key            string           `json:"key"`
	Year           int32            `json:"year"`
	SyncedAt       time.Time        `json:"synced_at"`
	ScoringSource  string           `json:"scoring_source"`
	Research       *AnalyticsPlayer `json:"research"`
	ResearchNote   string           `json:"research_note"`
	EligibleSplits int              `json:"eligible_splits"`
	Splits         int              `json:"splits"`
}
type FantasyProduction struct {
	SeasonID      pgtype.UUID `json:"season_id"`
	Year          int32       `json:"year"`
	Competition   string      `json:"competition"`
	FranchiseName string      `json:"franchise_name"`
	FranchiseSlug string      `json:"franchise_slug"`
	Games         int64       `json:"games"`
	Points        float64     `json:"points"`
}
type Profile struct {
	FantasyProduction []FantasyProduction `json:"fantasy_production"`

	Player             db.GetPlayerProfileRow         `json:"player"`
	Seasons            []ProfileSeason                `json:"seasons"`
	Games              GameLogPage                    `json:"game_log"`
	Timeline           TimelinePage                   `json:"timeline"`
	Ownership          []db.ListPlayerOwnershipRow    `json:"ownership"`
	Drafts             []db.ListPlayerDraftRecordRow  `json:"drafts"`
	Trades             []db.ListPlayerTradeRecordRow  `json:"trades"`
	Waivers            []db.ListPlayerWaiverStatusRow `json:"waivers"`
	StarterEligible    bool                           `json:"starter_eligible"`
	EligibilityNote    string                         `json:"eligibility_note"`
	Conference         string                         `json:"conference"`
	ScoringRules       map[string]map[string]float64  `json:"scoring_rules"`
	RulesSources       map[string]string              `json:"rules_sources"`
	ReserveLockedUntil map[string]time.Time           `json:"reserve_locked_until"`
}

func (s *Service) profileRules(ctx context.Context) (map[string]settings.League, map[string]string, error) {
	q := db.New(s.pool)
	rules, sources := map[string]settings.League{}, map[string]string{}
	for _, c := range s.registry {
		rules[c.Key] = c.Defaults
		sources[c.Key] = "defaults"
	}
	dynasty, err := q.GetDynasty(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return rules, sources, nil
	}
	if err != nil {
		return nil, nil, err
	}
	leagues, err := q.ListLeagues(ctx, dynasty.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, l := range leagues {
		r, e := settings.Parse[settings.League](l.Settings)
		if e != nil {
			return nil, nil, e
		}
		rules[l.Competition] = r
		sources[l.Competition] = "league"
	}
	return rules, sources, nil
}

func (s *Service) Timeline(ctx context.Context, id pgtype.UUID, page int) (TimelinePage, error) {
	q := db.New(s.pool)
	if _, err := q.GetPlayer(ctx, id); err != nil {
		return TimelinePage{}, err
	}
	page = max(1, min(100000, page))
	rows, err := q.ListPlayerTimeline(ctx, db.ListPlayerTimelineParams{PlayerID: id, PageSize: profilePageSize, PageOffset: int32((page - 1) * profilePageSize)})
	if err != nil {
		return TimelinePage{}, err
	}
	if rows == nil {
		rows = []db.ListPlayerTimelineRow{}
	}
	total, err := q.CountPlayerTimeline(ctx, id)
	return TimelinePage{Events: rows, Total: total, Page: page, PerPage: profilePageSize}, err
}
func (s *Service) GameLog(ctx context.Context, id pgtype.UUID, competition string, page int) (GameLogPage, error) {
	q := db.New(s.pool)
	if _, err := q.GetPlayer(ctx, id); err != nil {
		return GameLogPage{}, err
	}
	rules, sources, err := s.profileRules(ctx)
	if err != nil {
		return GameLogPage{}, err
	}
	page = max(1, min(100000, page))
	rows, err := q.ListPlayerGameLog(ctx, db.ListPlayerGameLogParams{PlayerID: id, Competition: competition, PageSize: profilePageSize, PageOffset: int32((page - 1) * profilePageSize)})
	if err != nil {
		return GameLogPage{}, err
	}
	result := GameLogPage{Games: []ProfileGame{}, Page: page, PerPage: profilePageSize}
	for _, row := range rows {
		g := ProfileGame{ListPlayerGameLogRow: row, ScoringSource: sources[row.Competition]}
		var stats map[string]float64
		if err := json.Unmarshal(row.Stats, &stats); err != nil {
			return result, err
		}
		for key, v := range stats {
			g.Points += v * rules[row.Competition].Scoring[key]
		}
		result.Games = append(result.Games, g)
	}
	result.Total, err = q.CountPlayerGameLog(ctx, db.CountPlayerGameLogParams{PlayerID: id, Competition: competition})
	return result, err
}

// Profile reads stored history only. Opening a profile never triggers a feed
// import, so the page remains predictable even when a provider is unavailable.
func (s *Service) Profile(ctx context.Context, id pgtype.UUID) (Profile, error) {
	q := db.New(s.pool)
	p, err := q.GetPlayerProfile(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	rules, sources, err := s.profileRules(ctx)
	if err != nil {
		return Profile{}, err
	}
	result := Profile{Player: p, Seasons: []ProfileSeason{}, ScoringRules: map[string]map[string]float64{}, RulesSources: sources, ReserveLockedUntil: map[string]time.Time{}}
	for key, r := range rules {
		result.ScoringRules[key] = r.Scoring
	}
	result.Conference, err = q.GetPlayerConference(ctx, id)
	if err != nil {
		return result, err
	}
	r := rules[p.Competition]
	result.StarterEligible = r.CanStart(p.Competition, result.Conference) && slices.ContainsFunc(r.Lineup.Slots, func(slot settings.Slot) bool {
		return slices.Contains(slot.Positions, settings.AnyPosition) || slices.ContainsFunc(p.Positions, func(pos string) bool { return slices.Contains(slot.Positions, pos) })
	})
	if !r.CanStart(p.Competition, result.Conference) {
		result.EligibilityNote = "Outside this league's starting conferences; draft and reserve eligibility are separate."
	} else if !result.StarterEligible {
		result.EligibilityNote = "No starting slot currently accepts this player's positions."
	} else {
		result.EligibilityNote = "Positions and conference meet the current starting rules. The player must be on the main roster to start."
	}
	rows, err := q.ListPlayerSeasons(ctx, id)
	if err != nil {
		return result, err
	}
	groups := map[string]*ProfileSeason{}
	pools := []ResearchPool{}
	seen := map[string]bool{}
	for _, row := range rows {
		key := fmt.Sprintf("%s|%d|%s", row.Competition, row.Year, row.League)
		line := groups[key]
		if line == nil {
			line = &ProfileSeason{Key: key, Year: row.Year, SeasonLine: SeasonLine{Competition: row.Competition, Season: row.Label, League: row.League, Stats: map[string]float64{}}, ScoringSource: sources[row.Competition]}
			groups[key] = line
		}
		line.Splits++
		if rules[row.Competition].CanStart(row.Competition, row.Conference) {
			line.EligibleSplits++
		}
		if row.Team != "" {
			if line.Team != "" {
				line.Team += " / "
			}
			line.Team += row.Team
		}
		line.Games += int(row.Games)
		if row.SyncedAt.Time.After(line.SyncedAt) {
			line.SyncedAt = row.SyncedAt.Time
		}
		var stats map[string]float64
		if err := json.Unmarshal(row.Stats, &stats); err != nil {
			return result, err
		}
		for k, v := range stats {
			line.Stats[k] += v
			line.Points += v * rules[row.Competition].Scoring[k]
		}
		poolKey := row.Competition + "|" + row.Label
		if row.League == "" && !seen[poolKey] {
			seen[poolKey] = true
			pools = append(pools, ResearchPool{Competition: row.Competition, Season: row.Label})
		}
	}
	metrics := map[string]AnalyticsPlayer{}
	// Analytics pools are bounded per request; batch a long career without
	// truncating its seasons, keeping complete league-season peer populations.
	for start := 0; start < len(pools); start += 24 {
		end := min(start+24, len(pools))
		a, e := s.Analytics(ctx, ResearchFilter{Pools: pools[start:end], PlayerID: id, PerPage: 1000})
		if e != nil {
			return result, e
		}
		for _, m := range a.Players {
			metrics[m.Competition+"|"+m.Season] = m
		}
	}
	for _, line := range groups {
		if line.Games > 0 {
			line.PointsPerGame = line.Points / float64(line.Games)
		}
		if line.League != "" {
			line.ResearchNote = "External-league stats valued with this competition's current rules; no matching peer benchmark."
		} else if m, ok := metrics[line.Competition+"|"+line.Season]; ok {
			line.Research = &m
			if !m.Qualified {
				line.ResearchNote = m.QualificationNote
				if line.ResearchNote == "" {
					line.ResearchNote = "Not enough scored appearances for comparative research."
				}
			} else if line.EligibleSplits < line.Splits {
				line.ResearchNote = "Research metrics cover only conference-eligible parts of this season."
			}
		} else if line.EligibleSplits == 0 {
			line.ResearchNote = "This season is outside the current starting-conference pool; raw stats remain available."
		} else {
			line.ResearchNote = "No comparable research cohort is available for this imported season."
		}
		result.Seasons = append(result.Seasons, *line)
	}
	sort.Slice(result.Seasons, func(i, j int) bool {
		a, b := result.Seasons[i], result.Seasons[j]
		if a.Year != b.Year {
			return a.Year > b.Year
		}
		return a.Key < b.Key
	})
	result.Ownership, err = q.ListPlayerOwnership(ctx, db.ListPlayerOwnershipParams{PlayerID: id, Day: sportsday.Date(sportsday.Today())})
	if err != nil {
		return result, err
	}
	if result.Ownership == nil {
		result.Ownership = []db.ListPlayerOwnershipRow{}
	}
	for _, owned := range result.Ownership {
		r := rules[owned.Competition]
		if owned.List != settings.ListReserve {
			continue
		}
		seasonEnds, err := roster.SeasonEnds(ctx, q, owned.LeagueID)
		if err != nil {
			return result, err
		}
		if until := roster.LockedUntil(r.Roster, owned.ReservedAt, seasonEnds); !until.IsZero() {
			result.ReserveLockedUntil[owned.LeagueID.String()] = until
		}
	}
	result.Drafts, err = q.ListPlayerDraftRecord(ctx, id)
	if err != nil {
		return result, err
	}
	if result.Drafts == nil {
		result.Drafts = []db.ListPlayerDraftRecordRow{}
	}
	result.Trades, err = q.ListPlayerTradeRecord(ctx, id)
	if err != nil {
		return result, err
	}
	if result.Trades == nil {
		result.Trades = []db.ListPlayerTradeRecordRow{}
	}
	result.Waivers, err = q.ListPlayerWaiverStatus(ctx, id)
	if err != nil {
		return result, err
	}
	if result.Waivers == nil {
		result.Waivers = []db.ListPlayerWaiverStatusRow{}
	}
	result.FantasyProduction, err = s.fantasyProduction(ctx, id)
	if err != nil {
		return result, err
	}
	result.Timeline, err = s.Timeline(ctx, id, 1)
	if err != nil {
		return result, err
	}
	result.Games, err = s.GameLog(ctx, id, "", 1)
	return result, err
}

// Reuse the standings query so lineup history, conference rules and weekly
// limits determine counted production. Filtering a player before that query
// would incorrectly change pitcher-start caps shared by a franchise.
func (s *Service) fantasyProduction(ctx context.Context, id pgtype.UUID) ([]FantasyProduction, error) {
	q := db.New(s.pool)
	result := []FantasyProduction{}
	dynasty, err := q.GetDynasty(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	seasons, err := q.ListSeasons(ctx, dynasty.ID)
	if err != nil {
		return nil, err
	}
	leagues, err := q.ListLeagues(ctx, dynasty.ID)
	if err != nil {
		return nil, err
	}
	franchises, err := q.ListFranchises(ctx, dynasty.ID)
	if err != nil {
		return nil, err
	}
	leagueMap := map[pgtype.UUID]db.League{}
	for _, l := range leagues {
		leagueMap[l.ID] = l
	}
	franchiseMap := map[pgtype.UUID]db.Franchise{}
	for _, f := range franchises {
		franchiseMap[f.ID] = f
	}
	for _, season := range seasons {
		to := season.EndsOn.Time
		if today := sportsday.Today(); today.Before(to) {
			to = today
		}
		if to.Before(season.StartsOn.Time) {
			continue
		}
		l := leagueMap[season.LeagueID]
		rules, err := settings.Parse[settings.League](l.Settings)
		if err != nil {
			return nil, err
		}
		rows, err := q.ListLineupPoints(ctx, db.ListLineupPointsParams{LeagueID: l.ID, FromDay: season.StartsOn, ToDay: sportsday.Date(to), WeekStart: sportsday.Date(sportsday.WeekStart(season.StartsOn.Time, rules.Lineup.WeekStart))})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.PlayerID == id {
				f := franchiseMap[row.FranchiseID]
				result = append(result, FantasyProduction{SeasonID: season.ID, Year: season.Year, Competition: l.Competition, FranchiseName: f.Name, FranchiseSlug: f.Slug, Games: row.Games, Points: row.Points})
			}
		}
	}
	return result, nil
}
