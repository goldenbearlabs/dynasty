#!/usr/bin/env python3
"""Test added CBB context against the frozen final baseline, not player-only.

Select on the two prior Nov-Dec validations before computing new test scores.
Includes an added-capacity control using old features, and joint ablations.
"""
from collections import defaultdict
import copy
import datetime as dt
import hashlib
import json
from pathlib import Path

import numpy as np
from sklearn.ensemble import HistGradientBoostingRegressor
from cbb_features import Dataset,NAMES as BASE_NAMES,STATS,WEIGHTS
from cbb_dataset import D1_CONFERENCES
from cbb_context_features import NAMES,GROUPS
from cbb_context_model import design,predict
from cbb_candidate import export_trees,predict as baseline_predict,fit_uncertainty,uncertainty,opening_phase
from cbb_model import Moments,metrics,masks,paired_bootstrap,variation
from cbb_export import same_origin_ranking
from cbb_strength import day_number
from cbb_study import FOLDS

VARIANTS={'baseline':None,'capacity_control':[],'recruiting':['recruiting'],'transfer_roles':['transfer_roles'],'observed_depth':['observed_depth'],'participation':['participation'],'joint':list(GROUPS),'joint_without_recruiting':['transfer_roles','observed_depth','participation'],'joint_without_depth':['recruiting','transfer_roles','participation']}
PENALTIES=[.01,.1,1.]


def save(path,d):path.write_text(json.dumps(d,indent=2,allow_nan=False)+'\n')
def digest(path):return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def selected_columns(groups,core):
    names=[n for g in groups for n in GROUPS[g]]
    return list(range(len(core)))+[len(core)+NAMES.index(n) for n in names]


def fit_baseline_validation(root,fold,cache,a,train):
    base=json.load(open(root/f'study/combined-{fold}-validation-model.json'));idx=np.flatnonzero(train);bm=baseline_predict(base,cache,idx)[0];cols=base['minutes']['columns']
    tree=HistGradientBoostingRegressor(loss='squared_error',max_iter=120,learning_rate=.06,max_leaf_nodes=15,min_samples_leaf=100,l2_regularization=10,early_stopping=False,random_state=20261106)
    tree.fit(cache['x'][idx][:,cols],a['minutes'][idx]-bm,sample_weight=.7**(int(a['season'][idx].max())-a['season'][idx]));base['minutes_correction']=export_trees(tree,cols)
    return base


def moments(base,cache,ctx,a,mask,core):
    idx=np.flatnonzero(mask);m,stats,_=baseline_predict(base,cache,idx);x=design(cache,ctx,idx,core);w=.7**(int(a['season'][idx].max())-a['season'][idx]);my=((a['minutes'][idx]-m)/10)[:,None];ry=(cache['stats'][idx]/a['minutes'][idx,None]-stats/m[:,None])/cache['scales'][idx]
    return Moments(x,my,w),Moments(x,ry,w*np.minimum(a['minutes'][idx],40)/25)


def model_for(base,name,mm,rm,mp,rp,core):
    model=dict(base)
    if VARIANTS[name] is not None:
        cols=selected_columns(VARIANTS[name],core);model['context_correction']={'version':1,'groups':VARIANTS[name],'core_columns':core,'feature_names':[BASE_NAMES[i] for i in core]+NAMES,'minutes':mm.fit(cols,mp),'rates':rm.fit(cols,rp)}
    return model


def summary(cache,a,idx,pred):
    actual=cache['stats'][idx]@WEIGHTS;groups=masks(cache,a,idx)
    return {'groups':{k:{'model':metrics(actual[m],pred[2][m]),'minutes':metrics(a['minutes'][idx][m],pred[0][m])} for k,m in groups.items()}}


