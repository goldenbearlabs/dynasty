# CBB context improvements — November 7, 2026

The new context model is the **offline default** at `models/cbb/model.json`.
`models/cbb/context-model.json` is the same candidate; the original champion is
preserved at `models/cbb/v1-model.json`. Live integration and scheduling are now
available through the [CBB production worker](cbb-production.md).
The NBA model and its production refresh service were not modified.

## Results against the previous CBB champion

All MAEs use the same conditional-on-positive-minutes, eligible-conference,
verified D1 cohorts as the original study. No forecast-error outliers are removed.

| Period | Games | Appearances | Previous champion | Added context, linear correction | Final context model |
|---|---:|---:|---:|---:|---:|
| Jan–Apr 2025 | 1,126 | 20,903 | 4.13413 | 4.12531 | **4.12394** |
| Jan–Apr 2026 | 1,118 | 20,541 | 4.21013 | **4.20223** | 4.20438 |
| Nov 2–5, 2026 | 123 | 1,537 | 5.27579 | 5.20958 | **5.16706** |
| Additional Nov 6 holdout | 44 | 546 | 4.66370 | — | **4.54771** |

The final recipe is selected by prior validation scores, not whichever variant
wins each test row. The linear correction slightly beats the nonlinear version
on the 2026 holdout. The nonlinear version wins validation overall and improves
all four checks against the original champion. Mature-season gains are small;
opening-week and additional-day gains are larger.

Paired game-bootstrap 95% intervals for MAE reduction versus the previous
champion are [0.0071, 0.0132], [0.0030, 0.0084], [0.0621, 0.1541] and
[0.0342, 0.1952]. These intervals do not correct for repeated experimentation.

For players whose pregame minutes prior was at least 20, historical MAE becomes
4.732 / 4.834; opening-week MAE becomes **5.443**, from 5.561. Opening-week
minutes MAE improves from **6.272 to 6.097**. The model still has substantial
uncertainty about first-week roles.

The additional holdout was collected after both context-family and minutes-tree
selection were saved. It contains 72 collected games, 1,504 conditional D1-player
rows, and 44 games / 546 appearances in the primary cohort. All verified-D1
accuracy also improves, from 4.421 to 4.349 on 1,338 appearances. This is a
newly collected **retrospective** check, not a claim these updated predictions
were issued before tipoff. The original history remains cut off after Nov 5;
Nov 6 box scores are labels only in this check.

## What was added

There are 52 new context features, layered on the previous champion:

- **Pre-college recruiting:** known-rating flag, grade, national-rank transforms,
  top-10/25/100 flags, years since class, high-school position/height and
  interactions that give pedigree more relevance when college history is short.
  Signed recruiting classes provide a limited incoming-talent context. Missing
  ratings are unknown, not a zero grade or a claim of low talent.
- **Persistent transfers:** previous-season dominant team and role, minutes,
  points and usage per 40, past starting share, current-team experience, and the
  fraction of recent history belonging to another team. Transfer context does
  not disappear after the first appearance at the new school.
- **Observed roster/depth:** the roster visible in the team's most recent
  completed game, including its DNP entries; returning prior-season minutes,
  incoming transfers' previous production, competition for roles and prior
  roster positions. The target game's roster and the final season roster are
  not supplied. Before a current-season roster has been observed, coverage is
  explicitly unknown.
- **Participation and season stage:** team games already completed, player's
  participation share, missed recent team games, inferred missing teammate
  minutes, current role experience and calendar phase. These describe observed
  past absences, not confirmed current injury status.

The original minutes and stat-rate model is retained as the foundation. Ridge
corrections adjust minutes and rates using new context. Shooting is rebuilt
from forecast attempts and conversion priors, preserving
`PTS = 2 × FGM + 3PM + FTM` and nested bonus probabilities.

