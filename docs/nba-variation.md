# NBA variation, trend and percentile study — November 5, 2026

Added a second evaluation layer for trends, player-specific variation, matchup
ordering and calibrated outcome probabilities. The ordinary expected-score
model remains unchanged. Calibration artifacts are experimental and separate
from its expected stat line.

These results describe the workload checkpoint. The [next-priority study](nba-priority-study.md)
subsequently improves the offline default and repeats same-origin ranking and
uncertainty evaluation for the new model.

## What is the current error measuring?

| Metric | Historical test | Current-season test |
|---|---:|---:|
| Full one-game MAE | 5.121 | 5.062 |
| Median absolute error | 4.217 | 4.532 |
| 90th-percentile absolute error | 10.457 | 9.867 |
| Share of total error from worst 10% of misses | 27.5% | 26.0% |
| MAE on common easiest 90% of baseline outcomes | 4.126 | 4.170 |
| Error on three-appearance block means | 3.083 | 3.055 |
| Error on five-appearance block means | 2.438 | Too few appearances |

Every headline one-game MAE retains all positive-minute appearances: 15,751
historical and 1,126 current-season observations. The easiest-90% cohort is a
**hindsight diagnostic**, defined by the current workload model's errors and
held constant across comparisons. We cannot identify it before a game. Its
4.13 error is not evidence that the full model has reached the 4s.

Trend blocks are non-overlapping blocks of consecutive appearances spanning no
more than 21 days. Each forecast was issued before its own game. These are
averages of successive one-game forecasts, not three/five-game projections
issued before the first game. There are 4,953 historical three-game blocks
and 2,718 five-game blocks.

## Can it identify the same player's better matchups?

Ordinary day-by-day forecasts assimilate earlier outcomes. Comparing those
forecasts after a week finishes can mix matchup skill with feedback from the
same week's results. That is not a valid simulation of picking a game before
the week starts.

The stricter benchmark freezes player form, elapsed rest and workload before
the first eligible day of each week, then varies only scheduled opponent/home
context. It infers the roster at that origin and excludes unknown/new members.
It does not assimilate intervening results, forecast future injury news or
model the workload of intervening scheduled games. Partial first weeks begin at
the model's legal test cutoff. It assumes the final historical schedule was
known at the origin; schedule-change snapshots are unavailable.

| Same-origin matchup metric | Historical | Current season |
|---|---:|---:|
| Pairwise correct ordering | 54.2% | 52.3% |
| Chance / flat per-player reference | 50.0% | 50.0% |
| Comparable non-tied game pairs | 15,398 | 369 |
| Player-weeks with at least two appearances | 4,905 | 383 |
| Best game selected | 41.9% | 54.0% |
| Best game selected by random choice | 38.1% | 51.8% |
| Points left behind versus hindsight best game | 4.31 | 2.98 |
| Points left behind by random choice | 4.95 | 3.12 |

There is modest historical matchup-ordering skill. The current-season sample
contains only six days and 369 comparable pairs; its result needs confirmation.
The frozen-origin cohort excludes 200 historical and 9 current appearances
without matching prior roster membership. Both forecast and random-choice
comparisons use identical eligible appearances. These are conditional-on-playing
comparisons and do not evaluate DNP risk. Equal forecast values receive random
choice credit; ties are never broken using actual outcomes.

The ordinary sequential forecast has a historical within-player Spearman
correlation of 0.080 averaged over 467 players with at least ten test
appearances. That is substantially more demanding than ranking stars above
bench players. Nearby-game and sequential weekly statistics are retained in the
JSON/CSV as retrospective diagnostics, not deployable game-selection claims.

## How much variation should the forecast have?

The median historical player's forecast standard deviation is
23.5% of his observed outcome standard deviation. This is not a target to
inflate to 100%: an expected score should omit unpredictable game noise.

A sensitivity experiment rescales each forecast's departure from his prior
20-appearance mean. It never uses his future/test mean as an anchor:

| Multiplier on player-specific adjustment | Historical MAE | Current MAE |
|---|---:|---:|
| 0× | 5.250 | 5.374 |
| 0.5× | 5.144 | 5.151 |
| 1× | 5.121 | 5.062 |
| 1.5× | 5.182 | 5.088 |
| 2× | 5.326 | 5.261 |

More movement alone worsened absolute accuracy. Within a shared origin, positive
rescaling around a fixed player reference also preserves game ordering, so it
cannot create new matchup information. Tailoring needs additional predictive
signal, not a larger multiplier.

## Percentiles, probabilities and prediction ranges

"Usual score" is the median of the prior 20 appearances. Actual percentile is
the empirical percentile against up to 60 prior appearances, with midpoint
credit for ties. These references exclude every same-day outcome. A player's
prior score standard deviation is shrunk toward an earlier pooled residual
variance with strength 20, so small samples do not create extreme confidence.

