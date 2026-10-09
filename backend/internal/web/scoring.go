package web

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/lineup"
	"crossover/internal/problem"
	"crossover/internal/scoring"
	"crossover/internal/sportsday"
)

// leagueStandings is one league's table, labelled for the page.
type leagueStandings struct {
	LeagueID    pgtype.UUID `json:"league_id"`
	Competition string      `json:"competition"`
	scoring.Standings
}

// listStandings returns every league's table for its latest season.
func (s *Server) listStandings(w http.ResponseWriter, r *http.Request) {
	view, err := s.loadDynasty(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tables := []leagueStandings{}
	for _, league := range view.Leagues {
		standings, err := s.Scoring.Standings(r.Context(), league)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		tables = append(tables, leagueStandings{LeagueID: league.ID, Competition: league.Competition, Standings: standings})
	}
	writeJSON(w, http.StatusOK, tables)
}

func (s *Server) getOverall(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	years, err := s.Scoring.Overall(r.Context(), d)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, years)
}

func (s *Server) listSeasons(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	seasons, err := s.Queries.ListSeasons(r.Context(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, seasons)
}

func (s *Server) createSeason(w http.ResponseWriter, r *http.Request) {
	var body scoring.NewSeason
	if !readJSON(w, r, &body) {
		return
	}
	league, err := s.Queries.GetLeague(r.Context(), body.LeagueID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// How the season before finished, read before there is a new one.
	last, err := s.Scoring.Standings(r.Context(), league)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	season, err := s.Scoring.CreateSeason(r.Context(), body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A new season sets the waiver order: last season's standings, worst
	// first. The commissioner can rearrange it. A first season has nothing
	// to go on and keeps the order as it is.
	if last.Season != nil && len(last.Rows) > 0 {
		var order []pgtype.UUID
		for i := len(last.Rows) - 1; i >= 0; i-- {
			order = append(order, last.Rows[i].FranchiseID)
		}
		if err := s.Waivers.ResetOrder(r.Context(), league, order); err != nil {
			s.Log.Warn("set the waiver order for a new season", "league", league.Name, "err", err)
		}
	}
	writeJSON(w, http.StatusCreated, season)
}

func (s *Server) setSeasonDates(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StartsOn string `json:"starts_on"`
		EndsOn   string `json:"ends_on"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Scoring.SetSeasonDates(r.Context(), id, body.StartsOn, body.EndsOn))
}

// closeSeason names the champion and ends the season; reopenSeason undoes it.
func (s *Server) closeSeason(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChampionFranchiseID pgtype.UUID `json:"champion_franchise_id"` // optional: defaults to first place
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Scoring.CloseSeason(r.Context(), id, body.ChampionFranchiseID))
}

func (s *Server) reopenSeason(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Scoring.ReopenSeason(r.Context(), id))
}

// getLineup returns a franchise's lineup for a day: ?franchise=<id>&day=YYYY-MM-DD.
// The day defaults to today.
func (s *Server) getLineup(w http.ResponseWriter, r *http.Request) {
	league, err := s.leagueFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var franchiseID pgtype.UUID
	if franchiseID.Scan(r.URL.Query().Get("franchise")) != nil {
		s.fail(w, r, problem.New("Say which franchise's lineup."))
		return
	}
	day := sportsday.Today()
	if given := r.URL.Query().Get("day"); given != "" {
		if day, err = sportsday.Parse(given); err != nil {
			s.fail(w, r, problem.New("The day must look like 2026-10-20."))
			return
		}
	}
	view, err := s.Lineups.Get(r.Context(), league, franchiseID, day)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// setLineup saves the signed-in franchise's starters for a day. A
// commissioner may set another franchise's, and may force past the locks.
func (s *Server) setLineup(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		Day         string         `json:"day"`
		Entries     []lineup.Entry `json:"entries"`
		FranchiseID pgtype.UUID    `json:"franchise_id"` // commissioner only
		Force       bool           `json:"force"`        // commissioner only
	}
	if !readJSON(w, r, &body) {
		return
	}
	league, err := s.leagueFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	day, err := sportsday.Parse(body.Day)
	if err != nil {
		s.fail(w, r, problem.New("The day must look like 2026-10-20."))
		return
	}
	franchiseID := me.ID
	if body.FranchiseID.Valid && body.FranchiseID != me.ID {
		if !me.IsCommissioner {
			writeError(w, http.StatusForbidden, "You can only set your own lineup.")
			return
		}
		franchiseID = body.FranchiseID
	}
	s.done(w, r, s.Lineups.Set(r.Context(), league, franchiseID, day, body.Entries, body.Force && me.IsCommissioner))
}

func (s *Server) leagueFromPath(r *http.Request) (db.League, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return db.League{}, err
	}
	return s.Queries.GetLeague(r.Context(), id)
}
