import copy
import datetime as dt
import math
from types import SimpleNamespace
import unittest

import numpy as np
from cbb_dataset import STATS,validate_stats,sports_day
from cbb_features import State,NAMES,WEIGHTS
from cbb_model import Moments,predict,columns,metrics
from cbb_strength import TeamStrength,day_number
from cbb_predict import validate_target
from cbb_candidate import tree_predict,opening_phase


def fixture():
    rows=[];games=[];stats=[]
    for day in range(8):
        teams=[{'team':1,'opponent':2,'score':56.,'conference':2},{'team':2,'opponent':1,'score':56.,'conference':7}]
        g={'game_id':100+day,'season':2026,'day':str(np.datetime64('2025-11-01')+np.timedelta64(day,'D')),'home':1,'away':2,'neutral':False,'season_type':2,'duration':40,'pace':68.,'possessions':68.,'teams':teams,'d1_matchup':True};games.append(g)
        for team in [1,2]:
            for p in range(7):
                row={'player_id':team*10+p,'team':team,'opponent':3-team,'season':2026,'day':day_number(g['day']),'game_id':100+day,'minutes':200/7,'starter':p<5,'played':True,'conference':2 if team==1 else 7,'opponent_conference':7 if team==1 else 2,'d1_matchup':True,'pf':1,'tpa':2.}
                rows.append(row);line=np.zeros(len(STATS));line[STATS.index('pts')]=8;line[STATS.index('fgm')]=3;line[STATS.index('fga')]=7;line[STATS.index('tpm')]=1;line[STATS.index('ftm')]=1;line[STATS.index('fta')]=2;line[STATS.index('reb')]=3;stats.append(line)
    a={k:np.array([r[k] for r in rows]) for k in rows[0]}
    return SimpleNamespace(a=a,stats=np.array(stats),positions=np.ones(len(rows),dtype=int),n=len(rows),gmap={g['game_id']:g for g in games},games=games)


