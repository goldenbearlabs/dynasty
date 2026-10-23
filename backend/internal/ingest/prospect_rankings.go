package ingest

import (
	"context"
	"errors"
	"fmt"

	"crossover/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Syncer) SyncProspectRankings(ctx context.Context, competition string, src ProspectRankingSource) error {
	return s.run(ctx, competition, "prospect_rankings", func(pgtype.Timestamptz) (int, error) { return s.syncProspectRankings(ctx, competition, src) })
}

func (s *Syncer) syncProspectRankings(ctx context.Context, competition string, src ProspectRankingSource) (int, error) {
	list, err := src.ProspectRankings(ctx)
	if err != nil {
		return 0, err
	}
	if list.Source == "" || list.URL == "" || list.Year < 1900 || list.Year > 2200 || len(list.Rankings) == 0 {
		return 0, fmt.Errorf("prospect ranking list is incomplete")
	}
	seenRanks, seenIDs := map[int]bool{}, map[string]bool{}
	for _, r := range list.Rankings {
		key := r.Player.Provider + "/" + r.Player.ProviderID
		if r.Rank < 1 || r.Rank > 100 || r.Player.Provider == "" || r.Player.ProviderID == "" || seenRanks[r.Rank] || seenIDs[key] {
			return 0, fmt.Errorf("prospect ranking contains an invalid or duplicate rank/player ID")
		}
		seenRanks[r.Rank], seenIDs[key] = true, true
	}
	err = s.q.Tx(ctx, func(q *db.Queries) error {
		if err := q.LockProspectRankingSource(ctx, list.Source); err != nil {
			return err
		}
		previous, err := q.ProspectRankingYear(ctx, list.Source)
		if err != nil {
			return err
		}
		if previous > int32(list.Year) {
			return fmt.Errorf("refusing to replace a newer prospect ranking with %d", list.Year)
		}

		if err := q.DeleteProspectRankingSource(ctx, list.Source); err != nil {
			return err
		}
		seenPlayers := map[pgtype.UUID]bool{}
		for _, r := range list.Rankings {
			id, err := find(ctx, q, r.Player)
			if errors.Is(err, pgx.ErrNoRows) {
				if err := upsertProspect(ctx, q, competition, r.Player); err != nil {
					return err
				}
				id, err = find(ctx, q, r.Player)
			}
			if err != nil {
				return err
			}
			player, err := q.GetPlayer(ctx, id)
			if err != nil {
				return err
			}
			if player.Competition != competition || seenPlayers[id] {
				return fmt.Errorf("prospect ranking player ID does not uniquely match this competition")
			}
			seenPlayers[id] = true
			if err := q.InsertProspectRanking(ctx, db.InsertProspectRankingParams{PlayerID: id, Source: list.Source, Year: int32(list.Year), Rank: int32(r.Rank), Eta: r.ETA, SourceUrl: list.URL}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(list.Rankings), nil
}