A later validation-selected minutes correction learns nonlinear interactions
between old and new context: 120 shallow trees, up to 15 leaves, learning rate
0.05, minimum leaf size 100 and L2 penalty 10. These trees are exported as
numeric JSON and run without loading a pickle. A control using only old
features tests whether additional fitting capacity explains the improvement.

## Sources and identity audit

Historical recruiting was retrieved from ESPN's public basketball recruiting
class operations for **2018–2026**. These records supply `alternateId`, an
explicit link to the college athlete ID. We do not infer IDs from similar names.
The [existing SportsDataverse ESPN corpus](https://github.com/sportsdataverse/hoopR/blob/main/R/load_mbb.R)
provides the prior college box scores, rosters and schedule identity catalog.

Across 4,143 recruiting records:

- 1,804 records pass the explicit-ID and normalized-name join.
- 86 name conflicts are quarantined, including potentially valid nicknames.
- 571 have no college ID, and 1,682 fall outside the current identity catalog.
  They do not become individual player-grade features.
- Recruiting school-signing summaries can include records without a college ID;
  they do not identify a current playing roster or current starting lineup.

A class must strictly precede the ending season and the player's first observed
college season. Later recruiting-class records cannot change the prior of a
player already observed in college. Grades/ranks describe pre-college recruiting,
but were downloaded now; the provider responses are not original historical
HTTP captures. This archive-revision limitation remains explicit.

No CBBD API key was configured. No accounts were created and no subscriptions
were purchased. The useful numeric additions here do not depend on new paid
access. Genuine preseason depth charts, complete portal movement and coach
continuity remain separate data needs.

## Availability: audited, not backfilled with guessed timestamps

The [SEC archive](https://www.secsports.com/mbbreports-archive) links to a public
reporting application. Its public read-only archive operations were inspected
along with ACC and NCAA components in the same reporting application.
We retrieved **443 SEC**, **743 ACC** and **239 NCAA** archive rows. The queried
Big 12 archive returned no rows. This does not imply that Big 12 reporting does
not exist; it describes this particular archive response.

The records contain initial/game-day statuses and game date or round labels,
but **no report publication timestamps**. A game date or reporting-policy
deadline is not proof of when a particular status was available. All 1,425 rows
are therefore quarantined from historical features. Capture time today is
not relabeled as a historical observation time.

`tools/cbb_availability.py` includes the timestamp eligibility check for future
imports: an explicit timezone-aware publication/observation time must precede
both tipoff and the forecast information cutoff. Missing reports are not healthy
players. [SEC reporting policy](https://www.secsports.com/reports) is useful
background, but does not supply missing historical timestamps.

This model has **not** acquired validated historical injury/status features.
Current availability collection and player-to-game matching must be added to
the eventual live worker without inventing historical training coverage.

## Ablations and selection

MAE before the later nonlinear minutes correction:

| Added to frozen champion | 2025 | 2026 | Opening 2027 |
|---|---:|---:|---:|
| Previous champion | 4.13413 | 4.21013 | 5.27579 |
| More fitting capacity, old features only | 4.13303 | 4.20928 | 5.26800 |
| Recruiting | 4.13271 | 4.20704 | 5.22592 |
| Persistent transfer/role | 4.12796 | 4.20594 | 5.26589 |
| Observed depth | 4.12814 | 4.20660 | 5.25035 |
| Participation/stage | 4.13363 | 4.20980 | 5.26334 |
| Joint context | 4.12531 | 4.20223 | 5.20958 |
| Joint without recruiting | 4.12586 | 4.20473 | 5.25397 |
| Joint without depth | 4.12797 | 4.20341 | 5.22120 |

Joint context wins mean relative validation MAE. All variants screen the same
nine minute/rate ridge pairs, with an all-D1 validation guard against worsening
a fold by more than 0.5%. Both the 2025 and 2026 validation windows are Nov–Dec,
using baseline models fitted strictly before each validation period.

The new minutes study compares no extra tree, an old-feature capacity control,
and two context-aware tree configurations on those same validation windows.
The old-feature tree control worsens validation; adding the new context helps.
Opening 2027 outcomes select neither feature families nor hyperparameters.

Original benchmark windows were already inspected during previous research.
The new context choices use validation, but the entire research sequence is
not a pristine prospective trial. Selection files, raw source checksums and
cohort hashes are saved. Earlier source at fitting is preserved under the
local `data/cbb/context-study/source-at-fit/`; later changes add an optional
minutes-tree path and protect against using the wrong prediction reader.
They do not change forecasts of the earlier linear-only artifacts.

## Ranking and uncertainty

Historical same-origin weekly pair ordering is approximately unchanged:
**54.66% → 54.71%** in 2025 and **56.13% → 56.17%** in 2026. Regret modestly
improves. Opening-week ordering rises from 56.9% to 58.5% on only 65 pairs;
that sample is too small to establish a strong ranking improvement.

Nominal 80% coverage is 80.3% / 80.2% historically and **75.7%** in opening
week. Nominal 95% coverage is 95.1% / 94.8% / 93.4%. Opening ranges still
undercover, and the probability-of-playing problem remains untrained. The
point-projection improvement is better supported than an early-season claim
of fully calibrated confidence bands.

## Artifacts and reproducibility

- `models/cbb/model.json`: promoted offline default.
- `models/cbb/v1-model.json`: immutable previous champion for comparison.
- `models/cbb/context-model.json`: explicit new candidate path.
- [Checked-in comparison](cbb-context-results.json): ablations, confidence
  intervals, ranking, subgroup and new-day results.
- `data/cbb/context-review/{2025,2026,opening2027}-review.html`: interactive
  comparisons against the previous champion on identical appearances.
- `data/cbb/context-forward/results.json`: additional Nov 6 check and provenance.

The existing isolated CBB requirements apply. From the repository root:

```sh
.venv-cbb/bin/python tools/cbb_context_sources.py --recruits-only
.venv-cbb/bin/python tools/cbb_availability.py
.venv-cbb/bin/python tools/cbb_context_features.py
VECLIB_MAXIMUM_THREADS=2 OMP_NUM_THREADS=2 .venv-cbb/bin/python tools/cbb_context_study.py
VECLIB_MAXIMUM_THREADS=2 OMP_NUM_THREADS=2 .venv-cbb/bin/python tools/cbb_context_minutes.py
VECLIB_MAXIMUM_THREADS=2 OMP_NUM_THREADS=2 .venv-cbb/bin/python tools/cbb_context_forward.py
.venv-cbb/bin/python tools/cbb_context_finalize.py
```

Existing result folders reject source/protocol changes in place. Preserve the
original files and source-at-fit audit; use a clean research directory to
reproduce a changed protocol. The finalizer requires every original fold and
the additional day to improve before updating the offline default.

The context-aware CLI supports both old and new artifacts:

```sh
.venv-cbb/bin/python tools/cbb_predict.py evaluate \
  --model data/cbb/context-minutes/2026-model.json \
  --from 2026-01-01 --to 2026-04-16 --out data/cbb/context-review/check.json
.venv-cbb/bin/python tools/cbb_predict.py predict \
  --targets PATH/targets.json --out PATH/predictions.json
```

Evaluation validates context cohort, source-code, recruiting-data and corpus
fingerprints. Prospective prediction rebuilds context using prior closed-game
rosters and the recruiting source. The legacy baseline-only reader rejects a
context artifact instead of silently ignoring its corrections. The original
champion remains usable through the same CLI with `--model
models/cbb/v1-model.json`.

The [CBB production worker](cbb-production.md) now supplies scheduled refreshes,
current roster target enumeration and live application integration. Timestamped
availability observations and calibrated appearance probabilities remain future
work. Model deployment does not change the NBA coefficients or collection job.
