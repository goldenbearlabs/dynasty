import datetime as dt
import unittest

from nba_injury_history import parse_report, normalized
from nba_pregame_snapshot import pregame_events
from nba_rank_study import fit, evaluate, utility, sigmoid


class PriorityTests(unittest.TestCase):
    def test_official_report_dates_statuses_and_missing_teams(self):
        text='''Injury Report: 01/01/26 12:00 PM
Game Date Game Time Matchup Team Player Name Current Status Reason
01/01/2026 07:00 (ET) BOS@NYK Boston Celtics Player, First Out Injury/Illness - Right Foot;
Recovery
New York Knicks NOT YET SUBMITTED
01/02/2026 07:00 (ET) BOS@NYK Boston Celtics Other, Second Questionable Illness
Page 1 of 1
'''
        published,records=parse_report(text,{'Boston Celtics':'2','New York Knicks':'18'})
        self.assertEqual(published.astimezone(dt.timezone.utc).hour,17)
        self.assertTrue(records[('2026-01-01','BOS@NYK','2')]['submitted'])
        self.assertEqual(records[('2026-01-01','BOS@NYK','2')]['players'][0]['status'],'Out')
        self.assertIn('Recovery',records[('2026-01-01','BOS@NYK','2')]['players'][0]['reason'])
        self.assertFalse(records[('2026-01-01','BOS@NYK','18')]['submitted'])
        self.assertEqual(records[('2026-01-02','BOS@NYK','2')]['players'][0]['status'],'Questionable')
        self.assertEqual(normalized('Alperen Şengün Jr.'),normalized('Alperen Sengun'))

    def test_prospective_capture_excludes_live_finished_and_late_events(self):
        now=dt.datetime(2026,11,6,12,tzinfo=dt.timezone.utc)
        payload={'events':[{'id':str(i),'date':date,'status':{'type':{'state':state}}} for i,date,state in [
            (1,'2026-11-06T20:00Z','pre'),(2,'2026-11-06T10:00Z','pre'),(3,'2026-11-06T20:00Z','post')]]}
        self.assertEqual([g for g,_ in pregame_events(payload,now)],['1'])

    def test_ranker_learns_ordering_without_changing_expected_points(self):
        rows=[]
        for week in range(16):
            for home in [0,1,0]:
                x=[0.0]*33;x[0]=(20-5*home)/20;x[2]=home
                rows.append({'player':'p','forecast_origin':str(week),'rank_features':x,'actual':10+5*home})
        before=[r['rank_features'][0] for r in rows]
        model=fit(rows,.1)
        self.assertGreater(evaluate(rows,model)['pair_accuracy'],evaluate(rows,None)['pair_accuracy'])
        self.assertEqual(before,[r['rank_features'][0] for r in rows])
        delta=utility(rows[0],model)-utility(rows[1],model)
        self.assertAlmostEqual(sigmoid(delta)+sigmoid(-delta),1)


if __name__=='__main__':unittest.main()
