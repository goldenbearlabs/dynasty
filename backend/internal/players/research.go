package players

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/settings"
)

// Research is the player's current identity and the last ten recorded final
// games. Points use today's league rules, just like the live game pages.
type Research struct {
	Player db.GetPlayerProfileRow    `json:"player"`
	Games  []db.ListPlayerHistoryRow `json:"games"`
}

func (s *Service) Research(ctx context.Context, id pgtype.UUID) (Research, error) {
	q := db.New(s.pool)
	player, err := q.GetPlayerProfile(ctx, id)
	if err != nil {
		return Research{}, err
	}
	games, err := q.ListPlayerHistory(ctx, db.ListPlayerHistoryParams{PlayerID: id, Competition: player.Competition})
	if err != nil {
		return Research{}, err
	}
	return Research{Player: player, Games: games}, nil
}

// SeasonLine is one season of a player's, with what it would have been
// worth under the league's scoring rules as they stand today.
type SeasonLine struct {
	Competition   string             `json:"competition"` // where it was played
	Season        string             `json:"season"`
	Team          string             `json:"team"`
	League        string             `json:"league"` // named when it is not the player's current league
	Games         int                `json:"games"`
	Stats         map[string]float64 `json:"stats"`
	Points        float64            `json:"points"`
	PointsPerGame float64            `json:"points_per_game"`
}

// careerFresh is how long a player's seasons outside his league are trusted
// before his profile is read again.
const careerFresh = 7 * 24 * time.Hour

// Seasons returns a player's stored seasons, most recent first: his own
// league's, any earlier competition's (an NBA player's college years), and
// for hockey the leagues he came through. Every line is valued under the
// rules of the league he plays in now, so the seasons can be compared.
func (s *Service) Seasons(ctx context.Context, id pgtype.UUID) ([]SeasonLine, error) {
	q := db.New(s.pool)
	player, err := q.GetPlayer(ctx, id)
	if err != nil {
		return nil, err
	}
	s.fillCareer(ctx, q, player)

	rows, err := q.ListPlayerSeasons(ctx, id)
	if err != nil {
		return nil, err
	}
	scoring, err := scoringRules(ctx, q, player.Competition)
	if err != nil {
		return nil, err
	}
	lines := []SeasonLine{}
	for _, row := range rows {
		line := SeasonLine{Competition: row.Competition, Season: row.Label, Team: row.Team, League: row.League, Games: int(row.Games)}
		if err := json.Unmarshal(row.Stats, &line.Stats); err != nil {
			return nil, err
		}
		// A season in another competition is labelled with its name.
		if c, ok := s.registry.Get(row.Competition); ok && row.Competition != player.Competition && line.League == "" {
			line.League = c.Name
		}
		for stat, value := range line.Stats {
			line.Points += value * scoring[stat]
		}
		if line.Games > 0 {
			line.PointsPerGame = line.Points / float64(line.Games)
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// fillCareer stores a player's seasons outside his own league, for sports
// whose season feed lacks them. It asks the feed about a player at most
// once a week, and a failure only means those lines are missing for now.
func (s *Service) fillCareer(ctx context.Context, q *db.Queries, player db.Player) {
	c, ok := s.registry.Get(player.Competition)
	if !ok || c.Career == nil {
		return
	}
	if player.CareerCheckedAt.Valid && time.Since(player.CareerCheckedAt.Time) < careerFresh {
		return
	}
	providerID, err := q.GetPlayerExternalID(ctx, db.GetPlayerExternalIDParams{PlayerID: player.ID, Provider: c.Career.IDSpace()})
	if err != nil {
		return // a player this feed does not know
	}
	seasons, err := c.Career.Career(ctx, providerID)
	if err != nil {
		return
	}
	for _, season := range seasons {
		stats, _ := json.Marshal(season.Stats)
		if err := q.UpsertPlayerSeason(ctx, db.UpsertPlayerSeasonParams{
			PlayerID: player.ID, Competition: player.Competition, Year: int32(season.Year), Label: season.Label,
			Team: season.Team, League: season.League, Games: int32(season.Games), Stats: stats,
		}); err != nil {
			return
		}
	}
	q.MarkCareerChecked(ctx, player.ID)
}

// scoringRules returns the points per stat in the league that plays a
// competition, or nothing when no league does.
func scoringRules(ctx context.Context, q *db.Queries, competition string) (map[string]float64, error) {
	dynasty, err := q.GetDynasty(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	league, err := q.GetLeagueByCompetition(ctx, db.GetLeagueByCompetitionParams{DynastyID: dynasty.ID, Competition: competition})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	return rules.Scoring, err
}
