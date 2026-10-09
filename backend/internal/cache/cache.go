// Package cache keeps reusable public JSON in Redis. Database generations
// make old entries unreachable immediately after a relevant write commits.
package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const Research = "research"
const Public = "public"
const maxEntryBytes = 8 << 20

// Backend is the small Redis surface needed by the cache; tests can simulate
// outages and eviction without changing application behavior.
type Backend interface {
	Get(context.Context, string) *redis.StringCmd
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
}
type Store struct {
	// RevisionAge is how long a generation read from Postgres is trusted
	// before it is read again. Zero asks Postgres on every request. Above
	// zero, a write made outside this server (a feed sync, direct SQL) can
	// go unseen for that long; the server's own writes call Forget.
	RevisionAge time.Duration

	mu     sync.Mutex
	recent map[string]generation // by scope

	redis            Backend
	revision         func(context.Context, string) (string, error)
	flights          singleflight.Group
	unavailableUntil atomic.Int64
	log              *slog.Logger
}

type generation struct {
	revision string
	read     time.Time
}

// Revision is the current generation of a scope's data: it changes whenever
// a write that scope depends on commits.
func (s *Store) Revision(ctx context.Context, scope string) (string, error) {
	if s.RevisionAge > 0 {
		s.mu.Lock()
		last, known := s.recent[scope]
		s.mu.Unlock()
		if known && time.Since(last.read) < s.RevisionAge {
			return last.revision, nil
		}
	}
	return s.read(ctx, scope)
}

// read asks Postgres, and notes the answer for Revision.
func (s *Store) read(ctx context.Context, scope string) (string, error) {
	now := time.Now()
	revision, err := s.revision(ctx, scope)
	if err == nil && s.RevisionAge > 0 {
		s.mu.Lock()
		if s.recent == nil {
			s.recent = map[string]generation{}
		}
		s.recent[scope] = generation{revision, now}
		s.mu.Unlock()
	}
	return revision, err
}

// Forget makes the next read of every scope ask Postgres. It is called
// after anything this server changes, so whoever made a change sees it.
func (s *Store) Forget() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.recent)
	s.mu.Unlock()
}

func New(client Backend, pool *pgxpool.Pool, log *slog.Logger) *Store {
	return &Store{redis: client, log: log, revision: func(ctx context.Context, scope string) (string, error) {
		var revision string
		// Scope is chosen by the application, never supplied by the request.
		column := "public"
		if scope == Research {
			column = "research"
		}
		err := pool.QueryRow(ctx, "select namespace::text || ':' || "+column+"::text from cache_revision where singleton").Scan(&revision)
		return revision, err
	}}
}

func Connect(url string) (*redis.Client, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	options.DialTimeout = 250 * time.Millisecond
	options.ReadTimeout = 250 * time.Millisecond
	options.WriteTimeout = 250 * time.Millisecond
	options.ContextTimeoutEnabled = true
	options.MaxRetries = -1
	options.PoolSize = 4
	options.MinIdleConns = 0
	return redis.NewClient(options), nil
}

func (s *Store) failed(err error) {
	if err == nil || err == redis.Nil {
		return
	}
	// Stop retrying an unavailable Redis on every request. Postgres remains the
	// authority; no old local fallback is used during an outage.
	now := time.Now()
	if s.unavailableUntil.Swap(now.Add(5*time.Second).UnixNano()) <= now.UnixNano() && s.log != nil {
		s.log.Warn("Redis cache unavailable; serving fresh data", "err", err)
	}
}
func (s *Store) get(ctx context.Context, key string) ([]byte, bool) {
	if time.Now().UnixNano() < s.unavailableUntil.Load() {
		return nil, false
	}
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	value, err := s.redis.Get(short, key).Bytes()
	if err != nil {
		s.failed(err)
		return nil, false
	}
	return value, true
}

// Remember coalesces identical cold reads. Cache population outlives any one
// waiting browser, but is bounded to one minute. Errors are never cached.
// A second revision check prevents a result calculated across a concurrent
// write from being stored. Expiration and Redis LRU bound unused old entries.
func (s *Store) Remember(ctx context.Context, scope, identity string, ttl time.Duration, load func(context.Context) ([]byte, error)) ([]byte, error) {
	if s == nil {
		return load(ctx)
	}
	revision, err := s.Revision(ctx, scope)
	if err != nil {
		return load(ctx)
	} // unavailable revision means bypass, never stale
	key := fmt.Sprintf("crossover:cache:v1:%s:%s:%x", scope, revision, sha256.Sum256([]byte(identity)))
	if value, ok := s.get(ctx, key); ok {
		return value, nil
	}
	result := s.flights.DoChan(key, func() (any, error) {
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if value, ok := s.get(shared, key); ok {
			return value, nil
		}
		value, err := load(shared)
		if err != nil {
			return nil, err
		}
		after, err := s.read(shared, scope)
		if err == nil && after == revision && len(value) <= maxEntryBytes && time.Now().UnixNano() >= s.unavailableUntil.Load() {
			short, cancel := context.WithTimeout(shared, 300*time.Millisecond)
			s.failed(s.redis.Set(short, key, value, ttl).Err())
			cancel()
		}
		return value, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return value.Val.([]byte), nil
	}
}
