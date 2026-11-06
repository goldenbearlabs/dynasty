# NBA feature experiments — November 5, 2026

Tested seven additions individually and four combined models against the frozen
original NBA model. **Rest and workload features are now enabled in the offline
default model.** The improvement is small; other additions remain experimental.
Live application projections still use the existing production proxy.

## Fixed benchmark results

Fantasy-point mean absolute error, conditional on a positive-minute appearance;
lower is better. Historical testing covers 734 games / 15,751 appearances from
January–April 2026. Current-season testing covers 50 games / 1,126 appearances
from October 30–November 4, 2026. Every comparison uses identical player-games.

| Addition to the original model | Historical MAE | Current-season MAE |
|---|---:|---:|
| Frozen original model | 5.1270 | 5.0705 |
| Teammate minute opportunity | 5.1041 | 5.0439 |
| Teammate stat-rate opportunity | 5.1283 | 5.0757 |
| 240 expected team-minute budget | 5.1151 | 5.1037 |
| Rest and workload | 5.1213 | 5.0622 |
| Offseason minutes reset | 5.1120 | 5.1239 |
| Estimated pace | 5.1251 | 5.0707 |
| Expected competitiveness | 5.1277 | 5.0720 |
| All seven together | 5.0919 | 5.1402 |
| Workload + pace + competitiveness | 5.1201 | 5.0643 |
| Workload + both teammate groups | 5.1044 | 5.0490 |
| Workload + pace + competitiveness + offseason reset | 5.1068 | 5.0884 |

Workload reduced error by 0.11% historically and
0.16% on this season's benchmark. The 95% game-bootstrap intervals
for absolute MAE reduction were [0.0001, 0.0111] historically and
[-0.0092, 0.0274] this season. The current-season interval
includes zero; this is a small candidate improvement, not strong evidence of a
large or certain gain.

## Why these features were enabled

Feature/configuration selection used earlier validation periods, before
opening the new test reports. Workload was the most consistent validation
choice. Each experiment selected its half-life and ridge penalty from the same
six-configuration grid, then refitted before the benchmark start date.

The teammate-minute model did better on both test windows, but its current-
season validation error rose from 5.4934 to 5.5316. Workload plus teammate
features also helped on the test windows but worsened current-season validation
to 5.5544. Those are promising research candidates, not replacements chosen
using favorable test results. Neither the all-feature bundle nor the practical
smaller bundles beat workload alone consistently across validation and testing.

The benchmarks have been reused during development. Results are exploratory;
future games are needed to confirm whether these gains persist. Bootstrap
intervals resample whole games, do not account for all model-selection
uncertainty, and are not adjusted for multiple comparisons.

## What each experiment actually uses

- **Workload (enabled):** team rest days; team games in the preceding 4 and 7
  days; player minutes in the preceding 3 and 7 days; time since the player's
  last appearance. This supplements the original appearance-based rest flag.
- **Teammate minutes:** estimated missing teammate minutes, missing minutes at
  the same broad position, expected rotation workload, estimated participation
  and roster size. Rosters come from prior box-score listings; participation is
  a smoothed, recency-weighted appearance fraction over the last eight team games.
- **Teammate stat rates:** expected points, assists and rebounds missing from
  teammates, plus missing same-position minutes and estimated participation.
  These are aggregate opportunity features, not player-pair on/off estimates.
- **240-minute budget:** constrain the sum of estimated participation probability
  times each known player's conditional minutes, plus an unknown-rotation
  reserve, to 240. Individual minutes are capped at 48. The reserve starts with
  weight 8 and absorbs minutes for unseen/uncertain rotation members. First-time
  players retain their cold-start forecast rather than being inserted into a
  roster using target-game knowledge. This enforces expected regulation minutes;
  it does not simulate confirmed active lineups or overtime.
- **Offseason reset:** for veterans with at least ten prior-season appearances,
  pull the minutes baseline and recent-minute features toward the prior-season
  average during their first ten appearances, tapering the pull to zero.
- **Pace:** estimate possessions as FGA + 0.44 × FTA − offensive rebounds +
  turnovers; normalize prior game totals for player-minutes; blend recent team
  and opponent pace with a five-game neutral prior. Use expected pace, opponent
  pace and the change from the team's usual pace as rate inputs.
- **Competitiveness:** use the difference between smoothed prior team and
  opponent point differentials as a margin proxy, including an interaction
  with recent starting share. No archived betting spreads were supplied.

The historical dataset was enriched with offensive/defensive rebounds and
fouls from cached responses. All 100,511 original records were checked for
identical identity, context, participation, minutes and original scoring stats.
The baseline replay matched the original saved MAEs within 1e-10.

## Teammate-data limitation

This earlier experiment did **not** use archived pregame injury reports,
confirmed lineups, minutes restrictions or historical depth charts. The
[follow-on priority study](nba-priority-study.md) recovered official injury
reports and supersedes the offline default described here. In this study,
participation estimates use prior
appearances and DNPs only. We never supply who ultimately played, target-game
minutes, target-game starting status or the final participant list as pregame
features. Opponent and teammate histories update after a completed sports day.

The minute-budget result does not mean the physical 240-minute constraint is
wrong. Inaccurate participation probabilities or an outdated inferred rotation
can make enforcing it distort otherwise reasonable conditional forecasts.
Reliable timestamped availability and rotation data is the next important
input before judging a stronger team-allocation model.

## Artifacts and reproduction

- `data/nba/workload-model.json` and `data/nba/improved-model.json`: workload checkpoint,
  trained before October 30, 2026.
- `data/nba/v1-model.json`: preserved original default model.
- `data/nba/feature-study/current-review.html`: interactive comparison with the
  frozen original model on all 1,126 current-season test appearances.
- `data/nba/feature-study/expanded-results.json`: full experiment results.
- [Checked-in comparison data](nba-feature-study-results.json): validation
  scores, MAE/RMSE/bias, minute errors, subgroups and paired bootstrap intervals.

```sh
python3 tools/nba_feature_study.py --out data/nba/new-feature-study --promote
python3 tools/nba_combination_study.py --study data/nba/new-feature-study --promote
```

A fresh output directory records dataset/source hashes and the protocol. Existing
outputs are cached only for an unchanged protocol. The supplementary combinations
are declared before their validation fitting and selected before their test
reports are opened. Promotion requires the validation-selected candidate to
reduce mean test error on both fixed benchmarks. Other experiments remain
available via `train --features ...`; artifact feature sets govern inference.

The CLI prefers the enriched local dataset when present; explicit `--data`
overrides that choice. Pace models reject datasets without offensive rebounds.
Model format v2 retains inference compatibility with the frozen v1 artifacts.

Focused Go tests, race checks, vet and Python tests passed. Tests cover same-day
and target-game leakage, held-out-label invariance, expected minute totals,
absence sensitivity, unknown-roster handling, offseason resets, inference/replay
parity and paired-cohort checks. The previously identified unrelated web-auth
failures were not changed by this offline work.
