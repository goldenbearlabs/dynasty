package web

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/dynasty"
	"crossover/internal/roster"
	"crossover/internal/settings"
)

// leagueRoster is one franchise's holdings in one league.
type leagueRoster struct {
	LeagueID    pgtype.UUID     `json:"league_id"`
	Competition string          `json:"competition"`
	Name        string          `json:"name"`
	Limits      settings.Roster `json:"limits"`
	Overage     int             `json:"overage"` // above zero: over the limits
	Players     []rosterPlayer  `json:"players"`
}

type rosterPlayer struct {
	db.ListFranchiseRosterRow
	LockedUntil *time.Time `json:"locked_until"` // set while a reserve lock holds him
}

// getFranchise returns a franchise with everything it holds: its roster in
// every league and its unused draft picks.
func (s *Server) getFranchise(w http.ResponseWriter, r *http.Request) {
	view, err := s.loadDynasty(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	franchise, err := s.Queries.GetFranchiseBySlug(r.Context(), db.GetFranchiseBySlugParams{DynastyID: view.ID, Slug: r.PathValue("slug")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	players, err := s.Queries.ListFranchiseRoster(r.Context(), franchise.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	rosters := []leagueRoster{}
	for _, league := range view.Leagues {
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		lr := leagueRoster{
			LeagueID: league.ID, Competition: league.Competition, Name: league.Name,
			Limits: rules.Roster, Players: []rosterPlayer{},
		}
		var entries []roster.Entry
		for _, p := range players {
			if p.LeagueID == league.ID {
				player := rosterPlayer{ListFranchiseRosterRow: p}
				if until := roster.LockedUntil(rules.Roster, p.ReservedAt); time.Now().Before(until) {
					player.LockedUntil = &until
				}
				lr.Players = append(lr.Players, player)
				entries = append(entries, roster.Entry{PlayerID: p.PlayerID, List: p.List, Prospect: p.Status == "prospect"})
			}
		}
		lr.Overage = roster.Overage(rules.Roster, entries)
		rosters = append(rosters, lr)
	}
	picks, err := s.Queries.ListFranchisePicks(r.Context(), franchise.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"franchise": franchise, "rosters": rosters, "picks": picks})
}

// invite is a franchise as the commissioner manages it: whether a manager
// has an account yet, and the invite link if one is outstanding.
type invite struct {
	db.Franchise
	Email       string `json:"email"`        // empty until the manager creates an account
	InviteToken string `json:"invite_token"` // empty when no link is outstanding
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows, err := s.Queries.ListInvites(r.Context(), d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	invites := []invite{}
	for _, row := range rows {
		invites = append(invites, invite{
			Franchise: db.Franchise{
				ID: row.ID, DynastyID: row.DynastyID, Name: row.Name, ManagerName: row.ManagerName,
				Slug: row.Slug, IsCommissioner: row.IsCommissioner, UserID: row.UserID,
			},
			Email:       row.Email,
			InviteToken: row.InviteToken.String,
		})
	}
	writeJSON(w, http.StatusOK, invites)
}

func (s *Server) addFranchise(w http.ResponseWriter, r *http.Request) {
	var body dynasty.FranchiseSetup
	if !readJSON(w, r, &body) {
		return
	}
	d, err := s.Queries.GetDynasty(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	franchise, err := s.Dynasty.AddFranchise(r.Context(), d.ID, body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A new franchise needs matchups in every season in progress.
	if leagues, err := s.Queries.ListLeagues(r.Context(), d.ID); err == nil {
		for _, league := range leagues {
			if err := s.Scoring.Reschedule(r.Context(), league.ID); err != nil {
				s.Log.Warn("reschedule after adding a franchise", "league", league.Name, "err", err)
			}
		}
	}
	writeJSON(w, http.StatusCreated, invite{Franchise: franchise, InviteToken: franchise.InviteToken.String})
}

func (s *Server) updateFranchise(w http.ResponseWriter, r *http.Request) {
	var body struct {
		dynasty.FranchiseSetup
		IsCommissioner bool `json:"is_commissioner"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	franchise, err := s.franchiseFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Dynasty.UpdateFranchise(r.Context(), franchise, body.FranchiseSetup, body.IsCommissioner); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reissueInvite(w http.ResponseWriter, r *http.Request) {
	franchise, err := s.franchiseFromPath(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	token, err := s.Dynasty.ReissueInvite(r.Context(), franchise.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"invite_token": token})
}

func (s *Server) franchiseFromPath(r *http.Request) (db.Franchise, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return db.Franchise{}, err
	}
	return s.Queries.GetFranchise(r.Context(), id)
}
