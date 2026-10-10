// Package trade moves players and draft picks between franchises.
//
// A trade is a list of items, each one asset going from one franchise to
// another. Every player item names the league he is rostered in, so a single
// trade can span sports with no special handling. Nothing here judges
// whether a trade is fair.
package trade

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/db"
	"crossover/internal/lineup"
	"crossover/internal/problem"
	"crossover/internal/roster"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Item is one asset changing hands: a rostered player (with his league) or
// a draft pick.
type Item struct {
	From     pgtype.UUID `json:"from_franchise_id"`
	To       pgtype.UUID `json:"to_franchise_id"`
	LeagueID pgtype.UUID `json:"league_id"`
	PlayerID pgtype.UUID `json:"player_id"`
	PickID   pgtype.UUID `json:"draft_pick_id"`
}

type Offer struct {
	Note  string `json:"note"`
	Items []Item `json:"items"`
}

// Propose records an offer. The proposer has accepted by proposing; every
// other franchise in it must accept before anything moves.
func (s *Service) Propose(ctx context.Context, proposer db.Franchise, offer Offer) (db.Trade, error) {
	if len(offer.Items) == 0 {
		return db.Trade{}, problem.New("A trade needs at least one player or pick.")
	}
	var parties []pgtype.UUID
	seen := map[Item]bool{}
	for _, item := range offer.Items {
		asset := Item{LeagueID: item.LeagueID, PlayerID: item.PlayerID, PickID: item.PickID}
		switch {
		case item.From == item.To:
			return db.Trade{}, problem.New("An asset cannot be traded to the franchise that has it.")
		case item.PlayerID.Valid == item.PickID.Valid, item.PlayerID.Valid != item.LeagueID.Valid:
			return db.Trade{}, problem.New("Each item is either a player with his league, or a draft pick.")
		case seen[asset]:
			return db.Trade{}, problem.New("The same asset is in this trade twice.")
		}
		seen[asset] = true
		for _, id := range []pgtype.UUID{item.From, item.To} {
			if !slices.Contains(parties, id) {
				parties = append(parties, id)
			}
		}
	}
	if !slices.Contains(parties, proposer.ID) {
		return db.Trade{}, problem.New("You can only propose trades your franchise is part of.")
	}

	var trade db.Trade
	err := db.InTx(ctx, s.pool, func(q *db.Queries) (err error) {
		// Refuse now what could never go through; roster limits are left
		// for acceptance, since rosters can change before then.
		if _, err := inspect(ctx, q, proposer.DynastyID, offer.Items, false); err != nil {
			return err
		}
		trade, err = q.CreateTrade(ctx, db.CreateTradeParams{
			DynastyID: proposer.DynastyID, ProposedBy: proposer.ID, Note: strings.TrimSpace(offer.Note),
		})
		if err != nil {
			return err
		}
		for _, id := range parties {
			accepted := pgtype.Timestamptz{Time: time.Now(), Valid: id == proposer.ID}
			if err := q.AddTradeParty(ctx, db.AddTradePartyParams{TradeID: trade.ID, FranchiseID: id, AcceptedAt: accepted}); err != nil {
				return err
			}
		}
		for _, item := range offer.Items {
			if err := q.AddTradeItem(ctx, db.AddTradeItemParams{
				TradeID: trade.ID, FromFranchise: item.From, ToFranchise: item.To,
				LeagueID: item.LeagueID, PlayerID: item.PlayerID, DraftPickID: item.PickID,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return trade, err
}

// Accept records one franchise's agreement. When it is the last one needed,
// the trade executes at once, or waits for the commissioner in a league
// that requires approval.
func (s *Service) Accept(ctx context.Context, tradeID pgtype.UUID, franchise db.Franchise) error {
	return s.change(ctx, tradeID, func(q *db.Queries, trade db.Trade) error {
		if trade.Status != "proposed" {
			return problem.New("This trade is no longer open.")
		}
		parties, err := q.ListTradePartiesOf(ctx, tradeID)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(parties, func(p db.TradeParty) bool { return p.FranchiseID == franchise.ID })
		if i < 0 {
			return problem.New("Your franchise is not part of this trade.")
		}
		if err := q.AcceptTrade(ctx, db.AcceptTradeParams{TradeID: tradeID, FranchiseID: franchise.ID}); err != nil {
			return err
		}
		parties[i].AcceptedAt.Valid = true
		if slices.ContainsFunc(parties, func(p db.TradeParty) bool { return !p.AcceptedAt.Valid }) {
			return nil // still waiting on someone
		}

		items, err := items(ctx, q, tradeID)
		if err != nil {
			return err
		}
		needsApproval, err := inspect(ctx, q, trade.DynastyID, items, true)
		if err != nil {
			return err
		}
		if needsApproval {
			return q.SetTradeStatus(ctx, db.SetTradeStatusParams{ID: tradeID, Status: "accepted"})
		}
		return execute(ctx, q, trade, items, false)
	})
}

// Reject is any party turning the offer down.
func (s *Service) Reject(ctx context.Context, tradeID pgtype.UUID, franchise db.Franchise) error {
	return s.end(ctx, tradeID, "rejected", func(q *db.Queries, trade db.Trade) error {
		parties, err := q.ListTradePartiesOf(ctx, tradeID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(parties, func(p db.TradeParty) bool { return p.FranchiseID == franchise.ID }) {
			return problem.New("Your franchise is not part of this trade.")
		}
		return nil
	})
}

// Cancel is the proposer withdrawing the offer.
func (s *Service) Cancel(ctx context.Context, tradeID pgtype.UUID, franchise db.Franchise) error {
	return s.end(ctx, tradeID, "cancelled", func(_ *db.Queries, trade db.Trade) error {
		if trade.ProposedBy != franchise.ID {
			return problem.New("Only the franchise that proposed a trade can cancel it.")
		}
		return nil
	})
}

// Veto is the commissioner stopping a trade that has not executed.
func (s *Service) Veto(ctx context.Context, tradeID pgtype.UUID) error {
	return s.end(ctx, tradeID, "rejected", func(*db.Queries, db.Trade) error { return nil })
}

// end closes a trade that has not executed, once allowed says the caller may.
func (s *Service) end(ctx context.Context, tradeID pgtype.UUID, status string, allowed func(*db.Queries, db.Trade) error) error {
	return s.change(ctx, tradeID, func(q *db.Queries, trade db.Trade) error {
		if trade.Status != "proposed" && trade.Status != "accepted" {
			return problem.New("This trade is no longer open.")
		}
		if err := allowed(q, trade); err != nil {
			return err
		}
		return q.SetTradeStatus(ctx, db.SetTradeStatusParams{ID: tradeID, Status: status})
	})
}

// Approve is the commissioner letting an agreed trade go through.
func (s *Service) Approve(ctx context.Context, tradeID pgtype.UUID) error {
	return s.change(ctx, tradeID, func(q *db.Queries, trade db.Trade) error {
		if trade.Status != "accepted" {
			return problem.New("Only a trade every side has accepted can be approved.")
		}
		items, err := items(ctx, q, tradeID)
		if err != nil {
			return err
		}
		if _, err := inspect(ctx, q, trade.DynastyID, items, true); err != nil {
			return err
		}
		return execute(ctx, q, trade, items, false)
	})
}

// Reverse is the commissioner undoing an executed trade: every asset goes
// back, provided it is still where the trade left it. Roster limits are not
// applied; a franchise left over its limits must get back under them.
func (s *Service) Reverse(ctx context.Context, tradeID pgtype.UUID) error {
	return s.change(ctx, tradeID, func(q *db.Queries, trade db.Trade) error {
		if trade.Status != "executed" {
			return problem.New("Only an executed trade can be reversed.")
		}
		items, err := items(ctx, q, tradeID)
		if err != nil {
			return err
		}
		for i := range items {
			items[i].From, items[i].To = items[i].To, items[i].From
		}
		if err := owned(ctx, q, items, true); err != nil {
			return err
		}
		return execute(ctx, q, trade, items, true)
	})
}

// change runs fn with the trade row locked, so two responses to the same
// trade cannot interleave.
func (s *Service) change(ctx context.Context, tradeID pgtype.UUID, fn func(q *db.Queries, trade db.Trade) error) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		trade, err := q.LockTrade(ctx, tradeID)
		if err != nil {
			return err
		}
		return fn(q, trade)
	})
}

func items(ctx context.Context, q *db.Queries, tradeID pgtype.UUID) ([]Item, error) {
	rows, err := q.ListTradeItemsOf(ctx, tradeID)
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(rows))
	for i, r := range rows {
		items[i] = Item{From: r.FromFranchise, To: r.ToFranchise, LeagueID: r.LeagueID, PlayerID: r.PlayerID, PickID: r.DraftPickID}
	}
	return items, nil
}

// ---- the rules ----

// inspect decides whether a set of items may be traded right now: everyone
// involved is in the dynasty, each asset is still held by the franchise
// giving it, no league's deadline has passed and, when withLimits is set,
// every roster is legal afterwards. It also reports whether any league
// involved wants the commissioner's approval.
func inspect(ctx context.Context, q *db.Queries, dynastyID pgtype.UUID, items []Item, withLimits bool) (needsApproval bool, err error) {
	franchises, err := q.ListFranchises(ctx, dynastyID)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		for _, id := range []pgtype.UUID{item.From, item.To} {
			if !slices.ContainsFunc(franchises, func(f db.Franchise) bool { return f.ID == id }) {
				return false, problem.New("A franchise in this trade is not in this dynasty.")
			}
		}
	}
	if withLimits {
		// Lock every roster involved, in a fixed order, before reading any.
		if err := lockRosters(ctx, q, items); err != nil {
			return false, err
		}
	}
	if err := owned(ctx, q, items, false); err != nil {
		return false, err
	}

	leagues, err := leaguesOf(ctx, q, items)
	if err != nil {
		return false, err
	}
	today := time.Now().Format(time.DateOnly)
	rules := map[pgtype.UUID]settings.League{}
	for _, league := range leagues {
		r, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return false, err
		}
		rules[league.ID] = r
		if r.Trades.Deadline != "" && today > r.Trades.Deadline {
			return false, problem.New("The %s trade deadline (%s) has passed.", league.Name, r.Trades.Deadline)
		}
		needsApproval = needsApproval || r.Trades.Approval == settings.ApprovalCommissioner
	}
	if !withLimits {
		return needsApproval, nil
	}

	// Check each roster the trade touches, league by league.
	for _, key := range rosterKeys(items) {
		rows, err := q.ListRosterEntries(ctx, db.ListRosterEntriesParams{LeagueID: key.league, FranchiseID: key.franchise})
		if err != nil {
			return false, err
		}
		league := leagues[slices.IndexFunc(leagues, func(l db.League) bool { return l.ID == key.league })]
		var before, after []roster.Entry
		for _, r := range rows {
			entry := roster.Entry{PlayerID: r.PlayerID, List: r.List, Prospect: r.Status == "prospect", Rookie: r.Rookie, Startup: r.Startup, InjuryReserveLocked: r.InjuryReserveLocked, StarterIneligible: !rules[key.league].CanStart(league.Competition, r.Conference)}
			before = append(before, entry)
			leaving := slices.ContainsFunc(items, func(i Item) bool {
				return i.PlayerID == r.PlayerID && i.LeagueID == key.league && i.From == key.franchise
			})
			if !leaving {
				after = append(after, entry)
			}
		}
		// An arriving player keeps the list he was on.
		for _, item := range items {
			if item.PlayerID.Valid && item.LeagueID == key.league && item.To == key.franchise {
				arriving, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: item.LeagueID, PlayerID: item.PlayerID})
				if err != nil {
					return false, err
				}
				player, err := q.GetPlayer(ctx, item.PlayerID)
				if err != nil {
					return false, err
				}
				conference, err := q.GetPlayerConference(ctx, item.PlayerID)
				if err != nil {
					return false, err
				}
				injuryLocked, err := q.HasInjuryReserveLock(ctx, db.HasInjuryReserveLockParams{LeagueID: key.league, PlayerID: item.PlayerID, Day: sportsday.Date(sportsday.Today())})
				if err != nil {
					return false, err
				}
				after = append(after, roster.Entry{PlayerID: item.PlayerID, List: arriving.List, Prospect: player.Status == "prospect", Rookie: arriving.Rookie, Startup: arriving.Startup, InjuryReserveLocked: injuryLocked, StarterIneligible: !rules[key.league].CanStart(league.Competition, conference)})
			}
		}
		if err := roster.Check(rules[key.league].Roster, before, after); err != nil {
			league := leagues[slices.IndexFunc(leagues, func(l db.League) bool { return l.ID == key.league })]
			franchise := franchises[slices.IndexFunc(franchises, func(f db.Franchise) bool { return f.ID == key.franchise })]
			return false, problem.New("%s in %s: %s", franchise.Name, league.Name, err)
		}
	}
	return needsApproval, nil
}

