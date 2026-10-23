package ingest_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/ingest"
	"github.com/jackc/pgx/v5/pgtype"
)

type rankingSource struct {
	list ingest.ProspectRankingList
	err  error
}

func (s *rankingSource) ProspectRankings(context.Context) (ingest.ProspectRankingList, error) {
	return s.list, s.err
}
func (s *rankingSource) Prospects(context.Context) ([]ingest.Player, error) { return nil, nil }

func TestProspectRankingsMatchIDsAndRefreshAtomically(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	const competition = "test_pipeline"
	var active, unrelated pgtype.UUID
	if err := tx.QueryRow(ctx, `insert into players(competition,status,full_name,positions,note) values($1,'active','Existing Name',array['SP'],'keep this note') returning id`, competition).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `insert into player_external_ids(provider,provider_id,player_id) values('pipeline_test','known',$1)`, active); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `insert into players(competition,status,full_name) values($1,'prospect','New Name') returning id`, competition).Scan(&unrelated); err != nil {
		t.Fatal(err)
	}

	source := &rankingSource{list: ingest.ProspectRankingList{
		Source: "mlb_pipeline", Year: 2026, URL: "https://www.mlb.com/milb/prospects/top100/",
		Rankings: []ingest.ProspectRanking{
			{Rank: 2, ETA: "2027", Player: ingest.Player{Provider: "pipeline_test", ProviderID: "known", FullName: "Changed Name", Positions: []string{"P"}, Note: "replace note"}},
			{Rank: 1, ETA: "2028", Player: ingest.Player{Provider: "pipeline_test", ProviderID: "new", FullName: "New Name", Positions: []string{"SS"}}},
		},
	}}

	syncer := ingest.NewSyncer(q, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// The normal prospect sync also refreshes published rankings.
	if err := syncer.SyncProspects(ctx, competition, source); err != nil {
		t.Fatal(err)
	}
	profile, err := q.GetPlayerProfile(ctx, active)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Status != "active" || profile.FullName != "Existing Name" || profile.Note != "keep this note" || profile.Positions[0] != "SP" || profile.ProspectRank.Int32 != 2 {
		t.Fatalf("existing identity changed: %+v", profile)
	}
	fresh, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: "pipeline_test", ProviderID: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if fresh == unrelated {
		t.Fatal("ranking matched an unrelated player by name")
	}
	rows, err := q.ListPlayers(ctx, db.ListPlayersParams{Competition: competition, SortBy: "prospect_rank", PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].ID != fresh || rows[1].ID != active || rows[2].ProspectRank.Valid {
		t.Fatalf("rank sorting = %+v", rows)
	}
	if err := syncer.SyncProspectRankings(ctx, competition, source); err != nil {
		t.Fatal(err)
	}
	again, err := q.FindPlayerByExternalID(ctx, db.FindPlayerByExternalIDParams{Provider: "pipeline_test", ProviderID: "new"})
	if err != nil || again != fresh {
		t.Fatal("refresh duplicated a player")
	}
	// Removing a player from the next snapshot clears their current badge.
	source.list.Rankings = source.list.Rankings[1:]
	source.list.Year = 2027
	if err := syncer.SyncProspectRankings(ctx, competition, source); err != nil {
		t.Fatal(err)
	}
	profile, err = q.GetPlayerProfile(ctx, active)
	if err != nil || profile.ProspectRank.Valid {
		t.Fatal("outdated rank was retained")
	}
	source.list.Year = 2026
	if err := syncer.SyncProspectRankings(ctx, competition, source); err == nil {
		t.Fatal("older ranking replaced the new year's list")
	}
	source.list.Year = 2027
	source.err = errors.New("feed unavailable")
	if err := syncer.SyncProspectRankings(ctx, competition, source); err == nil {
		t.Fatal("feed error ignored")
	}
	source.err = nil
	source.list.Rankings = append(source.list.Rankings, source.list.Rankings[0])
	if err := syncer.SyncProspectRankings(ctx, competition, source); err == nil {
		t.Fatal("duplicate ranking accepted")
	}
	profile, err = q.GetPlayerProfile(ctx, fresh)
	if err != nil || profile.ProspectRank.Int32 != 1 || profile.ProspectRankYear.Int32 != 2027 {
		t.Fatal("failed refresh lost the last good ranking")
	}
	// Ranking metadata survives the same move used by commissioner player merges.
	if err := q.MoveProspectRankings(ctx, db.MoveProspectRankingsParams{KeepID: unrelated, DuplicateID: fresh}); err != nil {
		t.Fatal(err)
	}
	profile, err = q.GetPlayerProfile(ctx, unrelated)
	if err != nil || profile.ProspectRank.Int32 != 1 {
		t.Fatal("merge lost ranking")
	}
}
