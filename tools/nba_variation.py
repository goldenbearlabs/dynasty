"""Leakage-safe history and metrics for NBA within-player forecast variation.

All percentile references used in forecasts are built from completed PRIOR
sports days. Test-window demeaning is used only to SCORE within-player skill.
"""
from bisect import bisect_left, bisect_right
from collections import defaultdict, deque
import datetime as dt
import json
import math
from statistics import mean, median, pstdev
from zoneinfo import ZoneInfo

STATS = ["pts", "reb", "ast", "stl", "blk", "tov", "tpm", "fgm", "fga", "ftm", "fta", "dd", "td", "pts40", "pts50"]
EASTERN = ZoneInfo("America/New_York")


def sports_day(timestamp):
    return (dt.datetime.fromisoformat(timestamp.replace("Z", "+00:00")).astimezone(EASTERN) - dt.timedelta(hours=5)).date()


def empirical_cdf(sorted_values, value, midpoint=False):
    if not sorted_values:
        return .5
    right = bisect_right(sorted_values, value)
    return ((bisect_left(sorted_values, value) + right) / 2 if midpoint else right) / len(sorted_values)


def build_priors(path, scoring):
    """Return immutable pregame features. No same-day result enters its prior."""
    days = defaultdict(list)
    with open(path) as source:
        for line in source:
            row = json.loads(line)
            if row["played"] and row["minutes"] > 0:
                days[sports_day(row["starts_at"])].append(row)
    history = defaultdict(lambda: deque(maxlen=60))
    squared, count = 0.0, 0
    features = {}
    for day, rows in sorted(days.items()):
        pending = []
        for row in rows:
            values = list(history[row["player_id"]])
            recent = values[-20:]
            prior_mean = mean(recent) if recent else 10.0
            prior_median = median(recent) if recent else 10.0
            historical_mean = mean(values) if values else 0.0
            variance = sum((x - historical_mean) ** 2 for x in values)
            pooled_variance = squared / count if count else 49.0
            sd = math.sqrt((variance + 20 * pooled_variance) / (max(0, len(values) - 1) + 20))
            actual = sum(row["stats"].get(k, 0) * scoring.get(k, 0) for k in STATS)
            features[(row["game_id"], row["player_id"])] = {
                "day": day.isoformat(), "week": (day - dt.timedelta(days=day.weekday())).isoformat(),
                "name": row.get("name", ""), "history_n": len(values), "mean": prior_mean, "median": prior_median,
                "sd": max(1.0, sd), "history": sorted(values),
            }
            pending.append((row["player_id"], actual, prior_mean, len(values)))
        for player, actual, prior_mean, n in pending:
            if n >= 10:
                squared += (actual - prior_mean) ** 2
                count += 1
            history[player].append(actual)
    return features


def load_errors(path, priors):
    records = []
    seen = set()
    with open(path) as source:
        for line in source:
            record = json.loads(line)
            key = record["game"], record["player"]
            if key in seen:
                raise ValueError("duplicate game/player")
            seen.add(key)
            record = dict(record, prior=priors[key], actual_percentile=empirical_cdf(priors[key]["history"], record["actual"], True))
            records.append(record)
    return sorted(records, key=lambda r: (r["prior"]["day"], r["game"], r["player"]))


def pearson(a, b):
    if len(a) < 2:
        return None
    ma, mb = mean(a), mean(b)
    va, vb = sum((x - ma) ** 2 for x in a), sum((x - mb) ** 2 for x in b)
    if va <= 1e-12 or vb <= 1e-12:
        return None
    return sum((x - ma) * (y - mb) for x, y in zip(a, b)) / math.sqrt(va * vb)


def ranks(values):
    result = [0.0] * len(values)
    order = sorted(range(len(values)), key=lambda i: values[i])
    first = 0
    while first < len(order):
        last = first + 1
        while last < len(order) and values[order[last]] == values[order[first]]:
            last += 1
        for i in order[first:last]:
            result[i] = (first + last - 1) / 2 + 1
        first = last
    return result


def pair_accuracy(records, field="predicted", nearby_days=None):
    wins, pairs = 0.0, 0
    for i, a in enumerate(records):
        for b in records[i + 1:]:
            if nearby_days is not None and abs((dt.date.fromisoformat(a["prior"]["day"]) - dt.date.fromisoformat(b["prior"]["day"])).days) > nearby_days:
                continue
            actual = a["actual"] - b["actual"]
            if actual == 0:
                continue
            predicted = a[field] - b[field]
            wins += .5 if abs(predicted) <= 1e-12 else float(actual * predicted > 0)
            pairs += 1
    return wins, pairs


