# NBA projection laboratory

The first model predicts **minutes conditional on appearing**, then individual
stat rates per minute, and converts the projected stats using league scoring.
Per-minute rates are equivalent to per-60 rates divided by 60; using minutes
directly avoids an extra conversion. This is the offline research harness. The validated combined model is now
served through the [production integration](nba-production.md).

The [initial held-out results](nba-model-results.md) record the first completed
training runs, comparisons, local review artifacts and remaining limitations.
The [follow-on feature study](nba-feature-study.md) tests teammate context, team
minutes, workload, offseason resets, pace and competitiveness. Its selected
workload checkpoint is preserved as `data/nba/workload-model.json`; the original
artifact is `data/nba/v1-model.json`. The [next-priority study](nba-priority-study.md)
adds official pregame availability, role changes and shooting structure; that
combined model is now the offline default. Commands below reproduce the
initial baseline; `--features workload` reproduces the earlier extension.
The [variation and percentile study](nba-variation.md) adds within-player
ranking, shared-origin matchup tests, outlier/trend diagnostics and calibrated
outcome ranges. Experimental typical-score calibration remains separate.

## Data

Run from the repository root:

```sh
python3 tools/nba_history.py --from 2023-10-01 --to 2026-11-04
go -C backend run ./cmd/nba-model inspect
```

The downloader uses only Python's standard library. It fetches ESPN schedules
and completed regular-season box scores, spacing requests by at least 250ms
across four workers. Raw responses are cached under `data/nba/raw/`; rerunning
the same range resumes from those files. The final dataset is replaced only
after every requested game has been downloaded and parsed successfully.

The dataset retains all players in historical box scores, including players no
longer on current rosters. Each JSONL row contains ESPN game/player/team IDs,
timestamp, season ending year, opponent, home/away, position, actual starting
status, minutes, participation, and canonical scoring stats. All-Star games,
the Cup championship, preseason, play-in and playoff games are excluded.
Unknown minutes are explicitly marked `minutes_missing`; those rows and
zero-minute appearances are excluded from fitting/evaluation, along with DNPs.
Unidentified historical DNP entries containing only a surname are omitted;
an actual appearance without a player ID stops the import.
The loader rejects duplicate player-games, missing required counting stats,
negative/nonfinite values and invalid game context. Do not replace missing
counting stats with zero.

The existing application also retains `min` on new basketball stat lines now,
including fractional `MM:SS` values. The historical download is independent of
application tables and does not rewrite settled scores or historical rosters.

Cache files, downloaded data, model artifacts and generated review pages are
ignored by Git. Cache entries are immutable: remove a specific game's cache
file to fetch a correction. Refresh today's schedule by removing that day's
schedule cache file; the initial download ends at the last completed day.

## Features and training

The minutes model learns a correction to a weighted historical minutes prior
using the last 3 and 10 appearances, current/prior-season minutes, recent
starting share, rest, home court, season experience and recent team changes.
Recent starting share comes from prior games; the target game's actual starter
flag is never an input. Minutes predictions are bounded to 2–48; overtime is
not modeled explicitly in this version.

Each stat model learns a correction to a smoothed per-minute rate. Inputs are
the baseline rate, recent and current-season deviations, minutes trend, rest,
home court, prior starts/team changes, and these contextual features:

- **Opponent defense by position:** current-season production allowed to G/F/C
  players, divided by what those players' own pregame baselines expected in
  their actual minutes. This reduces schedule-strength confounding. It is
  shrunk toward neutral with 1,000 equivalent minutes, bounded to 0.5–1.5, and
  supplied as an input whose weight is learned. These are broad position
  buckets, not individual defensive assignments. Per-minute allowed rates also
  reflect pace; this first version does not separate pace from defensive quality.
- **Previous meeting:** the player's per-minute performance in his most recent
  meeting with this opponent within 365 days, compared with his current
  baseline. Its contribution is shrunk by `minutes / (minutes + 120)`. One
  unusually good game is weak evidence; the regression learns its weight.

Player rates use a 240-minute positional prior and exponentially weighted
history. League/position priors use up to three seasons, with older years
downweighted. Player history retains up to 180 appearances. Opponent defense
resets each season; current-season player rates/minutes are separate features.
Training examples from older seasons receive weight `0.65 ** seasons_ago`.

Models are regularized linear regressions fitted with a small, dependency-free
ridge solver. Stat-rate examples are weighted by minutes to reduce noise from
very short appearances. Model selection compares six configurations: history
half-lives of 10 or 25 appearances and ridge penalties of 0.001, 0.01 or 0.1.
The validation criterion is fantasy-point MAE under the requested scoring.

Double/triple-double and 40/50-point bonuses are learned as event rates and
bounded to valid probabilities after scaling by projected minutes. Triple-
double probability cannot exceed double-double probability, and 50-point
probability cannot exceed 40-point probability. These are initial empirical
estimates, not a jointly simulated or calibrated stat distribution.
Counting stats are fitted separately; shot totals and point totals are not yet
constrained to form a fully coherent simulated box score.

## Reproducible evaluations

Dates are **sports days** (the application's Eastern calendar with a 5am
rollover). All ranges are start-inclusive and end-exclusive. No random split
is used. Initial fitting uses data before `--train-before`, configuration
selection uses `[train-before, test-from)`, and the chosen configuration is
refitted on all eligible history before `--test-from`. Test outcomes are
excluded from both model fitting and configuration selection.

