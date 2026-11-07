#!/usr/bin/env python3
"""Export and verify JSON models, then summarize the final CBB candidate."""
from collections import defaultdict
import copy
import datetime as dt
import hashlib
import json
from pathlib import Path
import pickle
import numpy as np
import pyarrow as pa
import pyarrow.parquet as pq
from sklearn.ensemble import HistGradientBoostingRegressor

from cbb_features import Dataset,NAMES,STATS,WEIGHTS
from cbb_model import predict as linear_predict,metrics,masks,paired_bootstrap,variation
from cbb_candidate import export_trees,predict,tree_predict,fit_uncertainty,uncertainty,opening_phase
from cbb_strength import day_number
from cbb_dataset import D1_CONFERENCES
from cbb_study import FOLDS


def same_origin_ranking(cache,a,idx,points):
    actual=cache['stats'][idx]@WEIGHTS;groups=defaultdict(list)
    for k,(player,day) in enumerate(zip(a['player_id'][idx],a['day'][idx])):groups[(int(player),int(day)-(int(day)+3)%7)].append(k)
    correct=pairs=0;regret=[];chosen=[]
    for ks in groups.values():
        if len(ks)<2:continue
        best=max(ks,key=lambda k:points[k]);regret.append(float(max(actual[ks])-actual[best]));chosen.append(float(actual[best]))
        for j,l in enumerate(ks):
            for r in ks[j+1:]:
                delta=actual[l]-actual[r]
                if abs(delta)<1e-8:continue
                p=points[l]-points[r];pairs+=1;correct+=.5 if abs(p)<1e-8 else float(p*delta>0)
    return {'pairs':pairs,'player_weeks':len(regret),'pair_accuracy':correct/pairs if pairs else None,'regret':float(np.mean(regret)) if regret else None,'chosen_actual_mean':float(np.mean(chosen)) if chosen else None}


