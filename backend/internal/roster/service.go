package roster

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"crossover/internal/db"
	"crossover/internal/lineup"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Source is how a roster change came about, which decides the rules it
// must pass.
type Source string

const (
	FreeAgency   Source = "free_agent"   // free agency rules and roster limits
	Waivers      Source = "waivers"      // a winning waiver claim: weekly limit and roster limits
	Draft        Source = "draft"        // roster limits only
	Commissioner Source = "commissioner" // no rules; the franchise must get back under its limits
)

// Change is one roster move for one franchise in one league.
type Change struct {
	League    db.League
	Franchise db.Franchise
	PlayerID  pgtype.UUID
	List      string      // destination list, for Add and Move
	Source    Source      // defaults to FreeAgency
	PickID    pgtype.UUID // the draft pick used, when Source is Draft
	Bid       int         // what the claim cost, when Source is Waivers
}

// Add puts an unrostered player on the franchise's roster.
func (s *Service) Add(ctx context.Context, c Change) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error { return AddIn(ctx, q, c) })
}

// AddIn is Add inside a transaction the caller owns, so that a draft pick
// and its roster entry commit together.
func AddIn(ctx context.Context, q *db.Queries, c Change) error {
	if c.Source == "" {
		c.Source = FreeAgency
	}
	kind := "add"
	switch c.Source {
	case Draft:
		kind = "pick"
	case Waivers:
		kind = "claim"
	}
	// A rookie-draft pick is held on no list, so no limit can refuse it: not
	// even for a franchise that is over its limits already.
	checked := c.List != settings.ListRights
	return apply(ctx, q, c, kind, checked, func(rules settings.League, roster []Entry) ([]Entry, error) {
		player, err := q.GetPlayer(ctx, c.PlayerID)
		if err != nil {
			return nil, err
		}
		if player.Competition != c.League.Competition {
			return nil, problem.New("%s does not play in this league's competition.", player.FullName)
		}
		if c.Source == FreeAgency {
			drafting, err := q.LeagueHasOpenDraft(ctx, c.League.ID)
			if err != nil {
				return nil, err
			}
			if drafting {
				return nil, problem.New("Free agency is paused while this league's draft is running.")
			}
			if err := Acquirable(rules.FreeAgency, c.League.LastDraftAt, player.EligibleSince.Time); err != nil {
				return nil, err
			}
			waiver, err := q.GetWaiver(ctx, db.GetWaiverParams{LeagueID: c.League.ID, PlayerID: c.PlayerID})
			if err == nil {
				return nil, problem.New("%s is on waivers until %s. Put in a claim for him instead.", player.FullName, sportsday.Clock(waiver.ClearsAt.Time))
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		}
		if c.Source == FreeAgency || c.Source == Waivers {
			if err := underWeeklyLimit(ctx, q, rules, c); err != nil {
				return nil, err
			}
		}

		err = q.InsertRosterEntry(ctx, db.InsertRosterEntryParams{
			LeagueID:    c.League.ID,
			FranchiseID: c.Franchise.ID,
			PlayerID:    c.PlayerID,
			List:        c.List,
			AcquiredVia: string(c.Source),
			ReservedAt:  reservedNow(c.Source == FreeAgency && c.List == settings.ListReserve && player.Status != "prospect"),
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique violation: first come, first served
			return nil, problem.New("%s is already on a roster in this league.", player.FullName)
		}
		if err != nil {
			return nil, err
		}
		conference, err := q.GetPlayerConference(ctx, c.PlayerID)
		if err != nil {
			return nil, err
		}
		return append(roster, Entry{PlayerID: c.PlayerID, List: c.List, Prospect: player.Status == "prospect", StarterIneligible: !rules.CanStart(c.League.Competition, conference)}), nil
	})
}

// Drop releases a player. It is always allowed. In a league with waivers he
// goes on them before anyone can add him.
func (s *Service) Drop(ctx context.Context, c Change) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error { return DropIn(ctx, q, c) })
}

