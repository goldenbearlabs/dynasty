#!/usr/bin/env python3
"""Select CBB model additions on validation, then open chronological holdouts.

Primary cohort: positive-minute, verified D1 matchups in the application's eight
eligible conferences. Also report all D1 players, prior-20-minute rotations,
transfers, cold starts, neutral courts and tournament games.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import platform
import time

import numpy as np
import pyarrow.parquet as pq
from cbb_features import Dataset,NAMES,STATS,SCORING,WEIGHTS
from cbb_model import VARIANTS,columns,training_moments,predict,evaluate,masks,metrics,paired_bootstrap,variation,ranking,calibration
from cbb_strength import day_number

FOLDS={'2025':('2024-11-01','2025-01-01','2025-04-16'),'2026':('2025-11-01','2026-01-01','2026-04-16')}
PENALTIES=[.01,.1,1.]


def digest(path):return hashlib.sha256(Path(path).read_bytes()).hexdigest()
def save(path,value):path.write_text(json.dumps(value,indent=2,allow_nan=False)+'\n')


def build_model(name,minute_moments,rate_moments,mp,rp,cutoff):
    groups=VARIANTS[name]
    return {'version':1,'sport':'cbb','trained_before':cutoff,'groups':groups,'feature_names':NAMES,'scoring':SCORING,'regulation_minutes':40,
            'minutes':minute_moments.fit(columns(groups,True),mp),'rates':rate_moments.fit(columns(groups),rp)}


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--data',default='data/cbb');parser.add_argument('--out',default='data/cbb/study');args=parser.parse_args()
    root=Path(args.data);out=Path(args.out);out.mkdir(parents=True,exist_ok=True)
    feature_manifest=json.loads((root/'features.json').read_text())
    feature_source=Path('tools/cbb_features.py').read_text()+Path('tools/cbb_strength.py').read_text()+Path('tools/cbb_dataset.py').read_text()
    if feature_manifest['feature_source_sha256']!=hashlib.sha256(feature_source.encode()).hexdigest():raise ValueError('feature source changed; rebuild the feature cache')
    if feature_manifest['dataset_sha256']!=digest(root/'history.parquet'):raise ValueError('dataset changed; rebuild the feature cache')
    dataset=Dataset(root);loaded=np.load(root/'features.npz');cache={k:loaded[k] for k in loaded.files};rows=cache['rows'];a={k:v[rows] for k,v in dataset.a.items()};del dataset
    source_files=['tools/cbb_model.py','tools/cbb_study.py','tools/cbb_features.py','tools/cbb_strength.py','tools/cbb_dataset.py']
    manifest={'source_sha256':{f:digest(f) for f in source_files},'features':feature_manifest,'folds':FOLDS,'penalties':PENALTIES,'variants':VARIANTS,'primary_cohort':'eligible_conferences, verified D1-v-D1, positive-minute appearances','training_start':'2023-01-01',
              'selection':'Lowest mean relative primary validation MAE; no all-D1 validation fold >0.5% worse than player-only. Test scores opened afterward. Opening 2027 never tunes a model.'}
    if (out/'manifest.json').exists() and json.loads((out/'manifest.json').read_text())!=json.loads(json.dumps(manifest)):raise ValueError('changed data/source/protocol; use a new output directory')
    save(out/'manifest.json',manifest)
    d1=np.isin(a['conference'],list(__import__('cbb_dataset').D1_CONFERENCES));warm=a['day']>=day_number('2023-01-01');config={name:{} for name in VARIANTS};validation_predictions={}
    # No held-out test predictions are calculated in this phase.
    for fold,(start,test,end) in FOLDS.items():
        train=d1&warm&(a['day']<day_number(start));val=np.flatnonzero(d1&(a['day']>=day_number(start))&(a['day']<day_number(test)))
        print('Training validation moments',fold,int(train.sum()),'rows;',len(val),'validation rows',flush=True);mm,rm=training_moments(cache,a,train)
        primary=a['eligible'][val]&a['d1_matchup'][val];actual=cache['stats'][val]@WEIGHTS;actual_d1=a['d1_matchup'][val]
        for name in VARIANTS:
            candidates=[];best=None
            for mp in PENALTIES:
                for rp in PENALTIES:
                    model=build_model(name,mm,rm,mp,rp,start);_,_,points=predict(model,cache,val);score=float(np.abs(points[primary]-actual[primary]).mean());all_score=float(np.abs(points[actual_d1]-actual[actual_d1]).mean())
                    candidate={'minute_ridge':mp,'rate_ridge':rp,'primary_mae':score,'all_d1_mae':all_score};candidates.append(candidate)
                    if best is None or score<best[0]:best=(score,all_score,mp,rp,model,points)
            config[name][fold]={'primary_mae':best[0],'all_d1_mae':best[1],'minute_ridge':best[2],'rate_ridge':best[3],'candidates':candidates}
            validation_predictions[(name,fold)]=(val,best[5]);save(out/f'{name}-{fold}-validation-model.json',best[4]);print('Validation',fold,name,round(best[0],5),flush=True)
    relative={name:float(np.mean([v[f]['primary_mae']/config['player_only'][f]['primary_mae'] for f in FOLDS])) for name,v in config.items()}
    eligible=[name for name in VARIANTS if all(config[name][f]['all_d1_mae']<=config['player_only'][f]['all_d1_mae']*1.005 for f in FOLDS)]
    selected=min(eligible,key=relative.get);choice={'selected':selected,'relative_validation':relative,'configurations':config,'saved_before_test_predictions':dt.datetime.now(dt.timezone.utc).isoformat()};save(out/'selection-before-testing.json',choice);print('Selected',selected,flush=True)
    evaluations={name:{} for name in VARIANTS};models={};test_predictions={};comparison={};detail={}
    for fold,(start,test,end) in FOLDS.items():
        train=d1&warm&(a['day']<day_number(test));idx=np.flatnonzero(d1&(a['day']>=day_number(test))&(a['day']<day_number(end)))
        print('Refit moments',fold,int(train.sum()),flush=True);mm,rm=training_moments(cache,a,train)
        for name in VARIANTS:
            c=config[name][fold];model=build_model(name,mm,rm,c['minute_ridge'],c['rate_ridge'],test);evaluations[name][fold],pred=evaluate(model,cache,a,idx)
            models[(name,fold)]=model;test_predictions[(name,fold)]=(idx,pred);save(out/f'{name}-{fold}-model.json',model)
            print('Test',fold,name,round(evaluations[name][fold]['groups']['eligible_conferences']['model']['mae'],5),flush=True)
    # Opening-week check: use the selected 2026 validation hyperparameters and
    # refit before the season. No 2027 results or current validation selection.
    test='2026-11-01';train=d1&warm&(a['day']<day_number(test));idx=np.flatnonzero(d1&(a['day']>=day_number(test)))
    print('Opening-season refit',int(train.sum()),'training rows;',len(idx),'test rows',flush=True);mm,rm=training_moments(cache,a,train)
    for name in VARIANTS:
        c=config[name]['2026'];model=build_model(name,mm,rm,c['minute_ridge'],c['rate_ridge'],test);evaluations[name]['opening2027'],pred=evaluate(model,cache,a,idx)
        models[(name,'opening2027')]=model;test_predictions[(name,'opening2027')]=(idx,pred);save(out/f'{name}-opening2027-model.json',model)
        print('Opening test',name,round(evaluations[name]['opening2027']['groups']['eligible_conferences']['model']['mae'],5),flush=True)
    for fold in ['2025','2026','opening2027']:
        idx,baseline=test_predictions[('player_only',fold)];_,new=test_predictions[(selected,fold)];actual=cache['stats'][idx]@WEIGHTS;primary=a['eligible'][idx]&a['d1_matchup'][idx]
        comparison[fold]=paired_bootstrap(a,idx,actual,baseline[2],new[2],primary)
        details={'variation':{},'frozen_week_ranking':{}}
        for name in dict.fromkeys(['player_only',selected]):
            _,pred=test_predictions[(name,fold)];details['variation'][name]=variation(cache,a,idx[primary],pred[2][primary]);details['frozen_week_ranking'][name]=ranking(models[(name,fold)],cache,a,idx)
        _,_,oracle=predict(models[(selected,fold)],cache,idx,actual_minutes=a['minutes'][idx]);details['oracle_actual_minutes_not_deployable']=metrics(actual[primary],oracle[primary]);errors=np.abs(new[2][primary]-actual[primary]);details['easiest90_diagnostic_only_mae']=float(np.sort(errors)[:int(.9*len(errors))].mean())
        val_fold='2026' if fold=='opening2027' else fold;vi,vp=validation_predictions[(selected,val_fold)];vm=a['eligible'][vi]&a['d1_matchup'][vi]
        details['uncertainty']=calibration((cache['stats'][vi]@WEIGHTS)[vm],vp[vm],cache['prior_mean'][vi][vm],cache['prior_sd'][vi][vm],actual[primary],new[2][primary],cache['prior_mean'][idx][primary],cache['prior_sd'][idx][primary]);detail[fold]=details
        # Full review output, including every scored eligible appearance.
        names_table=pq.read_table(root/'history.parquet',columns=['name','starts_at']).take(__import__('pyarrow').array(rows[idx].tolist()));names=names_table['name'].to_pylist();starts=names_table['starts_at'].to_pylist()
        with (out/f'{selected}-{fold}-predictions.jsonl').open('w') as file,(out/f'player_only-{fold}-errors.jsonl').open('w') as bf:
            for local in np.flatnonzero(primary):
                i=int(idx[local]);target={k:str(a[k][i]) for k in ['game_id','player_id','team','opponent']};target.update(starts_at=starts[local],season=int(a['season'][i]),home=bool(a['home'][i]),neutral=bool(a['neutral'][i]))
                item={'target':target,'name':names[local],'actual_minutes':float(a['minutes'][i]),'actual_points':float(actual[local]),'actual_stats':{s:float(cache['stats'][i,j]) for j,s in enumerate(STATS)},'prediction':{'minutes':float(new[0][local]),'stats':{s:float(new[1][local,j]) for j,s in enumerate(STATS)},'fantasy_points_if_active':float(new[2][local])},'proxy':float(cache['proxy'][i,1]),'proxy_if_active':float(cache['proxy'][i,0]),'recent_10':float(cache['prior_mean'][i]),'features':{'position':['UNK','G','F','C'][cache['position'][i]],'history_games':int(cache['history'][i]),'season_games':int(cache['season_n'][i]),'baseline_minutes':float(cache['x'][i,0]*40),'feature_values':dict(zip(NAMES,map(float,cache['x'][i])))}}
                file.write(json.dumps(item,separators=(',',':'))+'\n');bf.write(json.dumps({'game':target['game_id'],'player':target['player_id'],'actual':item['actual_points'],'predicted':float(baseline[2][local])},separators=(',',':'))+'\n')
    result={'manifest':manifest,'selection':choice,'evaluations':evaluations,'paired_comparisons':comparison,'diagnostics':detail,'limits':['Conditional on positive-minute appearance. No availability/DNP skill established.','Archives were downloaded now and can contain later stat corrections; they are not original HTTP snapshots.','Team/conference/neutral metadata are retrospective archive identities, not a verified historical schedule publication archive.','Opening-season sample never selects hyperparameters.','Reported in-sample-error trimming and oracle minutes are diagnostics only.']}
    save(out/'results.json',result)
    candidate=models[(selected,'opening2027')];candidate['data_sha256']=manifest['features']['dataset_sha256'];candidate['source_sha256']=manifest['source_sha256'];save(root/'candidate-model.json',candidate)
    print('Saved',out/'results.json',flush=True)


if __name__=='__main__':main()
