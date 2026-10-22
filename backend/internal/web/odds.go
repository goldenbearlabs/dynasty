package web

import (
	"net/http"
	"sync"
	"time"

	"crossover/internal/scoring"
)

// oddsAge is how long one run of the playoff odds is shown. Scores change
// all day; the odds need not be run again on every one of them.
const oddsAge = 10 * time.Minute

// oddsRun is the latest run of the odds.
type oddsRun struct {
	sync.Mutex
	at   time.Time
	last scoring.Odds
}

// getOdds returns each league's playoff odds and the overall title odds.
func (s *Server) getOdds(w http.ResponseWriter, r *http.Request) {
	odds := &s.odds
	odds.Lock()
	defer odds.Unlock()
	if time.Since(odds.at) > oddsAge {
		d, err := s.Queries.GetDynasty(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		run, err := s.Scoring.Odds(r.Context(), d, s.Lineups.Projected)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		odds.at, odds.last = time.Now(), run
	}
	writeJSON(w, http.StatusOK, odds.last)
}
