# NBA model deployment and refresh

NBA lineup/game projections and the matchup outlook now read the validated
combined model's predicted stat lines. Each league applies its own scoring.
Other competitions retain their existing projection method. The API's game
objects include `projection_source: "nba_model"` when that model was used.

The selected coefficients are versioned in `models/nba/model.json` (SHA-256
`c52b0fe19e39a378bf415b6b7f76eb8f0285631086726ac4596ae3d52fcf9841`).
The collector refreshes the inputs and forecasts; it does not retrain or select
a different model automatically. Accuracy is conditional on playing. A verified
Out for that game produces zero; other statuses do not imply a calibrated
probability of appearing.

## Production service

The application uses `NBA_PROJECTIONS_PATH=/nba/live-projections.json`, mounted
read-only from `./data/nba`. It reloads every minute, maps ESPN identities to
database identities, and invalidates response-cache generations on changes.
No new database migration is required. `GET /healthz/nba` reports the artifact
hash, input-history cutoff, generation time and mapped forecast count. An
unavailable/stale configured model returns HTTP 503 there; `/healthz` remains
application/database health.

A scheduled forecast older than six hours, missing identity, moved tip time,
or unsupported cold-start player uses the original proxy. Live/final games
retain their last pre-tip forecast, including across refreshes and restarts.
When no pre-tip forecast exists, their expectation uses the existing proxy.
A missing or malformed new snapshot retains the previous valid snapshot.

`deploy/nba-refresh.timer` runs at 00:15, 02:15, 04:15, … UTC, with up to two
minutes of jitter. It persists across reboots and catches up after downtime.
The oneshot job uses a process lock and has a 30-minute timeout.

Each run:

1. Refreshes recent completed regular-season box scores (three-day correction
   window), preserving the earlier historical corpus.
2. Reads current team rosters and schedules for the next 14 days.
3. Reads recent official injury-report PDFs for upcoming games, using the
   actual publication timestamp and current roster identities. Missing and
   unsubmitted reports remain unknown.
4. Rebuilds historical form and opponent/role features using completed prior
   sports days. Today's game outcomes do not enter forecasts.
5. Publishes the complete snapshot by atomic rename and archives it compressed.
   Forecasts for games already started remain locked.

Raw capture folders are retained seven days; compressed forecast snapshots
are retained 90 days. The merged historical game corpus persists. History
older than 96 hours blocks publishing new forecasts, so feed failures cannot
silently create apparently fresh projections from old season form. Long NBA
breaks can therefore use the proxy until completed games refresh history.

## Operations

On the production host, from `/root/mosaic`:

```sh
systemctl status nba-refresh.timer
systemctl list-timers nba-refresh.timer
systemctl start nba-refresh.service
journalctl -u nba-refresh.service -n 80 --no-pager
curl -fsS https://mosaicfantasy.com/healthz/nba
```

The host needs Python 3.9+ with standard libraries, `poppler-utils` and the
Linux `backend/nba-refresh` binary. The initial corpus at
`data/nba/history-enriched.jsonl` is seeded from the research data, and
`data/nba/model.json` is copied from the versioned model. The collector never
writes application rosters, scores or settings.

Install the schedule after a successful manual run:

```sh
install -m 644 deploy/nba-refresh.service deploy/nba-refresh.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now nba-refresh.timer
```

To disable model serving, set `NBA_PROJECTIONS_PATH` to an empty string in the
app's environment and recreate the app container. To pause input collection,
stop/disable `nba-refresh.timer`. Do not remove the history or forecast files
as part of rollback. Keep the previous app image tagged before rebuilding.

Model refitting remains a separately evaluated release: compare chronological
and prospective errors, version the approved artifact, then replace the live
artifact and run the collector. The [offline results](nba-priority-study.md)
remain the accuracy benchmark for this release.
