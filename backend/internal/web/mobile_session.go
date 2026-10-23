// Exposes mobile sign-in and invite claiming with revocable bearer tokens.

package web

import (
	"net/http"
	"strings"
	"time"

	"crossover/internal/auth"
	"crossover/internal/db"
)

type mobileSessionView struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	Session   sessionView `json:"session"`
}

func mobileBearer(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	return token, ok && strings.EqualFold(scheme, "Bearer") && token != "" && !strings.ContainsAny(token, " \t\r\n")
}

func (s *Server) mobileReply(w http.ResponseWriter, r *http.Request, user db.User) {
	token, expires, err := s.Auth.IssueMobile(r.Context(), user)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	session, err := s.Auth.VerifyMobile(r.Context(), token)
	if err != nil {
		_ = s.Auth.RevokeMobile(r.Context(), token)
		s.fail(w, r, err)
		return
	}
	view, err := s.viewSession(r, session)
	if err != nil {
		_ = s.Auth.RevokeMobile(r.Context(), token)
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mobileSessionView{
		Token: token, ExpiresAt: expires,
		Session: view,
	})
}

func (s *Server) mobileLogin(w http.ResponseWriter, r *http.Request) {
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
	s.mobileReply(w, r, user)
}

func (s *Server) mobileClaimInvite(w http.ResponseWriter, r *http.Request) {
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
	s.mobileReply(w, r, user)
}

func (s *Server) mobileAcceptInvite(w http.ResponseWriter, r *http.Request) {
	token, ok := mobileBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to claim this invite.")
		return
	}
	session, err := s.Auth.VerifyMobile(r.Context(), token)
	if err == auth.ErrNoSession {
		writeError(w, http.StatusUnauthorized, "Your session has ended. Please sign in again.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Auth.AcceptInvite(r.Context(), session.User, r.PathValue("token")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.mobileRefresh(w, r)
}

func (s *Server) mobileSession(w http.ResponseWriter, r *http.Request) {
	token, ok := mobileBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to do that.")
		return
	}
	session, err := s.Auth.VerifyMobile(r.Context(), token)
	if err == auth.ErrNoSession {
		writeError(w, http.StatusUnauthorized, "Your session has ended. Please sign in again.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view, err := s.viewSession(r, session)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) mobileRefresh(w http.ResponseWriter, r *http.Request) {
	token, ok := mobileBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to do that.")
		return
	}
	fresh, expires, session, err := s.Auth.RefreshMobile(r.Context(), token)
	if err == auth.ErrNoSession {
		writeError(w, http.StatusUnauthorized, "Your session has ended. Please sign in again.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	view, err := s.viewSession(r, session)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mobileSessionView{
		Token: fresh, ExpiresAt: expires,
		Session: view,
	})
}

func (s *Server) mobileLogout(w http.ResponseWriter, r *http.Request) {
	token, ok := mobileBearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to do that.")
		return
	}
	if err := s.Auth.RevokeMobile(r.Context(), token); err == auth.ErrNoSession {
		writeError(w, http.StatusUnauthorized, "Your session has ended. Please sign in again.")
		return
	} else if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
