import unittest
import datetime as dt
from nba_live_refresh import merge_history, availability_fallback, merge_availability

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

    def test_injury_fallback_scoped_to_next_game_and_preserves_official(self):
        now=dt.datetime(2026,11,6,15,tzinfo=dt.timezone.utc)
        def event(game,date):return {'id':game,'date':date,'competitions':[{'competitors':[{'team':{'id':'a'}},{'team':{'id':'b'}}]}]}
        events=[event('g','2026-11-06T23:00Z'),event('later','2026-11-07T23:00Z')]
        rosters={'a':[{'id':'p','injuries':[{'status':'Out','date':'2026-11-01'}]}],'b':[]}
        reports=availability_fallback(events,rosters,now)
        self.assertEqual(set(reports),{'g'})
        self.assertEqual(reports['g']['teams']['a']['players'][0]['status'],'Out')
        self.assertEqual(reports['g']['source'],'espn_roster_observed')
        official={'g':{'source':'nba_official','published_at':now.isoformat(),'teams':{'a':{'submitted':True,'players':[{'player_id':'p','status':'Available'}]}}}}
        merged=merge_availability(official,reports)
        self.assertEqual(merged['g']['teams']['a']['players'][0]['status'],'Available')
        self.assertEqual(merged['g']['teams']['b']['source'],'espn_roster_observed')
