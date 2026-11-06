#!/usr/bin/env python3
"""Run NBA feature ablations and validation-selected combinations.

Uses the original fixed historical/current benchmarks. Selects features before
opening their test reports. All participation features use prior days only.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import random
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
GROUPS = ["teammate_minutes", "teammate_rates", "team_budget", "workload", "offseason", "pace", "competitiveness"]
FOLDS = {
    "historical": ("2025-10-01", "2026-01-01", "2026-07-01"),
    "current": ("2026-10-20", "2026-10-30", "2026-11-05"),
}


def digest(path):
    h = hashlib.sha256()
    with open(path, "rb") as source:
        for chunk in iter(lambda: source.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def paired(base_path, new_path):
    def read(path):
        result = {}
        with path.open() as source:
            for line in source:
                r = json.loads(line)
                key = (r["game"], r["player"])
                if key in result:
                    raise ValueError("duplicate paired player-game")
                result[key] = r
        return result
    base, new = read(base_path), read(new_path)
    if base.keys() != new.keys():
        raise ValueError("comparison cohorts differ")
    games = {}
    for key, a in base.items():
        b = new[key]
        if a["actual"] != b["actual"] or a["actual_minutes"] != b["actual_minutes"]:
            raise ValueError("comparison outcomes differ")
        gain = abs(a["predicted"] - a["actual"]) - abs(b["predicted"] - b["actual"])
        total, n = games.get(key[0], (0, 0))
        games[key[0]] = (total + gain, n + 1)
    values = [games[key] for key in sorted(games)]
    rng = random.Random(20261105)
    samples = []
    for _ in range(2000):
        selected = rng.choices(values, k=len(values))
        samples.append(sum(x[0] for x in selected) / sum(x[1] for x in selected))
    samples.sort()
    return {"mae_reduction": sum(x[0] for x in values) / sum(x[1] for x in values),
            "game_bootstrap_95_interval": [samples[49], samples[1949]], "games": len(games), "appearances": len(base)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", default="data/nba/history-enriched.jsonl")
    parser.add_argument("--out", default="data/nba/feature-study")
    parser.add_argument("--promote", action="store_true", help="make a selected candidate the offline default only if both held-out windows improve")
    args = parser.parse_args()
    dataset, output = (ROOT / args.data).resolve(), (ROOT / args.out).resolve()
    output.mkdir(parents=True, exist_ok=True)
    source_hash = hashlib.sha256()
    for path in sorted((ROOT / "backend/internal/nbamodel").glob("*.go")):
        if not path.name.endswith("_test.go"):
            source_hash.update(path.name.encode()); source_hash.update(path.read_bytes())
    source_hash.update((ROOT / "backend/cmd/nba-model/main.go").read_bytes())
    source_hash.update(Path(__file__).read_bytes())
    manifest = {"dataset_sha256": digest(dataset), "source_sha256": source_hash.hexdigest(), "folds": FOLDS,
                "screening_rule": "mean relative validation MAE reduction >=0.1%; no fold worsens by >0.5%",
                "selection_rule": "lowest mean relative validation MAE across singles and screened/all combinations, subject to the same per-fold guard",
                "notes": ["Reuses previously inspected benchmarks; improvements are exploratory and require future-game confirmation.",
                          "Historical availability and roster membership are inferred from prior box scores, not target-game participants.",
                          "No archived pregame injury reports, confirmed lineups, minutes restrictions or betting spreads are available."]}
    manifest_path = output / "manifest.json"
    if manifest_path.exists() and json.loads(manifest_path.read_text()) != json.loads(json.dumps(manifest)):
        raise ValueError("study source/data/protocol changed; use a fresh --out directory")
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    env = dict(os.environ, GOCACHE=os.environ.get("GOCACHE", "/tmp/cbb-go-cache"))

    def command(name, fold, phase, extra):
        report = output / f"{name}-{fold}-{phase}.json"
        if report.exists():
            return json.loads(report.read_text())
        log = output / f"{name}-{fold}-{phase}.log"
        start = time.monotonic()
        with log.open("w") as stream:
            result = subprocess.run(["go", "run", "./cmd/nba-model", phase, "--data", str(dataset), "--report", str(report)] + extra,
                                    cwd=ROOT / "backend", env=env, stdout=subprocess.DEVNULL, stderr=stream)
        if result.returncode:
            raise RuntimeError(log.read_text()[-5000:])
        r = json.loads(report.read_text())
        if phase == "train":
            score = min(c["validation"]["mae"] for c in r["validation_candidates"])
            print(f"{name} / {fold}: validation MAE {score:.5f} ({time.monotonic()-start:.1f}s)", flush=True)
        else:
            print(f"{name} / {fold}: test MAE {r['evaluation']['model']['mae']:.5f}", flush=True)
        return r

    variants = {"baseline": []}
    variants.update({name: [name] for name in GROUPS})
    fitted = {}

    def fit(name, features):
        fitted[name] = {}
        for fold, (train, test, _) in FOLDS.items():
            model = output / f"{name}-{fold}-model.json"
            report = command(name, fold, "train", ["--features", ",".join(features), "--train-before", train, "--test-from", test,
                                                    "--skip-evaluation", "--model", str(model)])
            fitted[name][fold] = min(c["validation"]["mae"] for c in report["validation_candidates"])

    for name, features in variants.items():
        fit(name, features)
    baseline = fitted["baseline"]

    def relative(name):
        return sum(fitted[name][fold] / baseline[fold] for fold in FOLDS) / len(FOLDS)

    def guarded(name):
        return all(fitted[name][fold] / baseline[fold] <= 1.005 for fold in FOLDS)

    screened = [name for name in GROUPS if relative(name) <= .999 and guarded(name)]
    for name, features in [("combined_screened", screened), ("combined_all", GROUPS)]:
        if len(features) > 1:
            variants[name] = features
            fit(name, features)
    selected = min((name for name in variants if guarded(name)), key=relative)
    selection = {"screened_groups": screened, "selected_variant": selected, "features": variants[selected], "validation_scores": fitted,
                 "mean_relative_validation_mae": {name: relative(name) for name in variants}}
    (output / "selection-before-testing.json").write_text(json.dumps(selection, indent=2) + "\n")
    print("Validation selection:", selected, variants[selected], flush=True)

    evaluations = {}
    for name in variants:
        evaluations[name] = {}
        for fold, (_, start, end) in FOLDS.items():
            report = command(name, fold, "evaluate", ["--model", str(output / f"{name}-{fold}-model.json"), "--test-from", start,
                              "--test-to", end, "--errors", str(output / f"{name}-{fold}-errors.jsonl")])
            evaluations[name][fold] = report["evaluation"]
        if name == "baseline":
            for fold in FOLDS:
                frozen = json.loads((ROOT / f"data/nba/{fold}-report.json").read_text())["evaluation"]["model"]["mae"]
                actual = evaluations[name][fold]["model"]["mae"]
                if abs(frozen - actual) > 1e-10:
                    raise ValueError(f"baseline replay changed for {fold}: {frozen} vs {actual}")
    comparisons = {}
    for name in variants:
        if name == "baseline":
            continue
        comparisons[name] = {fold: paired(output / f"baseline-{fold}-errors.jsonl", output / f"{name}-{fold}-errors.jsonl") for fold in FOLDS}
    improved = selected != "baseline" and all(comparisons[selected][fold]["mae_reduction"] > 0 for fold in FOLDS)
    if args.promote and improved:
        old_default = ROOT / "data/nba/model.json"
        backup = ROOT / "data/nba/v1-model.json"
        if not backup.exists():
            shutil.copyfile(old_default, backup)
        source = output / f"{selected}-current-model.json"
        shutil.copyfile(source, ROOT / "data/nba/improved-model.json")
        temporary = old_default.with_suffix(".json.tmp")
        shutil.copyfile(source, temporary); temporary.replace(old_default)
    results = {"manifest": manifest, "selection": selection, "variants": variants, "evaluations": evaluations,
               "paired_comparisons": comparisons, "selected_improved_both_benchmarks": improved, "promoted_offline_default": bool(args.promote and improved)}
    (output / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    print("Study complete; selected:", selected, "improved both benchmarks:", improved, "promoted:", bool(args.promote and improved), flush=True)


if __name__ == "__main__":
    main()
