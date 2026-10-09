package web

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/competition"
	"crossover/internal/db"
)

const playersPerPage = 50

func (s *Server) researchPlayer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	research, err := s.Players.Research(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, research)
}

// playerSeasons returns a player's season-by-season stats with their
// fantasy value under today's rules.
func (s *Server) playerSeasons(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	seasons, err := s.Players.Seasons(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, seasons)
}

// listCompetitions returns the registry: each sport's positions, stats and
// default rules, plus how many players are in its pool.
func (s *Server) listCompetitions(w http.ResponseWriter, r *http.Request) {
	counts, err := s.Queries.CountPlayersByCompetition(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	players := map[string]int64{}
	for _, c := range counts {
		players[c.Competition] = c.Players
	}

	type item struct {
		competition.Competition
		Players      int64 `json:"players"`
		HasProspects bool  `json:"has_prospects"` // whether a prospect feed exists to sync
	}
	items := []item{}
	for _, c := range s.Registry {
		items = append(items, item{Competition: c, Players: players[c.Key], HasProspects: c.Prospects != nil})
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) listPlayers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	competition, status, search := query.Get("competition"), query.Get("status"), query.Get("q")
	var availableIn pgtype.UUID // invalid (no filter) unless a league id is given
	availableIn.Scan(query.Get("available_in"))
	var draftID pgtype.UUID // likewise, for every league a draft covers
	draftID.Scan(query.Get("draft_id"))

	players, err := s.Queries.ListPlayers(r.Context(), db.ListPlayersParams{
		Competition: competition,
		Status:      status,
		Search:      search,
		AvailableIn: availableIn,
		DraftID:     draftID,
		PageSize:    playersPerPage,
		PageOffset:  int32((page - 1) * playersPerPage),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	total, err := s.Queries.CountPlayers(r.Context(), db.CountPlayersParams{
		Competition: competition,
		Status:      status,
		Search:      search,
		AvailableIn: availableIn,
		DraftID:     draftID,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"players":  players,
		"total":    total,
		"page":     page,
		"per_page": playersPerPage,
	})
}
