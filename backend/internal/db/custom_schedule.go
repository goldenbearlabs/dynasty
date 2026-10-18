package db

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
)

// LockSchedule serializes commissioner schedule changes for a season.
func (q *Queries) LockSchedule(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `select id from seasons where id=$1 for update`, id)
	return err
}

func (q *Queries) HasCustomSchedule(ctx context.Context, id pgtype.UUID) (bool, error) {
	var custom bool
	err := q.db.QueryRow(ctx, `select exists(select 1 from custom_schedules where season_id=$1)`, id).Scan(&custom)
	return custom, err
}

func (q *Queries) SetCustomSchedule(ctx context.Context, id pgtype.UUID, custom bool) error {
	if custom {
		_, err := q.db.Exec(ctx, `insert into custom_schedules(season_id) values($1) on conflict do nothing`, id)
		return err
	}
	_, err := q.db.Exec(ctx, `delete from custom_schedules where season_id=$1`, id)
	return err
}

// ClearSeasonSchedule removes periods and their dependent matchups/scores.
func (q *Queries) ClearSeasonSchedule(ctx context.Context, id pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `delete from periods where season_id=$1`, id)
	return err
}
