package ingest

import (
	"context"
	"strings"
	"unicode"

	"crossover/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/unicode/norm"
)

type InjurySource interface {
	Injuries(context.Context) ([]Injury, error)
}
type Injury struct{ Provider, ProviderID, FullName, Designation string }

// Injury updates are atomic. A failed feed leaves the previous snapshot intact.
func (s *Syncer) SyncInjuries(ctx context.Context, competition string, src InjurySource) error {
	return s.run(ctx, competition, "injuries", func(pgtype.Timestamptz) (int, error) {
		entries, err := src.Injuries(ctx)
		if err != nil {
			return 0, err
		}
		matched := 0
		err = s.q.Tx(ctx, func(q *db.Queries) error {
			players, err := q.ListInjuryPlayers(ctx, competition)
			if err != nil {
				return err
			}
			match := playerMatcher(ctx, q, players)
			listed := map[pgtype.UUID]string{}
			for _, entry := range entries {
				if id, ok := match(entry.Provider, entry.ProviderID, entry.FullName); ok {
					listed[id] = entry.Designation
				}
			}
			matched = len(listed)
			// Only players whose designation changed are written, so a feed
			// read often does not keep emptying the cache.
			for _, p := range players {
				if listed[p.ID] != p.InjuryDesignation {
					if err := q.SetInjuryDesignation(ctx, db.SetInjuryDesignationParams{ID: p.ID, InjuryDesignation: listed[p.ID]}); err != nil {
						return err
					}
				}
			}
			return nil
		})
		return matched, err
	})
}

// playerMatcher finds a competition's player from another feed's entry: by
// the feed's id when he has one there, otherwise by an exact normalized
// name, which is how ESPN entries reach the NHL and MLB feeds' players.
func playerMatcher(ctx context.Context, q *db.Queries, players []db.ListInjuryPlayersRow) func(provider, providerID, name string) (pgtype.UUID, bool) {
	names := map[string][]pgtype.UUID{}
	known := map[pgtype.UUID]bool{}
	for _, p := range players {
		names[injuryName(p.FullName)] = append(names[injuryName(p.FullName)], p.ID)
		known[p.ID] = true
	}
	return func(provider, providerID, name string) (pgtype.UUID, bool) {
		if providerID != "" {
			if id, _ := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: provider, ProviderID: providerID}); known[id] {
				return id, true
			}
		}
		// Only an unambiguous name can bridge two feeds.
		if candidates := names[injuryName(name)]; len(candidates) == 1 {
			return candidates[0], true
		}
		return pgtype.UUID{}, false
	}
}

func injuryName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, norm.NFD.String(name))
}
