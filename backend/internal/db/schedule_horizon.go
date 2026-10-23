package db

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
)

// ActiveScheduleEnd includes commissioner seasons that extend beyond the
// normal calendar horizon, including seasons that cross New Year's Day.
func (q *Queries) ActiveScheduleEnd(ctx context.Context, competition string) (pgtype.Date, error) {
	var end pgtype.Date
	err := q.db.QueryRow(ctx, `select max(s.ends_on)::date from seasons s join leagues l on l.id=s.league_id where l.competition=$1 and s.status='active'`, competition).Scan(&end)
	return end, err
}
