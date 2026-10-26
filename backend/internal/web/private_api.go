// Protects league data at the API boundary, including cached reads and live feeds.

package web

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	_, err := s.Queries.GetDynasty(r.Context())
	if err != nil && err != pgx.ErrNoRows {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"configured": err == nil})
}

func publicAPI(r *http.Request) bool {
	p := r.URL.Path
	switch p {
	case "/api/bootstrap", "/api/competitions", "/api/session", "/api/mobile/session",
		"/api/my-leagues",
		"/api/auth/login", "/api/auth/logout", "/api/mobile/auth/login",
		"/api/mobile/auth/logout", "/api/mobile/auth/refresh",
		"/api/accounts", "/api/accounts/verify", "/api/accounts/verification":
		return true
	case "/api/accounts/password-reset", "/api/accounts/password-reset/confirm":
		return true
	case "/api/account/password", "/api/account/email-change", "/api/account/email-change/confirm", "/api/account/deletion-options", "/api/account":
		return true // each account handler authenticates a cookie or mobile bearer itself
	case "/api/dynasty":
		return r.Method == http.MethodPost // the handler allows this only on a fresh installation
	default:
		return strings.HasPrefix(p, "/api/invites/") || strings.HasPrefix(p, "/api/mobile/invites/") || strings.HasPrefix(p, "/api/my-leagues/")
	}
}

func (s *Server) privateAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || publicAPI(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/admin/") && s.AdminToken != "" {
			token, ok := mobileBearer(r)
			if ok && subtle.ConstantTimeCompare([]byte(token), []byte(s.AdminToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}
		if _, ok := s.me(r); !ok {
			writeError(w, http.StatusUnauthorized, "Join a league to see its details.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
