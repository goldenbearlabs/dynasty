// Exposes account-owned leagues while keeping the original gameplay API isolated.

package web

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"crossover/internal/dynasty"
)

func (s *Server) listMyLeagues(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to see your leagues.")
		return
	}
	memberships, err := s.Dynasty.ListMemberships(r.Context(), account.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, memberships)
}

func (s *Server) createMyLeague(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to create a league.")
		return
	}
	verified, err := s.Auth.IsVerified(r.Context(), account.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !verified {
		writeError(w, http.StatusForbidden, "Verify your email before creating a league.")
		return
	}
	var input dynasty.PresetLeagueInput
	if !readJSON(w, r, &input) {
		return
	}
	id, err := s.Dynasty.CreatePresetLeague(r.Context(), account.User.ID, input)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getMyLeague(w http.ResponseWriter, r *http.Request) {
	account, ok := s.session(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in to see this league.")
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	membership, err := s.Dynasty.GetMembership(r.Context(), account.User.ID, id)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "League not found.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, membership)
}
