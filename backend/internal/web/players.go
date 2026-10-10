package web

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/players"
	"crossover/internal/problem"
)

const playersPerPage = 50

// listResearch values imported season totals using current league rules.
func (s *Server) listResearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	integer := func(key string, fallback, maximum int) (int, error) {
		if query.Get(key) == "" {
			return fallback, nil
		}
		value, err := strconv.Atoi(query.Get(key))
		if err != nil || value < 0 || value > maximum {
			return 0, problem.New("%s must be between 0 and %d.", key, maximum)
		}
		return value, nil
	}
	real := func(key string) (float64, error) {
		if query.Get(key) == "" {
			return 0, nil
		}
		value, err := strconv.ParseFloat(query.Get(key), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, problem.New("%s must be a finite number.", key)
		}
		return value, nil
	}
	f := players.ResearchFilter{
		MinRateSet: query.Get("min_rate") != "", MaxRateSet: query.Get("max_rate") != "", MinIndexSet: query.Get("min_index") != "", MaxIndexSet: query.Get("max_index") != "", MinPARSet: query.Get("min_par") != "", MaxGamesSet: query.Get("max_games") != "",
		Competition: query.Get("competition"), Season: query.Get("season"), Status: query.Get("status"), Search: query.Get("q"),
		RawPerGame: query.Get("raw_per_game") == "true", QualifiedOnly: query.Get("qualified_only") == "true", MissingOnly: query.Get("missing_only") == "true", IncludeChart: query.Get("include_chart") == "true",
		StatKey: query.Get("stat_key"), ChartX: query.Get("chart_x"), ChartY: query.Get("chart_y"), ChartGroup: query.Get("chart_group"),
		Position: query.Get("position"), Team: query.Get("team"), Owner: query.Get("owner"), Sort: query.Get("sort"),
		AboveReplacement: query.Get("above_replacement") == "true", Ascending: query.Get("ascending") == "true", PerPage: playersPerPage,
	}
	workload, err := integer("pitcher_workload_percent", 25, 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f.PitcherWorkloadPercent = &workload
	for _, field := range []struct {
		key               string
		value             *int
		fallback, maximum int
	}{
		{"max_games", &f.MaxGames, 0, 10000}, {"page", &f.Page, 1, 1000000}, {"min_games", &f.MinGames, 0, 10000},
		{"benchmark_games", &f.BenchmarkGames, 5, 10000}, {"replacement_rank", &f.ReplacementRank, 0, 100000},
	} {
		value, err := integer(field.key, field.fallback, field.maximum)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		*field.value = value
	}
	for _, field := range []struct {
		key   string
		value *float64
	}{{"min_rate", &f.MinRate}, {"min_index", &f.MinIndex}, {"max_rate", &f.MaxRate}, {"max_index", &f.MaxIndex}, {"min_par", &f.MinPAR}} {
		value, err := real(field.key)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		*field.value = value
	}
	for _, field := range []struct {
		key   string
		value **float64
	}{{"min_stat", &f.MinStat}, {"max_stat", &f.MaxStat}} {
		if query.Get(field.key) == "" {
			continue
		}
		value, err := real(field.key)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		*field.value = &value
	}
	if raw := query.Get("pools"); raw != "" {
		if len(raw) > 8192 || json.Unmarshal([]byte(raw), &f.Pools) != nil {
			s.fail(w, r, problem.New("Research pools must be a list of league and season pairs."))
			return
		}
	}
	result, err := s.Players.Analytics(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

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

// listStatSeasons names the seasons there are stats for in each sport,
// newest first, so a list of players can be put in any of them.
func (s *Server) listStatSeasons(w http.ResponseWriter, r *http.Request) {
	seasons, err := s.Queries.ListStatSeasons(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if seasons == nil {
		seasons = []db.ListStatSeasonsRow{}
	}
	writeJSON(w, http.StatusOK, seasons)
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

	seasonBack, _ := strconv.Atoi(query.Get("season_back"))
	// An age filter leaves out anyone whose birth date is not known.
	age := func(key string) pgtype.Int4 {
		n, err := strconv.Atoi(query.Get(key))
		return pgtype.Int4{Int32: int32(n), Valid: err == nil && n >= 0}
	}
	position, minAge, maxAge := query.Get("position"), age("min_age"), age("max_age")

	players, err := s.Queries.ListPlayers(r.Context(), db.ListPlayersParams{
		Competition: competition,
		Status:      status,
		Search:      search,
		Position:    position,
		MinAge:      minAge,
		MaxAge:      maxAge,
		AvailableIn: availableIn,
		DraftID:     draftID,
		SortBy:      query.Get("sort"),         // "points" or "index": highest first; otherwise by name
		SeasonBack:  int32(max(seasonBack, 0)), // 0 is each sport's latest season, 1 the one before
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
		Position:    position,
		MinAge:      minAge,
		MaxAge:      maxAge,
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
