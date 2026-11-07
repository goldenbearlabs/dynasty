# CBB projection model and research harness

Built November 6, 2026. The offline candidate is `models/cbb/model.json`.
CBB live projections have not been switched, and the deployed NBA model is
unchanged. This release includes historical ingestion, audited data, trained
models, feature ablations, chronological evaluation, same-origin game ordering,
uncertainty diagnostics, a prospective prediction command and interactive reviews.

## Accuracy

Fantasy-point MAE under the application's default basketball scoring. The
primary cohort is positive-minute appearances in verified Division I matchups
for the eight conferences eligible under the application's default CBB rules:
ACC, Big Ten, Big 12, Big East, SEC, Mountain West, Atlantic 10 and Pac-12.
Conference membership comes from each historical schedule, not today's list.

| Test | Games involving eligible players | Player appearances | Conditional proxy | Player-only learned model | Combined linear model | Final model |
|---|---:|---:|---:|---:|---:|---:|
| Jan–Apr 2025 | 1,126 | 20,903 | 4.432 | 4.243 | 4.160 | **4.134** |
| Jan–Apr 2026 | 1,118 | 20,541 | 4.520 | 4.327 | 4.234 | **4.210** |
| Nov 2–5, 2026 | 123 | 1,537 | 5.996 | 5.807 | 5.601 | **5.276** |

The final model adds a shallow nonlinear minutes correction to the combined
linear model's minutes/rate architecture. Its reduction versus the conditional
proxy is 6.7%, 6.9% and 12.0%. Game-bootstrap 95% intervals for the point-MAE
reduction are [0.275, 0.319], [0.286, 0.335] and [0.567, 0.882]. Intervals do
not account for repeated research or model selection.

For players whose **pregame prior** was at least 20 minutes, final MAE is
4.745 / 4.843 / 5.561. This prevents the headline from hiding the difficulty
of forecasting established fantasy rotation players. No main metric removes
outliers or filters on the target's actual minutes beyond requiring a positive
appearance. NBA and CBB MAEs describe different game lengths and cohorts;
these numbers do not establish that one sport is intrinsically easier.

## Data and quality checks

