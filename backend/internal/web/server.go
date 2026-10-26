// Package web is the HTTP layer. Handlers stay thin: read the request, call
// one function from another package, write the result.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/auth"
	"crossover/internal/cache"
	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/draft"
	"crossover/internal/dynasty"
	"crossover/internal/ingest"
	"crossover/internal/lineup"
	"crossover/internal/live"
	"crossover/internal/players"
	"crossover/internal/problem"
	"crossover/internal/roster"
	"crossover/internal/scoring"
	"crossover/internal/trade"
	"crossover/internal/waiver"
)

type Server struct {
	Cache    *cache.Store
	Queries  *db.Queries
	Auth     *auth.Service
	Registry competition.Registry
	Syncer   *ingest.Syncer
	Dynasty  *dynasty.Service
	Roster   *roster.Service
	Players  *players.Service
	Drafts   *draft.Service
	Trades   *trade.Service
	Waivers  *waiver.Service
	Lineups  *lineup.Service
	Scoring  *scoring.Service
	Live     *live.Service
	Log      *slog.Logger

	Ping             func(context.Context) error // database health
	Background       context.Context             // lifetime of work that outlives a request
	AdminToken       string                      // optional bearer token for scripts; see commissioner
	PublicURL        string                      // public origin for account email links
	SendAccountEmail func(context.Context, string, string, string) error
	// SeasonBackfill is how many seasons of stat history a sync keeps.
	SeasonBackfill int
	// ScheduleDays extends the full-year schedule horizon when larger than 365.
	// Active fantasy season end dates can extend it further.
	ScheduleDays int

	odds oddsRun
}

