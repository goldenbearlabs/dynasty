package roster

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/lineup"
	"crossover/internal/problem"
	"crossover/internal/settings"
)

// Graduate handles a rostered player moving from one competition to
// another, as a college player does on reaching the NBA. In a league whose
// rules carry players into the new competition he moves to the same
// franchise's roster there; otherwise he is released, to be drafted afresh.
// Roster limits are not applied: a franchise pushed over them must make room.
func (s *Service) Graduate(ctx context.Context, playerID pgtype.UUID, from, to string) error {
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		entries, err := q.ListPlayerRosterEntries(ctx, playerID)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			league, err := q.GetLeague(ctx, entry.LeagueID)
			if err != nil {
				return err
			}
			if league.Competition != from {
				continue
			}
			if err := graduate(ctx, q, league, entry, to); err != nil {
				return err
			}
		}
		return nil
	})
}

// CarryOver is the commissioner graduating a player by hand: someone who
// left college without appearing in the next league's feed, but whose
// franchise keeps his rights. He joins the destination's pool as a prospect.
func (s *Service) CarryOver(ctx context.Context, league db.League, playerID pgtype.UUID) error {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	if rules.Continuity == nil {
		return problem.New("%s does not carry players over into another league.", league.Name)
	}
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		entry, err := q.GetRosterEntry(ctx, db.GetRosterEntryParams{LeagueID: league.ID, PlayerID: playerID})
		if errors.Is(err, pgx.ErrNoRows) {
			return problem.New("That player is not on a roster in %s.", league.Name)
		}
		if err != nil {
			return err
		}
		if err := q.MovePlayerCompetition(ctx, db.MovePlayerCompetitionParams{ID: playerID, Competition: rules.Continuity.Into}); err != nil {
			return err
		}
		return graduate(ctx, q, league, entry, rules.Continuity.Into)
	})
}

// graduate moves one roster entry out of its league: on to the league the
// rules name, or off the roster altogether.
func graduate(ctx context.Context, q *db.Queries, league db.League, entry db.RosterEntry, to string) error {
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return err
	}
	log := func(leagueID pgtype.UUID, kind string, detail map[string]any) error {
		raw, _ := json.Marshal(detail)
		return q.InsertTransaction(ctx, db.InsertTransactionParams{
			DynastyID: league.DynastyID, LeagueID: leagueID, FranchiseID: entry.FranchiseID,
			Kind: kind, PlayerID: entry.PlayerID, Detail: raw,
		})
	}

	if err := lineup.Bench(ctx, q, league.ID, entry.FranchiseID, entry.PlayerID); err != nil {
		return err
	}
	if _, err := q.DeleteRosterEntry(ctx, db.DeleteRosterEntryParams{
		LeagueID: league.ID, FranchiseID: entry.FranchiseID, PlayerID: entry.PlayerID,
	}); err != nil {
		return err
	}

	var destination db.League
	if rules.Continuity != nil && rules.Continuity.Into == to {
		destination, err = q.GetLeagueByCompetition(ctx, db.GetLeagueByCompetitionParams{DynastyID: league.DynastyID, Competition: to})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if !destination.ID.Valid {
		return log(league.ID, "released", map[string]any{"reason": "moved to " + to})
	}

	// Each attempt is a savepoint: if someone already has him in the
	// destination league, he is simply released from this one.
	err = q.Savepoint(ctx, func(q *db.Queries) error {
		return q.InsertRosterEntry(ctx, db.InsertRosterEntryParams{
			LeagueID: destination.ID, FranchiseID: entry.FranchiseID, PlayerID: entry.PlayerID,
			List: rules.Continuity.LandOn, AcquiredVia: "graduated",
		})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return log(league.ID, "released", map[string]any{"reason": "already rostered in " + destination.Name})
	}
	if err != nil {
		return err
	}
	return log(destination.ID, "graduated", map[string]any{"list": rules.Continuity.LandOn, "from": league.Name})
}
