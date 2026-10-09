// Package live keeps scores current while games are being played and tells
// the browsers that are watching.
//
// One loop on the server polls the feeds for the games in play, so the cost
// is the same whether one manager is watching or twenty. Each change is
// written to the database and then pushed, from the database, to whoever is
// listening on that sport's scoreboard or that game's page.
package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/hub"
	"crossover/internal/ingest"
	"crossover/internal/sportsday"
)

type Service struct {
	// Interval is the pause between polls of the games in play.
	Interval time.Duration

	pool     *pgxpool.Pool
	syncer   *ingest.Syncer
	registry competition.Registry
	hub      *hub.Hub
	log      *slog.Logger
}

func NewService(pool *pgxpool.Pool, syncer *ingest.Syncer, registry competition.Registry, hub *hub.Hub, log *slog.Logger) *Service {
	return &Service{Interval: 30 * time.Second, pool: pool, syncer: syncer, registry: registry, hub: hub, log: log}
}

// Scoreboard is a sport's games on one sports day.
type Scoreboard struct {
	Competition string                 `json:"competition"`
	Day         string                 `json:"day"`
	Games       []db.ListGamesOnDayRow `json:"games"`
}

// Game is one game with every player's line.
type Game struct {
	db.GetGameSummaryRow
	Lines []db.ListGameLinesRow `json:"lines"`
}

func (s *Service) Scoreboard(ctx context.Context, competition string, day time.Time) (Scoreboard, error) {
	games, err := db.New(s.pool).ListGamesOnDay(ctx, db.ListGamesOnDayParams{Competition: competition, Day: sportsday.Date(day)})
	return Scoreboard{Competition: competition, Day: day.Format(time.DateOnly), Games: games}, err
}

func (s *Service) Game(ctx context.Context, id pgtype.UUID) (Game, error) {
	q := db.New(s.pool)
	summary, err := q.GetGameSummary(ctx, id)
	if err != nil {
		return Game{}, err
	}
	lines, err := q.ListGameLines(ctx, id)
	return Game{GetGameSummaryRow: summary, Lines: lines}, err
}

// WatchScores subscribes to a sport's scoreboard for today; WatchGame to
// one game. Each message is the whole Scoreboard or Game as JSON.
func (s *Service) WatchScores(competition string) (<-chan []byte, func()) {
	return s.hub.Subscribe(scoresTopic(competition))
}

func (s *Service) WatchGame(id pgtype.UUID) (<-chan []byte, func()) {
	return s.hub.Subscribe(gameTopic(id))
}

func scoresTopic(competition string) string { return "scores:" + competition }
func gameTopic(id pgtype.UUID) string       { return "game:" + id.String() }

// Run polls the games in play until ctx ends, for every sport someone here
// plays. Sports are polled side by side; the feed client spaces out the
// requests to each host.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			played, err := db.New(s.pool).ListLeagueCompetitions(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.log.Error("list league competitions", "err", err)
				}
				continue
			}
			var wait sync.WaitGroup
			for _, key := range played {
				wait.Go(func() { s.Poll(ctx, key) })
			}
			wait.Wait()
		}
	}
}

// Poll refreshes one sport's games in play now and pushes what changed.
func (s *Service) Poll(ctx context.Context, key string) {
	c, ok := s.registry.Get(key)
	if !ok {
		return
	}
	update, err := s.syncer.PollLive(ctx, c.Key, c.Games)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("live poll failed", "competition", c.Key, "err", err)
		}
		return
	}
	// A refreshed box score changes fantasy points even when the score has
	// not moved, so either kind of change is announced on the scoreboard.
	if update.Scoreboard || len(update.Games) > 0 {
		s.Announce(ctx, c.Key)
	}
	for _, id := range update.Games {
		if !s.hub.Listening(gameTopic(id)) {
			continue
		}
		if game, err := s.Game(ctx, id); err == nil {
			s.publish(gameTopic(id), game)
		}
	}
}

// Announce pushes today's scoreboard for a sport to everyone watching it.
func (s *Service) Announce(ctx context.Context, competition string) {
	if !s.hub.Listening(scoresTopic(competition)) {
		return
	}
	board, err := s.Scoreboard(ctx, competition, sportsday.Today())
	if err != nil {
		s.log.Error("announce scores", "competition", competition, "err", err)
		return
	}
	s.publish(scoresTopic(competition), board)
}

func (s *Service) publish(topic string, message any) {
	raw, _ := json.Marshal(message)
	s.hub.Publish(topic, raw)
}
