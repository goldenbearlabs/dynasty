package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/problem"
	"crossover/internal/scoring"
	"crossover/internal/sportsday"
)

// getScores returns a sport's games for a day: ?day=YYYY-MM-DD, default today.
func (s *Server) getScores(w http.ResponseWriter, r *http.Request) {
	c, ok := s.Registry.Get(r.PathValue("competition"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown competition")
		return
	}
	day := sportsday.Today()
	if given := r.URL.Query().Get("day"); given != "" {
		var err error
		if day, err = sportsday.Parse(given); err != nil {
			s.fail(w, r, problem.New("The day must look like 2026-10-20."))
			return
		}
	}
	board, err := s.Live.Scoreboard(r.Context(), c.Key, day)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

// watchScores is today's scoreboard for a sport, pushed again whenever the
// server's poll of the games in play changes something.
func (s *Server) watchScores(w http.ResponseWriter, r *http.Request) {
	c, ok := s.Registry.Get(r.PathValue("competition"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown competition")
		return
	}
	messages, leave := s.Live.WatchScores(c.Key)
	defer leave()
	board, err := s.Live.Scoreboard(r.Context(), c.Key, sportsday.Today())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	snapshot, _ := json.Marshal(board)
	s.stream(w, r, snapshot, messages)
}

func (s *Server) getGame(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	game, err := s.Live.Game(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

// watchGame is one game's score and box score, pushed as it changes.
func (s *Server) watchGame(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	messages, leave := s.Live.WatchGame(id)
	defer leave()
	game, err := s.Live.Game(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	snapshot, _ := json.Marshal(game)
	s.stream(w, r, snapshot, messages)
}

// ---- head-to-head ----

// listMatchups returns a period's matchups: ?period=<number>, default the
// one in progress.
func (s *Server) listMatchups(w http.ResponseWriter, r *http.Request) {
	league, err := s.leagueFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	seq, _ := strconv.Atoi(r.URL.Query().Get("period"))
	matchups, err := s.Scoring.Matchups(r.Context(), league, seq)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, matchups)
}

// getSchedule returns every period of a league's season with its matchups.
func (s *Server) getSchedule(w http.ResponseWriter, r *http.Request) {
	league, err := s.leagueFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	schedule, err := s.Scoring.Schedule(r.Context(), league)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (s *Server) getMatchup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	matchup, err := s.Scoring.Matchup(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	projected, err := s.projectMatchup(r.Context(), matchup)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projected)
}

// projectedMatchup is a matchup with each side's projected final score:
// its points so far and what its starters are projected to add. The
// projections are absent once the matchup is final.
type projectedMatchup struct {
	scoring.MatchupDetail
	HomeProjected *float64 `json:"home_projected"`
	AwayProjected *float64 `json:"away_projected"`
}

func (s *Server) projectMatchup(ctx context.Context, m scoring.MatchupDetail) (projectedMatchup, error) {
	out := projectedMatchup{MatchupDetail: m}
	if m.Final {
		return out, nil
	}
	season, err := s.Queries.GetSeason(ctx, m.Period.SeasonID)
	if err != nil {
		return out, err
	}
	league, err := s.Queries.GetLeague(ctx, season.LeagueID)
	if err != nil {
		return out, err
	}
	side := func(franchise pgtype.UUID, points float64) (*float64, error) {
		if !franchise.Valid {
			return nil, nil // a bye
		}
		ahead, err := s.Lineups.Projected(ctx, league, franchise, m.Period.StartsOn.Time, m.Period.EndsOn.Time)
		total := points + ahead
		return &total, err
	}
	if out.HomeProjected, err = side(m.Home, m.HomePoints); err != nil {
		return out, err
	}
	out.AwayProjected, err = side(m.Away, m.AwayPoints)
	return out, err
}

// setMatchups replaces a period's matchups with the commissioner's own.
func (s *Server) setMatchups(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Matchups []scoring.Pairing `json:"matchups"`
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !readJSON(w, r, &body) {
		return
	}
	s.done(w, r, s.Scoring.SetMatchups(r.Context(), id, body.Matchups))
}

// resetMatchups hands a period's matchups back to the schedule.
func (s *Server) resetMatchups(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Scoring.ResetMatchups(r.Context(), id))
}

// generateSchedule rebuilds the part of a head-to-head season that has not
// started.
func (s *Server) generateSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Scoring.GenerateSchedule(r.Context(), id))
}

// advancePlayoffs sets any playoff round that has come due. The scores sync
// does this on its own; this is for not having to wait.
func (s *Server) advancePlayoffs(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.Scoring.AdvancePlayoffs(r.Context()))
}

// ---- players who have left their competition ----

// listReviewQueue returns rostered players who are no longer in their
// competition's feed, for the commissioner to release or carry over.
func (s *Server) listReviewQueue(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	queue, err := s.Queries.ListRosteredInactive(r.Context(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, queue)
}

// carryOver graduates a player by hand into the league his own league's
// rules carry players into.
func (s *Server) carryOver(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlayerID pgtype.UUID `json:"player_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	league, err := s.leagueFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Roster.CarryOver(r.Context(), league, body.PlayerID))
}

// setSchedule saves the commissioner's dates and matchups for future periods.
func (s *Server) setSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Periods []scoring.CustomPeriod `json:"periods"`
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !readJSON(w, r, &body) {
		return
	}
	s.done(w, r, s.Scoring.SetSchedule(r.Context(), id, body.Periods))
}

// getGameCounts supplies a compact calendar for previewing game totals
// while a commissioner adjusts unsaved matchup dates. It works for every sport.
func (s *Server) getGameCounts(w http.ResponseWriter, r *http.Request) {
	league, err := s.leagueFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err1 := sportsday.Parse(r.URL.Query().Get("starts_on"))
	to, err2 := sportsday.Parse(r.URL.Query().Get("ends_on"))
	if err1 != nil || err2 != nil || to.Before(from) || to.After(from.AddDate(3, 0, 0)) {
		s.fail(w, r, problem.New("Choose valid first and last days, no more than three years apart."))
		return
	}
	counts, err := s.Queries.ListGameCounts(r.Context(), league.Competition, sportsday.Date(from), sportsday.Date(to))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, counts)
}
