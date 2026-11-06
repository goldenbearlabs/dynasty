#!/usr/bin/env python3
"""Earlier-season robustness check of the validation-selected priority model.

The feature recipe was selected on later folds, so this is a retrospective
transfer check, not a claim of a pristine future holdout.
"""
import json
import os
from pathlib import Path
import subprocess

from nba_priority_study import ROOT,VARIANTS


def main():
 out=ROOT/'data/nba/priority-study';selected=json.loads((out/'selection-before-testing.json').read_text())['selected']
 env=dict(os.environ,GOCACHE=os.environ.get('GOCACHE','/tmp/cbb-go-cache'));results={}
 for name in dict.fromkeys(['baseline',selected]):
  groups=VARIANTS[name];model=out/f'{name}-earlier-model.json';report=out/f'{name}-earlier-report.json'
  flags=['--features',','.join(groups),'--train-before','2024-10-01','--test-from','2025-01-01','--test-to','2025-07-01',
         '--model',str(model),'--report',str(report),'--errors',str(out/f'{name}-earlier-errors.jsonl')]
  if 'reported_availability' in groups:flags+=['--availability',str(ROOT/'data/nba/pregame-injuries.json')]
  if not report.exists():
   with (out/f'{name}-earlier.log').open('w') as log:
    subprocess.run(['go','run','./cmd/nba-model','train','--data',str(ROOT/'data/nba/history-enriched.jsonl')]+flags,
                   cwd=ROOT/'backend',env=env,stdout=log,stderr=log,check=True)
  results[name]=json.loads(report.read_text());print(name,'earlier-season MAE',results[name]['evaluation']['model']['mae'],flush=True)
 (out/'earlier-confirmation.json').write_text(json.dumps({'selected':selected,'results':results,
  'note':'Earlier-season retrospective transfer check; feature recipe was selected on later folds, but all fitted coefficients and rolling features exclude the 2025 test outcomes.'},indent=2)+'\n')


if __name__=='__main__':main()