We fitted outcome residual distributions on the first 60% of earlier validation
sports days and selected them on the remaining 40% using quantile pinball loss.
The selected distribution uses a shared standardized residual shape, scaled by
the player's historical variability. Fully player-specific residual mixtures
were tested but did not win validation. The selected distribution was refitted
on all validation observations before scoring test games.

- Historical: 80% ranges covered **78.9%** of outcomes; 95% ranges covered **94.2%**. Brier error for beating the prior usual score was 0.237, compared with 0.250 for always predicting 50%.
- Current season: 80% ranges covered **84.4%** of outcomes; 95% ranges covered **96.4%**. Brier error for beating the prior usual score was 0.226, compared with 0.250 for always predicting 50%.

Probability-bin tables compare predicted chance of beating usual with the actual
fraction that did so, and with mean actual prior-history percentile. This tests
whether higher probabilities correspond to stronger results. The 50% reference
is a simple fixed benchmark; it is not a fitted base-rate competitor. Current-
season ranges are somewhat conservative. DNP/availability risk is excluded.

## Did robust point calibration get the model into the 4s?

A validation-selected typical-score calibration reached **4.868** on the
current-season test, but worsened the larger historical test to
**5.144**, compared with 5.121. It was not promoted.
The distribution's median forecast scored 5.085 historically and
4.973 this season. We have not established a broadly sub-5 one-game model.

Point calibration explored shrinking/amplifying around prior mean/median and
robust global, player or score-band offsets. It was fitted on the early
validation segment and selected on the later segment, with a guard against
losing more than 0.5 percentage points of same-player same-week pair accuracy
on either tuning window. It was then refitted before testing. Those calibration
ranking checks use sequential forecasts; the stricter frozen-origin test above
is the primary benchmark for matchup-choice claims.

Expected fantasy points and the median/typical outcome are different targets.
Expected points remain the scored projected stat line. A typical-score
calibration is kept separate rather than silently making the scoring breakdown
inconsistent. No new point calibration changed the default model.

## What offers the best route to a genuine improvement?

Using **actual minutes** while retaining the pregame stat-rate predictions
reduced historical error from 5.12 to **3.75**, and current error from
5.06 to **3.56**. This is a hindsight oracle, not a deployable model or a
proven error floor. It nevertheless points to a much larger opportunity in
minutes/roles than in small matchup multipliers. Removing bonus scoring only
changed historical error to 5.05, so bonuses are not the main issue.

Next experiments should prioritize:

1. Timestamped pregame injuries, confirmed lineups, minutes restrictions and
   expected rotations, with a learned availability model before enforcing the
   team minute budget.
2. A player role/state model that detects persistent minutes or usage changes
   without overreacting to one unusually good or bad game; separate shortened
   injury/foul/blowout appearances where the available data permits it.
3. Shot/assist/rebound opportunity volume separately from conversion or rare
   events, plus hierarchical player-specific matchup effects with shrinkage.
4. A game-choice objective alongside point error: retain a model only when its
   validation same-origin ordering/weekly score improves, and validate the
   predicted probabilities and ranges as well as its average.
5. Robust median or quantile training as a separate typical-outcome model,
   evaluated on future dates rather than repeatedly choosing winners from the
   same historical benchmark.

## Artifacts and reproduction

- `data/nba/variation-study/player-review.html`: interactive per-player score,
  forecast, percentile, probability and 80% range charts.
- `data/nba/variation-study/per-player.csv`: observed/forecast variation,
  correlation, pair ranking and error for each eligible player/model/window.
- `data/nba/variation-study/point-calibration-current.json`: experimental
  typical-score calibration; not the expected-score default.
- `data/nba/variation-study/distribution-current.json`: fitted residual
  distribution for probabilities/quantiles.
- [Checked-in results](nba-variation-results.json): complete aggregate metrics,
  model comparisons, same-origin benchmarks, calibration bins and provenance.

The Go harness supports `evaluate --refit-before DATE` for chronological
validation forecasts, `--diagnostics` for labelled oracle-minute/bonus outputs,
and `--freeze-week` for the same-origin matchup benchmark. Earlier validation
MAEs replayed exactly (difference below 1e-10). Original model forecasts were
unchanged by adding diagnostics.

After running the feature study to obtain its frozen workload artifacts:

```sh
python3 tools/nba_variation_forecasts.py
python3 tools/nba_variation_study.py
python3 tools/nba_variation_report.py
```

The preparation command generates the chronological calibration/test forecasts
and frozen-origin matchup forecasts. The study expects
`historical-validation.jsonl`, `historical-test.jsonl`, `current-validation.jsonl`
and `current-test.jsonl` under its output directory; optional same-origin files
add the stricter matchup test. Historical validation is Oct–Dec 2025; current
validation is October 20–29, 2026. Training/calibration never use test outcomes.
The benchmarks are reused research data, so final gains still require future-
game confirmation.

Focused Go/Python tests, race checks and vet pass. Tests cover prior-day leakage,
refit cutoffs, frozen-origin invariance, oracle isolation, ranking ties,
calibration quantiles and the distinction between more variation and more skill.