// DropIn is Drop inside a transaction the caller owns.
func DropIn(ctx context.Context, q *db.Queries, c Change) error {
	return apply(ctx, q, c, "drop", false, func(rules settings.League, roster []Entry) ([]Entry, error) {
		i := index(roster, c.PlayerID)
		if i < 0 {
			return nil, problem.New("That player is not on this roster.")
		}
		if _, err := q.DeleteRosterEntry(ctx, db.DeleteRosterEntryParams{
			LeagueID: c.League.ID, FranchiseID: c.Franchise.ID, PlayerID: c.PlayerID,
		}); err != nil {
			return nil, err
		}
		// A released draft pick is on waivers for a day in every league; anyone
		// else goes on them for as long as the league's own rule says, if it has one.
		clears := time.Time{}
		switch {
		case roster[i].List == settings.ListRights:
			clears = time.Now().Add(ReleasedPickWaiver)
		case rules.Waivers.Mode != settings.WaiversNone:
			clears = time.Now().AddDate(0, 0, rules.Waivers.Days)
		}
		if !clears.IsZero() {
			if err := q.PutOnWaivers(ctx, db.PutOnWaiversParams{
				LeagueID: c.League.ID, PlayerID: c.PlayerID, ClearsAt: pgtype.Timestamptz{Time: clears, Valid: true},
			}); err != nil {
				return nil, err
			}
		}
		return slices.Delete(roster, i, i+1), lineup.Bench(ctx, q, c.League.ID, c.Franchise.ID, c.PlayerID)
	})
}

// ReleasedPickWaiver is how long a rookie-draft pick stays on waivers after
// it is released or left unsigned, before he becomes a free agent.
const ReleasedPickWaiver = 24 * time.Hour

// ReleaseUnsigned releases every rookie-draft pick whose time to be signed
// has run out, onto waivers. It reports how many were released.
func (s *Service) ReleaseUnsigned(ctx context.Context) (int, error) {
	expired, err := db.New(s.pool).ListExpiredRights(ctx)
	if err != nil {
		return 0, err
	}
	for _, e := range expired {
		err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
			if _, err := q.DeleteRosterEntry(ctx, db.DeleteRosterEntryParams{LeagueID: e.LeagueID, FranchiseID: e.FranchiseID, PlayerID: e.PlayerID}); err != nil {
				return err
			}
			if err := q.PutOnWaivers(ctx, db.PutOnWaiversParams{
				LeagueID: e.LeagueID, PlayerID: e.PlayerID, ClearsAt: pgtype.Timestamptz{Time: time.Now().Add(ReleasedPickWaiver), Valid: true},
			}); err != nil {
				return err
			}
			return q.InsertTransaction(ctx, db.InsertTransactionParams{
				DynastyID: e.DynastyID, LeagueID: e.LeagueID, FranchiseID: e.FranchiseID,
				Kind: "unsigned", PlayerID: e.PlayerID, Detail: json.RawMessage(`{}`),
			})
		})
		if err != nil {
			return 0, err
		}
	}
	return len(expired), nil
}

// Acquisitions counts the players a franchise has acquired for itself in
// the league's current week.
func Acquisitions(ctx context.Context, q *db.Queries, rules settings.League, leagueID, franchiseID pgtype.UUID) (int, error) {
	week := sportsday.Start(sportsday.WeekStart(sportsday.Today(), rules.Lineup.WeekStart))
	n, err := q.CountAcquisitions(ctx, db.CountAcquisitionsParams{
		LeagueID: leagueID, FranchiseID: franchiseID, Since: pgtype.Timestamptz{Time: week, Valid: true},
	})
	return int(n), err
}

func underWeeklyLimit(ctx context.Context, q *db.Queries, rules settings.League, c Change) error {
	limit := rules.FreeAgency.WeeklyLimit
	if limit == 0 {
		return nil
	}
	made, err := Acquisitions(ctx, q, rules, c.League.ID, c.Franchise.ID)
	if err != nil {
		return err
	}
	if made >= limit {
		return problem.New("This franchise has made all %d of its acquisitions for the week.", limit)
	}
	return nil
}

