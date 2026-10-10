package web

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/roster"
)

// changeRoster adds, drops or moves a player on the signed-in franchise's
// roster, or sets its whole reserve list. A commissioner may act for another franchise and may force a
// change past the free agency rules and roster limits.
func (s *Server) changeRoster(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		ReplacementID pgtype.UUID   `json:"replacement_id"`
		PlayerID      pgtype.UUID   `json:"player_id"`
		List          string        `json:"list"`
		Reserve       []pgtype.UUID `json:"reserve"`      // for "set": the whole reserve list
		FranchiseID   pgtype.UUID   `json:"franchise_id"` // commissioner only
		Force         bool          `json:"force"`        // commissioner only
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx := r.Context()

	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	league, err := s.Queries.GetLeague(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	franchise := me
	if body.FranchiseID.Valid && body.FranchiseID != me.ID {
		if !me.IsCommissioner {
			writeError(w, http.StatusForbidden, "You can only change your own roster.")
			return
		}
		if franchise, err = s.Queries.GetFranchise(ctx, body.FranchiseID); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if league.DynastyID != franchise.DynastyID {
		s.fail(w, r, problem.New("That franchise is not in this league."))
		return
	}

	change := roster.Change{
		League:    league,
		Franchise: franchise,
		PlayerID:  body.PlayerID,
		List:      body.List,
	}
	if body.Force && me.IsCommissioner {
		change.Source = roster.Commissioner
	}
	switch r.PathValue("action") {
	case "add":
		err = s.Roster.Add(ctx, change)
	case "drop":
		err = s.Roster.Drop(ctx, change)
	case "injury-swap":
		err = s.Roster.InjurySwap(ctx, change, body.ReplacementID)
	case "move":
		err = s.Roster.Move(ctx, change)
	case "set":
		err = s.Roster.Set(ctx, change, body.Reserve)
	default:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listActivity(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	activity, err := s.Queries.ListActivity(r.Context(), db.ListActivityParams{DynastyID: d.ID, PageSize: 50})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, activity)
}
