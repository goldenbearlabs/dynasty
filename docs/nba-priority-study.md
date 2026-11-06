# NBA next-priority experiments — November 5–6, 2026

The selected combined model improves all three tested periods. It is now the
**offline default** at `data/nba/model.json`. The subsequent
[production integration](nba-production.md) now serves it in the application. The prior workload model is preserved at
`data/nba/workload-model.json`, and the original at `data/nba/v1-model.json`.

## Results

MAE is fantasy points per positive-minute player appearance, under our league
scoring. No outliers were removed. These are sequential pregame forecasts;
coefficients stay frozen while completed prior-day history updates.

| Added to workload baseline | Jan–Apr 2026 MAE | Oct 30–Nov 4, 2026 MAE |
|---|---:|---:|
| Workload baseline | 5.12130 | 5.06224 |
| Recent role changes | 5.11301 | 5.03697 |
| Stat-specific windows | 5.12268 | 5.06907 |
| Shot opportunities × conversion | 5.11899 | 5.06019 |
| Reported availability + teammate minutes | 5.00773 | 4.95967 |
| Same + forced 240-minute budget | 5.39557 | 5.30761 |
| Role + windows + opportunities | 5.11001 | 5.04485 |
| **All except forced budget: selected** | **5.00285** | **4.95569** |

Historical testing includes 734 games / 15,751 appearances. The current-season
window includes 50 games / 1,126 appearances. The selected reduction is 2.31%
and 2.10%, respectively. Paired game-bootstrap 95% intervals for the MAE
reduction are [0.0964, 0.1425] and [0.0233, 0.1945]. These intervals do not
account for repeated experimentation or feature selection.

Minutes MAE falls from 5.059 to **4.839** historically and 5.129 to **4.918**
currently. A separate earlier-season transfer check (746 games / 15,802
appearances, Jan–Apr 2025) improves fantasy MAE from **5.17852 to 5.08781**.
That recipe was chosen on later folds, so this is a retrospective robustness
check, not an untouched prospective holdout. Coefficients and rolling features
in that check use legal earlier cutoffs.

The current window reaches the 4s; the larger historical window remains just
over 5.00 and the earlier check is 5.09. We have not achieved consistent
sub-5 error across seasons.

## What the model now uses

The existing model learns minutes and stat rates from weighted prior
appearances, season form, recent form, home court, rest/workload, opponent
current-season defense by G/F/C bucket, and previous opponent meetings.
The additions are:

- **Role state:** median minutes over the last 5 and 15 appearances, recent
  changes in starting frequency, last-5 versus preceding-10 minute shifts,
  short/foul-heavy appearance frequency, minutes spread and shot-volume changes.
  These describe prior roles; target-game starts and minutes are never inputs.
- **Stat-specific smoothing:** FGA, FTA, turnovers and assists use half the
  selected baseline half-life and prior strength. Steals/blocks use twice the
  half-life and triple the prior. Rare bonus events use twice the half-life and
  six times the prior. This changes the historical rate features, not outcomes.
- **Opportunities and conversion:** learned FGA/FTA heads predict volume.
  Three-point attempt share uses a 10-game half-life and a 50-attempt prior;
  conversion uses a 75-game half-life and positional priors equivalent to 200
  two-point attempts, 300 threes and 100 free throws. Projected makes and points
  are coherent: 2PM = FGA × (1 − three-share) × 2P%; 3PM = FGA × three-share ×
  3P%; FTM = FTA × FT%; PTS = 2 × FGM + 3PM + FTM.
- **Reported availability and teammates:** team-report submission, own listed
  status, report age and known-out teammates' recent-10 minutes enter the
  model. In the rotation context, a reported Out overrides inferred
  participation to zero; Doubtful/Questionable/Probable multiply inferred
  participation by 0.25/0.5/0.9. These multipliers are assumptions, not calibrated
  play probabilities. Unlisted/unmatched players and unsubmitted reports are
  not automatically treated as healthy. Teammate minutes use recent-10 averages.

