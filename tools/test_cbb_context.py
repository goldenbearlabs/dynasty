import datetime as dt
import json
from pathlib import Path
import tempfile
import unittest

import numpy as np
import pyarrow as pa
from cbb_availability import eligible_timestamp
from cbb_context_features import Recruiting,ContextState,NAMES
from cbb_context_model import predict
from cbb_candidate import predict as baseline_predict
from cbb_features import STATS,NAMES as BASE_NAMES
from cbb_strength import day_number
from test_cbb_model import fixture


class ContextTests(unittest.TestCase):
    def recruiting(self,records=None):
        folder=tempfile.TemporaryDirectory();self.addCleanup(folder.cleanup);path=Path(folder.name)/'recruiting.json'
        records=records or [{'player_id':11,'class_year':2025,'normalized_name':'sample','grade':95,'national_rank':25,'position':'G','height_inches':76,'signed_team_ids':[1]}]
        path.write_text(json.dumps({'records':records}));return Recruiting(path,{11:{'sample'},21:{'other'}})

    def data(self):
        data=fixture();data.table=pa.table({'position':['G']*data.n});return data

    def test_recruit_class_must_precede_college_and_cannot_update_after_enrollment(self):
        records=[{'player_id':11,'class_year':year,'normalized_name':'sample','grade':grade,'national_rank':1,'signed_team_ids':[]} for year,grade in [(2025,90),(2026,100)]]
        recruits=self.recruiting(records)
        self.assertEqual(recruits.get(11,2026)['grade'],90)
        self.assertEqual(recruits.get(11,2027,2026)['grade'],90)
        self.assertIsNone(recruits.get(11,2025))

    def test_identity_conflict_and_missing_rating_stay_unknown(self):
        recruits=self.recruiting([{'player_id':11,'class_year':2025,'normalized_name':'wrong','grade':100,'national_rank':1,'signed_team_ids':[]}])
        self.assertIsNone(recruits.get(11,2026));self.assertEqual(recruits.audit['name_conflict'],1)
        values=recruits.features(11,1,2026,0,0);self.assertEqual(values[0],0);self.assertEqual(values[2],0)

    def test_target_roster_and_position_never_enter_context(self):
        data=self.data();state=ContextState(data,self.recruiting());state.observe(range(42));day=int(data.a['day'][42]);before=state.build(42,day)
        data.a['minutes'][42:]=40;data.a['starter'][42:]=True;data.stats[42:]=999;data.table=pa.table({'position':['G']*42+['C']*(data.n-42)})
        after=state.build(42,day);np.testing.assert_array_equal(before,after)
        self.assertEqual(before[NAMES.index('current_roster_known')],1)

    def test_week_origin_cannot_use_later_roster_or_role(self):
        data=self.data();state=ContextState(data,self.recruiting());state.observe(range(42));origin=int(data.a['day'][42]);before=state.build(70,origin)
        data.a['minutes'][42:56]=40;state.observe(range(42,56));after=state.build(70,origin);np.testing.assert_array_equal(before,after)

    def test_transfer_flag_persists_after_first_new_team_appearance(self):
        data=self.data();data.a['season'][:42]=2025;state=ContextState(data,self.recruiting());state.observe(range(42));target={'player_id':21,'team':1,'season':2026};day=int(data.a['day'][42]);first=state.build(None,day,target)
        data.a['player_id'][42]=21;data.a['team'][42]=1;state.observe([42]);later=state.build(None,int(data.a['day'][56]),target)
        self.assertEqual(first[NAMES.index('persistent_transfer')],1);self.assertEqual(later[NAMES.index('persistent_transfer')],1)
        self.assertGreater(later[NAMES.index('current_team_games')],0)

    def test_date_only_availability_is_not_a_pregame_timestamp(self):
        starts=dt.datetime.fromisoformat('2026-01-01T23:00:00+00:00');cutoff=starts-dt.timedelta(hours=1)
        self.assertFalse(eligible_timestamp({'game_reference_day':'2026-01-01'},starts,cutoff)[0])
        self.assertFalse(eligible_timestamp({'published_at':starts.isoformat()},starts,cutoff)[0])
        self.assertFalse(eligible_timestamp({'observed_at':(cutoff+dt.timedelta(minutes=1)).isoformat()},starts,cutoff)[0])
        self.assertTrue(eligible_timestamp({'observed_at':(cutoff-dt.timedelta(minutes=1)).isoformat()},starts,cutoff)[0])

    def test_no_context_model_is_exactly_the_existing_champion(self):
        core=[0];base={'groups':['opportunities'],'minutes':{'columns':core,'coefficients':[[0]],'offset':[0]},'rates':{'columns':core,'coefficients':[[0]*15],'offset':[0]*15}}
        cache={'x':np.zeros((2,len(BASE_NAMES))),'base':np.full((2,15),.1),'scales':np.ones((2,15)),'shoot':np.array([[.4,.5,.35,.75]]*2)};cache['x'][:,0]=.5
        a=baseline_predict(base,cache,np.array([0,1]));b=predict(base,cache,{'x':np.zeros((2,len(NAMES)))},np.array([0,1]))
        for left,right in zip(a,b):np.testing.assert_array_equal(left,right)

    def test_context_tree_minutes_override_linear_head_and_keep_shooting_coherent(self):
        core=[0];model={'groups':['opportunities'],'minutes':{'columns':core,'coefficients':[[0]],'offset':[0]},'rates':{'columns':core,'coefficients':[[0]*15],'offset':[0]*15}}
        width=1+len(NAMES);zero={'columns':list(range(width)),'coefficients':np.zeros((width,1)).tolist(),'offset':[0]}
        model['context_correction']={'core_columns':core,'minutes':zero,'rates':{'columns':list(range(width)),'coefficients':np.zeros((width,15)).tolist(),'offset':[0]*15},'minutes_trees':{'columns':[0],'baseline':4.,'trees':[]}}
        cache={'x':np.zeros((1,len(BASE_NAMES))),'base':np.full((1,15),.1),'scales':np.ones((1,15)),'shoot':np.array([[.4,.5,.35,.75]])};cache['x'][:,0]=.5
        minutes,stats,fp=predict(model,cache,{'x':np.zeros((1,len(NAMES)))},np.array([0]))
        with self.assertRaises(ValueError):baseline_predict(model,cache,np.array([0]))
        self.assertAlmostEqual(minutes[0],24.)
        self.assertAlmostEqual(stats[0,0],2*stats[0,7]+stats[0,6]+stats[0,9])


if __name__=='__main__':unittest.main()
