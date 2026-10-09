# Crossover Dynasty

A multi-sport dynasty fantasy league: Go, Postgres, SvelteKit, one server.
The full build plan is in [docs/crossover-dynasty.html](docs/crossover-dynasty.html).

## Run it locally

Needs Go, Node and Docker.

```sh
make db     # Postgres on localhost:5433
make api    # API on http://localhost:8090 (applies migrations on start)
make web    # frontend dev server on http://localhost:5173
```

Open http://localhost:5173. With no dynasty yet, the landing page offers to set
one up: name it, pick sports and rules, create your account, list the managers.
Whoever does this becomes the commissioner. The commissioner page then has an
invite link for every other manager; opening it lets them create their account.

After that the landing page is a sign-in form, and a signed-in manager is taken
straight to their team. Sign-in lasts 30 days and renews on every visit. A
forgotten password is fixed by the commissioner issuing a new link ("Reset
access").

Player data comes from the commissioner page ("Data feeds"), or from the shell:

```sh
make sync c=nba               # teams and players: cbb | nba | nhl | nfl | mlb
make sync c=nhl j=prospects   # prospects: cbb (recruits) | nhl (draft picks, rankings) | mlb (draft picks)
make sync c=nhl j=games       # schedule and box scores for the days around today
make sync c=nba j=seasons     # season-by-season stat history for every player
make runs                     # recent sync runs
```

Drafts are created from the commissioner page ("Drafts"), then run in the draft
room, which every manager can open at once and which updates live.
The room keeps the round-by-round board, your next pick, player search,
inline research, team rosters and your ranked queue in one workspace. Click a
player name in the pool, board, roster or queue to research them. Recent game
logs show up to ten imported final box scores using the current league's
scoring rules. Commissioner controls and the pre-draft pick-order editor are
under "Manage draft".

To play a season: the commissioner starts it (Commissioner, "Seasons"), and
each manager sets a lineup from their team page. Standings are on the League
page; a head-to-head league also gets a schedule, matchups and playoffs.

The Research tab is a research lab with a saved pool of league–season datasets.
Add combinations such as NHL 2025-26, WNBA 2025 and NHL 2024-25; the same player
gets a distinct row for each season, and each league-season keeps independent
benchmarks. Without a custom pool, the page uses the latest imported seasons.
Historical pools include stored seasons even after a player changes competitions.
Teams and stats are historical; positions, player status and fantasy ownership
reflect current records. Season history can be incomplete for older years.

The Players workspace starts with an overview, with separate advanced and raw
stat views. Raw stats include unscored stats and can be shown as season totals
or per-game rates. Hover or focus a column name for an explanation; click it to
sort and click again to reverse. Filters include multiple positions, team,
owner, status, game and production ranges, League+, replacement value,
qualification, missing scoring stats and raw-stat ranges.

Custom Charts supports scatter plots, grouped bars and histograms with chosen
axes, grouping, aggregation and titles, plus SVG and CSV downloads. Charts
use filtered observations across all table pages, with a stated 20,000-row
limit and even dot sampling for large scatter plots. League-wide Analysis
uses full selected datasets independently of player filters, with presets for
position strengths, scoring drivers, production concentration and season
scoring trends. It also shows imported coverage and replacement depth.

League+ normalizes **total season fantasy points** to a mean of 100 and a
standard deviation of 15 among all qualified players in the same league-season,
including every position and MLB pitching role. Percentile ranks those same
season totals. Availability matters: FP/game remains the separate rate metric.
Position+ normalizes FP/game among eligible positional peers. A 115 is one
standard deviation above average, not 15% more points. MLB workload requirements
still use season-specific pitching roles to exclude small samples. FPAR measures
production above positional replacement; auto depth estimates starting demand
from manager count and lineup slots, splitting flexible-slot demand equally
across positions, with an optional manual rank. Multi-position players use
their best advantage among eligible positions allowed by the position filters. WSA is a
custom illustrative matchup estimate, with its assumptions and formula in the
metric guide; it is not measured wins. Benchmark minimum games defaults to
five and is adjustable. Missing metrics and insufficient samples show a dash.
Research and player history use current fantasy rules when a league exists,
otherwise explicitly labelled sport defaults; missing scoring configuration
must not turn available raw stats into zero fantasy production.

Stat history is stored. A nightly job keeps every player's season totals in
`player_seasons`, fetched a whole league at a time. The first run for a sport
backfills ten seasons; after that only the two newest are refreshed. Any page
can read a player's history from there, including seasons in another league
(an NBA player's college years).

Scores are live. While games are being played, one loop on the server polls
the feeds every 30 seconds for the sports the dynasty plays, stores what
changed, and pushes it over websockets to whoever is watching the Scores page
or a game page. Browsers never poll.

`make build` produces a single binary, `backend/crossover`, with the frontend embedded.

## Layout

```
backend/
  main.go                 wires everything; the only binary
  migrations/             SQL migrations, applied on startup
  queries/                SQL that sqlc turns into internal/db (run `make gen`)
  internal/
    competition/          the registry: each sport's feed, positions, stats, default rules
    settings/             every league rule a commissioner can change, and its validation
    ingest/               feed client and sync rules; espn/ mlbam/ nhle/ adapters
    dynasty/              creating the dynasty, leagues and franchises
    roster/               every roster change, and the one check they all pass through
    auth/                 accounts, passwords, and the sign-in tokens (JWT)
    draft/                pick order, picks, the clock, queues, and the live room
    trade/                offers, acceptance, the rules a trade must pass, reversal
    lineup/               who starts each day or week, and when a starter locks
    scoring/              seasons, standings from stat lines, head-to-head, titles
    live/                 the loop that polls games in play, and what it pushes
    hub/                  topics that browsers listen to over a websocket
    sportsday/            which day a game belongs to
    players/              adding players by hand and merging duplicates
    problem/              errors meant for the user to read
    web/                  HTTP handlers and the embedded frontend
frontend/
  src/app.css             the design system: colour and type tokens, base styles
  src/lib/api.ts          the one place the frontend talks to the API
  src/lib/ui/             small shared pieces: icons, badges, tabs, toasts
  src/lib/                larger shared components: player list, settings form
  src/routes/             one folder per page
```

To add a league rule: add a field in `settings/`, a default in `competition/`,
a check in `Validate`, and an input in `frontend/src/lib/LeagueSettingsForm.svelte`.

## Tests

```sh
make test
```

This creates and uses a separate `crossover_test` database. Tests that need a
database refuse to run against one whose name does not end in `_test`.

## Deploy

Everything runs on one small server with Docker: Caddy (HTTPS), the app and
Postgres. On a fresh Ubuntu server with Docker installed:

```sh
git clone <your repo> crossover && cd crossover
cp .env.example .env        # set DOMAIN, DB_PASSWORD and ADMIN_TOKEN
docker compose up -d --build
```

Point the domain's A record at the server first; Caddy then gets its own
certificate. Open only ports 22, 80 and 443. The image is built on the
server, which needs about 2 GB of memory: on a 1 GB server add a swap file.

To update: `git pull && docker compose up -d --build`. Migrations run when
the app starts.

To back up the database (run it nightly from cron and copy the file off the
server):

```sh
docker compose exec -T db pg_dump -U crossover crossover | gzip > crossover-$(date +%F).sql.gz
```

## Environment

| Variable       | Default                          | Purpose                               |
| -------------- | -------------------------------- | ------------------------------------- |
| `DATABASE_URL` | local dev database on port 5433  | Postgres connection                   |
| `ADDR`         | `:8090`                          | listen address                        |
| `ADMIN_TOKEN`  | none                             | lets scripts call commissioner routes |
|                |                                  | (the token signing key is generated on first start and kept in the database) |
| `SYNC_CRON`    | `0 4 * * *`                      | when the daily roster sync runs       |
| `PROSPECT_SYNC_CRON` | `0 5 * * 1`                | when the weekly prospect sync runs    |
| `LIVE_POLL`    | `30s`                            | pause between polls of games in play  |
| `SEASONS_BACKFILL` | `10`                         | how many seasons of stat history to keep |
| `SCORES_SYNC_CRON` | `*/15 * * * *`               | the slower sync: schedule, finals, stat corrections |
| `FETCH_GAP`    | `1s`                             | minimum time between requests per host |
| `FETCH_VIA`    | none                             | relays for feeds that refuse the server's address: `host=https://relay`, comma separated |

CBB imports all Division I rosters. Starting lineups and research default to
Big 12, SEC, Big Ten, ACC, Big East, Mountain West, Atlantic 10 and Pac-12.
Commissioners can change **Starting conferences** in league settings; selecting
none allows all conferences. Research applies each season's membership before
calculating benchmarks or charts. Smaller-conference players remain draftable
and may be held on reserve under the default **Prospects or outside starting
conferences** reserve rule. Conference eligibility is also enforced by lineup
validation and scoring, including lineups saved before a rule change.

Replacement demand treats basketball position aliases and baseball outfield
positions as shared pools. Above-replacement season totals are raw fantasy
points under that league’s rules and are not comparable across sports.

Pitcher research defaults to a workload gate: at least 25% of both the highest
appearances and highest innings among peers in the same season and pitching
role, plus the benchmark minimum games. Small samples retain raw stats but
receive no comparative rankings. The research metric controls can change this
percentage (0 disables it); display filters do not affect qualification.

Player search on Research and Players opens a dedicated `/player/{id}` profile.
Profiles combine stored career seasons with the same full-population research
benchmarks used by the lab, raw stat scoring breakdowns, career charts and CSV
exports, paginated final-game logs, ownership and lineup status, actual counted fantasy production, reserve locks,
draft selections, completed trades and a paginated transaction timeline.
Each season and game uses its own competition's current scoring rules. Profiles
retain outside-conference college stats while clearly explaining why those
seasons do not receive comparative research metrics. Opening a profile reads
stored history without triggering provider imports. Private draft queues,
waiver claims and trade proposals are excluded from public profiles.

Managers can customize their organization name and image from **My team →
Customize organization & teams** and give each sport a separate team name and
image. Blank sport fields inherit the organization's identity. These settings
appear on team pages, league tables, matchups and lineups; shared league names
and franchise URLs stay unchanged. Images may use HTTP(S) URLs or uploaded
raster images, which the browser resizes to 256 pixels and stores with the team.
Managers can also set organization-scoped player nicknames on roster lists or
player profiles. Nicknames accompany real names in rosters, lineups and scoring
views, survive player-record merges, and do not replace provider identities or
research names. Managers may edit their own organization; commissioners may
assist other organizations.

Redis caches research route responses for ten minutes, and the other public
reads (player search, profiles, game logs, transaction history, standings,
matchups, scoreboards, game pages, franchise pages) for thirty seconds. The
season datasets behind research are held decoded in the app's own memory, the
last three selections at a time. Filtering or chart changes reuse the dataset
rather than repeating the full database aggregation. Identical concurrent requests share
one calculation; cold dataset loads are limited to one at a time per app process.

`docker compose up -d --build` includes a private Redis service with a 64 MB
cache limit, LRU eviction and no persistence. The app uses `REDIS_URL`; host
runs can point it at their own Redis, or leave it unset to disable caching.
Redis failures bypass caching with short timeouts and a five-second retry pause.
The cache is disposable; Postgres always holds the data.

Migration 0019 maintains separate research and public cache generations inside
Postgres transactions. Relevant committed imports, roster changes, player merges,
branding and commissioner rule changes make old keys unreachable immediately;
failed transactions do not invalidate good entries. Live game/lineup changes
invalidate player routes without invalidating season research. Raw feed payloads,
ingest-run logs and private queues do not invalidate research. A generation check
also prevents caching a response when its data changed during calculation.
Expiration bounds time-dependent profile information (such as a reserve lock
expiring) to thirty seconds. Browser/proxy caching is disabled on these routes.

The app trusts its last look at a cache generation for `CACHE_REVISION_AGE`
(one second by default) instead of asking Postgres on every request. Anything
changed through the app, and anything pushed to browsers, is seen at once; a
feed sync or direct SQL change can take that long to show. Set it to `0` to ask
every time.

Migration 0020 stores what the server used to recalculate on each read, kept
current by triggers in the same commit as the change behind it:

- fantasy points beside every season line and game stat line, rescored when a
  league's scoring rules or starting conferences change;
- `stat_seasons`, the seasons there are stats for;
- `period_scores`, what each side scored in a finished head-to-head period.
  It is written the first time standings need the period and cleared when a
  stat correction, lineup change or rule change touches it.

Feed syncs write each team, box score and season in one transaction, so a sync
starts one new cache generation per batch rather than one per row.

The compose file sizes Postgres and the app for a small server; `GOMEMLIMIT`
and `DB_POOL` in `.env` adjust it. To see which queries cost the most, run
this once and then read the view:

```sh
docker compose exec db psql -U crossover -c 'create extension if not exists pg_stat_statements'
docker compose exec db psql -U crossover -c 'select calls, round(total_exec_time) as ms, left(query, 80) from pg_stat_statements order by total_exec_time desc limit 20'
```
Session, private manager/admin routes, writes and live streams are not cached.
Keys include a database namespace, generation, route/query or dataset selection,
and a cache format version (`v1`); bump the version if cached JSON semantics change.
