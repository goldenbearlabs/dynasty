// Handles self-service account registration and email verification before league membership.

package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) sendVerification(ctx context.Context, email, token string) error {
	link := strings.TrimRight(s.PublicURL, "/") + "/verify?token=" + url.QueryEscape(token)
	return s.SendAccountEmail(ctx, email, "Verify your Mosaic Fantasy account", "Welcome to Mosaic Fantasy. Verify your email by opening this link:\n\n"+link+"\n\nThis link expires in 24 hours. If you did not register, you can ignore this email.")
}

func (s *Server) registerAccount(w http.ResponseWriter, r *http.Request) {
	if s.SendAccountEmail == nil || s.PublicURL == "" {
		writeError(w, http.StatusServiceUnavailable, "Account registration is not configured yet.")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	token, err := s.Auth.Register(r.Context(), body.Email, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.sendVerification(r.Context(), strings.ToLower(strings.TrimSpace(body.Email)), token); err != nil {
		s.Log.Warn("verification email failed", "err", err)
		writeJSON(w, http.StatusCreated, map[string]any{"email_sent": false})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"email_sent": true})
}

func (s *Server) verifyAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Auth.VerifyRegistration(r.Context(), body.Token); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	if s.SendAccountEmail == nil || s.PublicURL == "" {
		writeError(w, http.StatusServiceUnavailable, "Account email is not configured yet.")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	email, token, err := s.Auth.ResendVerification(r.Context(), body.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if token != "" {
		if err := s.sendVerification(r.Context(), email, token); err != nil {
			s.Log.Warn("verification email failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	if s.SendAccountEmail == nil || s.PublicURL == "" {
		writeError(w, http.StatusServiceUnavailable, "Account email is not configured yet.")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	email, token, err := s.Auth.RequestPasswordReset(r.Context(), body.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if token != "" {
		link := strings.TrimRight(s.PublicURL, "/") + "/reset-password?token=" + url.QueryEscape(token)
		if err := s.SendAccountEmail(r.Context(), email, "Reset your Mosaic Fantasy password", "Open this link to choose a new password:\n\n"+link+"\n\nThis link expires in one hour. If you did not ask for a reset, you can ignore this email."); err != nil {
			s.Log.Warn("password reset email failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) completePasswordReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Auth.CompletePasswordReset(r.Context(), body.Token, body.Password); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) acceptExistingInvite(w http.ResponseWriter, r *http.Request) {
	session, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to claim this invite.")
		return
	}
	if err := s.Auth.AcceptInvite(r.Context(), session.User, r.PathValue("token")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