During validation and testing, rolling player and opponent features update only
after an entire sports day's predictions have been made. Coefficients stay
frozen. Past test outcomes may inform later test games, as they would in live
use; target-game and same-day outcomes cannot.

A larger historical holdout:

```sh
go -C backend run ./cmd/nba-model train \
  --train-before 2025-10-01 --test-from 2026-01-01 --test-to 2026-07-01 \
  --model ../data/nba/historical-model.json \
  --report ../data/nba/historical-report.json
```

A preseason model trained on the three completed seasons, with the second half
of 2025–26 used for configuration selection, testing the beginning of 2026–27:

```sh
go -C backend run ./cmd/nba-model train \
  --train-before 2026-01-01 --test-from 2026-10-20 --test-to 2026-11-05 \
  --model ../data/nba/preseason-model.json \
  --report ../data/nba/preseason-report.json
```

A candidate tuned using this season's first ten days, keeping later games out:

```sh
go -C backend run ./cmd/nba-model train \
  --train-before 2026-10-20 --test-from 2026-10-30 --test-to 2026-11-05 \
  --model ../data/nba/current-baseline-model.json --report ../data/nba/current-baseline-report.json

go -C backend run ./cmd/nba-model evaluate \
  --model ../data/nba/preseason-model.json \
  --test-from 2026-10-30 --test-to 2026-11-05 \
  --report ../data/nba/preseason-late-report.json
```

Compare these last two reports on their identical test window. This season's
test sample is small; improvement there alone is not a promotion criterion.

## Review specific games and players

The checked-in ID list identifies the 14 completed NBA games with stat lines
in the local application database at the time this harness was built. They
occur on October 20–21, so use the **preseason** artifact, not the candidate
that was fitted through October 29:

```sh
go -C backend run ./cmd/nba-model evaluate \
  --model ../data/nba/preseason-model.json \
  --game-ids-file ../docs/nba-model-local-game-ids.txt \
  --report ../data/nba/local-report.json \
  --predictions ../data/nba/local-predictions.jsonl

python3 tools/nba_report.py --predictions data/nba/local-predictions.jsonl \
  --out data/nba/local-review.html
```

Open `data/nba/local-review.html` to filter games/players, compare methods, sort
errors and inspect individual stat predictions. The page contains its own data
and makes no network requests. For other subsets, use `--games ID,ID` and/or
`--players ID,ID`. Filters apply only to evaluation: the full prior dataset
still builds the player/opponent context. The harness rejects evaluation
windows that overlap the artifact's training cutoff.

Each result includes the actual box score/minutes, predicted stats/minutes,
three baselines, and the complete feature vectors, prior rates, opponent
factors and sample sizes. Aggregate reports contain MAE, RMSE and signed bias
for fantasy points, minutes and each stat, plus groups for cold starts,
positions, changing minutes, back-to-backs and previous-meeting coverage.
Reports include a dataset SHA-256 and every validation candidate.

Baselines are the last-10-appearance mean and the application's actual
`projection.Baseline`/`projection.Points` code replayed on prior game totals,
both with and without its historical-attendance discount. Historical pregame
odds and injury snapshots were unavailable in the initial proxy replay, so its
market adjustments are neutral and today's injury flags are never applied to old
games. The follow-on priority model separately uses archived official reports. This is explicitly
**not a complete replay of the old live proxy**.

All accuracy metrics here are conditional on appearing with positive minutes.
They do not evaluate expected fantasy scores including the chance of sitting
out. Bonus probabilities are not yet assessed for calibration.

## Predict a game and extend the model

`predict --target path.json --model path.json` accepts only pregame context:

```json
{
  "game_id": "example",
  "starts_at": "2026-11-06T00:00:00Z",
  "season": 2027,
  "player_id": "ESPN_PLAYER_ID",
  "team": "ESPN_TEAM_ID",
  "opponent": "ESPN_OPPONENT_ID",
  "home": true
}
```

It uses the exact same feature builder and prediction path as evaluation.
`--scoring path.json` accepts a complete replacement JSON map of canonical
stat weights. Without it, commands use the application's NBA default scoring.
`train --skip-evaluation` saves an artifact plus validation results when fitting
through the latest completed day for future predictions; it makes no claim
about held-out performance.

Next useful experiments are richer role/teammate-absence features, explicit
pregame availability and confirmed lineups, coherent team minute allocation,
and calibrated prediction ranges. Collect timestamped injury/lineup/odds
snapshots before training those features. Do not infer historical pregame
availability from whether somebody eventually appeared or reuse final-game
starter flags as pregame inputs. Change one component at a time and compare on
the same frozen game/player subset, including per-group regressions.

Verification:

```sh
python3 -m unittest discover -s tools -p 'test_nba_history.py'
go -C backend test ./internal/nbamodel ./internal/projection ./internal/competition ./internal/ingest/espn
```

Tests cover target/same-day leakage, held-out-label invariance, per-game/player
filtering, equivalence of prediction and evaluation paths, corruption rejection,
minute parsing, and learning known production in synthetic chronological data.

The initial full backend regression run also found existing HTTP 401 failures
in web authentication/integration tests. `TestDraftFlow` and
`TestResearchAnalyticsAPI` reproduced in a temporary workspace with the NBA
ingestion edits removed. The model, projection, competition, ingestion, lineup,
player and scoring package tests passed; the unrelated web failures remain.
