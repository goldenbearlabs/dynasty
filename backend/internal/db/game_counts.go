package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// GameDayCount counts real games once per game, independent of fantasy
// rosters, pairings and the number of players appearing in their box scores.
type GameDayCount struct {
	Day   string `json:"day"`
	Games int64  `json:"games"`
}

func (q *Queries) ListGameCounts(ctx context.Context, competition string, from, to pgtype.Date) ([]GameDayCount, error) {
	rows, err := q.db.Query(ctx, `select to_char(day,'YYYY-MM-DD'), count(*) from games where competition=$1 and day between $2 and $3 group by day order by day`, competition, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := []GameDayCount{}
	for rows.Next() {
		var count GameDayCount
		if err := rows.Scan(&count.Day, &count.Games); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}
