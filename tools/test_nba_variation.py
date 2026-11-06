import copy
import json
from pathlib import Path
import tempfile
import unittest

from nba_variation import build_priors, metrics, pair_accuracy, Distribution, weekly_selection


def records(predictions):
    return [{"game": str(i), "player": "p", "actual": float(10 * (i + 1)), "predicted": float(p),
             "prior": {"day": f"2026-01-{i+1:02d}", "week": "2025-12-29", "name": "Player", "sd": 5}}
            for i, p in enumerate(predictions)]


class VariationTests(unittest.TestCase):
    def test_tailoring_only_helps_when_correct(self):
        correct = records([10, 20, 30, 40])
        reversed_rows = records([40, 30, 20, 10])
        flat = records([25, 25, 25, 25])
        amplified = records([20, 40, 60, 80])
        self.assertEqual(pair_accuracy(correct), (6, 6))
        self.assertEqual(pair_accuracy(reversed_rows), (0, 6))
        self.assertEqual(pair_accuracy(flat), (3, 6))
        good = metrics(correct, minimum_games=3)[0]
        louder = metrics(amplified, minimum_games=3)[0]
        self.assertGreater(louder["mae"], good["mae"])
        self.assertEqual(louder["within_player"]["pair_accuracy"], good["within_player"]["pair_accuracy"])
        self.assertEqual(weekly_selection(flat)["mean_points_left_on_table"], 15)
        self.assertEqual(weekly_selection(correct)["mean_points_left_on_table"], 0)

    def test_quantiles_and_strict_above_probability(self):
        sample = [{"actual": x, "predicted": 0, "player": "p", "prior": {"sd": 1, "median": 0}}
                  for x in [-2, -1, 0, 1, 2]]
        d = Distribution(sample, personalized=False, scaled=True)
        self.assertAlmostEqual(d.predict_quantile(sample[0], .5), 0, places=9)
        self.assertAlmostEqual(d.beat_usual(sample[0]), .4)
        self.assertLessEqual(d.predict_quantile(sample[0], .1), d.predict_quantile(sample[0], .9))

    def test_prior_features_exclude_target_same_day_and_future(self):
        rows = []
        for day in range(1, 6):
            for player in ["p", "q"]:
                rows.append({"game_id": str(day), "player_id": player, "name": player, "played": True, "minutes": 30,
                             "starts_at": f"2026-01-{day:02d}T23:00:00Z", "stats": {"pts": day * 10}})
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "history.jsonl"
            def priors(data):
                path.write_text("".join(json.dumps(r) + "\n" for r in data))
                return build_priors(path, {"pts": 1})
            before = priors(rows)
            changed = copy.deepcopy(rows)
            for r in changed:
                if int(r["game_id"]) >= 3:
                    r["stats"]["pts"] = 999
            after = priors(changed)
            self.assertEqual(before[("3", "p")], after[("3", "p")])
            self.assertEqual(before[("3", "p")]["history"], [10, 20])
            self.assertNotIn("actual_percentile", before[("3", "p")])


if __name__ == "__main__":
    unittest.main()
