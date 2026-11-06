#!/usr/bin/env python3
"""Evaluate within-player trend/ranking, outliers, and calibrated NBA uncertainty.

Requires chronological validation/test compact forecasts from the Go harness.
No observation from either test window fits point or distribution calibration.
"""
import argparse
from collections import defaultdict
import hashlib
import json
import math
from pathlib import Path
from statistics import mean

from nba_variation import build_priors, load_errors, metrics, PointCalibration, Distribution, empirical_cdf

ROOT = Path(__file__).resolve().parents[1]


def tuning_pairs(rows):
    weeks = defaultdict(list)
    for i, r in enumerate(rows):
        weeks[(r["player"], r["prior"]["week"])].append(i)
    pairs = []
    for indexes in weeks.values():
        for a, i in enumerate(indexes):
            for j in indexes[a + 1:]:
                if rows[i]["actual"] != rows[j]["actual"]:
                    pairs.append((i, j, rows[i]["actual"] > rows[j]["actual"]))
    return pairs


def score(rows, predictions, pairs):
    correct = sum(.5 if abs(predictions[i] - predictions[j]) <= 1e-12 else float((predictions[i] > predictions[j]) == outcome)
                  for i, j, outcome in pairs)
    return {"mae": mean(abs(r["actual"] - p) for r, p in zip(rows, predictions)),
            "pair_accuracy": correct / len(pairs) if pairs else .5, "pairs": len(pairs)}


