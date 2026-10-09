package web

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"
)

type cachedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *cachedResponse) Header() http.Header { return w.header }
func (w *cachedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *cachedResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}
func (w *cachedResponse) Error() string { return "response is not cacheable" }
func (w *cachedResponse) replay(to http.ResponseWriter) {
	for key, values := range w.header {
		to.Header()[key] = append([]string(nil), values...)
	}
	to.WriteHeader(w.status)
	to.Write(w.body.Bytes())
}

// Only explicitly selected public JSON routes use this wrapper. Session,
// admin, private trade/waiver/draft state, writes and live streams bypass it.
func (s *Server) cached(scope string, ttl time.Duration, next http.HandlerFunc) http.HandlerFunc {
	if s.Cache == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Browser/proxy caching would bypass the database generation check.
		w.Header().Set("Cache-Control", "no-store")
		identity := "route:" + r.URL.Path + "?" + r.URL.Query().Encode()
		body, err := s.Cache.Remember(r.Context(), scope, identity, ttl, func(ctx context.Context) ([]byte, error) {
			captured := &cachedResponse{header: make(http.Header)}
			next(captured, r.Clone(ctx))
			if captured.status != http.StatusOK || captured.header.Get("Set-Cookie") != "" || captured.header.Get("Content-Type") != "application/json" {
				return nil, captured
			}
			return captured.body.Bytes(), nil
		})
		if err != nil {
			var captured *cachedResponse
			if errors.As(err, &captured) {
				captured.replay(w)
			} else if r.Context().Err() == nil {
				s.fail(w, r, err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}
}
