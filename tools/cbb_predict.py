#!/usr/bin/env python3
"""Evaluate CBB forecasts or predict explicit prospective targets.

Examples:
  cbb_predict.py evaluate --model data/cbb/final-study/2026-model.json --from 2026-01-01 --to 2026-04-16
  cbb_predict.py predict --targets data/cbb/upcoming-targets.json --out data/cbb/upcoming-predictions.json
"""
import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
import numpy as np
from cbb_features import Dataset,State,NAMES,STATS,WEIGHTS
from cbb_candidate import predict,uncertainty,opening_phase
from cbb_model import evaluate,metrics,masks
from cbb_dataset import sports_day,D1_CONFERENCES
from cbb_strength import day_number

ALLOWED={'game_id','player_id','team','opponent','season','starts_at','home','neutral','conference','opponent_conference','season_type'}


def validate_target(target,observed,model):
    unknown=set(target)-ALLOWED
    if unknown:raise ValueError('Target contains unsupported/outcome fields: '+','.join(sorted(unknown)))
    if not {'game_id','player_id','team','opponent','season','starts_at','home','neutral'}<=set(target):raise ValueError('Missing required target metadata')
    starts=dt.datetime.fromisoformat(target['starts_at'].replace('Z','+00:00'))
    if starts.tzinfo is None or observed.tzinfo is None:raise ValueError('Timestamps require time zones')
    if starts<=observed:raise ValueError('Target has started as of the information cutoff')
    if sports_day(starts)<model['trained_before']:raise ValueError('Target precedes the model training cutoff')
    if not isinstance(target['home'],bool) or not isinstance(target['neutral'],bool):raise ValueError('home and neutral must be booleans')
    if int(target.get('season_type',2)) not in [2,3]:raise ValueError('Only regular season and postseason are supported')
    if min(int(target[k]) for k in ['player_id','team','opponent'])<=0:raise ValueError('Positive ESPN identity ids are required')
    if int(target['team'])==int(target['opponent']):raise ValueError('A team cannot play itself')
    return starts


def replay_before(data,cache,cutoff):
    state=State(data);a=data.a;lookup={int(i):k for k,i in enumerate(cache['rows'])};days,starts=np.unique(a['day'],return_index=True);ends=np.r_[starts[1:],data.n]
    for day,first,last in zip(days,starts,ends):
        if day>=cutoff:break
        games=data.byday[int(day)];teams={side['team'] for g in games for side in g['teams']};season=int(a['season'][first]);state.ratings=state.strength.fit(season,int(day),teams);state.league=state.strength.league
        contexts={g['game_id']:state.strength.context(g,state.ratings,state.league) for g in games};base={}
        for i in range(int(first),int(last)):
            if i not in lookup:continue
            base[i]=cache['base'][lookup[i]];c=contexts[int(a['game_id'][i])][int(a['team'][i])];state.prior_opp_def[i]=c['opp_def'];state.prior_own_off[i]=c['own_off']
        state.observe(range(int(first),int(last)),games,base)
    return state


