// Package draft runs drafts: building the pick order, taking picks, keeping
// the clock, and telling everyone in the room what changed.
//
// A startup draft and a seasonal draft are the same thing; they differ only
// in who is already on a roster and where a pick lands: a startup draft
// fills both lists, the main roster and the reserve list, and a rookie
// draft holds its picks as rights. A combined draft is the same thing
// covering more than one league.
package draft

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
	"crossover/internal/settings"
)

// NewDraft is what the commissioner chooses when creating a draft.
type NewDraft struct {
	Name             string        `json:"name"`
	Kind             string        `json:"kind"` // startup | seasonal
	Year             int           `json:"year"`
	LeagueIDs        []pgtype.UUID `json:"league_ids"`
	Rounds           int           `json:"rounds"`
	Order            string        `json:"order"`           // settings.OrderLinear | settings.OrderSnake
	FranchiseOrder   []pgtype.UUID `json:"franchise_order"` // first pick first; empty means by name
	PickClockSeconds int           `json:"pick_clock_seconds"`
}

// Slot is one pick in the order the commissioner arranges before a draft.
type Slot struct {
	ID                  pgtype.UUID `json:"id"` // empty for a pick being added
	Round               int32       `json:"round"`
	OriginalFranchiseID pgtype.UUID `json:"original_franchise_id"`
	CurrentFranchiseID  pgtype.UUID `json:"current_franchise_id"`
}

