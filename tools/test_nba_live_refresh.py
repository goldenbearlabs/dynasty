import unittest
from nba_live_refresh import merge_history

class LiveRefreshTests(unittest.TestCase):
    def test_corrections_replace_entire_game_and_preserve_history(self):
        old=[dict(game_id='a',player_id='p',starts_at='2026-11-01',points=10),dict(game_id='b',player_id='p',starts_at='2026-11-02',points=20),dict(game_id='b',player_id='gone',starts_at='2026-11-02',points=0)]
        new=[dict(game_id='b',player_id='p',starts_at='2026-11-02',points=21)]
        merged=merge_history(old,new)
        self.assertEqual([r['game_id'] for r in merged],['a','b'])
        self.assertEqual(merged[-1]['points'],21)
        self.assertEqual(merge_history(merged,new),merged)

    def test_duplicate_outcomes_rejected(self):
        row=dict(game_id='a',player_id='p',starts_at='2026-11-01')
        with self.assertRaises(ValueError):merge_history([],[row,row])
