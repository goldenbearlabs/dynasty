import copy
import unittest

from nba_history import parse_boxscore


def fixture():
    players = []
    for team in ["1", "2"]:
        players.append({"team": {"id": team}, "statistics": [{
            "keys": ["minutes", "points", "rebounds", "assists", "fieldGoalsMade-fieldGoalsAttempted"],
            "athletes": [{"athlete": {"id": team + "0", "displayName": "Fixture", "position": {"abbreviation": "G"}},
                          "starter": True, "didNotPlay": False, "stats": ["32:30", "20", "10", "10", "8-15"]}]}]})
    return {"header": {"id": "game", "season": {"type": 2, "year": 2025},
                       "competitions": [{"date": "2025-01-01T00:00Z", "status": {"type": {"completed": True}},
                                         "competitors": [{"team": {"id": "1"}, "homeAway": "home"},
                                                         {"team": {"id": "2"}, "homeAway": "away"}]}]},
            "boxscore": {"players": players}}


class HistoryTests(unittest.TestCase):
    def test_minutes_sides_and_bonuses(self):
        rows = parse_boxscore(fixture())
        self.assertEqual(len(rows), 2)
        self.assertEqual(rows[0]["minutes"], 32.5)
        self.assertEqual(rows[0]["opponent"], "2")
        self.assertTrue(rows[0]["home"])
        self.assertFalse(rows[1]["home"])
        self.assertEqual(rows[0]["stats"]["dd"], 1)
        self.assertEqual(rows[0]["stats"]["td"], 1)
        self.assertEqual(rows[0]["stats"]["fga"], 15)
        self.assertEqual(rows[0]["starts_at"], "2025-01-01T00:00:00+00:00")

    def test_dnp_and_unknown_minutes_are_explicit(self):
        data = fixture()
        athlete = data["boxscore"]["players"][0]["statistics"][0]["athletes"][0]
        dnp = copy.deepcopy(athlete)
        dnp["athlete"]["id"] = "dnp"
        dnp["didNotPlay"] = True
        dnp["stats"] = []
        data["boxscore"]["players"][0]["statistics"][0]["athletes"].append(dnp)
        athlete["stats"][0] = "--"
        rows = parse_boxscore(data)
        self.assertTrue(rows[0]["minutes_missing"])
        self.assertEqual(rows[0]["minutes"], 0)
        self.assertFalse(rows[1]["played"])

    def test_nonregular_and_unfinished_games_excluded(self):
        data = fixture()
        data["header"]["season"]["type"] = 3
        self.assertEqual(parse_boxscore(data), [])
        data = fixture()
        data["header"]["competitions"][0]["status"]["type"]["completed"] = False
        self.assertEqual(parse_boxscore(data), [])

    def test_unidentified_dnp_is_not_a_player(self):
        data = fixture()
        group = data["boxscore"]["players"][0]["statistics"][0]
        group["athletes"].append({"athlete": {"shortName": "Unknown"}, "didNotPlay": True, "stats": []})
        self.assertEqual(len(parse_boxscore(data)), 2)
        group["athletes"][-1]["didNotPlay"] = False
        group["athletes"][-1]["stats"] = ["1", "2", "0", "0", "1-1"]
        with self.assertRaisesRegex(ValueError, "missing player ID"):
            parse_boxscore(data)


if __name__ == "__main__":
    unittest.main()