def main():
    root=Path('data/cbb');out=root/'context-study';out.mkdir(exist_ok=True);data=Dataset(root);cache={k:v for k,v in np.load(root/'features.npz').items()};ctx={k:v for k,v in np.load(root/'context/features.npz').items()};a={k:v[cache['rows']] for k,v in data.a.items()};del data
    if not np.array_equal(cache['rows'],ctx['rows']):raise ValueError('context cohorts differ')
    context_meta=json.load(open(root/'context/features.json'))
    for file,sha in context_meta['source_sha256'].items():
        if digest(file)!=sha:raise ValueError('Context source changed; rebuild context features')
    baseline_path=Path('models/cbb/v1-model.json') if Path('models/cbb/v1-model.json').exists() else Path('models/cbb/model.json')
    core=json.load(open(baseline_path))['minutes']['columns'];d1=np.isin(a['conference'],list(D1_CONFERENCES));warm=a['day']>=day_number('2023-01-01');val_predictions={};base_models={};config={name:{} for name in VARIANTS}
    manifest={'baseline_model_sha256':digest(baseline_path),'source_sha256':{p:digest(p) for p in ['tools/cbb_context_study.py','tools/cbb_context_model.py']},'context':context_meta,'folds':FOLDS,'variants':VARIANTS,'penalties':PENALTIES,'cohort':'same positive-minute eligible-conference D1 appearances as the original baseline','availability_used':False,'selection':'Lowest mean relative primary validation MAE; no all-D1 validation fold >0.5% worse. Opening results do not select recipes.'}
    if (out/'manifest.json').exists() and json.load(open(out/'manifest.json'))!=json.loads(json.dumps(manifest)):raise ValueError('Protocol changed; use a fresh study folder')
    save(out/'manifest.json',manifest)
    for fold,(start,test,end) in FOLDS.items():
        train=d1&warm&(a['day']<day_number(start));vi=np.flatnonzero(d1&(a['day']>=day_number(start))&(a['day']<day_number(test)));base=fit_baseline_validation(root,fold,cache,a,train);base_models[fold]=base;save(out/f'baseline-{fold}-validation-model.json',base)
        mm,rm=moments(base,cache,ctx,a,train,core);actual=cache['stats'][vi]@WEIGHTS;primary=a['eligible'][vi]&a['d1_matchup'][vi];all_d1=a['d1_matchup'][vi]
        for name in VARIANTS:
            candidates=[];best=None
            pairs=[(0,0)] if name=='baseline' else [(m,r) for m in PENALTIES for r in PENALTIES]
            for mp,rp in pairs:
                model=model_for(base,name,mm,rm,mp,rp,core);pred=predict(model,cache,ctx,vi);loss=float(np.abs(actual[primary]-pred[2][primary]).mean());all_loss=float(np.abs(actual[all_d1]-pred[2][all_d1]).mean());candidates.append({'minutes_ridge':mp,'rate_ridge':rp,'mae':loss,'all_d1_mae':all_loss})
                if best is None or loss<best[0]:best=(loss,all_loss,mp,rp,model,pred)
            config[name][fold]={'mae':best[0],'all_d1_mae':best[1],'minutes_ridge':best[2],'rate_ridge':best[3],'candidates':candidates};val_predictions[(name,fold)]=(vi,best[5]);save(out/f'{name}-{fold}-validation-model.json',best[4]);print('Context validation',fold,name,round(best[0],5),flush=True)
    relative={name:float(np.mean([v[f]['mae']/config['baseline'][f]['mae'] for f in FOLDS])) for name,v in config.items()};eligible=[name for name in VARIANTS if all(config[name][f]['all_d1_mae']<=config['baseline'][f]['all_d1_mae']*1.005 for f in FOLDS)];selected=min(eligible,key=relative.get)
    choice={'selected':selected,'relative':relative,'configurations':config,'saved_before_test_predictions':dt.datetime.now(dt.timezone.utc).isoformat()};save(out/'selection-before-testing.json',choice);print('Context selected',selected,flush=True)
    evaluations={name:{} for name in VARIANTS};test_predictions={};models={}
    for fold in ['2025','2026','opening2027']:
        test=FOLDS[fold][1] if fold in FOLDS else '2026-11-01';end=FOLDS[fold][2] if fold in FOLDS else '2026-11-06';train=d1&warm&(a['day']<day_number(test));idx=np.flatnonzero(d1&(a['day']>=day_number(test))&(a['day']<day_number(end)));base=json.load(open(root/f'final-study/{fold}-model.json'));mm,rm=moments(base,cache,ctx,a,train,core)
        for name in VARIANTS:
            c=config[name][fold if fold in FOLDS else '2026'];model=model_for(base,name,mm,rm,c['minutes_ridge'],c['rate_ridge'],core);pred=predict(model,cache,ctx,idx);evaluations[name][fold]=summary(cache,a,idx,pred);test_predictions[(name,fold)]=(idx,pred);models[(name,fold)]=model;save(out/f'{name}-{fold}-model.json',model)
            np.savez_compressed(out/f'{name}-{fold}-predictions.npz',idx=idx,minutes=pred[0],stats=pred[1],points=pred[2]);print('Context test',fold,name,round(evaluations[name][fold]['groups']['eligible_conferences']['model']['mae'],5),flush=True)
    diagnostics={}
    for fold in ['2025','2026','opening2027']:
        idx,bp=test_predictions[('baseline',fold)];_,new=test_predictions[(selected,fold)];actual=cache['stats'][idx]@WEIGHTS;primary=a['eligible'][idx]&a['d1_matchup'][idx];frozen=idx[primary&cache['frozen_known'][idx]];model=models[(selected,fold)];fpoints=predict(model,cache,ctx,frozen,True)[2]
        vi,vp=val_predictions[(selected,fold if fold in FOLDS else '2026')];vm=a['eligible'][vi]&a['d1_matchup'][vi];model['uncertainty']=fit_uncertainty((cache['stats'][vi]@WEIGHTS)[vm],vp[2][vm],cache['prior_mean'][vi][vm],cache['prior_sd'][vi][vm],opening_phase(a['day'][vi][vm]));u=uncertainty(model,new[2][primary],cache['prior_mean'][idx][primary],cache['prior_sd'][idx][primary],opening_phase(a['day'][idx][primary]))
        group_improvement={key:{'n':int(m.sum()),'before':metrics(actual[m],bp[2][m]),'after':metrics(actual[m],new[2][m])} for key,m in {'recruit_known':primary&(ctx['x'][idx,0]>0),'recruit_unknown':primary&(ctx['x'][idx,0]==0),'persistent_transfer':primary&(ctx['x'][idx,NAMES.index('persistent_transfer')]>0)}.items()}
        diagnostics[fold]={'paired':paired_bootstrap(a,idx,actual,bp[2],new[2],primary),'groups':group_improvement,'ranking':same_origin_ranking(cache,a,frozen,fpoints),'variation':variation(cache,a,idx[primary],new[2][primary]),'interval80_coverage':float(np.mean((actual[primary]>=u['lower80'])&(actual[primary]<=u['upper80']))),'interval95_coverage':float(np.mean((actual[primary]>=u['lower95'])&(actual[primary]<=u['upper95']))),'brier':float(np.mean((u['probability_above_usual']-(actual[primary]>cache['prior_mean'][idx][primary]))**2))};save(out/f'selected-{fold}-model.json',model)
    consistent=all(d['paired']['mae_reduction']>0 for d in diagnostics.values())
    result={'manifest':manifest,'selection':choice,'evaluations':evaluations,'diagnostics':diagnostics,'consistent_improvement':consistent,'default_model_changed':False,'live_application_changed':False,'note':'Existing benchmark windows have already been inspected in previous research. Context choice and ridge hyperparameters use historical validation; availability without publication times is excluded.'};save(out/'results.json',result)
    final=models[(selected,'opening2027')];final['context_source_manifest']=context_meta;save(root/'context-candidate-model.json',final)
    print('Context study saved; consistent improvement:',consistent,flush=True)


if __name__=='__main__':main()