def save(path, obj):
    Path(path).write_text(json.dumps(obj, indent=2, allow_nan=False) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", default="data/nba/variation-study")
    args = parser.parse_args()
    out = ROOT / args.out
    out.mkdir(parents=True, exist_ok=True)
    scoring = json.loads((ROOT / "data/nba/model.json").read_text())["selection_scoring"]
    dataset = ROOT / "data/nba/history-enriched.jsonl"
    print("Building prior-day player histories", flush=True)
    priors = build_priors(dataset, scoring)
    folds = {}
    for name in ["historical", "current"]:
        validation = load_errors(out / f"{name}-validation.jsonl", priors)
        test = load_errors(out / f"{name}-test.jsonl", priors)
        days = sorted(set(r["prior"]["day"] for r in validation))
        cutoff = days[max(1, math.floor(.6 * len(days)))]
        fit = [r for r in validation if r["prior"]["day"] < cutoff]
        tune = [r for r in validation if r["prior"]["day"] >= cutoff]
        pairs = tuning_pairs(tune)
        base_score = score(tune, [r["predicted"] for r in tune], pairs)
        baseline_errors = sorted(abs(r["actual"] - r["predicted"]) for r in test)
        threshold = baseline_errors[math.ceil(len(test) * .9) - 1]
        core = {(r["game"], r["player"]) for r in test if abs(r["actual"] - r["predicted"]) <= threshold}
        folds[name] = {"validation": validation, "fit": fit, "tune": tune, "test": test, "pairs": pairs,
                       "base_score": base_score, "core": core, "minimum": 10 if name == "historical" else 3}
        print(name, "calibration fit/tune/test:", len(fit), len(tune), len(test), flush=True)

    candidates = []
    for anchor in ["mean", "median"]:
        for alpha in [0, .5, .75, 1, 1.25, 1.5, 2]:
            for offset in ["none", "global", "player", "bucket"]:
                config = {"anchor": anchor, "alpha": alpha, "offset": offset}
                scores = {}
                for name, f in folds.items():
                    calibration = PointCalibration(config, f["fit"])
                    scores[name] = score(f["tune"], [calibration.predict(r) for r in f["tune"]], f["pairs"])
                relative = mean(scores[name]["mae"] / folds[name]["base_score"]["mae"] for name in folds)
                eligible = all(scores[name]["pair_accuracy"] >= folds[name]["base_score"]["pair_accuracy"] - .005 for name in folds)
                candidates.append({"config": config, "tuning": scores, "relative_mae": relative, "ranking_guard_passed": eligible})
    selected = min((c for c in candidates if c["ranking_guard_passed"]), key=lambda c: c["relative_mae"])
    best_mae = min(candidates, key=lambda c: c["relative_mae"])
    print("Point calibration selected on validation:", selected["config"], flush=True)

    distributions = []
    for personalized in [False, True]:
        for scaled in [False, True]:
            losses = {}
            for name, f in folds.items():
                d = Distribution(f["fit"], personalized, scaled)
                losses[name] = d.loss(f["tune"])
            distributions.append({"personalized": personalized, "scaled": scaled, "tuning_quantile_loss": losses})
    reference = distributions[1]["tuning_quantile_loss"]
    selected_distribution = min(distributions, key=lambda d: mean(d["tuning_quantile_loss"][name] / reference[name] for name in folds))
    selection = {"point": selected, "best_mae_without_ranking_guard": best_mae, "point_candidates": candidates,
                 "distribution": selected_distribution, "distribution_candidates": distributions,
                 "protocol": "Fit on first 60% of validation sports days; choose on last 40%; refit calibration on all validation before testing. Point selection rejects >0.5 percentage-point loss of within-player same-week pair accuracy on either tuning window."}
    save(out / "selection-before-testing.json", selection)
    print("Distribution selected on validation:", selected_distribution, flush=True)

    output = {"selection": selection, "folds": {}, "notes": [
        "All headline MAEs retain every positive-minute appearance. Core/outlier diagnostics are hindsight cohorts, never deployable filters.",
        "Block errors average consecutive one-game forecasts made at their respective times; they are not multi-game forecasts issued at block start.",
        "Within-player correlations and game selection are measured on test outcomes only for scoring. Prior percentiles and reference scores use completed prior days only.",
        "The current-season window spans six days and offers few games per player; historical within-player results are more informative.",
        "Benchmarks have been reused in development. Calibration improvements need future-game confirmation.",
        "Expected-score/stat models are unchanged. Median/typical-score calibration is a separate target and artifact; no automatic promotion."]}
    player_csv_rows = []
    for name, f in folds.items():
        print("Scoring variation:", name, flush=True)
        test = f["test"]
        result = {"methods": {}, "amplification": {}}
        methods = {}
        study = ROOT / "data/nba/feature-study"
        for path in sorted(study.glob(f"*-{name}-errors.jsonl")):
            variant = path.name.removesuffix(f"-{name}-errors.jsonl")
            if variant not in ["baseline", "workload", "teammate_minutes", "team_budget", "combined_all", "workload_teammates"]:
                continue
            records = load_errors(path, priors)
            if {(r["game"], r["player"]) for r in records} != {(r["game"], r["player"]) for r in test}:
                raise ValueError("variation comparison cohort changed")
            methods[variant] = records
        methods["rolling_player_mean"] = [dict(r, predicted=r["prior"]["mean"]) for r in test]
        methods["rolling_player_median"] = [dict(r, predicted=r["prior"]["median"]) for r in test]
        calibration = PointCalibration(selected["config"], f["validation"])
        distribution = Distribution(f["validation"], selected_distribution["personalized"], selected_distribution["scaled"])
        methods["calibrated_typical_score"] = [dict(r, predicted=calibration.predict(r)) for r in test]
        methods["distribution_median"] = [dict(r, predicted=distribution.predict_quantile(r, .5)) for r in test]
        for variant, records in methods.items():
            stats, players = metrics(records, common_core=f["core"], minimum_games=f["minimum"])
            result["methods"][variant] = stats
            player_csv_rows.extend(dict(row, fold=name, model=variant) for row in players)
        for alpha in [0, .5, 1, 1.5, 2]:
            c = PointCalibration({"anchor": "mean", "alpha": alpha, "offset": "none"}, [])
            records = [dict(r, predicted=c.predict(r)) for r in test]
            result["amplification"][str(alpha)] = metrics(records, common_core=f["core"], minimum_games=f["minimum"])[0]
        result["distribution"] = distribution.evaluate(test)
        same_origin_path=out/f"{name}-same-origin.jsonl"
        if same_origin_path.exists():
            frozen=load_errors(same_origin_path,priors)
            predictions=[r["predicted"] for r in frozen]
            flat=[dict(r,predicted=r.get("origin_reference_mean",0)) for r in frozen]
            same_report=json.loads((out/f"{name}-same-origin-report.json").read_text())["evaluation"]
            result["same_origin_matchup_test"]={
                "forecast":score(frozen,predictions,tuning_pairs(frozen)),
                "flat_reference":score(flat,[r["predicted"] for r in flat],tuning_pairs(flat)),
                "weekly_selection":metrics(frozen,minimum_games=f["minimum"])[0]["weekly_game_selection"],
                "flat_weekly_selection":metrics(flat,minimum_games=f["minimum"])[0]["weekly_game_selection"],
                "forecast_notes":same_report["notes"],
                "unknown_origin_roster_exclusions":same_report["skipped_unknown_frozen_origin_roster"],
                "note":"Player form/rest/workload are frozen at the origin; only future opponent/home context differs. Tests matchup ordering without intervening outcomes. Conditional on appearances of players on the inferred origin roster."}
        result["oracle_minutes_mae_diagnostic"] = mean(abs(r["diagnostics"]["actual_minutes_oracle"] - r["actual"]) for r in test)
        result["without_bonus_mae_diagnostic"] = mean(abs((r["predicted"] - r["diagnostics"]["predicted_bonus_points"]) -
                                                           (r["actual"] - r["diagnostics"]["actual_bonus_points"])) for r in test)
        output["folds"][name] = result
        save(out / f"point-calibration-{name}.json", {"fitted_before": min(r["prior"]["day"] for r in test), "calibration": calibration.artifact(),
             "note": "Typical-score calibration, separate from the expected fantasy score and stat projections."})
        save(out / f"distribution-{name}.json", {"fitted_before": min(r["prior"]["day"] for r in test), "distribution": distribution.artifact()})
        with (out / f"distribution-predictions-{name}.jsonl").open("w") as target:
            for r in test:
                target.write(json.dumps({"game": r["game"], "player": r["player"], "name": r["prior"]["name"], "day": r["prior"]["day"],
                    "actual": r["actual"], "expected": r["predicted"], "typical": calibration.predict(r),
                    "probability_above_usual": distribution.beat_usual(r), "usual_score": r["prior"]["median"],
                    "low80": distribution.predict_quantile(r, .1), "high80": distribution.predict_quantile(r, .9),
                    "actual_prior_percentile": r["actual_percentile"], "prior_games": r["prior"]["history_n"]}) + "\n")
        print(name, "full MAE", result["methods"]["workload"]["mae"], "typical MAE", result["methods"]["calibrated_typical_score"]["mae"],
              "within-player pairs", result["methods"]["workload"]["within_player"]["pair_accuracy"], flush=True)
    import csv
    with (out / "per-player.csv").open("w") as target:
        writer = csv.DictWriter(target, fieldnames=list(player_csv_rows[0])); writer.writeheader(); writer.writerows(player_csv_rows)
    output["provenance"] = {"dataset_sha256": hashlib.sha256(dataset.read_bytes()).hexdigest(),
                            "source_sha256": hashlib.sha256(Path(__file__).read_bytes() + (ROOT / "tools/nba_variation.py").read_bytes()).hexdigest(),
                            "calibration_validation_reports": {name: json.loads((out / f"{name}-validation-report.json").read_text())["model_trained_before"] for name in folds}}
    save(out / "results.json", output)
    print("Variation study complete", flush=True)


if __name__ == "__main__":
    main()
