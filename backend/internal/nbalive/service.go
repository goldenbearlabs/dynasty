package nbalive

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type loaded struct {
	snapshot Snapshot
	records  map[string]Record
}
type Service struct {
	pool    *pgxpool.Pool
	path    string
	log     *slog.Logger
	current atomic.Pointer[loaded]
}

func NewService(pool *pgxpool.Pool, path string, log *slog.Logger) *Service {
	return &Service{pool: pool, path: path, log: log}
}
func (s *Service) Reload(ctx context.Context) error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var snapshot Snapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	if err = snapshot.Validate(time.Now()); err != nil {
		return err
	}
	gameIDs, playerIDs := []string{}, []string{}
	games, players := map[string]string{}, map[string]string{}
	for _, r := range snapshot.Records {
		gameIDs = append(gameIDs, r.Target.GameID)
		playerIDs = append(playerIDs, r.Target.PlayerID)
	}
	rows, err := s.pool.Query(ctx, "select provider_id,id from games where competition='nba' and provider_id=any($1::text[])", gameIDs)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ext string
		var id pgtype.UUID
		if err = rows.Scan(&ext, &id); err != nil {
			rows.Close()
			return err
		}
		games[ext] = id.String()
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = s.pool.Query(ctx, "select e.provider_id,e.player_id from player_external_ids e join players p on p.id=e.player_id where e.provider='espn_basketball' and p.competition='nba' and e.provider_id=any($1::text[])", playerIDs)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ext string
		var id pgtype.UUID
		if err = rows.Scan(&ext, &id); err != nil {
			rows.Close()
			return err
		}
		players[ext] = id.String()
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	next := &loaded{snapshot: snapshot, records: map[string]Record{}}
	for _, r := range snapshot.Records {
		g, p := games[r.Target.GameID], players[r.Target.PlayerID]
		if g != "" && p != "" {
			next.records[Key(g, p)] = r
		}
	}
	prev := s.current.Load()
	changed := prev == nil || !prev.snapshot.GeneratedAt.Equal(snapshot.GeneratedAt) || len(prev.records) != len(next.records)
	if changed {
		// Invalidate response-cache generations so cached lineup totals cannot retain
		// the previous forecasts. Roll back the in-memory swap if this write fails.
		s.current.Store(next)
		if _, err = s.pool.Exec(ctx, "update cache_revision set public=public+1 where singleton"); err != nil {
			s.current.Store(prev)
			return fmt.Errorf("invalidate NBA forecasts: %w", err)
		}
		s.log.Info("NBA model forecasts loaded", "generated_at", snapshot.GeneratedAt, "history_through", snapshot.HistoryThrough, "forecasts", len(snapshot.Records), "mapped_forecasts", len(next.records), "model_sha256", snapshot.ModelSHA256)
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		if err := s.Reload(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("NBA model reload; retaining previous forecasts", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Service) Points(player, game pgtype.UUID, scoring map[string]float64, startsAt time.Time, scheduled bool) (float64, bool) {
	if s == nil {
		return 0, false
	}
	loaded := s.current.Load()
	if loaded == nil {
		return 0, false
	}
	r, ok := loaded.records[Key(game.String(), player.String())]
	if !ok || r.Target.StartsAt.Sub(startsAt).Abs() > time.Minute {
		return 0, false
	}
	return r.Points(scoring, time.Now(), scheduled)
}
func (s *Service) Status() map[string]any {
	status := map[string]any{"enabled": s != nil, "available": false}
	if s == nil {
		return status
	}
	if l := s.current.Load(); l != nil {
		status["available"] = time.Since(l.snapshot.GeneratedAt) <= MaxAge && (len(l.snapshot.Records) == 0 || len(l.records) > 0)
		status["generated_at"] = l.snapshot.GeneratedAt
		status["history_through"] = l.snapshot.HistoryThrough
		status["model_sha256"] = l.snapshot.ModelSHA256
		status["forecasts"] = len(l.snapshot.Records)
		status["mapped_forecasts"] = len(l.records)
	}
	return status
}
