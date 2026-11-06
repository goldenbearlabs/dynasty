import json
from pathlib import Path
import tempfile
import unittest

from nba_feature_study import paired


class PairedStudyTests(unittest.TestCase):
    def test_paired_game_bootstrap_direction_and_cohort(self):
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "base.jsonl"
            candidate = Path(directory) / "candidate.jsonl"
            rows = [{"game": str(i // 2), "player": str(i), "actual": 10,
                     "predicted": 12, "actual_minutes": 30, "predicted_minutes": 30} for i in range(4)]
            baseline.write_text("".join(json.dumps(r) + "\n" for r in rows))
            better = [dict(r, predicted=10) for r in rows]
            candidate.write_text("".join(json.dumps(r) + "\n" for r in better))
            result = paired(baseline, candidate)
            self.assertEqual(result["mae_reduction"], 2)
            self.assertEqual(result["game_bootstrap_95_interval"], [2, 2])
            self.assertEqual(result["games"], 2)
            self.assertEqual(result["appearances"], 4)
            candidate.write_text(json.dumps(better[0]) + "\n")
            with self.assertRaisesRegex(ValueError, "cohorts differ"):
                paired(baseline, candidate)


if __name__ == "__main__":
    unittest.main()
