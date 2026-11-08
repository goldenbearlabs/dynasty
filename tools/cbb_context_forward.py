#!/usr/bin/env python3
"""A newly collected Nov 6 holdout, scored only after context selection.

This is retrospective evaluation on previously uninspected game data, not a
claim that these improved-model predictions were published before tipoff.
"""
import datetime as dt
import hashlib
import json
from pathlib import Path
import time
import urllib.request
from collections import defaultdict

import numpy as np
import pyarrow.parquet as pq
from cbb_dataset import D1_CONFERENCES,ELIGIBLE,validate_stats,sports_day
from cbb_features import Dataset,STATS,WEIGHTS
from cbb_context_sources import normalized
from cbb_context_features import Recruiting,ContextState
from cbb_context_model import predict
from cbb_model import metrics,paired_bootstrap
from cbb_predict import replay_before
from cbb_strength import day_number
from nba_history import parse_boxscore

BASE='https://site.api.espn.com/apis/site/v2/sports/basketball/mens-college-basketball'


def main():
    root=Path('data/cbb');out=root/'context-forward';raw=out/'raw';raw.mkdir(parents=True,exist_ok=True);choice=json.load(open(root/'context-study/selection-before-testing.json'));tree_choice_path=root/'context-minutes/selection-before-testing.json';tree_choice=json.load(open(tree_choice_path));tree_selected_at=dt.datetime.fromtimestamp(tree_choice_path.stat().st_mtime,dt.timezone.utc).isoformat();day='2026-11-06';cutoff=day_number(day)
    log=[]
    def fetch(url,path):
        if not path.exists():
            with urllib.request.urlopen(urllib.request.Request(url,headers={'User-Agent':'mosaic-cbb-research/1.0'}),timeout=45) as response:body=response.read()
            path.write_bytes(body);time.sleep(.3)
        body=path.read_bytes();log.append({'url':url,'sha256':hashlib.sha256(body).hexdigest(),'bytes':len(body)});return json.loads(body)
    payload=fetch(BASE+'/scoreboard?dates=20261106&groups=50&limit=500',raw/'scoreboard.json');events=[e for e in payload.get('events',[]) if e.get('status',{}).get('type',{}).get('completed') and e.get('season',{}).get('type')==2]
    schedule={int(r['game_id']):r for r in pq.read_table(root/'raw/mbb_schedule_2027.parquet').to_pylist()};all_rows=[];game_objects={};skipped=[]
    for event in events:
        game=int(event['id'])
        if game not in schedule:skipped.append({'game':game,'reason':'no prior schedule context'});continue
        summary=fetch(BASE+'/summary?event='+str(game),raw/f'{game}.json');rows=parse_boxscore(summary);s=schedule[game]
        if not rows:skipped.append({'game':game,'reason':'no complete boxscore'});continue
        if any(sports_day(dt.datetime.fromisoformat(r['starts_at']))!=day for r in rows):continue
        home,away=int(s['home_id']),int(s['away_id']);confs={home:int(s.get('home_conference_id') or 0),away:int(s.get('away_conference_id') or 0)}
        scores={int(side['team']['id']):float(side['score']) for side in summary['header']['competitions'][0]['competitors']}
        if any(abs(sum(r['stats'].get('pts',0) for r in rows if int(r['team'])==t)-score)>.01 for t,score in scores.items()):skipped.append({'game':game,'reason':'player score mismatch'});continue
        g={'game_id':game,'home':home,'away':away,'neutral':bool(s['neutral_site']),'season':2027,'season_type':2,'teams':[{'team':t,'opponent':away if t==home else home,'conference':confs[t]} for t in [home,away]]};game_objects[game]=g
        for row in rows:
            if not row['played'] or row['minutes']<=0 or int(row['team']) not in confs:continue
            if any(k not in row['stats'] for k in STATS+['tpa']) or not validate_stats(row['stats']):continue
            team=int(row['team']);opp=int(row['opponent']);row.update(conference=confs[team],opponent_conference=confs[opp],eligible=confs[team] in ELIGIBLE,d1_matchup=all(c in D1_CONFERENCES for c in confs.values()),day=cutoff,neutral=bool(s['neutral_site']),season_type=2)
            if confs[team] in D1_CONFERENCES:all_rows.append(row)
    print('New holdout collected:',len(game_objects),'games,',len(all_rows),'conditional D1 player rows',flush=True)
    data=Dataset(root);cache={k:v for k,v in np.load(root/'features.npz').items()};state=replay_before(data,cache,cutoff);names=defaultdict(set)
    for player,name in zip(data.a['player_id'],data.table['name'].to_pylist()):names[int(player)].add(normalized(name))
    recruits=Recruiting(root/'context/recruiting.json',names);context_state=ContextState(data,recruits)
    days,starts=np.unique(data.a['day'],return_index=True);ends=np.r_[starts[1:],data.n]
    for past,first,last in zip(days,starts,ends):
        if past>=cutoff:break
        context_state.observe(range(int(first),int(last)))
    x=[];base=[];scales=[];shoot=[];ctx=[]
    all_teams={int(r[k]) for r in all_rows for k in ['team','opponent']};state.ratings=state.strength.fit(2027,cutoff,all_teams);state.league=state.strength.league
    for row in all_rows:
        target={k:row[k] for k in ['game_id','player_id','team','opponent','season','opponent_conference']};g=game_objects[int(row['game_id'])];state.context_cache={}
        xx,b,s,h,meta=state.build(None,cutoff,g,target=target);x.append(xx);base.append(b);scales.append(s);shoot.append(h);ctx.append(context_state.build(None,cutoff,target))
    c={'x':np.array(x),'base':np.array(base),'scales':np.array(scales),'shoot':np.array(shoot),'stats':np.array([[r['stats'][s] for s in STATS] for r in all_rows])};extra={'x':np.array(ctx)};idx=np.arange(len(all_rows));a={k:np.array([int(r[k]) if k in ['game_id','player_id','team','opponent','season'] else r[k] for r in all_rows]) for k in ['game_id','player_id','team','opponent','season','minutes','eligible','d1_matchup']};actual=c['stats']@WEIGHTS
    base_model=json.load(open(root/'final-study/opening2027-model.json'));model=json.load(open(root/'context-minutes/opening2027-model.json'));before=predict(base_model,c,extra,idx);after=predict(model,c,extra,idx);primary=a['eligible']&a['d1_matchup']
    if not primary.any():raise ValueError('No eligible holdout observations')
    result={'sports_day':day,'context_selected_before_collection':choice['saved_before_test_predictions'],'minutes_selection_saved_before_collection':tree_selected_at,'minutes_configuration':tree_choice['configuration'],'captured_at':dt.datetime.now(dt.timezone.utc).isoformat(),'selected':choice['selected'],'eligible_games':len(set(a['game_id'][primary])),'baseline':metrics(actual[primary],before[2][primary]),'context':metrics(actual[primary],after[2][primary]),'baseline_minutes':metrics(a['minutes'][primary],before[0][primary]),'context_minutes':metrics(a['minutes'][primary],after[0][primary]),'paired':paired_bootstrap(a,idx,actual,before[2],after[2],primary),'all_d1':{'baseline':metrics(actual[a['d1_matchup']],before[2][a['d1_matchup']]),'context':metrics(actual[a['d1_matchup']],after[2][a['d1_matchup']])},'sources':log,'skipped':skipped,'note':'Newly collected retrospective holdout. No target starts, positions, stats, DNP roster entries or target minutes are model inputs. Context uses history strictly before Nov 6. These predictions were not issued before the games.'}
    (out/'results.json').write_text(json.dumps(result,indent=2)+'\n');np.savez_compressed(out/'predictions.npz',actual=actual,before=before[2],after=after[2],eligible=primary)
    print('Fresh holdout baseline',result['baseline'],'context',result['context'],'paired',result['paired'],flush=True)


if __name__=='__main__':main()