// Handler routes every request. Reading is open to anyone who can reach the
// site; changing anything needs a signed-in manager, and league
// administration needs a commissioner.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/bootstrap", s.bootstrap)
	mux.HandleFunc("GET /api/my-leagues", s.listMyLeagues)
	mux.HandleFunc("POST /api/my-leagues", s.createMyLeague)
	mux.HandleFunc("GET /api/my-leagues/{id}", s.getMyLeague)

	mux.HandleFunc("GET /api/competitions", s.cached(cache.Research, 10*time.Minute, s.listCompetitions))
	mux.HandleFunc("GET /api/teams", s.cached(cache.Research, 10*time.Minute, s.listTeams))
	mux.HandleFunc("GET /api/players", s.cached(cache.Public, 30*time.Second, s.listPlayers))
	mux.HandleFunc("GET /api/stat-seasons", s.cached(cache.Research, 10*time.Minute, s.listStatSeasons))
	mux.HandleFunc("GET /api/prospect-rankings", s.cached(cache.Research, 10*time.Minute, s.listProspectRankings))
	mux.HandleFunc("GET /api/research", s.cached(cache.Research, 10*time.Minute, s.listResearch))
	mux.HandleFunc("GET /api/players/{id}", s.cached(cache.Public, 30*time.Second, s.researchPlayer))
	mux.HandleFunc("GET /api/players/{id}/seasons", s.cached(cache.Public, 30*time.Second, s.playerSeasons))
	mux.HandleFunc("GET /api/players/{id}/profile", s.cached(cache.Public, 30*time.Second, s.playerProfile))
	mux.HandleFunc("GET /api/players/{id}/transactions", s.cached(cache.Public, 30*time.Second, s.playerTimeline))
	mux.HandleFunc("GET /api/players/{id}/games", s.cached(cache.Public, 30*time.Second, s.playerGameLog))
	mux.HandleFunc("GET /api/dynasty", s.cached(cache.Public, 30*time.Second, s.getDynasty))
	mux.HandleFunc("POST /api/dynasty", s.createDynasty) // first run only
	mux.HandleFunc("GET /api/franchises/{slug}", s.cached(cache.Public, 30*time.Second, s.getFranchise))
	mux.HandleFunc("PUT /api/franchises/{id}/identity", s.member(s.setOrganizationIdentity))
	mux.HandleFunc("PUT /api/franchises/{id}/leagues/{league_id}/identity", s.member(s.setTeamIdentity))
	mux.HandleFunc("GET /api/franchises/{id}/players/{player_id}/nickname", s.getPlayerNickname)
	mux.HandleFunc("PUT /api/franchises/{id}/players/{player_id}/nickname", s.member(s.setPlayerNickname))
	mux.HandleFunc("GET /api/activity", s.cached(cache.Public, 30*time.Second, s.listActivity))
	mux.HandleFunc("GET /api/standings", s.cached(cache.Public, 30*time.Second, s.listStandings))
	mux.HandleFunc("GET /api/overall", s.cached(cache.Public, 30*time.Second, s.getOverall))
	mux.HandleFunc("GET /api/odds", s.getOdds)
	mux.HandleFunc("GET /api/seasons", s.cached(cache.Public, 30*time.Second, s.listSeasons))
	mux.HandleFunc("GET /api/leagues/{id}/lineup", s.getLineup)
	mux.HandleFunc("GET /api/leagues/{id}/roster-stats", s.cached(cache.Public, 30*time.Second, s.getRosterStats))
	mux.HandleFunc("GET /api/leagues/{id}/matchups", s.cached(cache.Public, 30*time.Second, s.listMatchups))
	mux.HandleFunc("GET /api/leagues/{id}/schedule", s.cached(cache.Public, 30*time.Second, s.getSchedule))
	mux.HandleFunc("GET /api/leagues/{id}/game-counts", s.cached(cache.Public, 30*time.Second, s.getGameCounts))
	mux.HandleFunc("GET /api/matchups/{id}", s.cached(cache.Public, 30*time.Second, s.getMatchup))
	mux.HandleFunc("GET /api/scores/{competition}", s.cached(cache.Public, 30*time.Second, s.getScores))
	mux.HandleFunc("GET /api/scores/{competition}/ws", s.watchScores)
	mux.HandleFunc("GET /api/games/{id}", s.cached(cache.Public, 30*time.Second, s.getGame))
	mux.HandleFunc("GET /api/games/{id}/ws", s.watchGame)
	mux.HandleFunc("GET /api/trades", s.listTrades)
	mux.HandleFunc("GET /api/leagues/{id}/waivers", s.getWaivers)
	mux.HandleFunc("GET /api/drafts", s.listDrafts)
	mux.HandleFunc("GET /api/drafts/{id}", s.getDraft)
	mux.HandleFunc("GET /api/drafts/{id}/ws", s.watchDraft)
	presence := newDraftPresence()
	mux.HandleFunc("GET /api/drafts/{id}/presence/ws", s.member(func(w http.ResponseWriter, r *http.Request, me db.Franchise) {
		s.watchDraftPresence(w, r, me, presence)
	}))

	mux.HandleFunc("GET /api/session", s.getSession)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/accounts", s.registerAccount)
	mux.HandleFunc("POST /api/accounts/verify", s.verifyAccount)
	mux.HandleFunc("POST /api/accounts/verification", s.resendVerification)
	mux.HandleFunc("POST /api/accounts/password-reset", s.requestPasswordReset)
	mux.HandleFunc("POST /api/accounts/password-reset/confirm", s.completePasswordReset)
	mux.HandleFunc("POST /api/account/password", s.changeAccountPassword)
	mux.HandleFunc("POST /api/account/email-change", s.beginAccountEmailChange)
	mux.HandleFunc("POST /api/account/email-change/confirm", s.confirmAccountEmailChange)
	mux.HandleFunc("GET /api/account/deletion-options", s.accountDeletionOptions)
	mux.HandleFunc("DELETE /api/account", s.deleteAccount)
	mux.HandleFunc("POST /api/mobile/auth/login", s.mobileLogin)
	mux.HandleFunc("POST /api/mobile/auth/logout", s.mobileLogout)
	mux.HandleFunc("POST /api/mobile/auth/refresh", s.mobileRefresh)
	mux.HandleFunc("GET /api/mobile/session", s.mobileSession)
	mux.HandleFunc("POST /api/mobile/invites/{token}/claim", s.mobileClaimInvite)
	mux.HandleFunc("GET /api/invites/{token}", s.getInvite)
	mux.HandleFunc("POST /api/invites/{token}/claim", s.claimInvite)
	mux.HandleFunc("POST /api/invites/{token}/accept", s.acceptExistingInvite)
	mux.HandleFunc("POST /api/mobile/invites/{token}/accept", s.mobileAcceptInvite)

	mux.HandleFunc("POST /api/leagues/{id}/roster/{action}", s.member(s.changeRoster))
	mux.HandleFunc("PUT /api/leagues/{id}/lineup", s.member(s.setLineup))
	mux.HandleFunc("POST /api/leagues/{id}/lineup/auto", s.member(s.autoSetLineup))
	mux.HandleFunc("POST /api/leagues/{id}/waivers/claims", s.member(s.claimWaiver))
	mux.HandleFunc("DELETE /api/waivers/claims/{id}", s.member(s.cancelWaiverClaim))
	mux.HandleFunc("POST /api/trades", s.member(s.proposeTrade))
	mux.HandleFunc("POST /api/trades/{id}/{action}", s.member(s.answerTrade))
	mux.HandleFunc("POST /api/drafts/{id}/pick", s.member(s.makePick))
	mux.HandleFunc("POST /api/drafts/{id}/pass", s.member(s.passPick))
	mux.HandleFunc("POST /api/drafts/{id}/picks/{pick}/react", s.member(s.reactToPick))
	mux.HandleFunc("GET /api/drafts/{id}/queue", s.member(s.getQueue))
	mux.HandleFunc("PUT /api/drafts/{id}/queue", s.member(s.setQueue))
	mux.HandleFunc("PUT /api/drafts/{id}/autopick", s.member(s.setAutoPick))
	mux.HandleFunc("POST /api/drafts/{id}/queue/import", s.member(s.importRanking))
	mux.HandleFunc("GET /api/trade-market", s.member(s.getTradeMarket))
	mux.HandleFunc("PUT /api/trade-market/block", s.member(s.setTradeBlock))
	mux.HandleFunc("POST /api/trade-market/wants", s.member(s.addTradeWant))
	mux.HandleFunc("DELETE /api/trade-market/wants/{id}", s.member(s.removeTradeWant))
	mux.HandleFunc("PUT /api/trade-market/targets", s.member(s.setTradeTarget))
	mux.HandleFunc("GET /api/notifications", s.member(s.listNotifications))
	mux.HandleFunc("GET /api/notifications/summary", s.member(s.notificationSummary))
	mux.HandleFunc("PATCH /api/notifications/{id}", s.member(s.readNotification))
	mux.HandleFunc("POST /api/notifications/read", s.member(s.readAllNotifications))
	mux.HandleFunc("GET /api/watchlist", s.member(s.getWatchlist))
	mux.HandleFunc("PUT /api/watchlist/order", s.member(s.orderWatchlist))
	mux.HandleFunc("PUT /api/watchlist/players/{player_id}", s.member(s.addWatchlistPlayer))
	mux.HandleFunc("PATCH /api/watchlist/players/{player_id}", s.member(s.editWatchlistPlayer))
	mux.HandleFunc("DELETE /api/watchlist/players/{player_id}", s.member(s.removeWatchlistPlayer))
	mux.HandleFunc("GET /api/rankings", s.member(s.listRankings))
	mux.HandleFunc("POST /api/rankings", s.member(s.saveRanking))
	mux.HandleFunc("GET /api/rankings/{id}", s.member(s.getRanking))
	mux.HandleFunc("PUT /api/rankings/{id}", s.member(s.saveRanking))
	mux.HandleFunc("DELETE /api/rankings/{id}", s.member(s.deleteRanking))

	mux.HandleFunc("PUT /api/dynasty", s.commissioner(s.updateDynasty))
	mux.HandleFunc("PUT /api/leagues/{id}/settings", s.commissioner(s.updateLeagueSettings))
	mux.HandleFunc("POST /api/admin/leagues", s.commissioner(s.addLeague))
	mux.HandleFunc("PUT /api/admin/leagues/{id}/waiver-order", s.commissioner(s.setWaiverOrder))
	mux.HandleFunc("GET /api/admin/franchises", s.commissioner(s.listInvites))
	mux.HandleFunc("POST /api/admin/franchises", s.commissioner(s.addFranchise))
	mux.HandleFunc("PUT /api/admin/franchises/{id}", s.commissioner(s.updateFranchise))
	mux.HandleFunc("POST /api/admin/franchises/{id}/invite", s.commissioner(s.reissueInvite))
	mux.HandleFunc("POST /api/admin/seasons", s.commissioner(s.createSeason))
	mux.HandleFunc("PUT /api/admin/seasons/{id}", s.commissioner(s.setSeasonDates))
	mux.HandleFunc("POST /api/admin/seasons/{id}/close", s.commissioner(s.closeSeason))
	mux.HandleFunc("POST /api/admin/seasons/{id}/reopen", s.commissioner(s.reopenSeason))
	mux.HandleFunc("PUT /api/admin/seasons/{id}/roster-deadline", s.commissioner(s.setRosterDeadline))
	mux.HandleFunc("PUT /api/admin/seasons/{id}/schedule", s.commissioner(s.setSchedule))
	mux.HandleFunc("POST /api/admin/seasons/{id}/schedule", s.commissioner(s.generateSchedule))
	mux.HandleFunc("POST /api/admin/playoffs/advance", s.commissioner(s.advancePlayoffs))
	mux.HandleFunc("PUT /api/admin/periods/{id}/matchups", s.commissioner(s.setMatchups))
	mux.HandleFunc("DELETE /api/admin/periods/{id}/matchups", s.commissioner(s.resetMatchups))
	mux.HandleFunc("GET /api/admin/review-queue", s.commissioner(s.listReviewQueue))
	mux.HandleFunc("POST /api/admin/leagues/{id}/carry-over", s.commissioner(s.carryOver))
	mux.HandleFunc("POST /api/admin/trades/{id}/{action}", s.commissioner(s.ruleOnTrade))
	mux.HandleFunc("POST /api/admin/drafts/future", s.commissioner(s.createFutureDrafts))
	mux.HandleFunc("POST /api/admin/drafts", s.commissioner(s.createDraft))
	mux.HandleFunc("PUT /api/admin/drafts/{id}", s.commissioner(s.setDraftClock))
	mux.HandleFunc("DELETE /api/admin/drafts/{id}", s.commissioner(s.deleteDraft))
	mux.HandleFunc("POST /api/admin/drafts/{id}/schedule", s.commissioner(s.scheduleDraft))
	mux.HandleFunc("PUT /api/admin/drafts/{id}/picks", s.commissioner(s.setDraftPicks))
	mux.HandleFunc("PUT /api/admin/drafts/{id}/picks/{pick}", s.commissioner(s.overridePick))
	mux.HandleFunc("POST /api/admin/drafts/{id}/{action}", s.commissioner(s.controlDraft))
	mux.HandleFunc("POST /api/admin/players", s.commissioner(s.addPlayers))
	mux.HandleFunc("GET /api/admin/players/merge-suggestions", s.commissioner(s.listMergeSuggestions))
	mux.HandleFunc("POST /api/admin/players/merge", s.commissioner(s.mergePlayers))
	mux.HandleFunc("POST /api/admin/sync/{competition}/{job}", s.commissioner(s.startSync))
	mux.HandleFunc("GET /api/admin/ingest-runs", s.commissioner(s.listIngestRuns))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", frontend())
	protected := s.privateAPI(mux)
	if s.Cache == nil {
		return protected
	}
	// Whoever changes something sees it at once: see cache.Store.RevisionAge.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protected.ServeHTTP(w, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.Cache.Forget()
		}
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// readJSON decodes the request body into v, answering 400 itself on failure.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return false
	}
	return true
}

// fail answers for an error from below: a problem is shown as written, a
// missing row is a 404, and anything else is logged and hidden.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var p problem.Error
	switch {
	case errors.As(err, &p):
		writeError(w, http.StatusUnprocessableEntity, p.Error())
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not found")
	default:
		s.Log.Error("request failed", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// done answers a request that returns nothing: 204, or the error.
func (s *Server) done(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathID reads a UUID path parameter. A malformed id cannot match any row,
// so it is reported as a missing one.
func pathID(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue(name)); err != nil {
		return id, pgx.ErrNoRows
	}
	return id, nil
}
