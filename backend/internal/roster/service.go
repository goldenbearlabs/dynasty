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
	Startup   bool        // a startup-draft pick, which may sit on reserve whatever the rule
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
		injuryLocked, err := q.HasInjuryReserveLock(ctx, db.HasInjuryReserveLockParams{LeagueID: c.League.ID, PlayerID: c.PlayerID, Day: sportsday.Date(sportsday.Today())})
		if err != nil {
			return nil, err
		}
		if injuryLocked && c.List != settings.ListReserve {
			return nil, problem.New("This player must remain on reserve for the rest of the season after an injury swap.")
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
			Startup:     c.Startup,
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
		return append(roster, Entry{PlayerID: c.PlayerID, List: c.List, Prospect: player.Status == "prospect", Startup: c.Startup, InjuryReserveLocked: injuryLocked, StarterIneligible: !rules.CanStart(c.League.Competition, conference)}), nil
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
		// Whoever is released, from whichever list, waits on waivers first.
		if err := waive(ctx, q, rules, c.League.ID, c.PlayerID); err != nil {
			return nil, err
		}
		return slices.Delete(roster, i, i+1), lineup.Bench(ctx, q, c.League.ID, c.Franchise.ID, c.PlayerID)
	})
}

// waive puts a released player on waivers for as long as the league's rule
// says, in the caller's transaction. A league without waivers skips it.
func waive(ctx context.Context, q *db.Queries, rules settings.League, leagueID, playerID pgtype.UUID) error {
	if rules.Waivers.Mode == settings.WaiversNone {
		return nil
	}
	clears := time.Now().Add(time.Duration(rules.Waivers.Hours) * time.Hour)
	return q.PutOnWaivers(ctx, db.PutOnWaiversParams{
		LeagueID: leagueID, PlayerID: playerID, ClearsAt: pgtype.Timestamptz{Time: clears, Valid: true},
	})
}

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
			league, err := q.GetLeague(ctx, e.LeagueID)
			if err != nil {
				return err
			}
			rules, err := settings.Parse[settings.League](league.Settings)
			if err != nil {
				return err
			}
			if err := waive(ctx, q, rules, e.LeagueID, e.PlayerID); err != nil {
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
	return db.InTx(ctx, s.pool, func(q *db.Queries) error { return moveIn(ctx, q, c, true) })
}

// Set declares a franchise's whole reserve list at once: the players named
// go to the reserve list and everyone else it has signed goes to the main
// roster. Each move obeys the reserve lock, but the limits are checked only
// on the result, so two full lists can swap players.
func (s *Service) Set(ctx context.Context, c Change, reserve []pgtype.UUID) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		rules, err := settings.Parse[settings.League](c.League.Settings)
		if err != nil {
			return err
		}
		before, err := entries(ctx, q, c, rules)
		if err != nil {
			return err
		}
		for _, id := range reserve {
			if i := index(before, id); i < 0 || before[i].List == settings.ListRights {
				return problem.New("One of those players is not signed to this roster.")
			}
		}
		for _, e := range before {
			list := settings.ListMain
			if slices.Contains(reserve, e.PlayerID) {
				list = settings.ListReserve
			}
			if e.List == settings.ListRights || e.List == list {
				continue
			}
			c.PlayerID, c.List = e.PlayerID, list
			if err := moveIn(ctx, q, c, false); err != nil {
				return err
			}
		}
		if c.Source == Commissioner {
			return nil
		}
		after, err := entries(ctx, q, c, rules)
		if err != nil {
			return err
		}
		return Check(rules.Roster, before, after)
	})
}

// moveIn is one move inside a transaction the caller owns. checked says
// whether the roster limits apply to it alone.
func moveIn(ctx context.Context, q *db.Queries, c Change, checked bool) error {
	// Signing a rookie-draft pick is a move too, recorded under its own name.
	kind := "move"
	if held, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: c.League.ID, PlayerID: c.PlayerID}); err == nil && held.List == settings.ListRights {
		kind = "sign"
	}
	return apply(ctx, q, c, kind, checked, func(rules settings.League, roster []Entry) ([]Entry, error) {
		i := index(roster, c.PlayerID)
		if i < 0 {
			return nil, problem.New("That player is not on this roster.")
		}
		entry, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: c.League.ID, PlayerID: c.PlayerID})
		if err != nil {
			return nil, err
		}
		if roster[i].InjuryReserveLocked && c.List != settings.ListReserve {
			return nil, problem.New("This player used an injury swap and must remain on reserve for the rest of the season.")
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
			if managed {
				if err := callable(ctx, q, c, rules.Roster, entry.ReservedAt); err != nil {
					return nil, err
				}
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
}

// callable refuses a call-up from the reserve list while a lock holds.
func callable(ctx context.Context, q *db.Queries, c Change, limits settings.Roster, reservedAt pgtype.Timestamptz) error {
	ends, err := SeasonEnds(ctx, q, c.League.ID)
	if err != nil {
		return err
	}
	locked, err := q.HasInjuryReserveLock(ctx, db.HasInjuryReserveLockParams{LeagueID: c.League.ID, PlayerID: c.PlayerID, Day: sportsday.Date(sportsday.Today())})
	if err != nil {
		return err
	}
	if locked {
		return problem.New("This player used an injury swap and must remain on reserve for the rest of the season.")
	}
	until := LockedUntil(limits, reservedAt, ends)
	if !time.Now().Before(until) {
		return nil
	}
	player, err := q.GetPlayer(ctx, c.PlayerID)
	if err != nil {
		return err
	}
	return problem.New("%s is locked on the reserve list until %s.", player.FullName, sportsday.Clock(until))
}

// SeasonEnds is the last day of the season a league is playing, or the zero
// time between seasons.
func SeasonEnds(ctx context.Context, q *db.Queries, leagueID pgtype.UUID) (time.Time, error) {
	ends, err := q.RunningSeasonEnd(ctx, db.RunningSeasonEndParams{LeagueID: leagueID, Day: sportsday.Date(sportsday.Today())})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	return ends.Time, err
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

	before, err := entries(ctx, q, c, rules)
	if err != nil {
		return err
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

// entries locks the franchise's roster until the transaction ends and
// returns it as the limits see it.
func entries(ctx context.Context, q *db.Queries, c Change, rules settings.League) ([]Entry, error) {
	if err := q.LockFranchiseRoster(ctx, db.LockFranchiseRosterParams{
		LeagueID: c.League.ID.String(), FranchiseID: c.Franchise.ID.String(),
	}); err != nil {
		return nil, err
	}
	rows, err := q.ListRosterEntries(ctx, db.ListRosterEntriesParams{LeagueID: c.League.ID, FranchiseID: c.Franchise.ID})
	if err != nil {
		return nil, err
	}
	roster := make([]Entry, len(rows))
	for i, r := range rows {
		roster[i] = Entry{PlayerID: r.PlayerID, List: r.List, Prospect: r.Status == "prospect", Rookie: r.Rookie, Startup: r.Startup, InjuryReserveLocked: r.InjuryReserveLocked, StarterIneligible: !rules.CanStart(c.League.Competition, r.Conference)}
	}
	return roster, nil
}

// reservedNow is the reserve lock's starting time: now when it starts, and
// empty when it does not.
func reservedNow(starts bool) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: starts}
}

func index(roster []Entry, playerID pgtype.UUID) int {
	return slices.IndexFunc(roster, func(e Entry) bool { return e.PlayerID == playerID })
}
