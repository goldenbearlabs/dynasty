package lineup

import (
	"context"
	"time"

	"crossover/internal/db"
	"crossover/internal/projection"
	"crossover/internal/settings"
	"crossover/internal/sportsday"

	"github.com/jackc/pgx/v5/pgtype"
)

// Projected is what a franchise is projected to add from now to the end
// of a span, and how sure that is: each starter's projected points in his
// games still to come that will count, under the lineup in force on each
// day, and the rest of any under way. What has been played already is in
// the score, not here.
func (s *Service) Projected(ctx context.Context, league db.League, franchiseID pgtype.UUID, from, to time.Time) (projection.Outlook, error) {
	var outlook projection.Outlook
	if today := sportsday.Today(); from.Before(today) {
		from = today
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		return outlook, err
	}
	c, _ := s.registry.Get(league.Competition)
	first, last, now := from.Format(time.DateOnly), to.Format(time.DateOnly), time.Now()
	for day := from; !day.After(to); {
		view, err := s.Get(ctx, league, franchiseID, day)
		if err != nil {
			return outlook, err
		}
		mult := 1
		if view.Doubled != nil {
			mult = 2
		}
		for _, p := range view.Players {
			if p.Slot != "" {
				projectedStarter(&outlook, p, gamesCounted(rules.Lineup, p.Slot)*mult, first, last, c.Play, now)
			}
		}
		end, err := time.Parse(time.DateOnly, view.LastDay)
		if err != nil {
			return outlook, err
		}
		day = end.AddDate(0, 0, 1) // the next day, or the next week
	}
	return outlook, nil
}

// projectedStarter adds to a side's outlook a starter's games to come
// between two days, and what is left of one under way. A slot that counts
// only limit games counts as many as it has left: the ones picked, or on
// auto his last.
func projectedStarter(o *projection.Outlook, p Player, limit int, first, last string, play projection.Shape, now time.Time) {
	inSpan := func(g Game) bool { return g.Day >= first && g.Day <= last }
	// The rest of a game under way, unless he is out hurt and has not played in it.
	rest := func(g Game) {
		if !g.market.Injured || recorded(g.Stats) {
			o.Add(g.Expected, play.Left(g.detail, g.StartsAt, now), play.Spread)
		}
	}
	var ahead []Game
	used := 0
	for _, g := range p.Games {
		switch {
		case g.Status == "final":
			if g.Counts {
				used++
			}
		case g.Status == "live" && g.Counts && inSpan(g):
			used++ // what he has so far is in the score
			rest(g)
		case inSpan(g) && (limit == 0 || p.CountsLast || g.Counts):
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
	for _, g := range ahead {
		if g.Status == "live" {
			// On auto a game counts only once he has none left, so even what
			// he has so far is not in the score yet.
			o.Points += g.Points
			rest(g)
			continue
		}
		o.Add(g.Projected, 1, play.Spread)
	}
}
