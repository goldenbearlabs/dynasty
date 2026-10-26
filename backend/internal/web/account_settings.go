// Exposes self-service account security and deletion to browser cookies and mobile bearer sessions.
package web

import (
	"net/http"
)

func (s *Server) changeAccountPassword(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to manage your account.")
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	s.done(w, r, s.Auth.ChangePassword(r.Context(), account.User.ID, body.CurrentPassword, body.NewPassword))
}

func (s *Server) beginAccountEmailChange(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to manage your account.")
		return
	}
	if s.SendAccountEmail == nil {
		writeError(w, http.StatusServiceUnavailable, "Account email is not configured yet.")
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewEmail        string `json:"new_email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	address, code, err := s.Auth.BeginEmailChange(r.Context(), account.User.ID, body.CurrentPassword, body.NewEmail)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.SendAccountEmail(r.Context(), address, "Confirm your Mosaic Fantasy email", "Enter this code in the Mosaic Fantasy app to confirm your new email:\n\n"+code+"\n\nIt expires in 15 minutes. If you did not request this change, ignore this email."); err != nil {
		s.Log.Warn("email change delivery failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "We could not send the code. Please try again shortly.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) confirmAccountEmailChange(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to manage your account.")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Auth.ConfirmEmailChange(r.Context(), account.User.ID, body.Code); err != nil {
		s.fail(w, r, err)
		return
	}
	if s.SendAccountEmail != nil {
		if err := s.SendAccountEmail(r.Context(), account.User.Email, "Your Mosaic Fantasy email changed", "Your account email was changed. If this was not you, contact Mosaic Fantasy support."); err != nil {
			s.Log.Warn("old address notice failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) accountDeletionOptions(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to manage your account.")
		return
	}
	options, err := s.Auth.DeletionOptions(r.Context(), account.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to manage your account.")
		return
	}
	var body struct {
		Password              string            `json:"password"`
		CommissionerTransfers map[string]string `json:"commissioner_transfers"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Auth.DeleteAccount(r.Context(), account.User.ID, body.Password, body.CommissionerTransfers); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
