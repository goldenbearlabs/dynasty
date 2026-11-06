#!/usr/bin/env python3
"""Prepare chronological forecasts and same-origin matchup tests for the variation study."""
import argparse
import json
import os
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[1]
FOLDS={'historical':('2025-10-01','2026-01-01','2026-07-01'),
       'current':('2026-10-20','2026-10-30','2026-11-05')}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out',default='data/nba/variation-study')
    parser.add_argument('--models',default='data/nba/feature-study')
    parser.add_argument('--data',default='data/nba/history-enriched.jsonl')
    args=parser.parse_args()
    output=(ROOT/args.out).resolve();output.mkdir(parents=True,exist_ok=True)
    models=(ROOT/args.models).resolve();dataset=(ROOT/args.data).resolve()
    for fold in FOLDS:
        if not (models/f'workload-{fold}-model.json').exists():
            raise FileNotFoundError('Run the feature study first: '+str(models/f'workload-{fold}-model.json'))
    commands=[]
    env=dict(os.environ,GOCACHE=os.environ.get('GOCACHE','/tmp/cbb-go-cache'))
    for fold,(validation,test,end) in FOLDS.items():
        stages=[('validation',validation,test,['--refit-before',validation,'--diagnostics']),
                ('test',test,end,['--diagnostics']),('same-origin',test,end,['--freeze-week'])]
        for stage,start,stop,extra in stages:
            command=['go','run','./cmd/nba-model','evaluate','--data',str(dataset),'--model',str(models/f'workload-{fold}-model.json'),
                     '--test-from',start,'--test-to',stop,'--errors',str(output/f'{fold}-{stage}.jsonl'),
                     '--report',str(output/f'{fold}-{stage}-report.json')]+extra
            commands.append(command)
            with (output/f'{fold}-{stage}.log').open('w') as log:
                subprocess.run(command,cwd=ROOT/'backend',env=env,stdout=log,stderr=log,check=True)
            print(fold,stage,'complete',flush=True)
    (output/'preparation-commands.json').write_text(json.dumps(commands,indent=2)+'\n')


if __name__=='__main__':main()