// owned checks that each asset is still held by the franchise giving it. A
// pick must also be unused and, unless the trade is being reversed, in a
// draft that has not started.
func owned(ctx context.Context, q *db.Queries, items []Item, reversing bool) error {
	for _, item := range items {
		if item.PlayerID.Valid {
			entry, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: item.LeagueID, PlayerID: item.PlayerID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err != nil || entry.FranchiseID != item.From {
				player, _ := q.GetPlayer(ctx, item.PlayerID)
				return problem.New("%s is no longer on the roster that is trading him.", player.FullName)
			}
			continue
		}
		pick, err := q.LockDraftPick(ctx, item.PickID)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem.New("A draft pick in this trade no longer exists.")
		}
		if err != nil {
			return err
		}
		switch {
		case pick.CurrentFranchiseID != item.From:
			return problem.New("A draft pick in this trade has changed hands.")
		case pick.PlayerID.Valid:
			return problem.New("A draft pick in this trade has already been used.")
		case !reversing && pick.DraftStatus != "scheduled":
			return problem.New("Picks can only be traded before their draft starts.")
		}
	}
	return nil
}

// leaguesOf lists every league the items touch: a player's league, and the
// leagues a pick's draft covers.
func leaguesOf(ctx context.Context, q *db.Queries, items []Item) ([]db.League, error) {
	var leagues []db.League
	add := func(found ...db.League) {
		for _, league := range found {
			if !slices.ContainsFunc(leagues, func(l db.League) bool { return l.ID == league.ID }) {
				leagues = append(leagues, league)
			}
		}
	}
	for _, item := range items {
		if item.PlayerID.Valid {
			league, err := q.GetLeague(ctx, item.LeagueID)
			if err != nil {
				return nil, err
			}
			add(league)
			continue
		}
		covered, err := q.ListDraftPickLeagues(ctx, item.PickID)
		if err != nil {
			return nil, err
		}
		add(covered...)
	}
	return leagues, nil
}

