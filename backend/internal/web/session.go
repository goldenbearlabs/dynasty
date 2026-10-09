package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"crossover/internal/auth"
	"crossover/internal/db"
)

// The sign-in token is a JWT kept in a cookie the page's scripts cannot
// read. The browser sends it on every request, so a manager stays signed in
// until it expires, and it is renewed whenever they come back.
const sessionCookie = "session"

// session returns who is signed in, if anyone.
func (s *Server) session(r *http.Request) (auth.Session, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return auth.Session{}, false
	}
	session, err := s.Auth.Verify(r.Context(), cookie.Value)
	return session, err == nil
}

// me returns the signed-in franchise, if any.
func (s *Server) me(r *http.Request) (db.Franchise, bool) {
	session, ok := s.session(r)
	return session.Franchise, ok
}

// member requires a signed-in manager.
func (s *Server) member(next func(http.ResponseWriter, *http.Request, db.Franchise)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me, ok := s.me(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Sign in to do that.")
			return
		}
		next(w, r, me)
	}
}

// commissioner requires a signed-in commissioner, or the ADMIN_TOKEN bearer
// token so scripts can trigger syncs.
func (s *Server) commissioner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		byToken := s.AdminToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.AdminToken)) == 1
		if me, ok := s.me(r); !byToken && !(ok && me.IsCommissioner) {
			writeError(w, http.StatusForbidden, "Only a commissioner can do that.")
			return
		}
		next(w, r)
	}
}

// sessionView is the signed-in manager as the frontend sees them.
type sessionView struct {
	db.Franchise
	Email string `json:"email"`
}

// getSession reports who is signed in, and renews a token past half its life.
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	session, ok := s.session(r)
	if !ok {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if time.Until(session.Expires) < auth.TokenLifetime/2 {
		s.signIn(w, r, session.User)
	}
	writeJSON(w, http.StatusOK, sessionView{Franchise: session.Franchise, Email: session.User.Email})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	user, err := s.Auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signIn(w, r, user)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// getInvite says which franchise an invite link is for, so the page can
// greet the manager before they choose a password.
func (s *Server) getInvite(w http.ResponseWriter, r *http.Request) {
	franchise, err := s.Queries.GetFranchiseByInvite(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "That invite link is not valid. Ask your commissioner for a new one.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"franchise": franchise, "reset": franchise.UserID.Valid})
}

// claimInvite creates the manager's account from an invite link and signs
// them in.
func (s *Server) claimInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	user, err := s.Auth.Claim(r.Context(), r.PathValue("token"), body.Email, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signIn(w, r, user)
	w.WriteHeader(http.StatusNoContent)
}

// signIn stores a fresh token for the user in the session cookie.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request, user db.User) {
	token, expires := s.Auth.Token(user)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}