def weekly_selection(records, field="predicted"):
    weeks = defaultdict(list)
    for r in records:
        weeks[(r["player"], r["prior"]["week"])].append(r)
    hits, regret, chosen, random_regret, n = 0.0, 0.0, 0.0, 0.0, 0
    for group in weeks.values():
        if len(group) < 2:
            continue
        top = max(r[field] for r in group)
        selected = [r for r in group if abs(r[field] - top) <= 1e-12]
        best = max(r["actual"] for r in group)
        score = mean(r["actual"] for r in selected)
        hits += mean(float(r["actual"] == best) for r in selected)
        regret += best - score
        chosen += score
        random_regret += best - mean(r["actual"] for r in group)
        n += 1
    return {"player_weeks": n, "best_game_hit_rate": hits / n if n else None,
            "mean_points_left_on_table": regret / n if n else None,
            "random_choice_points_left_on_table": random_regret / n if n else None,
            "chosen_game_actual_mean": chosen / n if n else None,
            "note": "Conditional on observed positive-minute appearances; ties in forecast are evaluated as uniform random choice, never resolved using outcomes."}


def metrics(records, field="predicted", common_core=None, minimum_games=10):
    if not records:
        raise ValueError("empty metric cohort")
    errors = [abs(r[field] - r["actual"]) for r in records]
    sorted_errors = sorted(errors)
    n = len(records)
    percentile = lambda q: sorted_errors[min(n - 1, math.ceil(n * q) - 1)]
    players = defaultdict(list)
    for r in records:
        players[r["player"]].append(r)
    player_rows, centered_actual, centered_predicted = [], [], []
    wins = pairs = nearby_wins = nearby_pairs = 0
    all_wins = all_pairs = 0
    for player, group in players.items():
        aw, ap = pair_accuracy(group, field)
        all_wins += aw; all_pairs += ap
        if len(group) < minimum_games:
            continue
        actual = [r["actual"] for r in group]
        predicted = [r[field] for r in group]
        correlation = pearson(ranks(actual), ranks(predicted))
        w, p = pair_accuracy(group, field)
        nw, np = pair_accuracy(group, field, 14)
        wins += w; pairs += p; nearby_wins += nw; nearby_pairs += np
        ma, mp = mean(actual), mean(predicted)
        centered_actual.extend(x - ma for x in actual)
        centered_predicted.extend(x - mp for x in predicted)
        actual_sd, pred_sd = pstdev(actual), pstdev(predicted)
        player_rows.append({"player": player, "name": group[-1]["prior"]["name"], "games": len(group),
                            "actual_sd": actual_sd, "forecast_sd": pred_sd,
                            "forecast_to_outcome_sd_ratio": pred_sd / actual_sd if actual_sd else None,
                            "spearman": correlation, "pair_accuracy": w / p if p else None,
                            "mae": mean(abs(a - b) for a, b in zip(actual, predicted))})
    correlations = [r["spearman"] for r in player_rows if r["spearman"] is not None]
    ratios = [r["forecast_to_outcome_sd_ratio"] for r in player_rows if r["forecast_to_outcome_sd_ratio"] is not None]
    core = [abs(r[field] - r["actual"]) for r in records if common_core and (r["game"], r["player"]) in common_core]
    trends = {}
    for size in [3, 5]:
        block_errors = []
        for group in players.values():
            for start in range(0, len(group) - size + 1, size):
                block = group[start:start + size]
                if (dt.date.fromisoformat(block[-1]["prior"]["day"]) - dt.date.fromisoformat(block[0]["prior"]["day"])).days <= 21:
                    block_errors.append(abs(mean(r[field] for r in block) - mean(r["actual"] for r in block)))
        trends[str(size)] = {"blocks": len(block_errors), "mae_of_block_mean": mean(block_errors) if block_errors else None}
    tail_n = math.ceil(n * .1)
    result = {"n": n, "mae": mean(errors), "rmse": math.sqrt(mean((r[field] - r["actual"]) ** 2 for r in records)),
              "bias": mean(r[field] - r["actual"] for r in records), "median_absolute_error": median(errors),
              "p90_absolute_error": percentile(.9), "p95_absolute_error": percentile(.95),
              "largest_10_percent_error_share": sum(sorted_errors[-tail_n:]) / sum(errors) if sum(errors) else 0,
              "common_baseline_core_n": len(core), "common_baseline_core_mae": mean(core) if core else None,
              "normalized_mae_by_prior_sd": mean(abs(r[field] - r["actual"]) / r["prior"]["sd"] for r in records),
              "within_player": {"minimum_games": minimum_games, "players": len(player_rows),
                                "undefined_constant_correlations": len(player_rows) - len(correlations),
                                "macro_spearman_nonconstant": mean(correlations) if correlations else None,
                                "pair_accuracy": wins / pairs if pairs else None, "pairs": pairs,
                                "all_repeat_players_pair_accuracy": all_wins / all_pairs if all_pairs else None,
                                "all_repeat_players_pairs": all_pairs,
                                "nearby_14_day_pair_accuracy": nearby_wins / nearby_pairs if nearby_pairs else None,
                                "nearby_14_day_pairs": nearby_pairs,
                                "pooled_demeaned_pearson": pearson(centered_actual, centered_predicted),
                                "median_forecast_to_outcome_sd_ratio": median(ratios) if ratios else None},
              "weekly_game_selection": weekly_selection(records, field), "trend_blocks": trends}
    return result, player_rows


def transformed(record, config):
    anchor = record["prior"][config["anchor"]]
    return anchor + config["alpha"] * (record["predicted"] - anchor)


