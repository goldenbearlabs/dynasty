package lineup

import (
	"context"
	"time"

	"crossover/internal/db"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

// Projected is what a franchise is projected to add from today to the end
// of a span: each starter's projected points in his games still to come
// that will count, under the lineup in force on each day. What has been
// played already is in the score, not here.
func (s *Service) Projected(ctx context.Context, league db.League, franchiseID pgtype.UUID, from, to time.Time) (float64, error) {
	if today := sportsday.Today(); from.Before(today) {
		from = today
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return 0, err
	}
	first, last := from.Format(time.DateOnly), to.Format(time.DateOnly)
	total := 0.0
	for day := from; !day.After(to); {
		view, err := s.Get(ctx, league, franchiseID, day)
		if err != nil {
			return 0, err
		}
		mult := 1
		if view.Doubled != nil {
			mult = 2
		}
		for _, p := range view.Players {
			if p.Slot != "" {
				total += projectedStarter(p, gamesCounted(rules.Lineup, p.Slot)*mult, first, last)
			}
		}
		end, err := time.Parse(time.DateOnly, view.LastDay)
		if err != nil {
			return 0, err
		}
		day = end.AddDate(0, 0, 1) // the next day, or the next week
	}
	return total, nil
}

// projectedStarter adds up a starter's projected points in his games to
// come between two days. A slot that counts only limit games counts as
// many as it has left: the ones picked, or on auto his last.
func projectedStarter(p Player, limit int, first, last string) float64 {
	var ahead []Game
	used := 0
	for _, g := range p.Games {
		switch {
		case g.Status != "scheduled":
			if g.Counts {
				used++
			}
		case g.Day >= first && g.Day <= last && (limit == 0 || p.CountsLast || g.Counts):
			ahead = append(ahead, g)
		}
	}
	if limit > 0 {
		left := max(0, limit-used)
		if p.CountsLast {
			ahead = ahead[max(0, len(ahead)-left):]
		} else {
			ahead = ahead[:min(left, len(ahead))]
		}
	}
	total := 0.0
	for _, g := range ahead {
		total += g.Projected
	}
	return total
}
