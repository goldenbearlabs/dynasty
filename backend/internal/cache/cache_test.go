package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"crossover/internal/dbtest"
	"github.com/redis/go-redis/v9"
)

type entry struct {
	value   []byte
	expires time.Time
}
type memoryBackend struct {
	mu      sync.Mutex
	entries map[string]entry
	broken  bool
}

func (m *memoryBackend) Get(ctx context.Context, key string) *redis.StringCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.broken {
		return redis.NewStringResult("", errors.New("offline"))
	}
	e, ok := m.entries[key]
	if !ok || time.Now().After(e.expires) {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(string(e.value), nil)
}
func (m *memoryBackend) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = entry{append([]byte(nil), value.([]byte)...), time.Now().Add(ttl)}
	return redis.NewStatusResult("OK", nil)
}
func newMemoryStore() (*Store, *memoryBackend, *atomic.Int64) {
	backend := &memoryBackend{entries: map[string]entry{}}
	revision := &atomic.Int64{}
	revision.Store(1)
	return &Store{redis: backend, revision: func(context.Context, string) (string, error) { return fmt.Sprint(revision.Load()), nil }}, backend, revision
}
func TestReuseKeysInvalidationExpiryAndErrors(t *testing.T) {
	store, _, revision := newMemoryStore()
	ctx := context.Background()
	calls := 0
	load := func(context.Context) ([]byte, error) { calls++; return []byte(fmt.Sprint(calls)), nil }
	get := func(scope, key string, ttl time.Duration) string {
		t.Helper()
		v, err := store.Remember(ctx, scope, key, ttl, load)
		if err != nil {
			t.Fatal(err)
		}
		return string(v)
	}
	if get(Research, "pool-a", time.Minute) != "1" || get(Research, "pool-a", time.Minute) != "1" {
		t.Fatal("did not reuse cache")
	}
	if get(Research, "pool-b", time.Minute) != "2" || get(Public, "pool-a", time.Minute) != "3" {
		t.Fatal("keys/scopes collided")
	}
	revision.Add(1)
	if get(Research, "pool-a", time.Minute) != "4" {
		t.Fatal("new revision reused stale cache")
	}
	get(Research, "short", time.Nanosecond)
	if get(Research, "short", time.Nanosecond) != "6" {
		t.Fatal("expired cache was reused")
	}
	failures := 0
	for i := 0; i < 2; i++ {
		_, err := store.Remember(ctx, Research, "error", time.Minute, func(context.Context) ([]byte, error) { failures++; return nil, errors.New("failed") })
		if err == nil {
			t.Fatal("error swallowed")
		}
	}
	if failures != 2 {
		t.Fatal("error cached")
	}
}
func TestConcurrentColdReadsAndCanceledWaiter(t *testing.T) {
	store, _, _ := newMemoryStore()
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	load := func(context.Context) ([]byte, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []byte("result"), nil
	}
	var wg sync.WaitGroup
	canceled, cancel := context.WithCancel(ctx)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := store.Remember(canceled, Research, "same", time.Minute, load)
		if err != context.Canceled {
			t.Errorf("canceled waiter: %v", err)
		}
	}()
	<-started
	cancel()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := store.Remember(ctx, Research, "same", time.Minute, load)
			if err != nil || string(v) != "result" {
				t.Errorf("shared result: %s %v", v, err)
			}
		}()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("stampede: %d loads", calls.Load())
	}
}
func TestWriteDuringCalculationIsNotCached(t *testing.T) {
	store, backend, revision := newMemoryStore()
	_, err := store.Remember(context.Background(), Research, "race", time.Minute, func(context.Context) ([]byte, error) { revision.Add(1); return []byte("old"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(backend.entries) != 0 {
		t.Fatal("a calculation spanning a write must not enter the cache")
	}
}
func TestRedisAndRevisionFailureServeFresh(t *testing.T) {
	store, backend, _ := newMemoryStore()
	backend.broken = true
	calls := 0
	load := func(context.Context) ([]byte, error) { calls++; return []byte("fresh"), nil }
	for i := 0; i < 2; i++ {
		v, err := store.Remember(context.Background(), Research, "offline", time.Minute, load)
		if err != nil || string(v) != "fresh" {
			t.Fatal("Redis outage broke route")
		}
	}
	if calls != 2 {
		t.Fatal("Redis outage used stale fallback")
	}
	store.revision = func(context.Context, string) (string, error) { return "", errors.New("revision unavailable") }
	v, err := store.Remember(context.Background(), Research, "offline", time.Minute, load)
	if err != nil || string(v) != "fresh" {
		t.Fatal("missing revision must bypass cache")
	}
}
func TestDatabaseGenerationsFollowCommittedChanges(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	store := New(&memoryBackend{entries: map[string]entry{}}, pool, nil)
	read := func(scope string) string {
		t.Helper()
		v, err := store.revision(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	initialResearch, initialPublic := read(Research), read(Public)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "update players set note=note where false"); err != nil {
		t.Fatal(err)
	}
	if read(Research) != initialResearch || read(Public) != initialPublic {
		t.Fatal("uncommitted changes invalidated reads")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if read(Research) != initialResearch {
		t.Fatal("rollback changed generation")
	}
	if _, err := pool.Exec(ctx, "update games set status=status where false"); err != nil {
		t.Fatal(err)
	}
	if read(Research) != initialResearch || read(Public) == initialPublic {
		t.Fatal("live changes must invalidate public only")
	}
	if _, err := pool.Exec(ctx, "update leagues set settings=settings where false"); err != nil {
		t.Fatal(err)
	}
	if read(Research) == initialResearch {
		t.Fatal("league rule changes did not invalidate research")
	}
	// Multiple statements in an import transaction only bump each scope once.
	var before int64
	pool.QueryRow(ctx, "select research from cache_revision").Scan(&before)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := tx.Exec(ctx, "update player_seasons set games=games where false"); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var after int64
	pool.QueryRow(ctx, "select research from cache_revision").Scan(&after)
	if after != before+1 {
		t.Fatalf("bulk transaction bumped %d times", after-before)
	}
}