class PointCalibration:
    def __init__(self, config, records):
        self.config = config
        residuals = defaultdict(list)
        for r in records:
            value = transformed(r, config)
            key = r["player"] if config["offset"] == "player" else int(max(0, value) // 5)
            residuals[key].append(r["actual"] - value)
        all_residuals = [r["actual"] - transformed(r, config) for r in records]
        self.global_offset = median(all_residuals) if config["offset"] != "none" else 0
        self.offsets = {key: (len(values) * median(values) + 20 * self.global_offset) / (len(values) + 20)
                        for key, values in residuals.items()}

    def predict(self, r):
        value = transformed(r, self.config)
        kind = self.config["offset"]
        if kind == "none":
            return value
        if kind == "global":
            return value + self.global_offset
        key = r["player"] if kind == "player" else int(max(0, value) // 5)
        return value + self.offsets.get(key, self.global_offset)

    def artifact(self):
        return {"config": self.config, "global_offset": self.global_offset, "offsets": self.offsets}


class Distribution:
    """Frozen empirical residual CDF with optional shrunk player specificity."""
    def __init__(self, records, personalized=True, scaled=True):
        self.personalized, self.scaled = personalized, scaled
        self.global_values = sorted((r["actual"] - r["predicted"]) / (r["prior"]["sd"] if scaled else 1) for r in records)
        self.players = defaultdict(list)
        for r in records:
            self.players[r["player"]].append((r["actual"] - r["predicted"]) / (r["prior"]["sd"] if scaled else 1))
        self.players = {k: sorted(v) for k, v in self.players.items()}
        self.quantiles = {}

    def cdf(self, player, value):
        local = self.players.get(player, []) if self.personalized else []
        w = len(local) / (len(local) + 20)
        return (1 - w) * empirical_cdf(self.global_values, value) + w * empirical_cdf(local, value)

    def quantile(self, player, probability):
        key = player, probability
        if key not in self.quantiles:
            lo = min(self.global_values[0], min(self.players.get(player, self.global_values))) - 1
            hi = max(self.global_values[-1], max(self.players.get(player, self.global_values))) + 1
            for _ in range(55):
                mid = (lo + hi) / 2
                if self.cdf(player, mid) < probability:
                    lo = mid
                else:
                    hi = mid
            # Preserve F(q) >= probability at discrete CDF jumps.
            self.quantiles[key] = hi
        return self.quantiles[key]

    def predict_quantile(self, r, probability):
        return r["predicted"] + self.quantile(r["player"], probability) * (r["prior"]["sd"] if self.scaled else 1)

    def beat_usual(self, r):
        z = (r["prior"]["median"] - r["predicted"]) / (r["prior"]["sd"] if self.scaled else 1)
        return 1 - self.cdf(r["player"], z)

    def loss(self, records):
        losses = []
        for r in records:
            for q in [.1, .25, .5, .75, .9]:
                delta = r["actual"] - self.predict_quantile(r, q)
                losses.append((q - float(delta < 0)) * delta)
        return mean(losses)

    def evaluate(self, records):
        probabilities = [self.beat_usual(r) for r in records]
        events = [float(r["actual"] > r["prior"]["median"]) for r in records]
        bins = []
        for i in range(10):
            selected = [j for j, p in enumerate(probabilities) if min(9, int(p * 10)) == i]
            if selected:
                bins.append({"bin": i, "n": len(selected), "mean_probability": mean(probabilities[j] for j in selected),
                             "actual_above_usual_fraction": mean(events[j] for j in selected),
                             "actual_mean_prior_percentile": mean(records[j]["actual_percentile"] for j in selected)})
        coverage = {str(q): mean(float(r["actual"] <= self.predict_quantile(r, q)+1e-9) for r in records) for q in [.1, .25, .5, .75, .9]}
        intervals = {}
        for mass in [.8, .95]:
            lo, hi = (1 - mass) / 2, (1 + mass) / 2
            intervals[str(mass)] = {"coverage": mean(float(self.predict_quantile(r, lo)-1e-9 <= r["actual"] <= self.predict_quantile(r, hi)+1e-9) for r in records),
                                    "mean_width": mean(self.predict_quantile(r, hi) - self.predict_quantile(r, lo) for r in records)}
        predicted_pct = [empirical_cdf(r["prior"]["history"], r["predicted"], True) for r in records]
        actual_pct = [r["actual_percentile"] for r in records]
        return {"n": len(records), "brier_beating_prior_median": mean((p - y) ** 2 for p, y in zip(probabilities, events)),
                "brier_always_50_percent": .25, "actual_above_usual_rate": mean(events), "calibration_bins": bins,
                "quantile_coverage": coverage, "intervals": intervals, "mean_quantile_pinball_loss": self.loss(records),
                "point_percentile_spearman": pearson(ranks(predicted_pct), ranks(actual_pct)),
                "point_percentile_mae": mean(abs(a - b) for a, b in zip(predicted_pct, actual_pct))}

    def artifact(self):
        return {"personalized": self.personalized, "scaled_by_prior_sd": self.scaled,
                "global_standardized_residuals": self.global_values, "player_standardized_residuals": self.players,
                "player_prior_strength": 20}
