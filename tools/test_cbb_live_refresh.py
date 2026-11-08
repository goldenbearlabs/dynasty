"""Runtime causal parity, publication, target and recovery checks."""
import datetime as dt
import json
from pathlib import Path
import tempfile
import unittest
import urllib.error
from unittest.mock import patch

import numpy as np
import pyarrow as pa
import pyarrow.parquet as pq
from cbb_live_refresh import make_targets, locked_records, merge_corpus, summary_rows, atomic_json, timestamp, collect_rosters

UTC = dt.timezone.utc


class LiveRefreshTests(unittest.TestCase):
    def test_missing_roster_does_not_abort_other_teams(self):
        from unittest.mock import Mock
        feed = Mock()
        feed.fetch.side_effect = [urllib.error.HTTPError('url', 404, 'missing', {}, None),
                                 {'athletes': [{'id': '123'}]}, {'athletes': []}]
        rosters, unavailable = collect_rosters(feed, {1, 2, 3})
        self.assertEqual(rosters, {2: [{'id': '123'}]})
        self.assertEqual([r['team'] for r in unavailable], [1, 3])

    def test_transient_roster_failure_and_duplicate_identity_still_fail(self):
        from unittest.mock import Mock
        feed = Mock()
        feed.fetch.side_effect = urllib.error.HTTPError('url', 503, 'unavailable', {}, None)
        with self.assertRaises(urllib.error.HTTPError): collect_rosters(feed, {1})
        feed.fetch.side_effect = None
        feed.fetch.return_value = {'athletes': [{'id': '1'}, {'id': '1'}]}
        with self.assertRaisesRegex(ValueError, 'Duplicate'): collect_rosters(feed, {1})
        feed.fetch.return_value = {'athletes': []}
        with self.assertRaisesRegex(ValueError, 'No upcoming'): collect_rosters(feed, {1})

    def event(self):
        return {'id': '12', 'season': {'year': 2027, 'type': 2}, 'competitions': [{
            'date': '2026-11-08T20:00:00Z', 'neutralSite': False, 'status': {'type': {'state': 'pre'}},
            'competitors': [{'team': {'id': '1'}, 'homeAway': 'home'}, {'team': {'id': '2'}, 'homeAway': 'away'}]}]}

    def test_current_rosters_only_enumerate_pregame_targets(self):
        before = dt.datetime(2026, 11, 8, 19, tzinfo=UTC)
        people = {1: [{'id': '123', 'stats': {'pts': 100}, 'starter': True, 'injuries': ['Out']}]}
        targets = make_targets([self.event()], people, {1: 8, 2: 23}, before)
        self.assertEqual(len(targets), 1)
        self.assertEqual(targets[0]['opponent_conference'], 23)
        self.assertNotIn('stats', targets[0]); self.assertNotIn('starter', targets[0]); self.assertNotIn('injuries', targets[0])
        self.assertEqual(make_targets([self.event()], people, {}, before + dt.timedelta(hours=2)), [])

    def test_started_forecasts_locked_and_expired_games_pruned(self):
        now = dt.datetime(2026, 11, 8, 22, tzinfo=UTC)
        record = {'target': {'starts_at': '2026-11-08T20:00:00Z'}, 'forecast_at': '2026-11-08T19:00:00Z'}
        old = {'records': {'good': record, 'posttip': {**record, 'forecast_at': '2026-11-08T21:00:00Z'},
                           'future': {**record, 'target': {'starts_at': '2026-11-09T20:00:00Z'}},
                           'expired': {**record, 'target': {'starts_at': '2026-09-01T20:00:00Z'}}}}
        self.assertEqual(locked_records(old, now), {'good': record})

    def test_corrections_replace_whole_game_preserve_other_history(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            old = [{'game_id': 1, 'player_id': 1, 'starts_at': '2026-11-01', 'pts': 3.},
                   {'game_id': 1, 'player_id': 2, 'starts_at': '2026-11-01', 'pts': 4.},
                   {'game_id': 2, 'player_id': 3, 'starts_at': '2026-11-02', 'pts': 5.}]
            pq.write_table(pa.Table.from_pylist(old), root / 'history.parquet')
            atomic_json(root / 'games.json', [{'game_id': 1, 'starts_at': '2026-11-01'}, {'game_id': 2, 'starts_at': '2026-11-02'}])
            merge_corpus(root, [{**old[0], 'pts': 10.}], [{'game_id': 1, 'starts_at': '2026-11-01'}])
            rows = pq.read_table(root / 'history.parquet').to_pylist()
            self.assertEqual([(r['game_id'], r['player_id'], r['pts']) for r in rows], [(1, 1, 10.), (2, 3, 5.)])
            self.assertFalse((root / 'corpus-writing').exists())

    def test_summary_excludes_corrupt_games_and_accepts_postseason(self):
        row = {'game_id': '12', 'player_id': '1', 'name': 'Player', 'team': '1', 'opponent': '2',
               'season': 2027, 'position': 'G', 'played': True, 'minutes': 20., 'minutes_missing': False,
               'starter': True, 'stats': {k: 0. for k in ['pts','reb','ast','stl','blk','tov','tpm','fgm','fga','ftm','fta','dd','td','pts40','pts50','tpa','pf']}}
        other = {**row, 'player_id': '2', 'team': '2', 'opponent': '1'}
        c = self.event()['competitions'][0]
        c['status'] = {'type': {'completed': True}, 'period': 2}
        for side in c['competitors']: side['score'] = '0'
        statistics = [{'name': k, 'displayValue': v} for k, v in [
            ('fieldGoalsMade-fieldGoalsAttempted','0-50'), ('freeThrowsMade-freeThrowsAttempted','0-10'),
            ('offensiveRebounds','10'), ('totalTurnovers','10'), ('turnovers','10')]]
        payload = {'header': {'id': '12', 'season': {'year': 2027, 'type': 3}, 'competitions': [c]},
                   'boxscore': {'teams': [{'statistics': statistics}, {'statistics': statistics}]}}
        with patch('cbb_live_refresh.parse_boxscore', return_value=[row, other]):
            rows, game = summary_rows(payload, {1:8, 2:23})
            self.assertEqual(game['season_type'], 3)
            self.assertAlmostEqual(game['possessions'], 54.75)
            self.assertTrue(rows[0]['d1_matchup'])
            c['status'] = {'type': {'completed': True, 'detail': 'Final/2OT'}}
            self.assertEqual(summary_rows(payload, {1:8,2:23})[1]['duration'], 50)
            c['competitors'][0]['score'] = '20'
            with self.assertRaisesRegex(ValueError, 'score mismatch'): summary_rows(payload, {1:8,2:23})


if __name__ == '__main__': unittest.main()
