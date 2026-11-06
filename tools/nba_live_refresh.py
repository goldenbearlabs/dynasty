#!/usr/bin/env python3
"""Refresh NBA model inputs and publish prospective forecasts; never refit.

Run from the repository root. Uses a process lock and atomic snapshot publish;
failed feeds leave the last valid forecast file in place.
"""
import argparse
import datetime as dt
import fcntl
import json
import gzip
import shutil
from pathlib import Path
import subprocess
import time
import urllib.request
from zoneinfo import ZoneInfo

from nba_history import parse_boxscore

BASE='https://site.api.espn.com/apis/site/v2/sports/basketball/nba'
NY=ZoneInfo('America/New_York')


def atomic_json(path,value):
    temp=path.with_suffix('.tmp');temp.write_text(json.dumps(value,separators=(',',':'))+'\n');temp.replace(path)


def merge_history(old,new):
    """Replace corrected games as a unit, retaining all older history."""
    replaced={r['game_id'] for r in new}
    rows=[r for r in old if r['game_id'] not in replaced]+new
    rows.sort(key=lambda r:(r['starts_at'],r['game_id'],r['player_id']))
    if len({(r['game_id'],r['player_id']) for r in rows})!=len(rows):raise ValueError('duplicate player/game')
    return rows


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',default='backend/nba-refresh')
    parser.add_argument('--extractor',default='tools/nba_pdf_text.py')
    parser.add_argument('--horizon-days',type=int,default=14)
    args=parser.parse_args()
    root=Path('data/nba');root.mkdir(parents=True,exist_ok=True)
    with (root/'refresh.lock').open('w') as lock:
        try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:print('Another NBA refresh is running',flush=True);return
        refresh(args,root)


def refresh(args,root):
    (root/'raw').mkdir(parents=True,exist_ok=True)
    started=dt.datetime.now(dt.timezone.utc);today=started.astimezone(NY).date()
    folder=root/'live-snapshots'/started.strftime('%Y%m%dT%H%M%S%fZ');folder.mkdir(parents=True)
    captures=[]
    def fetch(name,url):
        request=urllib.request.Request(url,headers={'User-Agent':'crossover-dynasty/0.1'})
        with urllib.request.urlopen(request,timeout=40) as response:
            raw=response.read();server_date=response.headers.get('Date')
        observed=dt.datetime.now(dt.timezone.utc)
        (folder/(name+'.json')).write_bytes(raw)
        captures.append({'name':name,'url':url,'observed_at':observed.isoformat(),'http_date':server_date})
        time.sleep(.25)
        return json.loads(raw)
    schedule=[];completed=set()
    for offset in range(-3,args.horizon_days+1):
        day=today+dt.timedelta(days=offset);stamp=day.strftime('%Y%m%d')
        payload=fetch('schedule-'+stamp,BASE+'/scoreboard?dates='+stamp)
        atomic_json(root/'raw'/('schedule-'+stamp+'.json'),payload)
        for event in payload.get('events',[]):
            c=event.get('competitions',[{}])[0]
            if event.get('season',{}).get('type')!=2 or c.get('type',{}).get('abbreviation')!='STD':continue
            if event.get('status',{}).get('type',{}).get('completed'):
                if offset<=0:completed.add(str(event['id']))
            elif event.get('status',{}).get('type',{}).get('state')=='pre':schedule.append(event)
    old=[json.loads(line) for line in (root/'history-enriched.jsonl').read_text().splitlines()]
    new=[]
    for game in sorted(completed):
        payload=fetch('game-'+game,BASE+'/summary?event='+game)
        rows=parse_boxscore(payload)
        if not rows or any(r['game_id']!=game for r in rows):raise ValueError('incomplete boxscore '+game)
        new.extend(rows)
    merged=merge_history(old,new)
    temporary=root/'history-enriched.tmp'
    with temporary.open('w') as out:
        for row in merged:out.write(json.dumps(row,separators=(',',':'))+'\n')
    temporary.replace(root/'history-enriched.jsonl')
    teams=fetch('teams',BASE+'/teams?limit=100')['sports'][0]['leagues'][0]['teams']
    roster={};identities=[]
    for item in teams:
        team=str(item['team']['id']);payload=fetch('roster-'+team,BASE+'/teams/'+team+'/roster')
        players=payload.get('athletes',[])
        if not players:raise ValueError('empty team roster '+team)
        roster[team]=[]
        for person in players:
            player=str(person['id']);roster[team].append(player);identities.append({'player_id':player,'name':person['displayName']})
    atomic_json(folder/'identities.json',identities)
    observed=dt.datetime.now(dt.timezone.utc);targets=[];injury_events=[]
    for event in schedule:
        start=dt.datetime.fromisoformat(event['date'].replace('Z','+00:00'))
        if start<=observed:continue
        sides=event['competitions'][0]['competitors']
        if len(sides)!=2:raise ValueError('invalid matchup')
        if start.astimezone(NY).date()<=today+dt.timedelta(days=1):injury_events.append(event)
        for side in sides:
            team=str(side['team']['id']);other=next(str(s['team']['id']) for s in sides if s is not side)
            for player in roster[team]:targets.append({'game_id':str(event['id']),'starts_at':start.isoformat(),'season':int(event['season']['year']),'player_id':player,'team':team,'opponent':other,'home':side['homeAway']=='home'})
    atomic_json(folder/'targets.json',{'observed_at':observed.isoformat(),'targets':targets})
    atomic_json(folder/'injury-scoreboard.json',{'events':injury_events})
    # Official reports can be absent before teams submit. A missing report stays
    # unknown; transport/parsing failures abort instead of publishing partial data.
    subprocess.run(['python3','tools/nba_injury_history.py','--scheduled-scoreboard',str(folder/'injury-scoreboard.json'),'--observed-at',observed.isoformat(),'--only-scheduled','--latest','--identity-catalog',str(folder/'identities.json'),'--extractor',args.extractor,'--out',str(folder/'injuries.json')],check=True)
    atomic_json(folder/'manifest.json',{'started_at':started.isoformat(),'observed_at':observed.isoformat(),'captures':captures,'targets':len(targets),'completed_games_refreshed':len(completed)})
    subprocess.run([args.binary,'--model',str(root/'model.json'),'--data',str(root/'history-enriched.jsonl'),'--targets',str(folder/'targets.json'),'--availability',str(folder/'injuries.json'),'--out',str(root/'live-projections.json')],check=True)
    archive=root/'forecast-archive';archive.mkdir(exist_ok=True)
    with (root/'live-projections.json').open('rb') as source,gzip.open(archive/(started.strftime('%Y%m%dT%H%M%SZ')+'.json.gz'),'wb') as target:
        shutil.copyfileobj(source,target)
    # Keep a week of bulky feed payloads and 90 days of compressed prospective
    # forecasts. This bounds unattended disk usage while retaining an audit set.
    for child in (root/'live-snapshots').iterdir():
        if child.is_dir() and child.stat().st_mtime < time.time()-7*86400:shutil.rmtree(child)
    for child in archive.glob('*.json.gz'):
        if child.stat().st_mtime < time.time()-90*86400:child.unlink()
    atomic_json(root/'last-refresh.json',{'completed_at':dt.datetime.now(dt.timezone.utc).isoformat(),'capture':str(folder),'targets':len(targets),'history_rows':len(merged),'model':'validated coefficients; no automatic retraining'})
    print('NBA refresh completed',flush=True)


if __name__=='__main__':main()
