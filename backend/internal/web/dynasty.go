package web

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"

	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/dynasty"
	"crossover/internal/problem"
	"crossover/internal/settings"
)

// dynastyView is everything the frontend needs to draw the league.
type dynastyView struct {
	db.Dynasty
	Leagues    []db.League    `json:"leagues"`
	Franchises []db.Franchise `json:"franchises"`
}

func (s *Server) loadDynasty(r *http.Request) (dynastyView, error) {
	ctx := r.Context()
	d, err := s.Queries.GetDynasty(ctx)
	if err != nil {
		return dynastyView{}, err
	}
	leagues, err := s.Queries.ListLeagues(ctx, d.ID)
	if err != nil {
		return dynastyView{}, err
	}
	// Leagues are shown in the registry's order.
	order := func(l db.League) int {
		return slices.IndexFunc(s.Registry, func(c competition.Competition) bool { return c.Key == l.Competition })
	}
	slices.SortFunc(leagues, func(a, b db.League) int { return order(a) - order(b) })

	franchises, err := s.Queries.ListFranchises(ctx, d.ID)
	return dynastyView{Dynasty: d, Leagues: leagues, Franchises: franchises}, err
}

func (s *Server) getDynasty(w http.ResponseWriter, r *http.Request) {
	view, err := s.loadDynasty(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// createDynasty is the first-run wizard's submit. It signs the creator in
// as the commissioner.
func (s *Server) createDynasty(w http.ResponseWriter, r *http.Request) {
	var setup dynasty.Setup
	if !readJSON(w, r, &setup) {
		return
	}
	commissioner, err := s.Dynasty.Create(r.Context(), setup)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signIn(w, r, commissioner)
	w.WriteHeader(http.StatusCreated)
}

// addLeague starts a league in one more sport for the existing dynasty.
func (s *Server) addLeague(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Competition string `json:"competition"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	league, err := s.Dynasty.AddLeague(r.Context(), d.ID, body.Competition)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, league)
}

func (s *Server) updateDynasty(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string           `json:"name"`
		Settings settings.Dynasty `json:"settings"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if body.Name == "" {
		s.fail(w, r, problem.New("The dynasty needs a name."))
		return
	}
	if err := body.Settings.Validate(); err != nil {
		s.fail(w, r, problem.Error(err.Error()))
		return
	}
	raw, _ := json.Marshal(body.Settings)
	if err := s.Queries.UpdateDynasty(r.Context(), db.UpdateDynastyParams{ID: d.ID, Name: body.Name, Settings: raw}); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateLeagueSettings(w http.ResponseWriter, r *http.Request) {
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
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	rules, err := settings.Parse[settings.League](raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
		return
	}
	if err := s.Dynasty.UpdateLeagueSettings(r.Context(), league, rules); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
