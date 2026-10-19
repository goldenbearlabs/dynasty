package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"crossover/internal/db"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
)

// PreviewSource reads what is known about a day's games before they start:
// the betting market's view of each and who is due to start in goal or on
// the mound.
type PreviewSource interface {
	Previews(ctx context.Context, day time.Time) ([]Preview, error)
}

type Preview struct {
	GameID   string // the feed's id for the game
	HomeTeam string // the home side's nickname ("Red Sox"), to match a game another feed supplied
	Odds     *Odds  // nil until a line is posted
	Starters []Starter
}

// Odds is the market's view of one game.
type Odds struct {
	HomeScore, AwayScore float64 // each side's implied score
	HomeWin              float64 // the home side's chance of winning; zero when unknown
}

type Starter struct {
	Provider, ProviderID, FullName string
	Confirmed                      bool // false while he is only expected
}

// SyncPreviews refreshes the odds and probable starters for each day from
// one to the other. Every day is read before any is written, so a failed
// feed leaves the last good view in place. Only the current line is kept.
func (s *Syncer) SyncPreviews(ctx context.Context, competition string, src PreviewSource, from, to time.Time) error {
	return s.run(ctx, competition, "previews", func(pgtype.Timestamptz) (int, error) {
		days := map[time.Time][]Preview{}
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			previews, err := src.Previews(ctx, day)
			if err != nil {
				return 0, fmt.Errorf("previews on %s: %w", day.Format(time.DateOnly), err)
			}
			days[day] = previews
		}
		stored := 0
		err := s.q.Tx(ctx, func(q *db.Queries) error {
			players, err := q.ListInjuryPlayers(ctx, competition)
			if err != nil {
				return err
			}
			match := playerMatcher(ctx, q, players)
			for day, previews := range days {
				date := sportsday.Date(day)
				games, err := q.ListPreviewGames(ctx, db.ListPreviewGamesParams{Competition: competition, Day: date})
				if err != nil {
					return err
				}
				odds := map[pgtype.UUID]*Odds{}
				for _, p := range previews {
					if id, ok := previewGame(games, p); ok {
						odds[id] = p.Odds
					}
				}
				for _, g := range games {
					o := odds[g.ID]
					arg := db.SetGameOddsParams{ID: g.ID}
					if o != nil {
						arg.HomeImplied = pgtype.Float8{Float64: o.HomeScore, Valid: true}
						arg.AwayImplied = pgtype.Float8{Float64: o.AwayScore, Valid: true}
						arg.HomeWin = pgtype.Float8{Float64: o.HomeWin, Valid: o.HomeWin > 0}
						stored++
					}
					if err := q.SetGameOdds(ctx, arg); err != nil {
						return err
					}
				}

				if err := q.ClearProbableStarters(ctx, db.ClearProbableStartersParams{Competition: competition, Day: date}); err != nil {
					return err
				}
				for _, p := range previews {
					for _, st := range p.Starters {
						id, ok := match(st.Provider, st.ProviderID, st.FullName)
						if !ok {
							continue
						}
						if err := q.SetProbableStarter(ctx, db.SetProbableStarterParams{
							Competition: competition, Day: date, PlayerID: id, Confirmed: st.Confirmed,
						}); err != nil {
							return err
						}
						stored++
					}
				}
			}
			return nil
		})
		return stored, err
	})
}

// previewGame finds the stored game a preview is about: by id where the
// games came from the same feed, otherwise by the one home side whose name
// ends in the preview's nickname.
func previewGame(games []db.ListPreviewGamesRow, p Preview) (pgtype.UUID, bool) {
	var found []pgtype.UUID
	for _, g := range games {
		if g.ProviderID == p.GameID {
			return g.ID, true
		}
		if nick := injuryName(p.HomeTeam); nick != "" && strings.HasSuffix(injuryName(g.HomeName), nick) {
			found = append(found, g.ID)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return pgtype.UUID{}, false
}