class CBBTests(unittest.TestCase):
    def test_fractional_minutes_and_early_morning_sports_day(self):
        self.assertEqual(sports_day(dt.datetime.fromisoformat('2026-11-06T07:00:00+00:00')),'2026-11-05')
        good={s:0. for s in STATS};good.update(pts=7,fgm=2,fga=4,tpm=1,tpa=2,ftm=2,fta=3)
        self.assertTrue(validate_stats(good));good['pts']=8;self.assertFalse(validate_stats(good))

    def test_ratings_cannot_see_same_day_or_future_results(self):
        data=fixture();ratings=TeamStrength();ratings.observe(data.games[:4]);cutoff=day_number(data.games[3]['day']);before=ratings.fit(2026,cutoff,[1,2]);ratings.observe(data.games[4:]);after=ratings.fit(2026,cutoff,[1,2]);self.assertEqual(before,after)

    def test_neutral_court_has_no_nominal_home_advantage(self):
        strength=TeamStrength();g=copy.deepcopy(fixture().games[0]);g['neutral']=True
        r={1:(110,100,70),2:(105,103,66)}
        first=strength.context(g,r,(105,68));g['home'],g['away']=g['away'],g['home'];second=strength.context(g,r,(105,68));self.assertEqual(first,second)
        g['neutral']=False;nonneutral=strength.context(g,r,(105,68));self.assertNotEqual(first[1]['margin'],nonneutral[1]['margin'])

    def build_prior_state(self):
        data=fixture();state=State(data)
        for step in range(3):
            game=data.games[step];day=day_number(game['day']);state.ratings=state.strength.fit(2026,day,[1,2]);state.league=state.strength.league;state.freeze(day);base={}
            for i in range(step*14,(step+1)*14):
                _,b,_,_,meta=state.build(i,day,game);base[i]=b;state.prior_opp_def[i]=meta['opp_def'];state.prior_own_off[i]=meta['own_off']
            state.observe(range(step*14,(step+1)*14),[game],base)
        day=day_number(data.games[3]['day']);state.ratings=state.strength.fit(2026,day,[1,2]);state.league=state.strength.league;state.freeze(day)
        return data,state,day

    def test_target_boxscore_start_position_and_game_result_do_not_enter_features(self):
        data,state,day=self.build_prior_state();g=data.games[3];before=state.build(42,day,g)
        data.a['minutes'][42:]=1;data.a['starter'][42:]=False;data.positions[42:]=3;data.stats[42:]=100;g['teams'][0]['score']=200;g['possessions']=160;g['pace']=160;g['duration']=80
        after=state.build(42,day,g)
        for left,right in zip(before[:4],after[:4]):np.testing.assert_array_equal(left,right)

    def test_week_origin_ignores_intervening_results(self):
        data,state,day=self.build_prior_state();target=data.games[5];future_day=day_number(target['day']);before=state.build(70,future_day,target,True)
        base={}
        for i in range(42,56):
            _,b,_,_,_=state.build(i,day,data.games[3]);base[i]=b
        data.stats[42:56]*=4;state.observe(range(42,56),[data.games[3]],base);state.context_cache={}
        after=state.build(70,future_day,target,True)
        for left,right in zip(before[:4],after[:4]):np.testing.assert_array_equal(left,right)

    def test_ridge_subsets_fit_only_the_supplied_training_examples(self):
        rng=np.random.default_rng(7);x=rng.normal(size=(300,4));y=(2*x[:,0]-3*x[:,2]+4)[:,None]
        m=Moments(x,y,np.ones(300));head=m.fit([0,2],.0001);pred=x[:,[0,2]]@np.array(head['coefficients'])+head['offset'];self.assertLess(np.mean(abs(pred-y)),.001)

    def test_college_minutes_and_shooting_identity(self):
        cols=columns(['opportunities']);mcols=columns(['opportunities'],True)
        model={'groups':['opportunities'],'minutes':{'columns':mcols,'coefficients':np.zeros((len(mcols),1)).tolist(),'offset':[20]},'rates':{'columns':cols,'coefficients':np.zeros((len(cols),15)).tolist(),'offset':[0]*15}}
        x=np.zeros((1,len(NAMES)));x[:,0]=1;cache={'x':x,'base':np.full((1,15),.1),'scales':np.ones((1,15)),'shoot':np.array([[.4,.5,.35,.75]])}
        minutes,stats,fp=predict(model,cache,np.array([0]));self.assertEqual(minutes[0],40)
        self.assertAlmostEqual(stats[0,STATS.index('pts')],2*stats[0,STATS.index('fgm')]+stats[0,STATS.index('tpm')]+stats[0,STATS.index('ftm')])
        self.assertAlmostEqual(fp[0],stats[0]@WEIGHTS)

    def test_target_contract_rejects_outcomes_and_future_trained_artifact(self):
        target={'game_id':'g','player_id':11,'team':1,'opponent':2,'season':2027,'starts_at':'2026-11-06T23:00:00Z','home':True,'neutral':False}
        now=dt.datetime.fromisoformat('2026-11-06T18:00:00+00:00');model={'trained_before':'2026-11-01'}
        validate_target(target,now,model)
        with self.assertRaises(ValueError):validate_target(dict(target,minutes=35),now,model)
        with self.assertRaises(ValueError):validate_target(target,now,{'trained_before':'2027-01-01'})
        with self.assertRaises(ValueError):validate_target(target,now+dt.timedelta(days=1),model)

    def test_portable_tree_threshold_and_opening_calendar(self):
        tree={'format':'numeric_hist_gradient_boosting_v1','columns':[0],'baseline':1,'trees':[{'feature':[0,0,0],'threshold':[2,0,0],'left':[1,0,0],'right':[2,0,0],'leaf':[0,1,1],'missing_left':[1,0,0],'value':[0,3,-2]}]}
        np.testing.assert_array_equal(tree_predict(tree,np.array([[1.],[2.],[3.],[float('nan')]])),[4,4,-1,4])
        np.testing.assert_array_equal(opening_phase(np.array(['2026-11-02','2026-11-15','2026-11-16','2026-01-03'],dtype='datetime64[D]')),[True,True,False,False])


if __name__=='__main__':unittest.main()
