package web

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/draft"
)

func (s *Server) listDrafts(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	drafts, err := s.Queries.ListDrafts(r.Context(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, drafts)
}

func (s *Server) getDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	state, err := s.Drafts.State(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// watchDraft is the draft room's live connection: the full state first,
// then every change as it happens. Clients only listen; picks go through
// the ordinary HTTP endpoints.
func (s *Server) watchDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Subscribe before reading the state so no change falls in between.
	messages, leave := s.Drafts.Watch(id)
	defer leave()
	snapshot, err := s.Drafts.Snapshot(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.stream(w, r, snapshot, messages)
}

func (s *Server) makePick(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		PlayerID pgtype.UUID `json:"player_id"`
		List     string      `json:"list"`
		PickID   pgtype.UUID `json:"pick_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.Drafts.Pick(r.Context(), draft.PickRequest{
		DraftID: id, Actor: me, PlayerID: body.PlayerID, List: body.List, PickID: body.PickID,
	})
	s.done(w, r, err)
}

// passPick gives up the pick on the clock in a rookie draft.
func (s *Server) passPick(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Drafts.Pass(r.Context(), id, me))
}

func (s *Server) getQueue(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	queue, err := s.Drafts.Queue(r.Context(), id, me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, queue)
}

func (s *Server) setQueue(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		PlayerIDs []pgtype.UUID `json:"player_ids"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Drafts.SetQueue(r.Context(), id, me.ID, body.PlayerIDs))
}

// ---- commissioner ----

func (s *Server) createDraft(w http.ResponseWriter, r *http.Request) {
	var body draft.NewDraft
	if !readJSON(w, r, &body) {
		return
	}
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	created, err := s.Drafts.Create(r.Context(), d.ID, body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) setDraftPicks(w http.ResponseWriter, r *http.Request) {
	var slots []draft.Slot
	if !readJSON(w, r, &slots) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Drafts.SetPicks(r.Context(), id, slots))
}

func (s *Server) setDraftClock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PickClockSeconds int `json:"pick_clock_seconds"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Drafts.SetClock(r.Context(), id, body.PickClockSeconds))
}

func (s *Server) deleteDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Drafts.Delete(r.Context(), id))
}

// controlDraft starts, pauses, resumes, undoes or finishes a draft.
func (s *Server) controlDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actions := map[string]func(context.Context, pgtype.UUID) error{
		"start":  s.Drafts.Start,
		"pause":  s.Drafts.Pause,
		"resume": s.Drafts.Resume,
		"undo":   s.Drafts.Undo,
		"finish": s.Drafts.Finish,
	}
	action, ok := actions[r.PathValue("action")]
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	s.done(w, r, action(r.Context(), id))
}
