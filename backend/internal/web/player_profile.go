package web

import (
	"crossover/internal/problem"
	"net/http"
	"strconv"
)

func profilePage(r *http.Request) (int, error) {
	if r.URL.Query().Get("page") == "" {
		return 1, nil
	}
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 || p > 100000 {
		return 0, problem.New("page must be between 1 and 100000.")
	}
	return p, nil
}
func (s *Server) playerProfile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	result, err := s.Players.Profile(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) playerTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page, err := profilePage(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	result, err := s.Players.Timeline(r.Context(), id, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) playerGameLog(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page, err := profilePage(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	result, err := s.Players.GameLog(r.Context(), id, r.URL.Query().Get("competition"), page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
