// Package waiver runs waivers: the wait a dropped player serves before he
// can be added, the claims franchises put in for him, and the award when
// his time is up. Whether a league has waivers, for how long, and whether
// claims are ordered by priority or by bid are league settings.
package waiver

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/roster"
	"crossover/internal/settings"
)

type Service struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

// Standing is one franchise's place in a league's waiver order.
type Standing struct {
	FranchiseID pgtype.UUID `json:"franchise_id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Priority    int         `json:"priority"`    // 1 claims first
	BudgetLeft  int         `json:"budget_left"` // FAAB only
}

// View is a league's waivers as one franchise sees them. Claims are the
// viewer's own: nobody sees another franchise's claims or bids.
type View struct {
	Rules        settings.Waivers            `json:"rules"`
	WeeklyLimit  int                         `json:"weekly_limit"` // 0 means no limit
	Acquisitions int                         `json:"acquisitions"` // the viewer's, this week
	Players      []db.ListWaiversRow         `json:"players"`
	Standings    []Standing                  `json:"standings"`
	Claims       []db.ListFranchiseClaimsRow `json:"claims"`
}

func (s *Service) View(ctx context.Context, league db.League, viewer *db.Franchise) (View, error) {
	q := db.New(s.pool)
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return View{}, err
	}
	view := View{Rules: rules.Waivers, WeeklyLimit: rules.FreeAgency.WeeklyLimit, Claims: []db.ListFranchiseClaimsRow{}}
	if view.Players, err = q.ListWaivers(ctx, league.ID); err != nil {
		return View{}, err
	}
	if view.Standings, err = standings(ctx, q, league, rules.Waivers); err != nil {
		return View{}, err
	}
	if viewer == nil {
		return view, nil
	}
	if view.Acquisitions, err = roster.Acquisitions(ctx, q, rules, league.ID, viewer.ID); err != nil {
		return View{}, err
	}
	claims, err := q.ListFranchiseClaims(ctx, db.ListFranchiseClaimsParams{LeagueID: league.ID, FranchiseID: viewer.ID})
	if claims != nil {
		view.Claims = claims
	}
	return view, err
}

// standings lists the league's franchises in waiver order.
func standings(ctx context.Context, q *db.Queries, league db.League, rules settings.Waivers) ([]Standing, error) {
	rows, err := q.ListWaiverStandings(ctx, db.ListWaiverStandingsParams{LeagueID: league.ID, DynastyID: league.DynastyID})
	if err != nil {
		return nil, err
	}
	list := make([]Standing, len(rows))
	for i, r := range rows {
		list[i] = Standing{FranchiseID: r.FranchiseID, Name: r.Name, Slug: r.Slug, Priority: i + 1}
		if rules.Mode == settings.WaiversFAAB {
			list[i].BudgetLeft = rules.Budget - int(r.Spent)
		}
	}
	return list, nil
}

// Claim is a request for a player on waivers.
type Claim struct {
	PlayerID     pgtype.UUID `json:"player_id"`
	DropPlayerID pgtype.UUID `json:"drop_player_id"` // optional: released if the claim wins
	Bid          int         `json:"bid"`            // FAAB only
}

// Claim puts in, or replaces, a franchise's claim for a player on waivers.
// Whether it can be afforded and fits the roster is settled when the
// player clears, since both may change before then.
func (s *Service) Claim(ctx context.Context, league db.League, franchise db.Franchise, c Claim) error {
	q := db.New(s.pool)
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	if _, err := q.GetWaiver(ctx, db.GetWaiverParams{LeagueID: league.ID, PlayerID: c.PlayerID}); errors.Is(err, pgx.ErrNoRows) {
		return problem.New("That player is not on waivers.")
	} else if err != nil {
		return err
	}
	if rules.Waivers.Mode != settings.WaiversFAAB {
		c.Bid = 0
	}
	if c.Bid < 0 {
		return problem.New("A bid cannot be negative.")
	}
	order, err := standings(ctx, q, league, rules.Waivers)
	if err != nil {
		return err
	}
	if left := budgetLeft(order, franchise.ID); c.Bid > left {
		return problem.New("That bid is more than the %d this franchise has left.", left)
	}
	if c.DropPlayerID.Valid {
		entry, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: league.ID, PlayerID: c.DropPlayerID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && entry.FranchiseID != franchise.ID) {
			return problem.New("The player to drop is not on this roster.")
		}
		if err != nil {
			return err
		}
	}
	return q.PutWaiverClaim(ctx, db.PutWaiverClaimParams{
		LeagueID: league.ID, FranchiseID: franchise.ID,
		PlayerID: c.PlayerID, DropPlayerID: c.DropPlayerID, Bid: int32(c.Bid),
	})
}

// Cancel withdraws a claim that has not been settled.
func (s *Service) Cancel(ctx context.Context, franchise db.Franchise, claimID pgtype.UUID) error {
	n, err := db.New(s.pool).CancelWaiverClaim(ctx, db.CancelWaiverClaimParams{ID: claimID, FranchiseID: franchise.ID})
	if err == nil && n == 0 {
		return problem.New("That claim has already been settled.")
	}
	return err
}

// Run settles waivers as they come due, until ctx ends.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if err := s.Process(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("process waivers", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Process settles every player whose time on waivers is up, earliest first,
// so that a claim won moves its franchise back before the next is decided.
func (s *Service) Process(ctx context.Context) error {
	due, err := db.New(s.pool).ListDueWaivers(ctx)
	if err != nil {
		return err
	}
	for _, w := range due {
		if err := db.InTx(ctx, s.pool, func(q *db.Queries) error { return settle(ctx, q, w) }); err != nil {
			return err
		}
	}
	return nil
}

// settle awards one player to the best claim that can actually be carried
// out, and takes him off waivers either way.
func settle(ctx context.Context, q *db.Queries, w db.Waiver) error {
	league, err := q.GetLeague(ctx, w.LeagueID)
	if err != nil {
		return err
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	claims, err := q.ListPendingClaims(ctx, db.ListPendingClaimsParams{LeagueID: w.LeagueID, PlayerID: w.PlayerID})
	if err != nil {
		return err
	}
	order, err := standings(ctx, q, league, rules.Waivers)
	if err != nil {
		return err
	}
	priority := func(c db.WaiverClaim) int {
		return slices.IndexFunc(order, func(s Standing) bool { return s.FranchiseID == c.FranchiseID })
	}
	// Highest bid first (bids are all zero without FAAB), then waiver order.
	slices.SortStableFunc(claims, func(a, b db.WaiverClaim) int {
		return cmp.Or(cmp.Compare(b.Bid, a.Bid), cmp.Compare(priority(a), priority(b)))
	})

	awarded := false
	for _, claim := range claims {
		result, reason := "lost", "Another franchise's claim came first."
		if !awarded {
			err := q.Savepoint(ctx, func(q *db.Queries) error { return award(ctx, q, league, claim, budgetLeft(order, claim.FranchiseID)) })
			var refused problem.Error
			switch {
			case err == nil:
				awarded, result, reason = true, "won", ""
			case errors.As(err, &refused):
				reason = refused.Error()
			default:
				return err
			}
		}
		if err := q.ResolveWaiverClaim(ctx, db.ResolveWaiverClaimParams{ID: claim.ID, Status: result, Reason: reason}); err != nil {
			return err
		}
	}
	return q.DeleteWaiver(ctx, db.DeleteWaiverParams{LeagueID: w.LeagueID, PlayerID: w.PlayerID})
}

// award carries out a claim: the drop that makes room, then the add, under
// the same roster rules as any other acquisition.
func award(ctx context.Context, q *db.Queries, league db.League, claim db.WaiverClaim, budget int) error {
	if int(claim.Bid) > budget {
		return problem.New("The bid of %d was more than the %d left in the budget.", claim.Bid, budget)
	}
	franchise, err := q.GetFranchise(ctx, claim.FranchiseID)
	if err != nil {
		return err
	}
	change := roster.Change{League: league, Franchise: franchise, Source: roster.Waivers}
	if claim.DropPlayerID.Valid {
		change.PlayerID = claim.DropPlayerID
		if err := roster.DropIn(ctx, q, change); err != nil {
			return err
		}
	}
	change.PlayerID, change.List, change.Bid = claim.PlayerID, settings.ListMain, int(claim.Bid)
	return roster.AddIn(ctx, q, change)
}

// budgetLeft is a franchise's remaining FAAB budget; zero without FAAB,
// which is also every bid then.
func budgetLeft(order []Standing, franchiseID pgtype.UUID) int {
	for _, s := range order {
		if s.FranchiseID == franchiseID {
			return s.BudgetLeft
		}
	}
	return 0
}
