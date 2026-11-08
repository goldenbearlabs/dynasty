#!/usr/bin/env python3
"""Validation-selected nonlinear interactions for the additional CBB context."""
import copy
import hashlib
import json
from pathlib import Path
import numpy as np
from sklearn.ensemble import HistGradientBoostingRegressor
from cbb_features import Dataset,WEIGHTS
from cbb_context_model import predict,design
from cbb_candidate import predict as baseline_predict,export_trees,fit_uncertainty,opening_phase,uncertainty
from cbb_dataset import D1_CONFERENCES
from cbb_strength import day_number
from cbb_study import FOLDS
from cbb_model import metrics,masks,paired_bootstrap,variation
from cbb_export import same_origin_ranking


def save(path,value):path.write_text(json.dumps(value,indent=2,allow_nan=False)+'\n')


def main():
    root=Path('data/cbb');study=root/'context-study';out=root/'context-minutes';out.mkdir(exist_ok=True);selected=json.load(open(study/'selection-before-testing.json'))['selected'];data=Dataset(root);cache={k:v for k,v in np.load(root/'features.npz').items()};ctx={k:v for k,v in np.load(root/'context/features.npz').items()};a={k:v[cache['rows']] for k,v in data.a.items()};del data
    d1=np.isin(a['conference'],list(D1_CONFERENCES));warm=a['day']>=day_number('2023-01-01');configs=[None,{'leaves':7,'iterations':80,'context':False},{'leaves':7,'iterations':80,'context':True},{'leaves':15,'iterations':120,'context':True}];valid={};preds={};models={}
    def fit(base,joint,train,config):
        idx=np.flatnonzero(train);bm=baseline_predict(base,cache,idx)[0];core=joint['context_correction']['core_columns'];x=design(cache,ctx,idx,core);cols=list(range(x.shape[1])) if config['context'] else list(range(len(core)))
        tree=HistGradientBoostingRegressor(loss='squared_error',max_iter=config['iterations'],max_leaf_nodes=config['leaves'],learning_rate=.05,min_samples_leaf=100,l2_regularization=10,early_stopping=False,random_state=20261107)
        tree.fit(x[:,cols],a['minutes'][idx]-bm,sample_weight=.7**(int(a['season'][idx].max())-a['season'][idx]));model=copy.deepcopy(joint);model['context_correction']['minutes_trees']=export_trees(tree,cols);return model
    for fold,(start,test,_) in FOLDS.items():
        base=json.load(open(study/f'baseline-{fold}-validation-model.json'));joint=json.load(open(study/f'{selected}-{fold}-validation-model.json'));train=d1&warm&(a['day']<day_number(start));vi=np.flatnonzero(d1&(a['day']>=day_number(start))&(a['day']<day_number(test)));actual=cache['stats'][vi]@WEIGHTS;primary=a['eligible'][vi]&a['d1_matchup'][vi]
        for k,config in enumerate(configs):
            model=joint if config is None else fit(base,joint,train,config);pred=predict(model,cache,ctx,vi);valid.setdefault(str(k),{})[fold]={'primary':metrics(actual[primary],pred[2][primary]),'all_d1':metrics(actual[a['d1_matchup'][vi]],pred[2][a['d1_matchup'][vi]])};models[(k,fold)]=model;preds[(k,fold)]=(vi,pred)
            save(out/f'{k}-{fold}-validation-model.json',model);print('Context minutes validation',fold,k,valid[str(k)][fold]['primary']['mae'],flush=True)
    candidates=[k for k in range(len(configs)) if all(valid[str(k)][f]['all_d1']['mae']<=valid['0'][f]['all_d1']['mae']*1.005 for f in FOLDS)];chosen=min(candidates,key=lambda k:np.mean([valid[str(k)][f]['primary']['mae']/valid['0'][f]['primary']['mae'] for f in FOLDS]));choice={'selected':chosen,'configuration':configs[chosen],'validation':valid,'rule':'Minimize mean relative historical validation MAE with all-D1 guard; current/fresh game results never select hyperparameters.'};save(out/'selection-before-testing.json',choice);print('Selected context minutes',chosen,flush=True)
    results={}
    for fold in ['2025','2026','opening2027']:
        cutoff=FOLDS[fold][1] if fold in FOLDS else '2026-11-01';end=FOLDS[fold][2] if fold in FOLDS else '2026-11-06';base=json.load(open(root/f'final-study/{fold}-model.json'));joint=json.load(open(study/f'selected-{fold}-model.json'));train=d1&warm&(a['day']<day_number(cutoff));idx=np.flatnonzero(d1&(a['day']>=day_number(cutoff))&(a['day']<day_number(end)));model=joint if chosen==0 else fit(base,joint,train,configs[chosen]);new=predict(model,cache,ctx,idx);bp=baseline_predict(base,cache,idx);jp=predict(joint,cache,ctx,idx);actual=cache['stats'][idx]@WEIGHTS;primary=a['eligible'][idx]&a['d1_matchup'][idx]
        vi,vp=preds[(chosen,fold if fold in FOLDS else '2026')];vm=a['eligible'][vi]&a['d1_matchup'][vi];model['uncertainty']=fit_uncertainty((cache['stats'][vi]@WEIGHTS)[vm],vp[2][vm],cache['prior_mean'][vi][vm],cache['prior_sd'][vi][vm],opening_phase(a['day'][vi][vm]));u=uncertainty(model,new[2][primary],cache['prior_mean'][idx][primary],cache['prior_sd'][idx][primary],opening_phase(a['day'][idx][primary]));groups=masks(cache,a,idx);fi=idx[primary&cache['frozen_known'][idx]]
        result={'model':metrics(actual[primary],new[2][primary]),'minutes':metrics(a['minutes'][idx][primary],new[0][primary]),'paired_vs_original':paired_bootstrap(a,idx,actual,bp[2],new[2],primary),'paired_vs_linear_context':paired_bootstrap(a,idx,actual,jp[2],new[2],primary),'groups':{g:{'model':metrics(actual[m],new[2][m]),'minutes':metrics(a['minutes'][idx][m],new[0][m])} for g,m in groups.items()},'ranking':same_origin_ranking(cache,a,fi,predict(model,cache,ctx,fi,True)[2]),'variation':variation(cache,a,idx[primary],new[2][primary]),'interval80_coverage':float(np.mean((actual[primary]>=u['lower80'])&(actual[primary]<=u['upper80']))),'interval95_coverage':float(np.mean((actual[primary]>=u['lower95'])&(actual[primary]<=u['upper95']))),'brier':float(np.mean((u['probability_above_usual']-(actual[primary]>cache['prior_mean'][idx][primary]))**2))};results[fold]=result;save(out/f'{fold}-model.json',model);np.savez_compressed(out/f'{fold}-predictions.npz',idx=idx,minutes=new[0],stats=new[1],points=new[2]);print('Context minutes test',fold,result['model']['mae'],flush=True)
    save(out/'results.json',{'selection':choice,'results':results,'consistently_beats_original':all(d['paired_vs_original']['mae_reduction']>0 for d in results.values()),'note':'Prior benchmark results had been inspected; added tree configurations select on prior validations only. A newly collected Nov 6 check remains separate.'})


if __name__=='__main__':main()
