#!/usr/bin/env python3
"""Publish a reviewable offline CBB candidate after all four checks improve."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import numpy as np
import pyarrow.parquet as pq

from cbb_context_features import NAMES
from cbb_features import STATS


def main():
    root=Path('data/cbb');out=root/'context-review';out.mkdir(exist_ok=True);study=json.load(open(root/'context-study/results.json'));trees=json.load(open(root/'context-minutes/results.json'));fresh=json.load(open(root/'context-forward/results.json'));old=json.load(open(root/'final-study/results.json'))
    improved=trees['consistently_beats_original'] and fresh['paired']['mae_reduction']>0 and fresh['all_d1']['context']['mae']<fresh['all_d1']['baseline']['mae']
    baseline=Path('models/cbb/v1-model.json')
    if not baseline.exists():shutil.copyfile('models/cbb/model.json',baseline)
    baseline_sha=hashlib.sha256(baseline.read_bytes()).hexdigest()
    if baseline_sha!=study['manifest']['baseline_model_sha256']:raise ValueError('original champion hash changed')
    candidate=json.load(open(root/'context-minutes/opening2027-model.json'));candidate['context_source_manifest']=json.load(open(root/'context/features.json'));candidate['context_selection_provenance']={'linear_selection_sha256':hashlib.sha256((root/'context-study/selection-before-testing.json').read_bytes()).hexdigest(),'minutes_selection_sha256':hashlib.sha256((root/'context-minutes/selection-before-testing.json').read_bytes()).hexdigest(),'baseline_model_sha256':baseline_sha,'fresh_holdout_sports_day':fresh['sports_day'],'availability_reports_used':False,'note':'Point model selection uses old chronological validation; historical benchmark reuse remains exploratory. Archives without report timestamps are not features.'};candidate['uncertainty_note']='Opening-week nominal 80% coverage remains about 76%; do not present it as fully calibrated.'
    encoded=json.dumps(candidate,indent=2,allow_nan=False)+'\n';Path('models/cbb/context-model.json').write_text(encoded);(root/'context-model.json').write_text(encoded)
    if improved:Path('models/cbb/model.json').write_text(encoded);(root/'model.json').write_text(encoded)
    cache=np.load(root/'features.npz');ctx={k:v for k,v in np.load(root/'context/features.npz').items()};table=pq.read_table(root/'history.parquet',columns=['game_id','player_id','eligible','d1_matchup']);rows=cache['rows'];a={k:table[k].to_numpy()[rows] for k in table.column_names}
    for fold in ['2025','2026','opening2027']:
        pred={k:v for k,v in np.load(root/f'context-minutes/{fold}-predictions.npz').items()};idx=pred['idx'];primary=a['eligible'][idx]&a['d1_matchup'][idx];local=np.flatnonzero(primary);old_rows=[json.loads(line) for line in (root/f'final-study/{fold}-predictions.jsonl').open()]
        if len(local)!=len(old_rows):raise ValueError('review cohorts differ')
        with (out/f'{fold}-predictions.jsonl').open('w') as output,(out/f'{fold}-baseline-errors.jsonl').open('w') as before:
            for old_row,j in zip(old_rows,local):
                i=int(idx[j]);target=old_row['target']
                if (str(a['game_id'][i]),str(a['player_id'][i]))!=(target['game_id'],target['player_id']):raise ValueError('review ordering changed')
                before.write(json.dumps({'game':target['game_id'],'player':target['player_id'],'actual':old_row['actual_points'],'predicted':old_row['prediction']['fantasy_points_if_active']})+'\n')
                old_row['prediction']={'minutes':float(pred['minutes'][j]),'stats':dict(zip(STATS,map(float,pred['stats'][j]))),'fantasy_points_if_active':float(pred['points'][j])};old_row['features']['context_values']=dict(zip(NAMES,map(float,ctx['x'][i])));output.write(json.dumps(old_row,separators=(',',':'))+'\n')
        subprocess.run(['python3','tools/cbb_report.py','--predictions',str(out/f'{fold}-predictions.jsonl'),'--baseline-errors',str(out/f'{fold}-baseline-errors.jsonl'),'--baseline-label','Previous CBB champion','--out',str(out/f'{fold}-review.html')],check=True)
    result={'previous_model_sha256':baseline_sha,'candidate_model_sha256':hashlib.sha256(encoded.encode()).hexdigest(),'offline_default_updated':bool(improved),'live_application_changed':False,'baseline':{f:d['primary'] for f,d in old['results'].items()},'new':trees['results'],'new_holdout':fresh,'context_ablation':study['evaluations'],'selection':{'linear':study['selection'],'minutes':trees['selection']},'sources':{'recruiting':candidate['context_source_manifest']['join_audit'],'availability':json.load(open(root/'context/availability-audit.json'))['audit']},'limits':['Positive-minute conditional accuracy, not DNP probability.','Recruiting labels are intrinsically pre-college but were downloaded now, not historical HTTP snapshots.','Past game rosters are observed after those games; target-game and final-season rosters are not inputs.','Archive availability lacks publication timestamps and is excluded.','Existing holdouts are reused research benchmarks; Nov 6 was collected after model selection, but remains retrospective.','Opening uncertainty remains undercovered.']}
    (out/'results.json').write_text(json.dumps(result,indent=2)+'\n');Path('docs/cbb-context-results.json').write_text(json.dumps(result,indent=2)+'\n');print('Offline default updated:',improved,'model sha',result['candidate_model_sha256'])


if __name__=='__main__':main()
