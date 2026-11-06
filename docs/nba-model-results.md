# Initial NBA model results — November 5, 2026

The first model and reproducible harness are implemented. The dataset contains
100,511 player-game records, including DNPs; 81,747 appearances have positive
minutes. It covers all 1,230 regular-season games in each of 2023–24, 2024–25
and 2025–26, plus 121 completed games in 2026–27 through November 4.

These are the original baseline results. The [follow-on feature experiments](nba-feature-study.md)
updated the offline default with workload features and retained the original
artifact as `data/nba/v1-model.json`.

The model estimates minutes and individual stat rates. Current-season opponent
production allowed by broad position and the player's previous meeting are
learned features, with small samples pulled toward neutral priors. See the
[implementation and commands](nba-model.md) and [complete results](nba-model-initial-results.json).

## Held-out accuracy

Numbers are mean absolute fantasy-point errors under the application's NBA
default scoring; lower is better. All methods are evaluated on the same
positive-minute appearances. These are **conditional-on-playing** comparisons.

| Experiment | Games | Appearances | Model | Proxy if active | Last 10 | Proxy with attendance discount |
|---|---:|---:|---:|---:|---:|---:|
| Historical: Jan–Apr 2026 | 734 | 15,751 | 5.127 | 5.344 | 5.255 | 6.363 |
| Preseason model: Oct 20–Nov 4 | 121 | 2,718 | 5.384 | 5.416 | 5.636 | 6.237 |
| Preseason model: Oct 30–Nov 4 | 50 | 1,126 | 5.123 | 5.199 | 5.306 | 6.068 |
| This-season tuned model: Oct 30–Nov 4 | 50 | 1,126 | 5.071 | 5.199 | 5.306 | 6.068 |
| Local database games: Oct 20–21 | 14 | 322 | 5.520 | 5.504 | 5.922 | 6.036 |

The historical model reduced MAE by 4.1% against the fairer
proxy-if-active baseline. The this-season candidate reduced it by
2.5% on its later 50-game holdout. This-season tuning improved MAE from
5.123 to 5.071 on that identical window. Those are descriptive results;
no significance claim is made from the small current-season sample.

The 14 local games were a useful counterexample: the model slightly trailed the
proxy-if-active baseline (5.520 vs 5.504). No live projection was switched.
The much larger apparent gain against the attendance-discounted proxy mixes
production and availability assumptions and should not be interpreted as a
fair measure of production-model improvement.

## Minutes and next experiments

Historical minutes MAE was 5.07, compared with 5.20 for recent average
minutes. On the local opening games it was 6.53, compared with
6.47 for recent average minutes. Early-season rotation changes and rookies
remain difficult. The review page shows each player's actual and projected
minutes alongside the stat and fantasy-point errors.

An opening-night issue is carrying late-season minutes into a new season. For
example, Anthony Edwards' prior three appearances averaged 25.3 minutes versus
35.0 over his previous season; the model projected 25.9 minutes, and he played
33. Cold starts also overprojected production on the local sample: average
signed error was +6.36 fantasy points across 31 appearances. Offseason minutes
priors and better rookie/role priors deserve explicit tests next.

The next experiments should focus on pregame role/lineup information, teammate
absences and explicit availability, then assess opponent and previous-meeting
features through controlled ablations. Collect timestamped pregame information
before backtesting those additions. Use the same held-out windows and retain
per-position, cold-start and changing-minutes checks.

## Artifacts and limits

- `data/nba/v1-model.json`: original candidate selected using October 20–29 validation data,
  refitted on all eligible history before October 30.
- `data/nba/preseason-model.json`: fitted before October 20; used for the local
  opening games so those games never enter training or model selection.
- `data/nba/historical-model.json`: fitted before January 1, 2026.
- `data/nba/local-review.html`: standalone interactive review of all 322
  positive-minute player-games from the local database's 14 completed games.
- `data/nba/local-predictions.jsonl`: outcomes, projections, baselines and feature
  vectors for those same games.

Downloaded data and generated artifacts remain local and ignored by Git.
The checked-in JSON records dataset hashes, cutoffs, scoring, configuration
selection and full aggregate metrics.

Historical pregame odds/injury snapshots are unavailable. The proxy replay uses
the existing production functions with neutral market factors and reconstructed
prior game totals; it is not a full replay of historical live projections.
DNPs and unknown/zero-minute appearances are excluded from these accuracy
metrics. Availability, team minute constraints, overtime and calibrated
prediction intervals remain future work. Live projections still use the
existing proxy.

The model/ingestion tests, leakage checks, race check and vet passed. The wider
backend suite found pre-existing web authentication failures; representative
failures reproduced with the ingestion edits removed.

The [next-priority study](nba-priority-study.md) subsequently recovered official
injury reports, improved the offline default and repeated chronological tests.
The limitations above describe this initial experiment and proxy replay.
