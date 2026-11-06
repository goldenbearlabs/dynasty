#!/usr/bin/env python3
"""Supplement the first ablations with practical combinations, selected on validation only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

from nba_feature_study import ROOT, FOLDS, paired

BUNDLES = {
    "workload_game_context": ["workload", "pace", "competitiveness"],
    "workload_teammates": ["workload", "teammate_minutes", "teammate_rates"],
    "context_without_budget": ["workload", "pace", "competitiveness", "offseason"],
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--study", default="data/nba/feature-study")
    parser.add_argument("--promote", action="store_true")
    args = parser.parse_args()
    output = ROOT / args.study
    initial = json.loads((output / "selection-before-testing.json").read_text())
    scores = initial["validation_scores"]
    dataset = ROOT / "data/nba/history-enriched.jsonl"
    env = dict(os.environ, GOCACHE=os.environ.get("GOCACHE", "/tmp/cbb-go-cache"))
    # Save the supplemental protocol before fitting or inspecting its test data.
    protocol = {"bundles": BUNDLES, "selection": "mean relative validation MAE with no fold >0.5% worse than baseline",
                "source_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    protocol_path = output / "combination-protocol.json"
    if protocol_path.exists() and json.loads(protocol_path.read_text()) != protocol:
        raise ValueError("combination protocol changed")
    protocol_path.write_text(json.dumps(protocol, indent=2) + "\n")

    def command(name, fold, phase, extra):
        path = output / f"{name}-{fold}-{phase}.json"
        if path.exists():
            return json.loads(path.read_text())
        start = time.monotonic()
        with (output / f"{name}-{fold}-{phase}.log").open("w") as log:
            result = subprocess.run(["go", "run", "./cmd/nba-model", phase, "--data", str(dataset), "--report", str(path)] + extra,
                                    cwd=ROOT / "backend", env=env, stdout=subprocess.DEVNULL, stderr=log)
        if result.returncode:
            raise RuntimeError((output / f"{name}-{fold}-{phase}.log").read_text()[-4000:])
        r = json.loads(path.read_text())
        score = min(c["validation"]["mae"] for c in r["validation_candidates"]) if phase == "train" else r["evaluation"]["model"]["mae"]
        print(f"{name} / {fold} / {phase}: MAE {score:.5f} ({time.monotonic()-start:.1f}s)", flush=True)
        return r

    for name, groups in BUNDLES.items():
        scores[name] = {}
        for fold, (train, test, _) in FOLDS.items():
            r = command(name, fold, "train", ["--features", ",".join(groups), "--train-before", train, "--test-from", test,
                        "--skip-evaluation", "--model", str(output / f"{name}-{fold}-model.json")])
            scores[name][fold] = min(c["validation"]["mae"] for c in r["validation_candidates"])
    def relative(name):
        return sum(scores[name][f] / scores["baseline"][f] for f in FOLDS) / len(FOLDS)
    selected = min((name for name in scores if all(scores[name][f] / scores["baseline"][f] <= 1.005 for f in FOLDS)), key=relative)
    selection = {"selected_variant": selected, "validation_scores": scores,
                 "mean_relative_validation_mae": {name: relative(name) for name in scores}}
    (output / "expanded-selection-before-testing.json").write_text(json.dumps(selection, indent=2) + "\n")
    print("Expanded validation selection:", selected, flush=True)
    while not (output / "results.json").exists():
        time.sleep(5)
    results = json.loads((output / "results.json").read_text())
    results["initial_selection"] = results["selection"]
    results["selection"] = selection
    results["combination_protocol"] = protocol
    for name, groups in BUNDLES.items():
        results["variants"][name] = groups
        results["evaluations"][name] = {}
        results["paired_comparisons"][name] = {}
        for fold, (_, start, end) in FOLDS.items():
            r = command(name, fold, "evaluate", ["--model", str(output / f"{name}-{fold}-model.json"), "--test-from", start,
                        "--test-to", end, "--errors", str(output / f"{name}-{fold}-errors.jsonl")])
            results["evaluations"][name][fold] = r["evaluation"]
            results["paired_comparisons"][name][fold] = paired(output / f"baseline-{fold}-errors.jsonl", output / f"{name}-{fold}-errors.jsonl")
    improved = selected != "baseline" and all(results["paired_comparisons"][selected][f]["mae_reduction"] > 0 for f in FOLDS)
    results["selected_improved_both_benchmarks"] = improved
    results["promoted_offline_default"] = bool(args.promote and improved)
    if args.promote:
        default = ROOT / "data/nba/model.json"
        backup = ROOT / "data/nba/v1-model.json"
        if improved:
            if not backup.exists():
                shutil.copyfile(default, backup)
            source = output / f"{selected}-current-model.json"
            shutil.copyfile(source, ROOT / "data/nba/improved-model.json")
            temporary = default.with_suffix(".json.tmp")
            shutil.copyfile(source, temporary); temporary.replace(default)
        elif backup.exists():
            # The final selection supersedes the first screening stage.
            shutil.copyfile(backup, default)
    (output / "expanded-results.json").write_text(json.dumps(results, indent=2) + "\n")
    print("Combinations complete:", selected, "improved both benchmarks:", improved, flush=True)


if __name__ == "__main__":
    main()