Source: [SportsDataverse's published ESPN archive loaders](https://github.com/sportsdataverse/hoopR/blob/main/R/load_mbb.R)
and the [underlying bulk releases](https://github.com/sportsdataverse/sportsdataverse-data/releases).
Player/team box scores and schedules were downloaded for ending seasons
2023–2027. This is four full seasons plus the start of 2026–27. Bulk downloads
avoid tens of thousands of per-game requests. SportsDataverse and ESPN share
an upstream source; they are not independent confirmations of the same stats.

- 25,366 completed scheduled games through sports day November 5, 2026.
- 25,279 retained games, 806,687 player rows and 508,323 positive-minute
  appearances. The current preseason-fitted model uses 421,727 prior D1-player
  training appearances after the initial warmup.
- 65 games excluded because player points did not sum to team scores; 22 lacked
  complete joined box-score coverage. These are corruption/coverage checks,
  not error-based outlier removal.
- Player identities, teams, opponents and schedule joins must agree. Duplicate
  player/game identities and impossible shooting arithmetic are rejected.
- DNPs, zero minutes and unknown minutes stay distinguishable from positive
  exposure. Archive null-minute counts include DNP entries; they are not all
  missing played-game measurements. Such entries do not train conditional
  minute/rate heads or enter accuracy metrics.
- Nine team/game minute sums outside the rounding tolerance are flagged in
  the audit. Individual lines and score sums passed; those games remain kept.
- Unknown conference identifiers make a matchup **unverified**, not automatically
  non-D1. These rows remain separately identifiable. Main strength ratings and
  reported primary accuracy use verified D1-versus-D1 matchups.
- Both regular-season and postseason games are retained. Final scores, target
  starts, target position, target fouls, tournament seeds, polls and final
  standings records are not pregame features.

Provenance: `data/cbb/raw/manifest.json` has download URLs and file checksums;
`data/cbb/dataset-audit.json` has exclusions and normalized-data checksums.
Archives were downloaded now and can contain later statistical corrections.
They are not original historical HTTP captures, and historical schedule
publication times are not verified by this archive.

## Forecast structure

The model predicts minutes conditional on playing, then stat rates per minute.
Projected stat = expected minutes × expected per-minute rate. Per-40 or per-60
rates are display conversions of the same rates. It uses a **40-minute**
regulation game and a **200-minute** team context. Historical overtime is
retained; team tempo is normalized back to 40 minutes. Forecast minutes are
bounded to 0.5–40; overtime creates some unavoidable positive-minute tails.

The core linear model learns ridge corrections to smoothed player priors.
Older seasons have smaller weights. Feature histories use a 10-appearance
half-life and 0.35 carry per earlier season; rate priors have 120 minutes of
positional pseudo-exposure. Fit weights decay by 0.7 per older training season.
Each stat is projected separately, including steals, blocks, turnovers,
shooting volume and the league's double/triple-double and scoring bonuses.

The final minutes correction is histogram gradient boosting: 120 trees,
15 leaves maximum, learning rate 0.06, minimum leaf sample 100 and L2 penalty
10. It learns residual minutes on the same permitted 45 minutes/context
features. It does not replace the rate heads. The correction and all linear
coefficients are exported as JSON; runtime prediction does not load pickles.
Exported JSON and the training estimator agree exactly on all three test sets.

## Features and calculations

| Family | Calculation and purpose |
|---|---|
| Player minutes | Weighted prior minutes, last 3/10 minutes, current/previous season means, past starting share, appearance counts, rest and a prior-team-change indicator. Target-game starting status is never supplied. |
| Player rates | Smoothed historical stat/minute rates; recent-10 and current-season deviations from those rates. Positional league priors stabilize short histories. Position is learned from prior appearances; first-ever appearances start with an unknown-position prior. |
| Team offense and defense | Our own schedule-adjusted points scored/allowed per 100 possessions, recalculated from completed prior D1 games. These separate offensive quality from defensive quality. |
| Opponent quality change | Target opponent's adjusted defense minus the defenses in the player's recent history. Transfer context compares the new team's offense with recent prior-team offensive environments. Prior unverified-opponent share is also supplied. |
| Tempo | Expected possessions, each team's adjusted tempo, and change versus the pace of the player's recent games. A fast game can increase opportunity independently of defensive quality. |
| Competitiveness | Signed/absolute expected margin, favorite and underdog blowout terms, and interaction with historical starting share. This permits different minute effects for stars and bench players. |
| Opponent by position | Actual production allowed to prior G/F/C players versus those players' pregame rate priors. Ratios shrink with 500 positional minutes of pseudo-exposure and are bounded to 0.5–1.5. These are bucket effects, not tracked individual defenders. |
| Previous meeting | Most recent meeting within 365 days, expressed as a per-minute deviation from the player's baseline and shrunk by meeting minutes / (meeting minutes + 120). |
| Role changes | Last-five median minutes, change from the preceding five, minute dispersion, starting-share changes, and prior short appearances with at least four fouls. No target-game foul count is used. |
| Teammates/rotation | Roster membership inferred from prior box scores, recent participation, expected team minutes, same-position competition and missing prior teammate minutes. Participation is estimated from previous games, not the target participant list. |
| Venue/schedule | Neutral court explicitly removes nominal home advantage. Includes home/away, rest, games in seven days and known postseason status. |
| Shooting opportunities | Forecast FGA/FTA, then estimate three-point attempt share and two-point/three-point/free-throw conversion with larger, slower priors. Makes and points obey PTS = 2×FGM + 3PM + FTM. |

No target outcomes enter a feature builder. Same-day outcomes are withheld
until the sports day closes, using the same Eastern 5 a.m. boundary as the app.

## Internal strength ratings

Possessions estimate = average of both teams' **FGA − OREB + 0.475×FTA + TOV**.
Tempo = possessions × 40 / actual game minutes. Efficiency = points /
possessions × 100. Using team box scores retains team rebounds/turnovers that
can be absent from summed individual lines.

Before each sports day, 14 damped opponent-adjustment iterations fit:

- Offensive efficiency after subtracting opponent defensive difficulty and
  a fixed home adjustment of 2 points per 100 possessions.
- Defensive efficiency after subtracting opponents' offensive quality and
  the corresponding home effect.
- Tempo after subtracting opponents' tempo relative to the league.

Each rating shrinks with six games of prior exposure. Previous-season rating
deviations carry at 35%; current-season games receive a 90-day recency half-life.
Expected pace combines both adjusted tempos around the league mean. Expected
margin combines offense and defense on both sides at that pace. No published
end-season KenPom/Torvik rating is replayed onto historical games. These are
our own regularized ratings, not a reproduction of either provider's formula.

## Individual and joint experiments

Eligible-conference test MAE, before the later nonlinear minutes correction:

| Added to player-only | 2025 | 2026 | Opening 2027 |
|---|---:|---:|---:|
| Player-only | 4.243 | 4.327 | 5.807 |
| Adjusted strength | 4.188 | 4.266 | 5.591 |
| Tempo | 4.235 | 4.326 | 5.820 |
| Position defense + previous meeting | 4.184 | 4.275 | 5.799 |
| Role state | 4.234 | 4.322 | 5.791 |
| Teammate rotation | 4.222 | 4.310 | 5.861 |
| Shooting opportunities | 4.244 | 4.315 | 5.772 |
| Combined | **4.160** | **4.234** | **5.601** |
| Combined without strength | 4.173 | 4.252 | 5.811 |
| Combined without tempo | 4.161 | 4.236 | 5.602 |

Strength and matchup history provide the clearest individual gains. Tempo
alone is small/inconsistent; removing it from the combination modestly worsens
all three checks. Rotation alone hurts opening week, when prior rosters are
least reliable. Shooting structure alone adds little but helps maintain a
coherent selected stat line. These are empirical comparisons, not causal
isolation of every interaction.

The minutes correction lowers eligible minutes MAE from the combined model's
4.745/4.713/7.143 to **4.660/4.621/6.272**. Its point-MAE gains were selected on
validation and confirmed on all three checks before offline adoption.

## Variation, ranking and uncertainty

Weekly ordering freezes player/team state before the first observed CBB sports
day of each week. Only the known target matchup/venue changes. Later scores,
roles and roster observations do not enter those forecasts. Players unknown
on the inferred origin roster are excluded and counted. These are comparisons
among actual positive-minute appearances, not availability-aware lineup picks.

| Metric | Player-only → final, 2025 | Player-only → final, 2026 |
|---|---:|---:|
| Within-player pair ordering | 52.47% → **54.66%** | 52.04% → **56.13%** |
| Regret from forecast-best game | 2.558 → **2.376** | 2.661 → **2.386** |

There are 8,304 and 8,143 non-tied historical pairs. Pairs sharing a player/week
are correlated. Opening-week ordering improves to 56.9% on only **65 pairs**;
that is too small for a firm claim. Broader current-week within-player
correlations cannot yet be estimated because nobody has five test games.

Three-game averages are sequential diagnostics, not three games forecast from
a single origin. Actual game-to-game spread is much larger than forecast
spread; artificially matching actual variance is not a target.

Uncertainty fits standardized validation residuals, with historical player
variance shrunk toward a pooled estimate. A fixed opening-calendar window
(November 1–15) gets its own residual pool; other games use the established
pool. This follow-on was motivated by opening-week undercoverage after the
initial linear benchmark was inspected, so its evaluation is exploratory.

Final 80%/95% interval coverage is **80.4%/95.0%** in 2025 and
**80.4%/94.8%** in 2026. Opening-week coverage is **74.2%/93.8%**: intervals
still understate early-season uncertainty. Mean 80% interval widths are
13.29/13.78/15.15 fantasy points. Do not present opening-week ranges as fully
calibrated guarantees.

Probabilities of beating the prior usual score improve Brier error versus a
validation-fitted base-rate competitor: **0.237 vs 0.247**, **0.237 vs 0.246**,
and **0.205 vs 0.249**. The usual-score threshold is a recent player mean; for
cold starts it is a pooled prior, not a personal historical percentile. Small
probability-tail buckets remain noisy; probability-bin counts and hit rates
are included in the machine-readable results.

## Evaluation protocol and baselines

Ending 2023 starts the history warmup. Learned fitting starts January 1, 2023.
For the 2025 fold, fit before Nov 1, 2024, select ridge penalties on Nov–Dec,
refit before Jan 1, 2025, then test through Apr 15. Repeat one year later for
2026. Each variant tests the same nine minute/rate ridge pairs (0.01/0.1/1).
Select the family combination by mean relative eligible validation MAE, with
no all-D1 validation fold allowed to worsen by more than 0.5%.

The 2027 opening check uses the later historical validation hyperparameters
and coefficients refit before Nov 1, 2026. It never selects parameters. Feature
histories update after closed prior sports days; fitted coefficients stay fixed.
All feature-family selection was saved before the initial holdout predictions.
The later minutes study screened 7 versus 15 leaves using historical validation
before opening its new test forecasts. The general linear research results
had already been inspected, so this is research, not a pristine prospective
experiment spanning every decision. Future games are the next stronger check.

The conditional proxy reconstructs the app's latest-season/full plus
previous-season/quarter-weight per-appearance rates with neutral market
factors. It does not replay old odds or injuries. The attendance-discount
proxy additionally uses average observed D1 team games as an approximation
for season length; production uses the maximum player-season game count.
Only the conditional version is the primary proxy comparison. Recent-10
averages and the player-only learned model are separate baselines.

Oracle actual-minute scores and easiest-90% error trims are labeled diagnostics,
never adopted forecasts or headline accuracy. Unknown minutes/DNPs are retained
in raw data but do not establish probability-of-playing accuracy. The model
cannot yet score a full game universe including DNP outcomes.

## Additional data sources and next priorities

1. **Roster turnover and freshmen.** Opening-week transfers score 6.34 MAE in
   the combined linear model; the new minute head helps but cannot identify
   freshman stars versus deep bench players from no history. Add preseason
   depth/roles, returning minutes, recruiting, transfers and coach continuity
   with timestamps before season start. [CollegeBasketballData documents](https://api.collegebasketballdata.com/data-availability)
   recruiting, portal history and roster operations; its API requires a key.
2. **Pregame availability and restrictions.** The [SEC report archive](https://www.secsports.com/mbbreports-archive)
   offers historical conference reports, and the [SEC policy](https://www.secsports.com/reports)
   covers basketball conference contests. The [Big 12's official announcement](https://big12sports.com/news/2025/8/13/general-big-12-conference-to-begin-player-availability-reporting-for-football-womens-and-mens-basketball.aspx)
   describes its conference-game reporting. The [NCAA's 2026 tournament program](https://www.ncaa.org/news/media-center-ncaa-releases-penalty-and-process-details-for-march-madness-player-availability-reports/)
   adds championship coverage. These do not establish universal coverage for
   every regular-season college game. No historical availability was supplied
   to this model, and target DNPs are never substituted for reports.
3. **Independent rating benchmarks.** [CBBData's daily Torvik archive](https://cbbdata.aweatherman.com/reference/cbd_torvik_ratings_archive.html)
   can test whether outside ratings add value over ours. Access needs an
   account/key; KenPom access separately requires an authorized subscription.
   Use strictly prior-date snapshots and audited team-ID crosswalks. A current
   or end-season rating file is not an acceptable historical feature source.
4. **Shot profile and lineup fit.** CBBD documents shooting, substitution and
   lineup operations. Rim/three-point opportunity, assist opportunities and
   rebounding competition may help distinguish matchup types beyond G/F/C.
   Their coverage must be measured before use.
5. **Prospective logging and stage calibration.** Lock forecasts before tip,
   collect current roster/report observations and retain DNP outcomes. Assess
   opening weeks and conference play separately before enabling unattended
   live CBB forecasts. Improve the current 74% coverage of nominal 80% ranges.

No accounts were created, subscriptions purchased or credentials requested.
We have not tested an external ratings provider against the internal ratings;
the current gains come from the audited bulk corpus and our own features.

## Reproduce and use

From the repository root, with Python 3.13 (the tested version):

```sh
python3 -m venv .venv-cbb
.venv-cbb/bin/pip install -r tools/requirements-cbb.txt
.venv-cbb/bin/python tools/cbb_history.py
.venv-cbb/bin/python tools/cbb_dataset.py --through 2026-11-05
.venv-cbb/bin/python tools/cbb_features.py
VECLIB_MAXIMUM_THREADS=2 OMP_NUM_THREADS=2 .venv-cbb/bin/python tools/cbb_study.py
VECLIB_MAXIMUM_THREADS=2 OMP_NUM_THREADS=2 .venv-cbb/bin/python tools/cbb_minutes_study.py
VECLIB_MAXIMUM_THREADS=2 OMP_NUM_THREADS=2 .venv-cbb/bin/python tools/cbb_export.py
.venv-cbb/bin/python -m unittest discover -s tools -p 'test_cbb*.py'
```

The studies save source/data fingerprints and reject changed protocols in the
same result directory. Downloaded releases can be revised: preserve the raw
manifest and exact files for numerical reproduction. Features are cached in
an ignored local NPZ; the model is portable JSON with an explicit feature schema.
The nonlinear-study pickles are trusted local intermediate artifacts only.

Evaluate any subset of the corresponding historical model's legal test range:

```sh
.venv-cbb/bin/python tools/cbb_predict.py evaluate \
  --model data/cbb/final-study/2026-model.json \
  --from 2026-01-01 --to 2026-04-16 \
  --games ESPN_GAME_ID --players ESPN_PLAYER_ID --out data/cbb/subset-report.json
```

Omit the game/player filters for a full fold. The CLI's eligible 2026 MAE
matches the study's **4.2101276185** exactly. An evaluation starting before the
artifact's fitting cutoff is rejected.

For prospective targets:

```sh
.venv-cbb/bin/python tools/cbb_predict.py predict \
  --targets data/cbb/upcoming-targets.json --out data/cbb/upcoming-predictions.json
```

Targets are a JSON list of `game_id`, `player_id`, `team`, `opponent`, `season`,
`starts_at` with timezone, boolean `home` and `neutral`, plus optional historical
conference IDs and known `season_type` (2 or 3). Outcome fields such as minutes,
starts, stats and position are rejected. `--observed-at` supports labeled
historical as-of replay. The prospective command reconstructs state using only
prior sports days as of observation; form/rest stay at that origin while the
known target matchup varies. It does not anticipate intervening results or
future lineup news. `--scoring FILE` rescores the stat line; uncertainty and
selection remain specific to default scoring, so custom-score intervals are
not returned.

A real future-game target was scored before its tip time with coherent shots,
minutes, a stat line and uncertainty. That is a pipeline smoke check, not
prospective accuracy or a confirmation that the player is available.

Interactive reviews:

- `data/cbb/final-study/2025-review.html`: 20,903 eligible appearances.
- `data/cbb/final-study/2026-review.html`: 20,541 eligible appearances.
- `data/cbb/final-study/opening2027-review.html`: 1,537 eligible appearances.

Each supports player/game filters, model/baseline comparisons, error sorting,
a scatter plot and detailed stat/minute comparisons. Generate one with
`tools/cbb_report.py --predictions FILE --baseline-errors FILE --out FILE`.
The [compact checked-in results](cbb-model-results.json) include selections,
ablations, uncertainty, paired confidence intervals and dataset coverage.
