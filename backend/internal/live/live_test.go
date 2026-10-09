package live_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/dbtest"
	"crossover/internal/hub"
	"crossover/internal/ingest"
	"crossover/internal/live"
)

// feed is a scoreboard the test changes between polls.
type feed struct{ game ingest.Game }

func (f *feed) Games(context.Context, time.Time) ([]ingest.Game, error) {
	return []ingest.Game{f.game}, nil
}

func (f *feed) BoxScore(context.Context, ingest.Game) ([]ingest.StatLine, error) {
	return nil, nil
}

// TestPollPushes checks the whole live path: a poll finds a change in the
// feed, stores it, and pushes the scoreboard to a browser that is watching.
func TestPollPushes(t *testing.T) {
	pool := dbtest.Open(t)
	ctx := context.Background()
	const key = "test_push"
	cleanup := func() {
		pool.Exec(ctx, `delete from games where competition = $1`, key)
		pool.Exec(ctx, `delete from pro_teams where competition = $1`, key)
		pool.Exec(ctx, `delete from ingest_runs where competition = $1`, key)
	}
	cleanup()
	defer cleanup()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// A game is only followed when one of its teams is in the competition.
	if _, err := pool.Exec(ctx, `insert into pro_teams (competition, provider_id, abbrev, name) values ($1, 'home', 'HOM', 'Home')`, key); err != nil {
		t.Fatal(err)
	}
	src := &feed{game: ingest.Game{ProviderID: "g", HomeTeam: "home", StartsAt: time.Now().Add(-time.Minute), Status: ingest.GameScheduled}}
	syncer := ingest.NewSyncer(db.New(pool), log)
	if err := syncer.SyncGames(ctx, key, src, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	service := live.NewService(pool, syncer, competition.Registry{{Key: key, Games: src}}, hub.New(), log)

	watching, leave := service.WatchScores(key)
	defer leave()

	// Nothing has changed, so nothing is pushed.
	service.Poll(ctx, key)
	select {
	case message := <-watching:
		t.Fatalf("pushed with nothing new: %s", message)
	default:
	}

	// The game starts and someone scores.
	src.game.Status, src.game.HomeScore, src.game.Detail = ingest.GameLive, 3, "12:00 - 1st"
	service.Poll(ctx, key)
	select {
	case message := <-watching:
		var board struct {
			Competition string
			Games       []struct {
				Status, Detail string
				HomeScore      int `json:"home_score"`
			}
		}
		if err := json.Unmarshal(message, &board); err != nil {
			t.Fatal(err)
		}
		if board.Competition != key || len(board.Games) != 1 || board.Games[0].Status != "live" ||
			board.Games[0].HomeScore != 3 || board.Games[0].Detail != "12:00 - 1st" {
			t.Errorf("pushed scoreboard = %s", message)
		}
	default:
		t.Fatal("the change was not pushed to the watcher")
	}
}
