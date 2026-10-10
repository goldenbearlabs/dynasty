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
			names := map[string][]pgtype.UUID{}
			known := map[pgtype.UUID]bool{}
			for _, p := range players {
				names[injuryName(p.FullName)] = append(names[injuryName(p.FullName)], p.ID)
				known[p.ID] = true
			}
			updates := map[pgtype.UUID]string{}
			for _, entry := range entries {
				var id pgtype.UUID
				if entry.ProviderID != "" {
					id, _ = q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: entry.Provider, ProviderID: entry.ProviderID})
				}
				if !known[id] {
					// NHL/MLB use different providers. Only an unambiguous exact normalized name can bridge them.
					candidates := names[injuryName(entry.FullName)]
					if len(candidates) != 1 {
						continue
					}
					id = candidates[0]
				}
				updates[id] = entry.Designation
			}
			if err := q.ClearInjuryDesignations(ctx, competition); err != nil {
				return err
			}
			for id, designation := range updates {
				if err := q.SetInjuryDesignation(ctx, db.SetInjuryDesignationParams{ID: id, InjuryDesignation: designation}); err != nil {
					return err
				}
				matched++
			}
			return nil
		})
		return matched, err
	})
}

func injuryName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, norm.NFD.String(name))
}
