package web

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/draft"
)

// Rankings are private: every route answers only for the signed-in manager's own.

func (s *Server) listRankings(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	rankings, err := s.Drafts.Rankings(r.Context(), me)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if rankings == nil {
		rankings = []db.ListRankingsRow{}
	}
	writeJSON(w, http.StatusOK, rankings)
}

func (s *Server) getRanking(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ranking, err := s.Drafts.Ranking(r.Context(), me, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ranking)
}

// saveRanking creates a ranking, or changes the one named in the path.
func (s *Server) saveRanking(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body draft.RankingChange
	if !readJSON(w, r, &body) {
		return
	}
	var id pgtype.UUID
	status := http.StatusCreated
	if r.PathValue("id") != "" {
		var err error
		if id, err = pathID(r, "id"); err != nil {
			s.fail(w, r, err)
			return
		}
		status = http.StatusOK
	}
	ranking, err := s.Drafts.SaveRanking(r.Context(), me, id, body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, ranking)
}

func (s *Server) deleteRanking(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Drafts.DeleteRanking(r.Context(), me, id))
}

// importRanking loads one of the manager's rankings into their queue for a draft.
func (s *Server) importRanking(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		RankingID pgtype.UUID `json:"ranking_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	added, err := s.Drafts.ImportRanking(r.Context(), id, me, body.RankingID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"added": added})
}
