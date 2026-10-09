package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"crossover/internal/cache"
	"crossover/internal/dbtest"
)

func TestCachedPublicRouteReuseAndFreshness(t *testing.T) {
	pool := dbtest.Open(t)
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	client, err := cache.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	server := &Server{Cache: cache.New(client, pool, nil)}
	count := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		count++
		writeJSON(w, http.StatusOK, map[string]int{"call": count})
	}
	handler := server.cached(cache.Research, time.Minute, next)
	request := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return w
	}
	path := "/api/research?cache_test=" + t.Name() + "&sort=points"
	first := request(path)
	second := request("/api/research?sort=points&cache_test=" + t.Name())
	if count != 1 || first.Body.String() != second.Body.String() {
		t.Fatal("equivalent query strings did not share cache")
	}
	if second.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("browser cache could bypass invalidation")
	}
	request(path + "&page=2")
	if count != 2 {
		t.Fatal("different page used wrong cache")
	}
	if _, err := pool.Exec(context.Background(), "update games set status=status where false"); err != nil {
		t.Fatal(err)
	}
	request(path)
	if count != 2 {
		t.Fatal("live game update invalidated season research")
	}
	if _, err := pool.Exec(context.Background(), "update player_seasons set games=games where false"); err != nil {
		t.Fatal(err)
	}
	request(path)
	if count != 3 {
		t.Fatal("season import did not invalidate route")
	}
	// HTTP errors and responses setting session cookies must never be cached.
	for _, kind := range []string{"error", "cookie"} {
		calls := 0
		handler = server.cached(cache.Public, time.Minute, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if kind == "error" {
				writeError(w, 422, "invalid")
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "private"})
			writeJSON(w, 200, map[string]int{"call": calls})
		})
		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest("GET", "/test/"+kind, nil))
			if kind == "error" && w.Code != 422 {
				t.Fatal("error response lost")
			}
			if kind == "cookie" && w.Header().Get("Set-Cookie") == "" {
				t.Fatal("cookie response lost")
			}
		}
		if calls != 2 {
			t.Fatal("error/cookie response was cached")
		}
	}
}
