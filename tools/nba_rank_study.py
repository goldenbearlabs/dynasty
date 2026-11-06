#!/usr/bin/env python3
"""Train a separate same-player/same-origin matchup ranker, without changing FP."""
import argparse
from collections import defaultdict
import json
import math
import os
from pathlib import Path
from statistics import mean
import time

ROOT=Path(__file__).resolve().parents[1]
STATS=['pts','reb','ast','stl','blk','tov','tpm','fgm','fga','ftm','fta','dd','td','pts40','pts50']


def read(path):
 with Path(path).open() as source:rows=[json.loads(line) for line in source]
 if any(len(r['rank_features'])!=33 for r in rows):raise ValueError('rank feature schema mismatch')
 return rows


def groups(rows):
 result=defaultdict(list)
 for r in rows:result[(r['player'],r['forecast_origin'])].append(r)
 return [g for g in result.values() if len(g)>=2]


def pairs(rows):
 result=[]
 for group in groups(rows):
  local=[]
  for i,a in enumerate(group):
   for b in group[i+1:]:
    if a['actual']==b['actual']:continue
    local.append(([x-y for x,y in zip(a['rank_features'],b['rank_features'])],float(a['actual']>b['actual'])))
  for x,y in local:result.append((x,y,1/len(local)))
 return result


def sigmoid(x):return 1/(1+math.exp(-min(30,max(-30,x))))


def fit(rows,penalty):
 examples=pairs(rows)
 if len(examples)<30:raise ValueError('too few training pairs')
 size=len(examples[0][0]);weight=sum(w for _,_,w in examples)
 scales=[max(.02,math.sqrt(sum(w*x[i]*x[i] for x,_,w in examples)/weight)) for i in range(size)]
 examples=[([min(5,max(-5,x/s)) for x,s in zip(values,scales)],y,w) for values,y,w in examples]
 weights=[0.0]*size
 for _ in range(100):
  gradient=[0.0]*size
  for x,y,w in examples:
   error=(sigmoid(sum(a*b for a,b in zip(weights,x)))-y)*w
   for i,v in enumerate(x):gradient[i]+=error*v
  for i in range(size):weights[i]-=.12*(gradient[i]/weight+penalty*weights[i])
 return {'weights':weights,'scales':scales,'ridge':penalty,'training_pairs':len(examples)}


def utility(row,model):
 if model is None:return row['rank_features'][0]
 # Row features are unclipped so score differences remain transitive. Input
 # differences were clipped only to make fitting robust to sparse extremes.
 return sum(w*x/s for w,x,s in zip(model['weights'],row['rank_features'],model['scales']))


def evaluate(rows,model):
 wins=count=0;regret=[];chosen=[];random_regret=[];brier=[];bins=defaultdict(list)
 for group in groups(rows):
  values=[utility(r,model) for r in group]
  top=max(values);picked=[r['actual'] for r,v in zip(group,values) if abs(v-top)<=1e-12]
  best=max(r['actual'] for r in group);chosen.append(mean(picked));regret.append(best-mean(picked));random_regret.append(best-mean(r['actual'] for r in group))
  for i,a in enumerate(group):
   for j in range(i+1,len(group)):
    b=group[j]
    if a['actual']==b['actual']:continue
    delta=values[i]-values[j];label=float(a['actual']>b['actual'])
    wins+=.5 if abs(delta)<=1e-12 else float((delta>0)==bool(label));count+=1
    if model is not None:
     p=sigmoid(delta);brier.append((p-label)**2);bins[min(9,int(p*10))].append((p,label))
 return {'pairs':count,'pair_accuracy':wins/count if count else .5,'player_weeks':len(regret),'regret':mean(regret),'selected_actual_score':mean(chosen),
  'random_regret':mean(random_regret),'brier':mean(brier) if brier else None,
  'probability_bins':[{'bin':i,'n':len(v),'forecast':mean(x[0] for x in v),'actual':mean(x[1] for x in v)} for i,v in sorted(bins.items())]}


def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--out',default='data/nba/priority-study');args=parser.parse_args()
 out=ROOT/args.out;data={}
 for fold in ['historical','current']:
  for stage in ['validation','test']:
   path=out/f'rank-{fold}-{stage}.jsonl'
   while not path.exists():time.sleep(3)
  validation=read(out/f'rank-{fold}-validation.jsonl')
  origins=sorted(set(r['forecast_origin'] for r in validation));cutoff=origins[max(1,int(.6*len(origins)))]
  data[fold]={'fit':[r for r in validation if r['forecast_origin']<cutoff],'tune':[r for r in validation if r['forecast_origin']>=cutoff],
              'validation':validation,'test':read(out/f'rank-{fold}-test.jsonl')}
 candidates=[{'penalty':None,'validation':{f:evaluate(d['tune'],None) for f,d in data.items()}}]
 for penalty in [.01,.1,1.0]:
  candidate={'penalty':penalty,'validation':{}}
  for fold,d in data.items():
   model=fit(d['fit'],penalty);candidate['validation'][fold]=evaluate(d['tune'],model)
  candidates.append(candidate);print('Ranking validation',penalty,{f:round(v['regret'],4) for f,v in candidate['validation'].items()},flush=True)
 baseline=candidates[0]['validation']
 def objective(c):return mean(c['validation'][f]['regret']/baseline[f]['regret'] for f in data)
 eligible=[c for c in candidates if all(c['validation'][f]['pair_accuracy']>=baseline[f]['pair_accuracy']-.005 for f in data)]
 chosen=min(eligible,key=objective)
 (out/'ranking-selection-before-testing.json').write_text(json.dumps({'chosen':chosen,'candidates':candidates},indent=2)+'\n')
 tests={};models={}
 for fold,d in data.items():
  model=fit(d['validation'],chosen['penalty']) if chosen['penalty'] is not None else None
  models[fold]=model;tests[fold]={'baseline':evaluate(d['test'],None),'selected':evaluate(d['test'],model)}
  print('Ranking test',fold,tests[fold]['baseline']['pair_accuracy'],tests[fold]['selected']['pair_accuracy'],flush=True)
 result={'selected':chosen,'candidates':candidates,'tests':tests,'models':models,
  'feature_names':['expected_fp_div20','expected_minutes_div36','home']+[f'{stat}_{feature}' for stat in STATS for feature in ['opponent_position_delta','last_meeting_delta']],
  'notes':['Same player and frozen origin; only pregame matchup features enter the ranking head. No expected fantasy-point forecasts are replaced.',
           'Fit on early validation origins; select on later validation origins; refit all validation before test. Test benchmarks are reused development data.',
           'Pairs are weighted equally per player-week. No intercept is fitted, ensuring pair antisymmetry.']}
 (out/'ranking-results.json').write_text(json.dumps(result,indent=2)+'\n')
 if chosen['penalty'] is not None:(ROOT/'data/nba/ranking-candidate.json').write_text(json.dumps({'model':models['current'],'feature_names':result['feature_names']},indent=2)+'\n')


if __name__=='__main__':main()