type rosterKey struct{ league, franchise pgtype.UUID }

// rosterKeys lists each (league, franchise) roster the player items touch,
// in a fixed order so concurrent trades lock them the same way round.
func rosterKeys(items []Item) []rosterKey {
	var keys []rosterKey
	for _, item := range items {
		if !item.PlayerID.Valid {
			continue
		}
		for _, franchise := range []pgtype.UUID{item.From, item.To} {
			key := rosterKey{item.LeagueID, franchise}
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	slices.SortFunc(keys, func(a, b rosterKey) int {
		return strings.Compare(a.league.String()+a.franchise.String(), b.league.String()+b.franchise.String())
	})
	return keys
}

func lockRosters(ctx context.Context, q *db.Queries, items []Item) error {
	for _, key := range rosterKeys(items) {
		if err := q.LockFranchiseRoster(ctx, db.LockFranchiseRosterParams{
			LeagueID: key.league.String(), FranchiseID: key.franchise.String(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// execute moves every item to its new franchise and logs it. The caller has
// already checked the trade is allowed.
func execute(ctx context.Context, q *db.Queries, trade db.Trade, items []Item, reversing bool) error {
	for _, item := range items {
		var err error
		if item.PlayerID.Valid {
			// He stops starting for the franchise he is leaving.
			if err := lineup.Bench(ctx, q, item.LeagueID, item.From, item.PlayerID); err != nil {
				return err
			}
			err = q.SetRosterOwner(ctx, db.SetRosterOwnerParams{LeagueID: item.LeagueID, PlayerID: item.PlayerID, ToFranchise: item.To})
		} else {
			err = q.SetDraftPickOwner(ctx, db.SetDraftPickOwnerParams{ID: item.PickID, FranchiseID: item.To})
		}
		if err != nil {
			return err
		}

		from, err := q.GetFranchise(ctx, item.From)
		if err != nil {
			return err
		}
		detail, _ := json.Marshal(map[string]any{"from": from.Name, "reversed": reversing})
		if err := q.InsertTransaction(ctx, db.InsertTransactionParams{
			DynastyID: trade.DynastyID, LeagueID: item.LeagueID, FranchiseID: item.To, Kind: "trade",
			PlayerID: item.PlayerID, DraftPickID: item.PickID, TradeID: trade.ID, Detail: detail,
		}); err != nil {
			return err
		}
	}
	status := "executed"
	if reversing {
		status = "reversed"
	}
	return q.SetTradeStatus(ctx, db.SetTradeStatusParams{ID: trade.ID, Status: status})
}

// ---- reading ----

// View is a trade with its sides and items, described for display.
type View struct {
	db.Trade
	Parties []db.TradeParty        `json:"parties"`
	Items   []db.ListTradeItemsRow `json:"items"`
}

// List returns the trades a viewer may see: completed trades are public,
// and open or abandoned offers are shown only to their parties and to
// commissioners. viewer is nil for someone not signed in.
func (s *Service) List(ctx context.Context, dynastyID pgtype.UUID, viewer *db.Franchise) ([]View, error) {
	q := db.New(s.pool)
	trades, err := q.ListTrades(ctx, dynastyID)
	if err != nil {
		return nil, err
	}
	parties, err := q.ListTradeParties(ctx, dynastyID)
	if err != nil {
		return nil, err
	}
	items, err := q.ListTradeItems(ctx, dynastyID)
	if err != nil {
		return nil, err
	}

	views := []View{}
	for _, trade := range trades {
		view := View{Trade: trade, Parties: []db.TradeParty{}, Items: []db.ListTradeItemsRow{}}
		for _, p := range parties {
			if p.TradeID == trade.ID {
				view.Parties = append(view.Parties, p)
			}
		}
		for _, i := range items {
			if i.TradeID == trade.ID {
				view.Items = append(view.Items, i)
			}
		}
		public := trade.Status == "executed" || trade.Status == "reversed"
		involved := viewer != nil && (viewer.IsCommissioner ||
			slices.ContainsFunc(view.Parties, func(p db.TradeParty) bool { return p.FranchiseID == viewer.ID }))
		if public || involved {
			views = append(views, view)
		}
	}
	return views, nil
}
