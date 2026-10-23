package web

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
)

// tradeMarket is the trade market as one manager sees it: every manager's
// block and wants, the targets this manager has set, and the interest
// others have shown in this manager's players. Targets are seen only by
// the manager who set them and the player's owner.
type tradeMarket struct {
	Block    []db.ListTradeBlockRow   `json:"block"`
	Wants    []db.ListTradeWantsRow   `json:"wants"`
	Targets  []db.ListTradeTargetsRow `json:"targets"`  // players this manager is after
	Interest []db.ListTradeTargetsRow `json:"interest"` // this manager's players others are after
}

func (s *Server) getTradeMarket(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	ctx := r.Context()
	block, err := s.Queries.ListTradeBlock(ctx, me.DynastyID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	wants, err := s.Queries.ListTradeWants(ctx, me.DynastyID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	targets, err := s.Queries.ListTradeTargets(ctx, me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	market := tradeMarket{Block: block, Wants: wants, Targets: []db.ListTradeTargetsRow{}, Interest: []db.ListTradeTargetsRow{}}
	for _, t := range targets {
		if t.ByFranchiseID == me.ID {
			market.Targets = append(market.Targets, t)
		} else {
			market.Interest = append(market.Interest, t)
		}
	}
	writeJSON(w, http.StatusOK, market)
}

// marketPlayer reads the league and player a trade-market change is about,
// and who holds him there.
func (s *Server) marketPlayer(r *http.Request, me db.Franchise, leagueID, playerID pgtype.UUID) (owner pgtype.UUID, err error) {
	league, err := s.Queries.GetLeague(r.Context(), leagueID)
	if err != nil || league.DynastyID != me.DynastyID {
		return owner, problem.New("That league is not in this dynasty.")
	}
	entry, err := s.Queries.GetRosterEntry(r.Context(), db.GetRosterEntryParams{LeagueID: leagueID, PlayerID: playerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return owner, problem.New("That player is not on a roster in this league.")
	}
	return entry.FranchiseID, err
}

// setTradeBlock puts one of the manager's own players on the trade block,
// with a note, or takes him off.
func (s *Server) setTradeBlock(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		LeagueID pgtype.UUID `json:"league_id"`
		PlayerID pgtype.UUID `json:"player_id"`
		On       bool        `json:"on"`
		Note     string      `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	owner, err := s.marketPlayer(r, me, body.LeagueID, body.PlayerID)
	switch {
	case err != nil:
		s.fail(w, r, err)
		return
	case owner != me.ID:
		s.fail(w, r, problem.New("You can only put your own players on the trade block."))
		return
	}
	if !body.On {
		s.done(w, r, s.Queries.TakeOffTradeBlock(r.Context(), db.TakeOffTradeBlockParams{LeagueID: body.LeagueID, PlayerID: body.PlayerID, FranchiseID: me.ID}))
		return
	}
	note, err := marketNote(body.Note)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.done(w, r, s.Queries.PutOnTradeBlock(r.Context(), db.PutOnTradeBlockParams{LeagueID: body.LeagueID, PlayerID: body.PlayerID, FranchiseID: me.ID, Note: note}))
}

// addTradeWant adds something the manager is looking for in one league:
// positions, a note, or both. A manager may have several in a league.
func (s *Server) addTradeWant(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		LeagueID  pgtype.UUID `json:"league_id"`
		Positions []string    `json:"positions"`
		Note      string      `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	league, err := s.Queries.GetLeague(r.Context(), body.LeagueID)
	if err != nil || league.DynastyID != me.DynastyID {
		s.fail(w, r, problem.New("That league is not in this dynasty."))
		return
	}
	note, err := marketNote(body.Note)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, _ := s.Registry.Get(league.Competition)
	positions := []string{}
	for _, p := range body.Positions {
		if !slices.Contains(c.Positions, p) {
			s.fail(w, r, problem.New("%s is not a position in %s.", p, c.Name))
			return
		}
		if !slices.Contains(positions, p) {
			positions = append(positions, p)
		}
	}
	if len(positions) == 0 && note == "" {
		s.fail(w, r, problem.New("Pick a position or write a note."))
		return
	}
	s.done(w, r, s.Queries.AddTradeWant(r.Context(), db.AddTradeWantParams{FranchiseID: me.ID, LeagueID: league.ID, Positions: positions, Note: note}))
}

// removeTradeWant takes back one of the manager's own wants.
func (s *Server) removeTradeWant(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.Queries.RemoveTradeWant(r.Context(), db.RemoveTradeWantParams{ID: id, FranchiseID: me.ID})
	if err == nil && n == 0 {
		err = problem.New("That is not one of your wants.")
	}
	s.done(w, r, err)
}

// setTradeTarget marks another manager's player as one this manager wants,
// so his owner knows, or takes the mark off.
func (s *Server) setTradeTarget(w http.ResponseWriter, r *http.Request, me db.Franchise) {
	var body struct {
		LeagueID pgtype.UUID `json:"league_id"`
		PlayerID pgtype.UUID `json:"player_id"`
		On       bool        `json:"on"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	key := db.SetTradeTargetParams{FranchiseID: me.ID, LeagueID: body.LeagueID, PlayerID: body.PlayerID}
	if !body.On {
		s.done(w, r, s.Queries.RemoveTradeTarget(r.Context(), db.RemoveTradeTargetParams(key)))
		return
	}
	owner, err := s.marketPlayer(r, me, body.LeagueID, body.PlayerID)
	switch {
	case err != nil:
		s.fail(w, r, err)
		return
	case owner == me.ID:
		s.fail(w, r, problem.New("That player is already yours."))
		return
	}
	s.done(w, r, s.Queries.SetTradeTarget(r.Context(), key))
}

// marketNote is a trade-market note, trimmed and kept short.
func marketNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 280 {
		return "", problem.New("Keep the note to 280 characters.")
	}
	return note, nil
}