The availability bundle provides most of the improvement. It also changes
teammate-minute estimates and feature encoding, so the experiment does not
isolate the causal contribution of the raw injury data alone. Role state adds
a smaller gain. Smoothing and opportunity features alone have little evidence
of improving MAE; their inclusion follows the combined validation result.

The forced budget experiment is rejected. NBA regulation minutes total 240,
but rescaling forecasts with uncertain participation/rotation estimates still
hurts. A stronger team allocation model needs an explicit distribution over
who plays and how the coach allocates minutes, including overtime.

## Recovered official injury history

The [NBA official injury-report archive](https://official.nba.com/nba-injury-report-2026-27-season/)
and historical PDF objects provide timestamped pregame reports. For example,
[January 1, 2026](https://ak-static.cms.nba.com/referee/injury/Injury-Report_2026-01-01_12_00PM.pdf).
The earlier feature study used inferred attendance only; this follow-on
successfully recovered official reports.

The downloader uses one conservative snapshot before the earliest tip in each
slate, generally around 5 p.m. Eastern or earlier. It validates the timestamp
printed **inside the PDF**, not merely the filename: 383 older filename/header
time aliases were found. Reports after the earliest tip are rejected.

- 493 of 502 slates recovered; 3,737 of 3,811 games have reports.
- Both teams submitted in 3,554 reported games.
- 39,168 status entries matched player identities; 419 were unmatched and 9
  ambiguous. Unknown identities remain unknown.
- Zero invalid publication timestamps in the retained archive.
- 10,570 matched reported players also appear in that game's box score;
  a separate identity audit found zero team mismatches.

Names use normalized accents/suffixes and conservative unique-name matching.
The identity catalog spans the dataset but does not expose future outcome
statistics as features. Missing archive objects and unsubmitted teams remain
missing. The archive covers injury designations, not confirmed starting
lineups or structured minute restrictions. Printed publication times establish
nominal historical availability; they do not prove when a consumer received
an old PDF over HTTP.

## Variation and within-player ordering

Same-origin evaluation freezes player history and available information before
the first eligible game of each player-week. It does not supply later injury
reports from that week. The prior schedule is treated as known, as in the
previous variation study. Actual tied outcomes are omitted from pair accuracy.

| Same-origin metric | Historical baseline → selected | Current baseline → selected |
|---|---:|---:|
| Within-player pair accuracy | 54.22% → **54.40%** | 52.30% → **56.37%** |
| Regret from choosing the forecast-best game | 4.306 → **4.261** | 2.975 → **2.664** |

There are 15,398 historical pairs and only 369 current pairs. The current gain
is encouraging but needs a larger prospective sample. Correlated pairs are
not independent trials.

A separate pairwise logistic ranker was also tested with 33 pregame features,
equal player-week weights and L2 penalties chosen on validation. It preserves
the expected-point model. Against the workload baseline its test pair accuracy
fell to 53.89% historically and 52.03% currently. Historical regret improved
only slightly while current regret worsened. It remains experimental and is
not the default ranker. It has not yet been retested on the new combined
model's feature vectors.

Uncertainty was recalibrated for the selected combined model using strictly
earlier forecasts. Its 80% interval coverage is 78.4% historical / 83.9%
current; 95% coverage is 93.3% / 96.1%. A median/typical-score output improves
current MAE to 4.886 but worsens historical MAE to 5.045, so it does not replace
the expected-score forecast. Outliers remain included in every main MAE.

## Selection and reproducibility

Each variant uses the same six-point validation grid (half-life 10 or 25;
ridge 0.001, 0.01 or 0.1). Structural selection minimizes mean relative
validation MAE across the two folds, with no fold more than 0.5% worse. Selection
was saved before the new test scores were opened. The combined model narrowly
beats the availability bundle on this criterion; that margin is small.

Training/validation/test dates are recorded in `manifest.json`. Main historical
validation starts Oct 1, 2025, test starts Jan 1, 2026; current validation starts
Oct 20, 2026, test starts Oct 30. Coefficients are refit before each test starts.
These test windows have been reused for research; prospective results are the
next stronger check. DNPs remain in the input, but reported MAEs are conditional
on positive-minute appearances and do not establish calibrated availability.

From the repository root, on macOS with Swift/PDFKit and Go installed:

```sh
swiftc -module-cache-path /tmp/cbb-swift-cache tools/nba_pdf_text.swift -o /tmp/cbb-nba-pdftext
python3 tools/nba_injury_history.py
python3 tools/nba_priority_study.py --out data/nba/priority-study-replay
python3 tools/nba_priority_confirmation.py
python3 tools/nba_priority_distribution.py
python3 -m unittest discover -s tools -p 'test_nba*.py'
GOCACHE=/tmp/cbb-go-cache go -C backend test ./internal/nbamodel ./cmd/nba-model ./internal/ingest/espn
```

The runner itself does not promote artifacts. `promotion.json` records the
subsequent offline promotion and old/new SHA-256 values. Its results file's
`default_model_changed: false` describes the runner's action, not that later
promotion. `source-at-fit/` preserves experiment source; later report metadata,
CLI loading safeguards, clarified evaluation notes and runner cleanup do not
alter forecast arithmetic.
Use a fresh output directory to rerun against changed source; the original
manifest intentionally rejects a mismatched replay in place. Confirmation and
distribution scripts consume the recorded primary study directory.

Ranker inputs are compact error rows exported with `--freeze-week
--rank-features` from models refit before validation, and separately from test
models. Files are named `rank-{historical,current}-{validation,test}.jsonl`.
Once those exist, run `python3 tools/nba_rank_study.py`. The saved selection,
models and full metrics are retained alongside the input rows.

Artifacts:

- [Compact results](nba-priority-results.json): aggregate experiment results.
- `data/nba/priority-study/results.json`: full metrics, selections and intervals.
- `data/nba/priority-study/current-review.html`: interactive comparison on all
  1,126 current test appearances; no outlier filtering.
- `data/nba/priority-study/earlier-confirmation.json`: earlier-season check.
- `data/nba/priority-study/ranking-results.json`: separate ranker experiment.
- `data/nba/priority-study/distribution-results.json`: new uncertainty study.
- `data/nba/priority-study/promotion.json`: offline promotion provenance.

## Next priorities

1. **Prospective evaluation with real information cutoffs.** A one-time capture
   for nine Nov 6 pregame events is saved under `data/nba/live-snapshots/`.
   It records observation times, raw responses and checksums. Matching official
   reports were loaded into `data/nba/upcoming-injuries.json`, and one target
   was predicted end to end with publication metadata. History still ends
   Nov 4: this verifies the pipeline, not new predictive accuracy. No scheduled
   collector or live integration has been installed.
2. **Richer rotation state:** confirmed lineups, return-from-injury restrictions,
   position overlap and coach-specific bench roles. Separate probability of
   playing from minutes conditional on playing, then allocate a team rotation
   jointly. This directly targets the remaining minutes error. Evaluate these
   with prospectively captured information, including missing-data cases.
3. **Player-specific game variation:** compare shrinkage by player/role against
   the new model and assess interval calibration by minutes uncertainty and
   player. Avoid increasing forecast spread merely to resemble actual spread.
   Matchup ranking must improve on unchanged same-origin cohorts.
4. **Nonlinear interactions after data checks:** test small tree/boosting models
   for absence × role × opponent interactions using the identical time splits.
   Preserve this linear combined model as the comparison and reserve new
   games before tuning again.

For future collection:

```sh
python3 tools/nba_pregame_snapshot.py --date YYYY-MM-DD --summaries
python3 tools/nba_injury_history.py --scheduled-scoreboard PATH/scoreboard.json --observed-at RFC3339 --only-scheduled --out data/nba/upcoming-injuries.json
```

Use the scoreboard's recorded observation time; reports published later are
rejected. ESPN pregame starter flags are raw observations, not labeled confirmed
lineups. A provider such as [SportsDataIO](https://sportsdata.io/developers/coverages/nba)
advertises projected/confirmed lineups, but no subscription was purchased or
integrated in this work.
