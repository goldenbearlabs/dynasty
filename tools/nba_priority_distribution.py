#!/usr/bin/env python3
"""Recalibrate uncertainty/typical scores for the newly selected point model."""
import json
import os
from pathlib import Path
from statistics import mean
import subprocess

from nba_priority_study import ROOT,FOLDS
from nba_variation import build_priors,load_errors,Distribution,metrics


def main():
 out=ROOT/'data/nba/priority-study';chosen=json.loads((out/'selection-before-testing.json').read_text())['selected']
 env=dict(os.environ,GOCACHE=os.environ.get('GOCACHE','/tmp/cbb-go-cache'))
 model=json.loads((out/f'{chosen}-current-model.json').read_text());scoring=model['selection_scoring']
 data=ROOT/'data/nba/history-enriched.jsonl';folds={}
 for fold,(start,test,_) in FOLDS.items():
  path=out/f'{chosen}-{fold}-calibration.jsonl'
  if not path.exists():
   cmd=['go','run','./cmd/nba-model','evaluate','--model',str(out/f'{chosen}-{fold}-model.json'),'--refit-before',start,'--test-from',start,'--test-to',test,
        '--availability',str(ROOT/'data/nba/pregame-injuries.json'),'--errors',str(path),'--report',str(out/f'{chosen}-{fold}-calibration-report.json')]
   with (out/f'{chosen}-{fold}-calibration.log').open('w') as log:subprocess.run(cmd,cwd=ROOT/'backend',env=env,stdout=log,stderr=log,check=True)
 print('Building score priors',flush=True);priors=build_priors(data,scoring)
 for fold in FOLDS:
  validation=load_errors(out/f'{chosen}-{fold}-calibration.jsonl',priors);days=sorted(set(r['prior']['day'] for r in validation));cutoff=days[int(.6*len(days))]
  folds[fold]={'validation':validation,'fit':[r for r in validation if r['prior']['day']<cutoff],'tune':[r for r in validation if r['prior']['day']>=cutoff],
               'test':load_errors(out/f'{chosen}-{fold}-errors.jsonl',priors)}
 candidates=[]
 for personalized in [False,True]:
  for scaled in [False,True]:
   losses={f:Distribution(d['fit'],personalized,scaled).loss(d['tune']) for f,d in folds.items()}
   candidates.append({'personalized':personalized,'scaled':scaled,'validation_losses':losses})
 reference=candidates[1]['validation_losses']
 selected=min(candidates,key=lambda c:mean(c['validation_losses'][f]/reference[f] for f in folds))
 (out/'distribution-selection-before-testing.json').write_text(json.dumps({'selected':selected,'candidates':candidates},indent=2)+'\n')
 result={'point_model':chosen,'selected':selected,'candidates':candidates,'tests':{}}
 for fold,d in folds.items():
  distribution=Distribution(d['validation'],selected['personalized'],selected['scaled'])
  typical=[dict(r,predicted=distribution.predict_quantile(r,.5)) for r in d['test']]
  actual=[r['actual'] for r in d['test']];expected=[r['predicted'] for r in d['test']]
  median_error=mean(abs(r['actual']-r['predicted']) for r in typical)
  result['tests'][fold]={'expected_mae':mean(abs(a-b) for a,b in zip(actual,expected)),'typical_mae':median_error,'distribution':distribution.evaluate(d['test'])}
  (out/f'{chosen}-{fold}-distribution.json').write_text(json.dumps({'fitted_before':FOLDS[fold][1],'point_model':str(out/f'{chosen}-{fold}-model.json'),
    'distribution':distribution.artifact(),'note':'Median/typical score is separate from the expected stat line.'},indent=2)+'\n')
  print(fold,'expected MAE',result['tests'][fold]['expected_mae'],'typical MAE',median_error,flush=True)
 (out/'distribution-results.json').write_text(json.dumps(result,indent=2)+'\n')


if __name__=='__main__':main()
