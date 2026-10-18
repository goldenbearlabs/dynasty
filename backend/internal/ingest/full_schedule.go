package ingest

import (
	"context"
	"time"

	"crossover/internal/sportsday"
)

// ScheduleHorizon covers at least a full year so every sport's published
// regular season is available for matchup planning. Longer fantasy seasons
// and explicitly larger horizons extend it further.
func ScheduleHorizon(today time.Time, days int, seasonEnd time.Time) time.Time {
	end := today.AddDate(0, 0, max(365, days))
	if seasonEnd.After(end) {
		end = seasonEnd
	}
	return end
}

// SyncFullSchedule shares the same horizon across startup, nightly jobs,
// commissioner refreshes and command-line backfills. It imports game
// schedules; completed games' player statistics keep their separate sync.
func (s *Syncer) SyncFullSchedule(ctx context.Context, competition string, src GameSource, from time.Time, days int) error {
	end, err := s.q.ActiveScheduleEnd(ctx, competition)
	if err != nil {
		return err
	}
	return s.SyncSchedule(ctx, competition, src, from, ScheduleHorizon(sportsday.Today(), days, end.Time))
}