// Create builds a draft and generates its picks: every franchise picks once
// per round, in the given order, reversed on even rounds for a snake draft.
func (s *Service) Create(ctx context.Context, dynastyID pgtype.UUID, in NewDraft) (db.Draft, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return db.Draft{}, problem.New("The draft needs a name.")
	case in.Kind != "startup" && in.Kind != "seasonal":
		return db.Draft{}, problem.New("A draft is either a startup draft or a rookie draft.")
	case len(in.LeagueIDs) == 0:
		return db.Draft{}, problem.New("Pick at least one league to draft.")
	case in.Kind == "seasonal" && len(in.LeagueIDs) > 1:
		return db.Draft{}, problem.New("A rookie draft covers one league. Only a startup draft can combine leagues.")
	case in.Rounds < 1 || in.Rounds > 500:
		return db.Draft{}, problem.New("Rounds must be between 1 and 500.")
	case in.Order != settings.OrderLinear && in.Order != settings.OrderSnake:
		return db.Draft{}, problem.New("Order must be linear or snake.")
	case in.PickClockSeconds < 0:
		return db.Draft{}, problem.New("The pick clock cannot be negative.")
	}

	var draft db.Draft
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)

		// A franchise cannot draft more players than its rosters can hold.
		capacity := 0
		for _, id := range in.LeagueIDs {
			league, err := q.GetLeague(ctx, id)
			if err != nil || league.DynastyID != dynastyID {
				return problem.New("One of those leagues is not in this dynasty.")
			}
			if in.Kind == "startup" {
				// A league is stocked once; every draft after that is a rookie draft.
				held, err := q.StartupDraftExists(ctx, id)
				if err != nil {
					return err
				}
				if held {
					return problem.New("%s has already had its startup draft. Every draft after that is a rookie draft.", league.Name)
				}
			}
			rules, err := settings.Parse[settings.League](league.Settings)
			if err != nil {
				return err
			}
			capacity += rules.Roster.Main + rules.Roster.Reserve
		}
		if in.Rounds > capacity {
			return problem.New("%d rounds is more than the %d roster spots each franchise has to fill.", in.Rounds, capacity)
		}

		order, err := franchiseOrder(ctx, q, dynastyID, in.FranchiseOrder)
		if err != nil {
			return err
		}

		draft, err = q.CreateDraft(ctx, db.CreateDraftParams{
			DynastyID: dynastyID, Name: name, Kind: in.Kind, Year: int32(in.Year), PickClockSeconds: int32(in.PickClockSeconds),
		})
		if err != nil {
			return err
		}
		for _, id := range in.LeagueIDs {
			if err := q.AddDraftLeague(ctx, db.AddDraftLeagueParams{DraftID: draft.ID, LeagueID: id}); err != nil {
				return err
			}
		}

		position := int32(0)
		for round := 1; round <= in.Rounds; round++ {
			roundOrder := slices.Clone(order)
			if in.Order == settings.OrderSnake && round%2 == 0 {
				slices.Reverse(roundOrder)
			}
			for _, franchise := range roundOrder {
				position++
				if err := q.InsertDraftPick(ctx, db.InsertDraftPickParams{
					DraftID: draft.ID, Round: int32(round), Position: position,
					OriginalFranchiseID: franchise, CurrentFranchiseID: franchise,
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return draft, err
}

// franchiseOrder returns the first-round order: the given one if it names
// every franchise exactly once, or all franchises by name if none is given.
func franchiseOrder(ctx context.Context, q *db.Queries, dynastyID pgtype.UUID, given []pgtype.UUID) ([]pgtype.UUID, error) {
	franchises, err := q.ListFranchises(ctx, dynastyID)
	if err != nil {
		return nil, err
	}
	all := make([]pgtype.UUID, len(franchises))
	for i, f := range franchises {
		all[i] = f.ID
	}
	if len(given) == 0 {
		return all, nil
	}
	if len(given) != len(all) {
		return nil, problem.New("The draft order must list every franchise once.")
	}
	for _, id := range all {
		if !slices.Contains(given, id) {
			return nil, problem.New("The draft order must list every franchise once.")
		}
	}
	return given, nil
}

// SetPicks replaces a scheduled draft's pick order with the commissioner's
// arrangement: picks can be reassigned, reordered, added and removed. A pick
// keeps its id when it is kept, so anything pointing at it stays valid.
func (s *Service) SetPicks(ctx context.Context, draftID pgtype.UUID, slots []Slot) error {
	return s.change(ctx, draftID, true, func(q *db.Queries, draft db.Draft) error {
		if draft.Status != "scheduled" {
			return problem.New("The pick order can only be edited before the draft starts.")
		}
		if len(slots) == 0 {
			return problem.New("A draft needs at least one pick.")
		}
		franchises, err := q.ListFranchises(ctx, draft.DynastyID)
		if err != nil {
			return err
		}
		known := func(id pgtype.UUID) bool {
			return slices.ContainsFunc(franchises, func(f db.Franchise) bool { return f.ID == id })
		}
		existing, err := q.ListDraftPicks(ctx, draftID)
		if err != nil {
			return err
		}

		kept := map[pgtype.UUID]bool{}
		for i, slot := range slots {
			if slot.Round < 1 || !known(slot.OriginalFranchiseID) || !known(slot.CurrentFranchiseID) {
				return problem.New("Pick %d needs a round and a franchise.", i+1)
			}
			position := int32(i + 1)
			if !slot.ID.Valid {
				err = q.InsertDraftPick(ctx, db.InsertDraftPickParams{
					DraftID: draftID, Round: slot.Round, Position: position,
					OriginalFranchiseID: slot.OriginalFranchiseID, CurrentFranchiseID: slot.CurrentFranchiseID,
				})
			} else {
				kept[slot.ID] = true
				err = q.UpdateDraftPickSlot(ctx, db.UpdateDraftPickSlotParams{
					ID: slot.ID, DraftID: draftID, Round: slot.Round, Position: position,
					OriginalFranchiseID: slot.OriginalFranchiseID, CurrentFranchiseID: slot.CurrentFranchiseID,
				})
			}
			if err != nil {
				return err
			}
		}
		for _, pick := range existing {
			if !kept[pick.ID] {
				if err := q.DeleteDraftPick(ctx, db.DeleteDraftPickParams{ID: pick.ID, DraftID: draftID}); err != nil {
					return inTrade(err)
				}
			}
		}
		return nil
	})
}

// SetClock changes the seconds allowed per pick. It applies from the next pick.
func (s *Service) SetClock(ctx context.Context, draftID pgtype.UUID, seconds int) error {
	if seconds < 0 {
		return problem.New("The pick clock cannot be negative.")
	}
	return s.change(ctx, draftID, true, func(q *db.Queries, _ db.Draft) error {
		return q.SetDraftClock(ctx, db.SetDraftClockParams{ID: draftID, PickClockSeconds: int32(seconds)})
	})
}

// Delete removes a draft that has not started.
func (s *Service) Delete(ctx context.Context, draftID pgtype.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		draft, err := q.LockDraft(ctx, draftID)
		if err != nil {
			return err
		}
		if draft.Status != "scheduled" {
			return problem.New("Only a draft that has not started can be deleted.")
		}
		return inTrade(q.DeleteDraft(ctx, draftID))
	})
}

// CreateFuture creates each league's seasonal drafts for the coming years,
// as far ahead as its future_years rule says, so their picks exist and can
// be traded. Drafts that already exist are left alone. It reports how many
// it created.
func (s *Service) CreateFuture(ctx context.Context, dynastyID pgtype.UUID) (int, error) {
	q := db.New(s.pool)
	leagues, err := q.ListLeagues(ctx, dynastyID)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, league := range leagues {
		rules, err := settings.Parse[settings.League](league.Settings)
		if err != nil {
			return created, err
		}
		for year := time.Now().Year() + 1; year <= time.Now().Year()+rules.Draft.FutureYears; year++ {
			exists, err := q.SeasonalDraftExists(ctx, db.SeasonalDraftExistsParams{LeagueID: league.ID, Year: int32(year)})
			if err != nil {
				return created, err
			}
			if exists {
				continue
			}
			if _, err := s.Create(ctx, dynastyID, NewDraft{
				Name:             fmt.Sprintf("%d %s Draft", year, league.Name),
				Kind:             "seasonal",
				Year:             year,
				LeagueIDs:        []pgtype.UUID{league.ID},
				Rounds:           rules.RookieRounds(),
				Order:            rules.Draft.Order,
				PickClockSeconds: rules.Draft.PickClockSeconds,
			}); err != nil {
				return created, fmt.Errorf("%s %d: %w", league.Name, year, err)
			}
			created++
		}
	}
	return created, nil
}

// inTrade turns a foreign-key refusal into a readable one: a pick that is
// part of a trade cannot be removed.
func inTrade(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return problem.New("A pick in this draft is part of a trade. Settle or cancel that trade first.")
	}
	return err
}
