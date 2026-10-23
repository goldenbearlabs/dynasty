package web_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"crossover/internal/db"
	"crossover/internal/scoring"
	"crossover/internal/settings"
	"crossover/internal/sportsday"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The selected-game, Auto, and pitcher-cap fixtures also verify the detailed
// breakdown agrees with their independently asserted official totals.
func assertCountedGameTotals(t *testing.T, pool *pgxpool.Pool, leagueID string) {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	var id pgtype.UUID
	if err := id.Scan(leagueID); err != nil {
		t.Fatal(err)
	}
	league, err := q.GetLeague(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := settings.Parse[settings.League](league.Settings)
	if err != nil {
		t.Fatal(err)
	}
	season, err := q.LatestSeason(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.ListLineupPoints(ctx, db.ListLineupPointsParams{LeagueID: id, FromDay: season.StartsOn, ToDay: sportsday.Date(sportsday.Today()), WeekStart: sportsday.Date(sportsday.WeekStart(season.StartsOn.Time, rules.Lineup.WeekStart)), IncludeGames: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		var games []scoring.ScoredGame
		if err := json.Unmarshal(row.GameLines, &games); err != nil {
			t.Fatal(err)
		}
		total := 0.0
		seen := map[pgtype.UUID]bool{}
		for _, game := range games {
			if seen[game.GameID] || !game.GameID.Valid || game.Slot == "" || game.Day < season.StartsOn.Time.Format("2006-01-02") || len(game.Stats) == 0 {
				t.Fatalf("invalid or duplicate counted game: %+v", game)
			}
			seen[game.GameID] = true
			total += game.Points
		}
		if int64(len(games)) != row.Games || math.Abs(total-row.Points) > 0.00001 {
			t.Fatalf("%s breakdown totals %v in %d games, official %v in %d", row.FullName, total, len(games), row.Points, row.Games)
		}
	}
}
