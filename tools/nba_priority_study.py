#!/usr/bin/env python3
"""Test structural NBA priorities against the frozen workload champion.

Select additions using validation before scoring reused historical/current test
windows. Keep the default artifact unchanged; write a separate candidate.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

from nba_feature_study import paired

ROOT=Path(__file__).resolve().parents[1]
FOLDS={'historical':('2025-10-01','2026-01-01','2026-07-01'),'current':('2026-10-20','2026-10-30','2026-11-05')}
VARIANTS={
 'baseline':['workload'],
 'role_state':['workload','role_state'],
 'stat_windows':['workload','stat_windows'],
 'opportunities':['workload','opportunities'],
 'official_absences':['workload','teammate_minutes','reported_availability'],
 'official_budget':['workload','teammate_minutes','reported_availability','team_budget'],
 'structural_combined':['workload','role_state','stat_windows','opportunities'],
 'reported_combined':['workload','role_state','stat_windows','opportunities','teammate_minutes','reported_availability'],
}


def digest(path):return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--out',default='data/nba/priority-study')
 args=parser.parse_args();out=ROOT/args.out;out.mkdir(parents=True,exist_ok=True)
 data=ROOT/'data/nba/history-enriched.jsonl';reports=ROOT/'data/nba/pregame-injuries.json'
 source=hashlib.sha256()
 for p in sorted((ROOT/'backend/internal/nbamodel').glob('*.go')):
  if not p.name.endswith('_test.go'):source.update(p.name.encode());source.update(p.read_bytes())
 source.update((ROOT/'backend/cmd/nba-model/main.go').read_bytes());source.update(Path(__file__).read_bytes())
 manifest={'data_sha256':digest(data),'availability_sha256':digest(reports),'source_sha256':source.hexdigest(),'folds':FOLDS,'variants':VARIANTS,
  'selection':'Lowest mean relative validation MAE; no fold >0.5% worse. Hyperparameters use the same six-point grid. Test windows are reused research benchmarks.'}
 manifest_path=out/'manifest.json'
 if manifest_path.exists() and json.loads(manifest_path.read_text())!=json.loads(json.dumps(manifest)):raise ValueError('Source/data/protocol changed; use a new output directory')
 manifest_path.write_text(json.dumps(manifest,indent=2)+'\n')
 env=dict(os.environ,GOCACHE=os.environ.get('GOCACHE','/tmp/cbb-go-cache'))
 def run(name,fold,phase,flags):
  path=out/f'{name}-{fold}-{phase}.json'
  if path.exists():return json.loads(path.read_text())
  start=time.monotonic()
  command=['go','run','./cmd/nba-model',phase,'--data',str(data),'--report',str(path)]+flags
  with (out/f'{name}-{fold}-{phase}.log').open('w') as log:
   p=subprocess.run(command,cwd=ROOT/'backend',env=env,stdout=subprocess.DEVNULL,stderr=log)
  if p.returncode:raise RuntimeError((out/f'{name}-{fold}-{phase}.log').read_text()[-4000:])
  report=json.loads(path.read_text())
  value=min(c['validation']['mae'] for c in report['validation_candidates']) if phase=='train' else report['evaluation']['model']['mae']
  print(name,fold,phase,f'MAE {value:.5f}',f'{time.monotonic()-start:.1f}s',flush=True)
  return report
 scores={}
 for name,features in VARIANTS.items():
  scores[name]={}
  for fold,(train,test,_) in FOLDS.items():
   flags=['--features',','.join(features),'--train-before',train,'--test-from',test,'--skip-evaluation','--model',str(out/f'{name}-{fold}-model.json')]
   if 'reported_availability' in features:flags+=['--availability',str(reports)]
   report=run(name,fold,'train',flags)
   scores[name][fold]=min(c['validation']['mae'] for c in report['validation_candidates'])
 baseline=scores['baseline']
 relative=lambda name:sum(scores[name][f]/baseline[f] for f in FOLDS)/len(FOLDS)
 selected=min((n for n in VARIANTS if all(scores[n][f]/baseline[f]<=1.005 for f in FOLDS)),key=relative)
 selection={'selected':selected,'scores':scores,'relative':{n:relative(n) for n in VARIANTS}}
 (out/'selection-before-testing.json').write_text(json.dumps(selection,indent=2)+'\n')
 print('Validation-selected point model:',selected,flush=True)
 evaluations={};comparisons={}
 for name,features in VARIANTS.items():
  evaluations[name]={}
  for fold,(_,test,end) in FOLDS.items():
   flags=['--model',str(out/f'{name}-{fold}-model.json'),'--test-from',test,'--test-to',end,'--errors',str(out/f'{name}-{fold}-errors.jsonl')]
   if 'reported_availability' in features:flags+=['--availability',str(reports)]
   report=run(name,fold,'evaluate',flags);evaluations[name][fold]=report['evaluation']
  if name=='baseline':
   for fold in FOLDS:
    old=json.loads((ROOT/f'data/nba/feature-study/workload-{fold}-evaluate.json').read_text())['evaluation']['model']['mae']
    if abs(old-evaluations[name][fold]['model']['mae'])>1e-10:raise ValueError('Frozen workload baseline changed')
  else:comparisons[name]={f:paired(out/f'baseline-{f}-errors.jsonl',out/f'{name}-{f}-errors.jsonl') for f in FOLDS}
 # Evaluate same-origin matchup ordering for the baseline and chosen candidate.
 origins={}
 for name in dict.fromkeys(['baseline',selected]):
  origins[name]={}
  for fold,(_,test,end) in FOLDS.items():
   flags=['--model',str(out/f'{name}-{fold}-model.json'),'--test-from',test,'--test-to',end,'--freeze-week','--rank-features','--errors',str(out/f'{name}-{fold}-origin.jsonl')]
   if 'reported_availability' in VARIANTS[name]:flags+=['--availability',str(reports)]
   # The CLI command remains evaluate; use separate report/log filenames.
   path=out/f'{name}-{fold}-origin-report.json'
   if not path.exists():
    with (out/f'{name}-{fold}-origin.log').open('w') as log:
     subprocess.run(['go','run','./cmd/nba-model','evaluate','--data',str(data),'--report',str(path)]+flags,cwd=ROOT/'backend',env=env,stdout=log,stderr=log,check=True)
   origins[name][fold]=json.loads(path.read_text())['evaluation']
 improved=selected!='baseline' and all(comparisons[selected][f]['mae_reduction']>0 for f in FOLDS)
 if improved:shutil.copyfile(out/f'{selected}-current-model.json',ROOT/'data/nba/priority-candidate-model.json')
 result={'manifest':manifest,'selection':selection,'evaluations':evaluations,'comparisons':comparisons,'origins':origins,'candidate_improved_both':improved,
  'injury_audit':json.loads(reports.read_text())['audit'],'default_model_changed':False}
 (out/'results.json').write_text(json.dumps(result,indent=2)+'\n')
 print('Priority study complete:',selected,'improves both:',improved,flush=True)


if __name__=='__main__':main()
