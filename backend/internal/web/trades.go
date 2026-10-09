package web

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/trade"
)

// listTrades returns the trades the viewer may see.
func (s *Server) listTrades(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var viewer *db.Franchise
	if me, ok := s.me(r); ok {
		viewer = &me
	}
	trades, err := s.Trades.List(r.Context(), d.ID, viewer)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, trades)
}

func (s *Server) proposeTrade(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var offer trade.Offer
	if !readJSON(w, r, &offer) {
		return
	}
	created, err := s.Trades.Propose(r.Context(), me, offer)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// answerTrade is a party accepting, rejecting or cancelling an offer.
func (s *Server) answerTrade(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actions := map[string]func(context.Context, pgtype.UUID, db.Franchise) error{
		"accept": s.Trades.Accept,
		"reject": s.Trades.Reject,
		"cancel": s.Trades.Cancel,
	}
	action, ok := actions[r.PathValue("action")]
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	s.done(w, r, action(r.Context(), id, me))
}

// ruleOnTrade is the commissioner approving, vetoing or reversing a trade.
func (s *Server) ruleOnTrade(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	actions := map[string]func(context.Context, pgtype.UUID) error{
		"approve": s.Trades.Approve,
		"veto":    s.Trades.Veto,
		"reverse": s.Trades.Reverse,
	}
	action, ok := actions[r.PathValue("action")]
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	s.done(w, r, action(r.Context(), id))
}

// createFutureDrafts makes the coming years' seasonal drafts so their picks
// can be traded.
func (s *Server) createFutureDrafts(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	created, err := s.Drafts.CreateFuture(r.Context(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"created": created})
}