def check_cache(root,cache):
    manifest=json.load(open(root/'features.json'))
    if manifest['dataset_sha256']!=hashlib.sha256((root/'history.parquet').read_bytes()).hexdigest():raise ValueError('History changed; rebuild the feature cache')
    if manifest['features']!=NAMES or cache['x'].shape[1]!=len(NAMES):raise ValueError('Feature schema mismatch')
    current=Path('tools/cbb_features.py').read_text()+Path('tools/cbb_strength.py').read_text()+Path('tools/cbb_dataset.py').read_text()
    if hashlib.sha256(current.encode()).hexdigest()!=manifest['feature_source_sha256']:raise ValueError('Feature code changed; rebuild the cache')


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('command',choices=['evaluate','predict']);p.add_argument('--data',default='data/cbb');p.add_argument('--model',default='models/cbb/model.json');p.add_argument('--from',dest='since');p.add_argument('--to',dest='until');p.add_argument('--games');p.add_argument('--players');p.add_argument('--targets');p.add_argument('--observed-at');p.add_argument('--out');p.add_argument('--context',help='context data folder, default DATA/context');p.add_argument('--scoring',help='optional JSON map; rescoring only, not model reselection');args=p.parse_args()
    root=Path(args.data);model=json.load(open(args.model));data=Dataset(root);cache={k:v for k,v in np.load(root/'features.npz').items()};check_cache(root,cache)
    if model.get('sport')!='cbb' or model.get('feature_names')!=NAMES:raise ValueError('Unsupported model/schema')
    scoring=json.load(open(args.scoring)) if args.scoring else None
    weights=WEIGHTS if scoring is None else np.array([scoring.get(s,0) for s in STATS])
    context_root=Path(args.context) if args.context else root/'context'
    uses_context=bool(model.get('context_correction'))
    if uses_context:
        from cbb_context_model import predict as context_predict
        from cbb_context_features import NAMES as CONTEXT_NAMES,Recruiting,ContextState
        from cbb_context_sources import normalized
        from collections import defaultdict
        spec=model['context_correction']
        expected=[NAMES[i] for i in spec['core_columns']]+CONTEXT_NAMES
        if spec['feature_names']!=expected:raise ValueError('Context feature schema mismatch')

    if args.command=='evaluate':
        if not args.since or not args.until:p.error('evaluate requires --from and exclusive --to')
        if args.since<model['trained_before']:raise ValueError('Evaluation starts before model training cutoff; use the corresponding historical artifact')
        a={k:v[cache['rows']] for k,v in data.a.items()};mask=np.isin(a['conference'],list(D1_CONFERENCES))&(a['day']>=day_number(args.since))&(a['day']<day_number(args.until))
        if args.games:mask&=np.isin(a['game_id'],[int(v) for v in args.games.split(',')])
        if args.players:mask&=np.isin(a['player_id'],[int(v) for v in args.players.split(',')])
        idx=np.flatnonzero(mask)
        if not len(idx):raise ValueError('No scored appearances match the evaluation filters')
        if uses_context:
            extra={k:v for k,v in np.load(context_root/'features.npz').items()}
            context_manifest=json.loads((context_root/'features.json').read_text())
            if context_manifest['data_sha256']!=hashlib.sha256((root/'history.parquet').read_bytes()).hexdigest():raise ValueError('Context dataset changed; rebuild context cache')
            if context_manifest['recruiting_sha256']!=hashlib.sha256((context_root/'recruiting.json').read_bytes()).hexdigest():raise ValueError('Recruiting data changed; rebuild context cache')
            for file,sha in context_manifest['source_sha256'].items():
                if hashlib.sha256(Path(file).read_bytes()).hexdigest()!=sha:raise ValueError('Context feature code changed; rebuild context cache')
            if not np.array_equal(extra['rows'],cache['rows']):raise ValueError('Context feature cohort mismatch; rebuild context cache')
            minutes,stats,points=context_predict(model,cache,extra,idx)
        else:minutes,stats,points=predict(model,cache,idx)
        actual=cache['stats'][idx]@weights;points=stats@weights
        result={'mode':'historical_evaluation','from':args.since,'to_exclusive':args.until,'games':len(set(a['game_id'][idx])),'model':metrics(actual,points),'minutes':metrics(a['minutes'][idx],minutes),
                'groups':{k:metrics(actual[m],points[m]) for k,m in masks(cache,a,idx).items()},'note':'Positive-minute conditional accuracy. No DNP or archived market/injury replay. Custom scoring does not retrain/select a model.'}
    else:
        if not args.targets:p.error('predict requires --targets, a JSON list of pregame metadata only')
        targets=json.load(open(args.targets));observed=dt.datetime.fromisoformat(args.observed_at.replace('Z','+00:00')) if args.observed_at else dt.datetime.now(dt.timezone.utc)
        if not isinstance(targets,list) or not targets:raise ValueError('Targets must be a nonempty JSON list')
        for target in targets:validate_target(target,observed,model)
        cutoff=day_number(sports_day(observed));state=replay_before(data,cache,cutoff);outputs=[]
        context_state=None
        if uses_context:
            names=defaultdict(set)
            for player,name in zip(data.a['player_id'],data.table['name'].to_pylist()):names[int(player)].add(normalized(name))
            context_state=ContextState(data,Recruiting(context_root/'recruiting.json',names))
            days,starts=np.unique(data.a['day'],return_index=True);ends=np.r_[starts[1:],data.n]
            for past,first,last in zip(days,starts,ends):
                if past>=cutoff:break
                context_state.observe(range(int(first),int(last)))

        for target in targets:
            season=int(target['season']);team=int(target['team']);opponent=int(target['opponent']);home=team if target['home'] else opponent;away=opponent if target['home'] else team
            game={'game_id':int(target['game_id']),'season':season,'home':home,'away':away,'neutral':bool(target['neutral']),'season_type':int(target.get('season_type',2)),
                  'teams':[{'team':team,'opponent':opponent,'conference':int(target.get('conference',0))},{'team':opponent,'opponent':team,'conference':int(target.get('opponent_conference',0))}]}
            state.ratings=state.strength.fit(season,cutoff,[team,opponent]);state.league=state.strength.league;state.context_cache={};x,base,scale,shoot,meta=state.build(None,cutoff,game,target=target)
            inputs={'x':x[None,:],'base':base[None,:],'scales':scale[None,:],'shoot':shoot[None,:]}
            context_values=context_state.build(None,cutoff,target) if context_state is not None else None
            if uses_context:minutes,stats,points=context_predict(model,inputs,{'x':context_values[None,:]},np.array([0]))
            else:minutes,stats,points=predict(model,inputs,np.array([0]))
            item={'target':target,'minutes':float(minutes[0]),'stats':dict(zip(STATS,map(float,stats[0]))),'fantasy_points_if_active':float(stats[0]@weights),'history_games':meta['history'],'reference_recent_mean':meta['recent_points'],'feature_values':dict(zip(NAMES,map(float,x)))}
            if context_values is not None:item['context_features']=dict(zip(CONTEXT_NAMES,map(float,context_values)))
            if 'uncertainty' in model and args.scoring is None:
                u=uncertainty(model,points,np.array([meta['recent_points']]),np.array([meta['prior_sd']]),opening_phase(np.array([day_number(sports_day(dt.datetime.fromisoformat(target['starts_at'].replace('Z','+00:00'))))])))
                item['uncertainty']={k:float(v[0]) for k,v in u.items() if k!='base_probability'};item['uncertainty']['reference']='player_recent_mean' if meta['history']>=5 else 'pooled_cold_start_prior'
            outputs.append(item)
        result={'mode':'prospective' if observed>=dt.datetime.now(dt.timezone.utc)-dt.timedelta(days=1) else 'historical_asof_replay','information_cutoff':observed.isoformat(),'history_cutoff_exclusive_sports_day':str(np.datetime64(cutoff,'D')),'generated_at':dt.datetime.now(dt.timezone.utc).isoformat(),'predictions':outputs,'notes':['Conditional on playing; no availability/DNP model.','This standalone forecast does not fetch current injuries or assert a confirmed starting lineup.','History uses completed prior sports days.']}
    text=json.dumps(result,indent=2,allow_nan=False)+'\n'
    if args.out:Path(args.out).write_text(text)
    else:print(text,end='')


if __name__=='__main__':main()
