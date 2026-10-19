package web

import (
	"crossover/internal/problem"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
)

func (s *Server) getRosterStats(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	league, err := s.Queries.GetLeague(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var franchise pgtype.UUID
	if err := franchise.Scan(r.URL.Query().Get("franchise")); err != nil || !franchise.Valid {
		s.fail(w, r, problem.New("Choose a franchise."))
		return
	}
	owner, err := s.Queries.GetFranchise(r.Context(), franchise)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if owner.DynastyID != league.DynastyID {
		s.fail(w, r, problem.New("That franchise is not in this league."))
		return
	}
	view, err := s.Players.RosterPerformance(r.Context(), league, franchise, r.URL.Query().Get("window"), r.URL.Query().Get("season"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
