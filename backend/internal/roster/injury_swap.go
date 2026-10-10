package roster

import (
	"context"
	"crossover/internal/db"
	"crossover/internal/lineup"
	"crossover/internal/problem"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
)

// InjurySwap makes both list changes together; neither list needs a free slot.
// The call-up bypasses a normal reserve lock, but never a previous injury swap.
func (s *Service) InjurySwap(ctx context.Context, c Change, replacement pgtype.UUID) error {
	c.List = settings.ListReserve
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		return apply(ctx, q, c, "move", true, func(rules settings.League, entries []Entry) ([]Entry, error) {
			injured, called := index(entries, c.PlayerID), index(entries, replacement)
			if injured < 0 || called < 0 || c.PlayerID == replacement || entries[injured].List != settings.ListMain || entries[called].List != settings.ListReserve {
				return nil, problem.New("Choose an injured player on your main roster and a replacement from your reserve list.")
			}
			season, err := q.InjurySwapSeason(ctx, db.InjurySwapSeasonParams{LeagueID: c.League.ID, Day: sportsday.Date(sportsday.Today())})
			if err != nil {
				return nil, problem.New("An injury swap requires a season in progress.")
			}
			player, err := q.GetPlayer(ctx, c.PlayerID)
			if err != nil {
				return nil, err
			}
			switch player.InjuryDesignation {
			case "IR", "OUT", "IL7", "IL10", "IL15", "IL60":
			default:
				return nil, problem.New("Only a player with an IR, injured-list or Out designation can use an injury swap.")
			}
			if entries[injured].InjuryReserveLocked || entries[called].InjuryReserveLocked {
				return nil, problem.New("A player locked to reserve by an injury swap cannot be called up again this season.")
			}
			if err := q.InsertInjuryReserveLock(ctx, db.InsertInjuryReserveLockParams{SeasonID: season.ID, PlayerID: c.PlayerID, FranchiseID: c.Franchise.ID, ReplacementID: replacement}); err != nil {
				return nil, err
			}
			if err := q.SetRosterList(ctx, db.SetRosterListParams{LeagueID: c.League.ID, FranchiseID: c.Franchise.ID, PlayerID: c.PlayerID, List: settings.ListReserve}); err != nil {
				return nil, err
			}
			if err := q.SetRosterList(ctx, db.SetRosterListParams{LeagueID: c.League.ID, FranchiseID: c.Franchise.ID, PlayerID: replacement, List: settings.ListMain}); err != nil {
				return nil, err
			}
			entries[injured].List, entries[injured].InjuryReserveLocked = settings.ListReserve, true
			entries[called].List, entries[called].Rookie = settings.ListMain, false
			if err := lineup.Bench(ctx, q, c.League.ID, c.Franchise.ID, c.PlayerID); err != nil {
				return nil, err
			}
			detail, _ := json.Marshal(map[string]any{"list": "main", "injury_swap_for": c.PlayerID.String()})
			if err := q.InsertTransaction(ctx, db.InsertTransactionParams{DynastyID: c.Franchise.DynastyID, LeagueID: c.League.ID, FranchiseID: c.Franchise.ID, Kind: "move", PlayerID: replacement, Detail: detail}); err != nil {
				return nil, err
			}
			return entries, nil
		})
	})
}
