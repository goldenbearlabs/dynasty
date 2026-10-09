// Crossover Dynasty: one binary serving the API, the embedded frontend and
// the scheduled feed syncs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"crossover/internal/auth"
	"crossover/internal/cache"
	"crossover/internal/competition"
	"crossover/internal/db"
	"crossover/internal/draft"
	"crossover/internal/dynasty"
	"crossover/internal/hub"
	"crossover/internal/ingest"
	"crossover/internal/lineup"
	"crossover/internal/live"
	"crossover/internal/players"
	"crossover/internal/roster"
	"crossover/internal/scoring"
	"crossover/internal/trade"
	"crossover/internal/waiver"
	"crossover/internal/web"
	"crossover/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://crossover:crossover@localhost:5433/crossover"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrations.Up(pool); err != nil {
		return err
	}

	var responseCache *cache.Store
	if url := os.Getenv("REDIS_URL"); url != "" {
		client, err := cache.Connect(url)
		if err != nil {
			return fmt.Errorf("REDIS_URL: %w", err)
		}
		defer client.Close()
		responseCache = cache.New(client, pool, log)
		responseCache.RevisionAge = envDuration("CACHE_REVISION_AGE", time.Second)
		log.Info("Redis response caching configured")
	}
	queries := db.New(pool)
	accounts, err := auth.NewService(ctx, pool)
	if err != nil {
		return err
	}
	client := ingest.NewClient(envDuration("FETCH_GAP", time.Second))
	// FETCH_VIA routes feeds through relays: "host=https://relay,host=https://relay".
	client.Via = map[string]string{}
	for _, pair := range strings.Split(os.Getenv("FETCH_VIA"), ",") {
		if host, relay, ok := strings.Cut(strings.TrimSpace(pair), "="); ok {
			client.Via[host] = strings.TrimRight(relay, "/")
		}
	}
	client.Save = func(ctx context.Context, url string, body []byte) {
		if err := queries.SaveRawPayload(ctx, db.SaveRawPayloadParams{Url: url, Body: string(body)}); err != nil {
			log.Warn("save raw payload", "url", url, "err", err)
		}
	}
	registry := competition.NewRegistry(client)
	syncer := ingest.NewSyncer(queries, log)
	rosters := roster.NewService(pool)
	// A player who turns up in a new competition takes his fantasy rights
	// with him, where the league's rules say so.
	syncer.OnMove = func(ctx context.Context, playerID pgtype.UUID, from, to string) {
		if err := rosters.Graduate(ctx, playerID, from, to); err != nil {
			log.Error("graduate player", "player", playerID, "from", from, "to", to, "err", err)
		}
	}
	events := hub.New()
	events.OnPublish = responseCache.Forget
	scores := scoring.NewService(pool)
	games := live.NewService(pool, syncer, registry, events, log)
	games.Interval = envDuration("LIVE_POLL", 30*time.Second)
	go games.Run(ctx)

	// Players come off waivers a minute or less after their time is up.
	waivers := waiver.NewService(pool, log)
	go waivers.Run(ctx, time.Minute)
	// Rookie-draft picks not signed in time are released, checked as often.
	go func() {
		for tick := time.NewTicker(time.Minute); ; {
			if n, err := rosters.ReleaseUnsigned(ctx); err != nil && ctx.Err() == nil {
				log.Error("release unsigned picks", "err", err)
			} else if n > 0 {
				log.Info("released unsigned picks", "players", n)
			}
			select {
			case <-ctx.Done():
				tick.Stop()
				return
			case <-tick.C:
			}
		}
	}()

	schedule := cron.New()
	// Rosters refresh daily and prospects weekly, one competition after another.
	schedule.AddFunc(env("SYNC_CRON", "0 4 * * *"), func() {
		for _, c := range registry {
			syncer.SyncRosters(ctx, c.Key, c.Source)
		}
	})
	schedule.AddFunc(env("PROSPECT_SYNC_CRON", "0 5 * * 1"), func() {
		for _, c := range registry {
			if c.Prospects != nil {
				syncer.SyncProspects(ctx, c.Key, c.Prospects)
			}
		}
	})
	// The live loop above follows games while they are played. This slower
	// sync keeps the schedule for the days around today current and catches
	// what the loop does not: games that finished unseen, and stat
	// corrections the day after. It runs only for sports someone here plays.
	syncGames := func(from, to time.Time) {
		played, err := queries.ListLeagueCompetitions(ctx)
		if err != nil {
			log.Error("list league competitions", "err", err)
			return
		}
		for _, key := range played {
			if c, ok := registry.Get(key); ok {
				syncer.SyncGames(ctx, c.Key, c.Games, from, to)
				games.Announce(ctx, c.Key)
			}
		}
		// A playoff round may have come due.
		if err := scores.AdvancePlayoffs(ctx); err != nil {
			log.Error("advance playoffs", "err", err)
		}
	}
	schedule.AddFunc(env("SCORES_SYNC_CRON", "*/15 * * * *"), func() { syncGames(scoring.Window()) })
	// Without this a fresh server would show no games until the first run above.
	go syncGames(scoring.Window())
	// Stat history: every player's season totals, refreshed with the nightly
	// sync. The first run for a sport fetches years of it; later runs only
	// keep the last two seasons current.
	backfill := envInt("SEASONS_BACKFILL", 10)
	schedule.AddFunc(env("SYNC_CRON", "0 4 * * *"), func() {
		for _, c := range registry {
			syncer.SyncSeasons(ctx, c.Key, c.Seasons, backfill)
		}
	})
	// A sport with no history yet gets it now and not at four in the morning.
	go func() {
		for _, c := range registry {
			if years, err := queries.ListSeasonYears(ctx, c.Key); err == nil && len(years) == 0 {
				syncer.SyncSeasons(ctx, c.Key, c.Seasons, backfill)
			}
		}
	}()
	schedule.Start()
	defer schedule.Stop()

	drafts := draft.NewService(pool, events, log)
	go drafts.RunClock(ctx)

	playerService := players.NewService(pool, registry)
	playerService.Cache = responseCache
	server := &web.Server{
		Cache:          responseCache,
		Queries:        queries,
		Auth:           accounts,
		Registry:       registry,
		Syncer:         syncer,
		Dynasty:        dynasty.NewService(pool, registry),
		Roster:         rosters,
		Players:        playerService,
		Drafts:         drafts,
		Trades:         trade.NewService(pool),
		Waivers:        waivers,
		Lineups:        lineup.NewService(pool),
		Scoring:        scores,
		Live:           games,
		Log:            log,
		Ping:           pool.Ping,
		Background:     ctx,
		AdminToken:     os.Getenv("ADMIN_TOKEN"),
		SeasonBackfill: backfill,
	}
	httpServer := &http.Server{
		Addr:              env("ADDR", ":8090"),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", httpServer.Addr)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return d
	}
	return fallback
}
