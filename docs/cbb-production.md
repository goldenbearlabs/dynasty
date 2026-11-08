# CBB model refresh and deployment

`docker compose up -d --build` now starts the `cbb-refresh` worker alongside
an app configured to read its forecasts. The worker runs **immediately on
startup**, then **two hours after each successful run**. Failed runs retry after
five minutes. Restarting or deploying a new worker image starts another refresh
immediately. Refreshes use the versioned CBB context model; they never retrain.

## First run and persistent data

The worker bootstraps itself: it downloads the SportsDataverse/ESPN season
archives from 2023 through the current ending season, normalizes them using the
research integrity checks, and downloads historical recruiting records. There
is no requirement to upload the ignored local research data or feature caches.
This cold start takes longer than regular refreshes and depends on public feed
speed. The logs distinguish downloading, history preparation and feature replay.

Its private corpus and outputs persist under `data/cbb/live`, mounted into the
worker and read-only into the application. Research data under `data/cbb` is
not modified. The worker is limited to one CPU and 1.5 GiB of memory; provision
room for it alongside the app/database and several GB of disk for archives.

Each refresh reads season-specific conference membership, recent completed
box scores (three-day corrections plus catch-up), the next seven days of
schedules, and current rosters for those games. Current rosters enumerate
prediction targets; model roster/role/depth features still use prior completed
games, matching the tested feature recipe. Historical state is replayed without
the large research feature matrices. Today's sports-day outcomes are excluded.
The worker handles regulation and overtime historical games, regular season
and postseason. A confirmed missing (HTTP 404) or empty roster is recorded in
`last-refresh.json` and the run manifest; that team uses the proxy while other
teams receive model forecasts. Transport errors and duplicate roster identities
still stop publication. Corrupt/incomplete completed games are quarantined in the run
manifest; more than 10% failing stops publication. During the regular season,
history more than seven sports days old also stops publication (with a first
week of November exception for the new season).

A complete validated forecast snapshot is published by atomic rename to
`data/cbb/live/live-projections.json`. A process lock prevents overlapping
workers. A failed feed or prediction run retains the previous published file.
Started games keep their last pre-tip prediction across refreshes/restarts, for
30 days. No forecast is newly issued after tipoff. Raw run captures are retained
seven days and compressed forecast snapshots 90 days.

## Serving and monitoring

The application uses `CBB_PROJECTIONS_PATH=/cbb/live-projections.json`. It reloads
once a minute, maps ESPN game/player IDs within CBB, invalidates response cache
generations, and scores the predicted stat line using each league's settings.
Game projections identify `projection_source: "cbb_model"`. Missing identities,
changed tip times, missing forecasts or scheduled forecasts older than six
hours fall back to the existing proxy. Live/final expectations retain their
last pre-tip forecast. A malformed new file retains the last valid in-memory
snapshot. No database migration is required.

`GET /healthz/cbb` reports generation time, history cutoff, model SHA, forecast
count and mapped forecast count. It returns 503 while an enabled model is not
ready or its snapshot is stale; ordinary `/healthz` is unchanged. The Docker
worker health check uses the last successful refresh, with a one-hour cold-start
grace. An unhealthy status does not itself restart a Docker container; the
worker's retry loop handles refresh failures.

From the deployment directory:

```sh
docker compose up -d --build
docker compose logs -f --tail=60 cbb-refresh
curl -fsS https://mosaicfantasy.com/healthz/cbb
```

The completion log and `data/cbb/live/last-refresh.json` report the actual elapsed
seconds. To refresh immediately again:

```sh
docker compose restart cbb-refresh
```

To stop refreshing, `docker compose stop cbb-refresh`. To return serving to the
proxy, set `CBB_PROJECTIONS_PATH=` in `.env` and recreate the app container. Keep
the corpus and forecast archives for rollback/audit.

## Limits

Forecasts remain conditional on playing. The refresh does not invent injury or
DNP probabilities, fetch un-timestamped archives into historical features, or
retrain/select coefficients unattended. The worker's published stat lines use
the same model and causal feature recipe as the [context study](cbb-context-study.md).
Prospective archives now allow monitoring live accuracy before a new model
release. NBA collection and model coefficients remain unchanged.