def main():
    root=Path('data/cbb');out=root/'final-study';out.mkdir(exist_ok=True);study=json.load(open(root/'study/results.json'));minutes=json.load(open(root/'minutes-study/results.json'));selected=study['selection']['selected'];use_trees=minutes['consistent_improvement_all_three']
    data=Dataset(root);cache={k:v for k,v in np.load(root/'features.npz').items()};a={k:v[cache['rows']] for k,v in data.a.items()};d1=np.isin(a['conference'],list(D1_CONFERENCES));results={};models={};parity={}
    for fold in ['2025','2026','opening2027']:
        model=json.load(open(root/f'study/{selected}-{fold}-model.json'))
        if use_trees:
            # These files are outputs of our own local training run, not external input.
            with (root/f'minutes-study/{fold}-minutes.pkl').open('rb') as file:trained=pickle.load(file)
            model['minutes_correction']=export_trees(trained['estimator'],trained['columns'])
            saved=np.load(root/f'minutes-study/{fold}-predictions.npz');idx=saved['idx'];new=predict(model,cache,idx)
            delta=float(np.max(abs(new[2]-saved['points'])));parity[fold]=delta
            if delta>1e-9:raise ValueError('JSON tree export changed predictions')
        else:
            start=FOLDS[fold][1] if fold in FOLDS else '2026-11-01';end=FOLDS[fold][2] if fold in FOLDS else '2026-11-06';idx=np.flatnonzero(d1&(a['day']>=day_number(start))&(a['day']<day_number(end)));new=predict(model,cache,idx)
        # Calibrate from genuine pre-validation-cutoff forecasts, not refitted
        # test coefficients evaluated on their own training rows.
        vf=fold if fold in FOLDS else '2026';start,test,_=FOLDS[vf];vi=np.flatnonzero(d1&(a['day']>=day_number(start))&(a['day']<day_number(test)));val_model=json.load(open(root/f'study/{selected}-{vf}-validation-model.json'))
        if use_trees:
            cols=trained['columns'];train_idx=np.flatnonzero(d1&(a['day']>=day_number('2023-01-01'))&(a['day']<day_number(start)));baseline_min=linear_predict(val_model,cache,train_idx)[0]
            estimator=HistGradientBoostingRegressor(loss='squared_error',max_iter=120,learning_rate=.06,max_leaf_nodes=minutes['selection']['selected_leaf_nodes'],min_samples_leaf=100,l2_regularization=10,early_stopping=False,random_state=20261106)
            estimator.fit(cache['x'][train_idx][:,cols],a['minutes'][train_idx]-baseline_min,sample_weight=.7**(int(a['season'][train_idx].max())-a['season'][train_idx]));val_model['minutes_correction']=export_trees(estimator,cols)
        vp=predict(val_model,cache,vi)[2];vm=a['eligible'][vi]&a['d1_matchup'][vi]
        model['uncertainty']=fit_uncertainty((cache['stats'][vi]@WEIGHTS)[vm],vp[vm],cache['prior_mean'][vi][vm],cache['prior_sd'][vi][vm],opening_phase(a['day'][vi][vm]))
        models[fold]=model;primary=a['eligible'][idx]&a['d1_matchup'][idx];actual=cache['stats'][idx]@WEIGHTS;groups=masks(cache,a,idx)
        baseline=json.load(open(root/f'study/player_only-{fold}-model.json'));bp=linear_predict(baseline,cache,idx)[2]
        r={'games':len(set(a['game_id'][idx][primary])),'primary':metrics(actual[primary],new[2][primary]),'minutes':metrics(a['minutes'][idx][primary],new[0][primary]),'proxy_if_active':metrics(actual[primary],cache['proxy'][idx,0][primary]),'recent10':metrics(actual[primary],cache['prior_mean'][idx][primary]),
           'groups':{k:{'model':metrics(actual[m],new[2][m]),'minutes':metrics(a['minutes'][idx][m],new[0][m])} for k,m in groups.items()},'vs_player_only':paired_bootstrap(a,idx,actual,bp,new[2],primary),'vs_proxy_if_active':paired_bootstrap(a,idx,actual,cache['proxy'][idx,0],new[2],primary),'variation':variation(cache,a,idx[primary],new[2][primary])}
        frozen=idx[primary&cache['frozen_known'][idx]];fp=predict(model,cache,frozen,True)[2];r['same_origin_ranking']=same_origin_ranking(cache,a,frozen,fp)
        u=uncertainty(model,new[2][primary],cache['prior_mean'][idx][primary],cache['prior_sd'][idx][primary],opening_phase(a['day'][idx][primary]));outcome=actual[primary]>cache['prior_mean'][idx][primary]
        r['uncertainty']={'interval80_coverage':float(np.mean((actual[primary]>=u['lower80'])&(actual[primary]<=u['upper80']))),'interval80_mean_width':float(np.mean(u['upper80']-u['lower80'])),'interval95_coverage':float(np.mean((actual[primary]>=u['lower95'])&(actual[primary]<=u['upper95']))),'interval95_mean_width':float(np.mean(u['upper95']-u['lower95'])),'probability_brier':float(np.mean((u['probability_above_usual']-outcome)**2)),'base_rate_brier':float(np.mean((u['base_probability']-outcome)**2)),'bins':[]}
        for lo in np.arange(0,1,.1):
            m=(u['probability_above_usual']>=lo)&((u['probability_above_usual']<lo+.1) if lo<.9 else u['probability_above_usual']<=1)
            if m.any():r['uncertainty']['bins'].append({'lower':round(float(lo),1),'n':int(m.sum()),'predicted':float(u['probability_above_usual'][m].mean()),'actual':float(outcome[m].mean())})
        results[fold]=r
        # Rewrite review rows around the final stat line; prior baselines unchanged.
        original=root/f'study/{selected}-{fold}-predictions.jsonl';original_rows=[json.loads(line) for line in original.open()];mapping={(str(a['game_id'][i]),str(a['player_id'][i])):j for j,i in enumerate(idx)}
        with (out/f'{fold}-predictions.jsonl').open('w') as file:
            for row in original_rows:
                j=mapping[(row['target']['game_id'],row['target']['player_id'])];row['prediction']={'minutes':float(new[0][j]),'stats':dict(zip(STATS,map(float,new[1][j]))),'fantasy_points_if_active':float(new[2][j])};file.write(json.dumps(row,separators=(',',':'))+'\n')
        (out/f'{fold}-model.json').write_text(json.dumps(model,indent=2)+'\n');print('Final',fold,r['primary'],'rank',r['same_origin_ranking'],'coverage',r['uncertainty']['interval80_coverage'],flush=True)
    final=models['opening2027'];final['data_sha256']=study['manifest']['features']['dataset_sha256'];final['feature_source_sha256']=study['manifest']['features']['feature_source_sha256'];final['selection_provenance']={'linear':'data/cbb/study/selection-before-testing.json','minutes':'data/cbb/minutes-study/selection-before-testing.json','uncertainty':'fixed calendar-opening versus established residual pools, fitted on validation only; recipe added after the initial opening-week undercoverage was inspected; exploratory benchmark reuse'}
    Path('models/cbb').mkdir(parents=True,exist_ok=True);Path('models/cbb/model.json').write_text(json.dumps(final,indent=2)+'\n');(root/'model.json').write_text(json.dumps(final,indent=2)+'\n')
    result={'selected':selected,'minutes_trees_adopted':use_trees,'json_export_max_absolute_parity_error':parity,'results':results,'model_sha256':hashlib.sha256(Path('models/cbb/model.json').read_bytes()).hexdigest(),'live_application_changed':False,'limits':study['limits']+['Stage-aware uncertainty is an exploratory follow-on to observed opening-week undercoverage.','Narrow probability bins and current-week ranking have small samples.']}
    (out/'results.json').write_text(json.dumps(result,indent=2)+'\n')


if __name__=='__main__':main()
