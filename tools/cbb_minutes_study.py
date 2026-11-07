#!/usr/bin/env python3
"""Test a shallow nonlinear minutes correction after the linear CBB baseline.

All stat-rate heads stay fixed. Selection uses earlier validation only; the
opening 2027 sample cannot choose hyperparameters. Trees are separately saved.
"""
import json
from pathlib import Path
import pickle
import numpy as np
from sklearn.ensemble import HistGradientBoostingRegressor
from cbb_features import Dataset,WEIGHTS
from cbb_dataset import D1_CONFERENCES
from cbb_model import columns,predict,metrics,masks,paired_bootstrap
from cbb_strength import day_number
from cbb_study import FOLDS


def main():
    root=Path('data/cbb');out=root/'minutes-study';out.mkdir(exist_ok=True);study=root/'study';chosen=json.load(open(study/'selection-before-testing.json'))['selected']
    d=Dataset(root);cache={k:v for k,v in np.load(root/'features.npz').items()};a={k:v[cache['rows']] for k,v in d.a.items()};del d
    d1=np.isin(a['conference'],list(D1_CONFERENCES));warm=a['day']>=day_number('2023-01-01');cols=columns(json.load(open(study/f'{chosen}-2026-model.json'))['groups'],True)
    def fit(model,mask,leaves):
        idx=np.flatnonzero(mask);baseline=predict(model,cache,idx)[0];y=a['minutes'][idx]-baseline;w=.7**(int(a['season'][idx].max())-a['season'][idx])
        tree=HistGradientBoostingRegressor(loss='squared_error',max_iter=120,learning_rate=.06,max_leaf_nodes=leaves,min_samples_leaf=100,l2_regularization=10,early_stopping=False,random_state=20261106)
        tree.fit(cache['x'][idx][:,cols],y,sample_weight=w)
        return tree
    def forecast(model,tree,idx):
        baseline=predict(model,cache,idx)[0];minutes=np.clip(baseline+tree.predict(cache['x'][idx][:,cols]),.5,40)
        return predict(model,cache,idx,actual_minutes=minutes)
    def summarize(model,tree,idx):
        pred=forecast(model,tree,idx);actual=cache['stats'][idx]@WEIGHTS;groups=masks(cache,a,idx)
        return {'groups':{k:{'model':metrics(actual[m],pred[2][m]),'minutes':metrics(a['minutes'][idx][m],pred[0][m])} for k,m in groups.items()}},pred
    validation={};base={}
    for fold,(start,test,_) in FOLDS.items():
        mask=d1&warm&(a['day']<day_number(start));idx=np.flatnonzero(d1&(a['day']>=day_number(start))&(a['day']<day_number(test)));model=json.load(open(study/f'{chosen}-{fold}-validation-model.json'))
        main_cohort=a['eligible'][idx]&a['d1_matchup'][idx];actual=cache['stats'][idx]@WEIGHTS;bp=predict(model,cache,idx)[2];base[fold]={'primary':metrics(actual[main_cohort],bp[main_cohort])['mae'],'all':metrics(actual[a['d1_matchup'][idx]],bp[a['d1_matchup'][idx]])['mae']}
        for leaves in [7,15]:
            tree=fit(model,mask,leaves);result,_=summarize(model,tree,idx);validation.setdefault(str(leaves),{})[fold]=result;print('Minutes validation',fold,leaves,result['groups']['eligible_conferences']['model']['mae'],flush=True)
    candidates=[None]+[leaves for leaves in [7,15] if all(validation[str(leaves)][f]['groups']['all_verified_d1']['model']['mae']<=base[f]['all']*1.005 for f in FOLDS)]
    def objective(leaves):return 1. if leaves is None else float(np.mean([validation[str(leaves)][f]['groups']['eligible_conferences']['model']['mae']/base[f]['primary'] for f in FOLDS]))
    selected=min(candidates,key=objective);choice={'selected_leaf_nodes':selected,'validation':validation,'baseline':base,'rule':'minimum mean relative eligible validation MAE, no all-D1 fold >0.5% worse','known_research_context':'The linear baseline test results were already inspected; the new tree variant selection itself uses validation only.'}
    (out/'selection-before-testing.json').write_text(json.dumps(choice,indent=2)+'\n');print('Selected minutes correction',selected,flush=True)
    tests={}
    if selected is not None:
        for fold in ['2025','2026','opening2027']:
            cutoff=FOLDS[fold][1] if fold in FOLDS else '2026-11-01';end=FOLDS[fold][2] if fold in FOLDS else '2026-11-06';mask=d1&warm&(a['day']<day_number(cutoff));idx=np.flatnonzero(d1&(a['day']>=day_number(cutoff))&(a['day']<day_number(end)))
            model=json.load(open(study/f'{chosen}-{fold}-model.json'));tree=fit(model,mask,selected);result,pred=summarize(model,tree,idx);baseline=predict(model,cache,idx)[2];actual=cache['stats'][idx]@WEIGHTS;m=a['eligible'][idx]&a['d1_matchup'][idx];result['paired']=paired_bootstrap(a,idx,actual,baseline,pred[2],m);tests[fold]=result
            with (out/f'{fold}-minutes.pkl').open('wb') as file:pickle.dump({'estimator':tree,'columns':cols,'linear_model':model},file)
            np.savez_compressed(out/f'{fold}-predictions.npz',idx=idx,minutes=pred[0],stats=pred[1],points=pred[2]);print('Minutes test',fold,result['groups']['eligible_conferences']['model']['mae'],flush=True)
    adopted=bool(tests and all(v['paired']['mae_reduction']>0 for v in tests.values()))
    (out/'results.json').write_text(json.dumps({'selection':choice,'tests':tests,'consistent_improvement_all_three':adopted,'note':'Keep the serializable linear model as the default unless every check improves; tree artifacts are trusted local training outputs, not third-party pickles.'},indent=2)+'\n')


if __name__=='__main__':main()
