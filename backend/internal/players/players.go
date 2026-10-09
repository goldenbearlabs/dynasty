// Package players holds the commissioner's tools for the player pool:
// adding people the feeds miss and merging two rows that are one person.
package players

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"crossover/internal/cache"
	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/problem"
)

type Service struct {
	pool           *pgxpool.Pool
	registry       competition.Registry
	Cache          *cache.Store
	analyticsSlots chan struct{}
	datasets       researchMemo
	loads          singleflight.Group
}

func NewService(pool *pgxpool.Pool, registry competition.Registry) *Service {
	return &Service{pool: pool, registry: registry, analyticsSlots: make(chan struct{}, 1)}
}

// NewPlayer is a hand-entered player: a recruit the feed missed, a
// teenager in Europe.
type NewPlayer struct {
	Competition string   `json:"competition"`
	FullName    string   `json:"full_name"`
	Positions   []string `json:"positions"`
	BirthDate   string   `json:"birth_date"` // YYYY-MM-DD or empty
	Note        string   `json:"note"`
	Status      string   `json:"status"` // defaults to prospect
}

// Add inserts every player or none.
func (s *Service) Add(ctx context.Context, list []NewPlayer) error {
	if len(list) == 0 {
		return problem.New("No players to add.")
	}
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		for i, p := range list {
			row := i + 1
			name := strings.TrimSpace(p.FullName)
			if name == "" {
				return problem.New("Row %d has no name.", row)
			}
			if _, ok := s.registry.Get(p.Competition); !ok {
				return problem.New("Row %d (%s): unknown sport %q.", row, name, p.Competition)
			}
			status := p.Status
			if status == "" {
				status = "prospect"
			}
			if !slices.Contains([]string{"prospect", "active", "inactive"}, status) {
				return problem.New("Row %d (%s): status must be prospect, active or inactive.", row, name)
			}
			var birth pgtype.Date
			if p.BirthDate != "" {
				t, err := time.Parse(time.DateOnly, p.BirthDate)
				if err != nil {
					return problem.New("Row %d (%s): birth date must look like 2008-05-17.", row, name)
				}
				birth = pgtype.Date{Time: t, Valid: true}
			}
			positions := p.Positions
			if positions == nil {
				positions = []string{}
			}

			if _, err := q.InsertManualPlayer(ctx, db.InsertManualPlayerParams{
				Competition: p.Competition,
				Status:      status,
				FullName:    name,
				Positions:   positions,
				BirthDate:   birth,
				Note:        strings.TrimSpace(p.Note),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Merge folds duplicate into keep: keep's own details stand, and it takes
// over duplicate's feed ids, roster spot and history.
func (s *Service) Merge(ctx context.Context, keepID, duplicateID pgtype.UUID) error {
	if keepID == duplicateID {
		return problem.New("Pick two different players to merge.")
	}
	err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
		keep, err := q.GetPlayer(ctx, keepID)
		if err != nil {
			return err
		}
		duplicate, err := q.GetPlayer(ctx, duplicateID)
		if err != nil {
			return err
		}
		if keep.Competition != duplicate.Competition {
			return problem.New("%s and %s are in different sports.", keep.FullName, duplicate.FullName)
		}

		ids := db.MoveExternalIDsParams{KeepID: keepID, DuplicateID: duplicateID}
		steps := []func() error{
			func() error { return q.MoveExternalIDs(ctx, ids) },
			func() error { return q.MoveRosterEntries(ctx, db.MoveRosterEntriesParams(ids)) },
			func() error { return q.MoveTransactions(ctx, db.MoveTransactionsParams(ids)) },
			func() error { return q.MoveTradeItems(ctx, db.MoveTradeItemsParams(ids)) },
			func() error { return q.MoveStatLines(ctx, db.MoveStatLinesParams(ids)) },
			func() error { return q.MoveLineupEntries(ctx, db.MoveLineupEntriesParams(ids)) },
			func() error { return q.MovePlayerSeasons(ctx, db.MovePlayerSeasonsParams(ids)) },
			func() error { return q.MovePlayerNicknames(ctx, db.MovePlayerNicknamesParams(ids)) },
			func() error { return q.KeepEarliestEligibility(ctx, db.KeepEarliestEligibilityParams(ids)) },
			func() error { return q.DeletePlayer(ctx, duplicateID) },
		}
		for _, step := range steps {
			if err := step(); err != nil {
				return err
			}
		}
		return nil
	})

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return problem.New("Both players are on rosters in the same league. Drop one, then merge.")
	}
	return err
}