// Move switches a player between the main roster and the reserve list. A
// manager who sends a player other than a prospect to reserve starts the
// league's reserve lock, and cannot bring him back until it runs out.
func (s *Service) Move(ctx context.Context, c Change) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		// Signing a rookie-draft pick is a move too, recorded under its own name.
		kind := "move"
		if held, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: c.League.ID, PlayerID: c.PlayerID}); err == nil && held.List == settings.ListRights {
			kind = "sign"
		}
		return apply(ctx, q, c, kind, true, func(rules settings.League, roster []Entry) ([]Entry, error) {
			i := index(roster, c.PlayerID)
			if i < 0 {
				return nil, problem.New("That player is not on this roster.")
			}
			entry, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: c.League.ID, PlayerID: c.PlayerID})
			if err != nil {
				return nil, err
			}
			managed := c.Source != Commissioner
			reserved := entry.ReservedAt // unchanged unless he changes lists
			rookie := entry.Rookie
			switch {
			case c.List == entry.List:
			case entry.List == settings.ListRights:
				// Signing a rookie-draft pick, to either list if it has room. On
				// the reserve list he may stay whatever the league's rule for it.
				rookie = c.List == settings.ListReserve
			case c.List == settings.ListReserve:
				reserved = reservedNow(managed && !roster[i].Prospect)
			default:
				rookie = false // once on the main roster he is a rookie no longer
				if until := LockedUntil(rules.Roster, entry.ReservedAt); managed && time.Now().Before(until) {
					return nil, problem.New("This player is locked on the reserve list until %s.", sportsday.Clock(until))
				}
				reserved = pgtype.Timestamptz{}
			}

			roster[i].List, roster[i].Rookie = c.List, rookie
			if err := q.SetRosterList(ctx, db.SetRosterListParams{
				LeagueID: c.League.ID, FranchiseID: c.Franchise.ID, PlayerID: c.PlayerID, List: c.List, ReservedAt: reserved, Rookie: rookie,
			}); err != nil {
				return nil, err
			}
			if c.List == settings.ListReserve { // reserve players cannot start
				return roster, lineup.Bench(ctx, q, c.League.ID, c.Franchise.ID, c.PlayerID)
			}
			return roster, nil
		})
	})
}

// apply runs one roster change inside q's transaction: it locks the
// franchise's roster, lets edit make the change and report the resulting
// roster, checks the result against the limits, and logs it.
func apply(ctx context.Context, q *db.Queries, c Change, kind string, checked bool,
	edit func(rules settings.League, roster []Entry) ([]Entry, error)) error {

	// Only a rookie-draft pick arrives as rights; nobody is moved there.
	rights := c.List == settings.ListRights && c.Source == Draft && kind == "pick"
	if kind != "drop" && c.List != settings.ListMain && c.List != settings.ListReserve && !rights {
		return problem.New("List must be main or reserve.")
	}
	rules, err := settings.Parse[settings.League](c.League.Settings)
	if err != nil {
		return err
	}

	if err := q.LockFranchiseRoster(ctx, db.LockFranchiseRosterParams{
		LeagueID: c.League.ID.String(), FranchiseID: c.Franchise.ID.String(),
	}); err != nil {
		return err
	}
	rows, err := q.ListRosterEntries(ctx, db.ListRosterEntriesParams{LeagueID: c.League.ID, FranchiseID: c.Franchise.ID})
	if err != nil {
		return err
	}
	before := make([]Entry, len(rows))
	for i, r := range rows {
		before[i] = Entry{PlayerID: r.PlayerID, List: r.List, Prospect: r.Status == "prospect", Rookie: r.Rookie, StarterIneligible: !rules.CanStart(c.League.Competition, r.Conference)}
	}

	after, err := edit(rules, slices.Clone(before))
	if err != nil {
		return err
	}
	forced := c.Source == Commissioner
	if checked && !forced {
		if err := Check(rules.Roster, before, after); err != nil {
			return err
		}
	}

	detail, _ := json.Marshal(map[string]any{"list": c.List, "forced": forced, "bid": c.Bid})
	return q.InsertTransaction(ctx, db.InsertTransactionParams{
		DynastyID:   c.Franchise.DynastyID,
		LeagueID:    c.League.ID,
		FranchiseID: c.Franchise.ID,
		Kind:        kind,
		PlayerID:    c.PlayerID,
		DraftPickID: c.PickID,
		Detail:      detail,
	})
}

// reservedNow is the reserve lock's starting time: now when it starts, and
// empty when it does not.
func reservedNow(starts bool) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: starts}
}

func index(roster []Entry, playerID pgtype.UUID) int {
	return slices.IndexFunc(roster, func(e Entry) bool { return e.PlayerID == playerID })
}
