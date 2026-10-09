package web

import (
	"net/http"

	"crossover/internal/db"
	"crossover/internal/waiver"
)

// getWaivers returns a league's waivers: who is on them, the waiver order,
// and the signed-in manager's own claims.
func (s *Server) getWaivers(w http.ResponseWriter, r *http.Request) {
	league, ok := s.pathLeague(w, r)
	if !ok {
		return
	}
	var viewer *db.Franchise
	if me, ok := s.me(r); ok {
		viewer = &me
	}
	view, err := s.Waivers.View(r.Context(), league, viewer)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) claimWaiver(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var claim waiver.Claim
	if !readJSON(w, r, &claim) {
		return
	}
	league, ok := s.pathLeague(w, r)
	if !ok {
		return
	}
	s.done(w, r, s.Waivers.Claim(r.Context(), league, me, claim))
}

func (s *Server) cancelWaiverClaim(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Waivers.Cancel(r.Context(), me, id))
}

// pathLeague loads the league named in the path, answering the request
// itself when there is none.
func (s *Server) pathLeague(w http.ResponseWriter, r *http.Request) (db.League, bool) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return db.League{}, false
	}
	league, err := s.Queries.GetLeague(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return db.League{}, false
	}
	return league, true
}
