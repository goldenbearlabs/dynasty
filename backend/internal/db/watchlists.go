package db

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
)

type WatchlistPlayer struct {
	PlayerID    pgtype.UUID `json:"player_id"`
	FullName    string      `json:"full_name"`
	Competition string      `json:"competition"`
	Positions   []string    `json:"positions"`
	HeadshotURL string      `json:"headshot_url"`
	TeamName    string      `json:"team_name"`
	Status      string      `json:"status"`
	OwnerName   string      `json:"owner_name"`
	OwnerSlug   string      `json:"owner_slug"`
	Category    string      `json:"category"`
	Notes       string      `json:"notes"`
}

func (q *Queries) Watchlist(ctx context.Context, franchise pgtype.UUID) ([]WatchlistPlayer, error) {
	rows, err := q.db.Query(ctx, `select w.player_id,p.full_name,p.competition,p.positions,p.headshot_url,
 coalesce(t.name,''),p.status,coalesce(f.name,''),coalesce(f.slug,''),w.category,w.notes
 from watchlist_entries w join players p on p.id=w.player_id
 left join pro_teams t on t.id=p.pro_team_id
 left join roster_entries r on r.player_id=p.id
 left join franchises f on f.id=r.franchise_id
 where w.franchise_id=$1 order by w.priority,w.created_at,w.player_id`, franchise)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WatchlistPlayer{}
	for rows.Next() {
		var e WatchlistPlayer
		if err := rows.Scan(&e.PlayerID, &e.FullName, &e.Competition, &e.Positions, &e.HeadshotURL, &e.TeamName, &e.Status, &e.OwnerName, &e.OwnerSlug, &e.Category, &e.Notes); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (q *Queries) AddWatchlistPlayer(ctx context.Context, franchise, player pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `insert into watchlist_entries(franchise_id,player_id,priority)
 values ($1,$2,coalesce((select max(priority)+1 from watchlist_entries where franchise_id=$1),0))
 on conflict (franchise_id,player_id) do nothing`, franchise, player)
	return err
}
func (q *Queries) RemoveWatchlistPlayer(ctx context.Context, franchise, player pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `delete from watchlist_entries where franchise_id=$1 and player_id=$2`, franchise, player)
	return err
}
func (q *Queries) EditWatchlistPlayer(ctx context.Context, franchise, player pgtype.UUID, category, notes string) (bool, error) {
	tag, err := q.db.Exec(ctx, `update watchlist_entries set category=$3,notes=$4 where franchise_id=$1 and player_id=$2`, franchise, player, category, notes)
	return tag.RowsAffected() == 1, err
}

// A full permutation prevents stale clients from overwriting additions or removals.
func (q *Queries) OrderWatchlist(ctx context.Context, franchise pgtype.UUID, ids []pgtype.UUID) (bool, error) {
	var ok bool
	err := q.db.QueryRow(ctx, `with supplied as (select id,ord from unnest($2::uuid[]) with ordinality as x(id,ord)),
 valid as (select count(*)=(select count(*) from watchlist_entries where franchise_id=$1)
 and count(*)=count(distinct id) and count(*)=count(w.player_id) as ok
 from supplied left join watchlist_entries w on w.franchise_id=$1 and w.player_id=supplied.id),
 changed as (update watchlist_entries w set priority=s.ord from supplied s,valid v
 where v.ok and w.franchise_id=$1 and w.player_id=s.id returning w.player_id)
 select ok from valid`, franchise, ids).Scan(&ok)
	return ok, err
}
func (q *Queries) MoveWatchlistPlayers(ctx context.Context, keep, duplicate pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `insert into watchlist_entries(franchise_id,player_id,priority,category,notes,created_at)
 select franchise_id,$1,priority,category,notes,created_at from watchlist_entries where player_id=$2
 on conflict(franchise_id,player_id) do nothing`, keep, duplicate)
	return err
}
