package web

import (
	"crossover/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Server) getWatchlist(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	entries, err := s.Queries.Watchlist(r.Context(), me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
func (s *Server) addWatchlistPlayer(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "player_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err = s.Queries.GetPlayer(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Queries.AddWatchlistPlayer(r.Context(), me.ID, id))
}
func (s *Server) removeWatchlistPlayer(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "player_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Queries.RemoveWatchlistPlayer(r.Context(), me.ID, id))
}
func (s *Server) editWatchlistPlayer(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "player_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var body struct {
		Category string `json:"category"`
		Notes    string `json:"notes"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	category, err := identityName(body.Category, false, 60)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	notes := strings.TrimSpace(body.Notes)
	if utf8.RuneCountInString(notes) > 2000 {
		writeError(w, 422, "Notes must be 2,000 characters or fewer.")
		return
	}
	ok, err := s.Queries.EditWatchlistPlayer(r.Context(), me.ID, id, category, notes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		writeError(w, 404, "Player is not on your watchlist.")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) orderWatchlist(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		PlayerIDs []pgtype.UUID `json:"player_ids"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.PlayerIDs == nil {
		writeError(w, 400, "Provide player_ids.")
		return
	}
	ok, err := s.Queries.OrderWatchlist(r.Context(), me.ID, body.PlayerIDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		writeError(w, 409, "Your watchlist changed. Reload it before reordering.")
		return
	}
	w.WriteHeader(204)
}
