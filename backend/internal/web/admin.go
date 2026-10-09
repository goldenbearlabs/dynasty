package web

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/players"
	"crossover/internal/scoring"
)

// startSync kicks off a feed sync and returns at once; progress is visible
// through the ingest runs.
func (s *Server) startSync(w http.ResponseWriter, r *http.Request) {
	c, ok := s.Registry.Get(r.PathValue("competition"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown competition")
		return
	}
	switch job := r.PathValue("job"); {
	case job == "rosters":
		go s.Syncer.SyncRosters(s.Background, c.Key, c.Source)
	case job == "prospects" && c.Prospects != nil:
		go s.Syncer.SyncProspects(s.Background, c.Key, c.Prospects)
	case job == "seasons":
		go s.Syncer.SyncSeasons(s.Background, c.Key, c.Seasons, s.SeasonBackfill)
	case job == "games":
		from, to := scoring.Window()
		go s.Syncer.SyncGames(s.Background, c.Key, c.Games, from, to)
	default:
		writeError(w, http.StatusNotFound, "no such sync for this competition")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (s *Server) listIngestRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.Queries.ListIngestRuns(r.Context(), 50)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) addPlayers(w http.ResponseWriter, r *http.Request) {
	var list []players.NewPlayer
	if !readJSON(w, r, &list) {
		return
	}
	if err := s.Players.Add(r.Context(), list); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"added": len(list)})
}

func (s *Server) listMergeSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := s.Queries.ListMergeSuggestions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (s *Server) mergePlayers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		KeepID      pgtype.UUID `json:"keep_id"`
		DuplicateID pgtype.UUID `json:"duplicate_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Players.Merge(r.Context(), body.KeepID, body.DuplicateID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
