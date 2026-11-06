#!/usr/bin/env python3
"""Recover official, timestamped NBA injury PDFs before each historical slate.

One snapshot per slate, selected around 5pm or before its earliest tip.
The timestamp PRINTED INSIDE each PDF is authoritative, not its filename.
Missing/not-submitted teams and unmatched identities remain explicitly unknown.
"""
import argparse
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import subprocess
import threading
import time
import unicodedata
import urllib.error
import urllib.request
from zoneinfo import ZoneInfo

NY=ZoneInfo('America/New_York')
LOCK=threading.Lock()
NEXT=0.0
BASE='https://ak-static.cms.nba.com/referee/injury/'


def normalized(name):
    text=''.join(c for c in unicodedata.normalize('NFKD',name) if not unicodedata.combining(c))
    parts=re.findall(r'[a-z0-9]+',text.lower())
    return ''.join(p for p in parts if p not in {'jr','sr','ii','iii','iv'})


def download(moment,cache):
    global NEXT
    date=moment.strftime('%Y-%m-%d');clock=moment.strftime('%I_%M%p');old=moment.strftime('%I%p') if moment.minute==0 else clock
    names=[f'Injury-Report_{date}_{clock}.pdf',f'Injury-Report_{date}_{old}.pdf']
    if moment.date()<dt.date(2026,1,1):names.reverse()
    for name in dict.fromkeys(names):
        path=cache/name
        if path.exists():return path,BASE+name
        if path.with_suffix('.unavailable.json').exists():return None,None
        with LOCK:
            delay=max(0,NEXT-time.monotonic());NEXT=max(NEXT,time.monotonic())+.25
        time.sleep(delay)
        try:
            request=urllib.request.Request(BASE+name,headers={'User-Agent':'crossover-dynasty/0.1'})
            with urllib.request.urlopen(request,timeout=30) as response:body=response.read()
            if not body.startswith(b'%PDF'):raise ValueError('non-PDF report: '+name)
            temporary=path.with_suffix('.tmp');temporary.write_bytes(body);temporary.replace(path)
            return path,BASE+name
        except urllib.error.HTTPError as error:
            # Missing archive objects may return S3 AccessDenied. Keep them
            # unavailable; do not retry a denied URL or manufacture coverage.
            if error.code==403:
                path.with_suffix('.unavailable.json').write_text(json.dumps({'url':BASE+name,'http_status':403}))
                return None,None
            if error.code not in [404,410]:raise
    return None,None


def parse_report(text,teams):
    header=re.search(r'Injury Report:\s*(\d{2}/\d{2}/\d{2,4})\s+(\d{1,2}:\d{2}\s*[AP]M)',text)
    if not header:raise ValueError('missing report timestamp')
    stamp=header[1]+' '+header[2]
    pattern='%m/%d/%Y %I:%M %p' if len(header[1].split('/')[-1])==4 else '%m/%d/%y %I:%M %p'
    published=dt.datetime.strptime(stamp,pattern).replace(tzinfo=NY)
    records={};day=None;match=None;team=None;last=None
    names=sorted(teams,key=len,reverse=True)
    for line in text.splitlines():
        line=' '.join(line.split())
        if not line or line.startswith(('Injury Report:','Game Date','Page ')):continue
        date=re.match(r'(\d{2}/\d{2}/\d{4})\s*',line)
        if date:day=dt.datetime.strptime(date[1],'%m/%d/%Y').date().isoformat();line=line[date.end():]
        line=re.sub(r'^\d{1,2}:\d{2}\s*\(ET\)\s*','',line)
        matchup=re.match(r'([A-Z]{2,3}@[A-Z]{2,3})\s*',line)
        if matchup:match=matchup[1];line=line[matchup.end():];team=None;last=None
        for name in names:
            if line==name or line.startswith(name+' '):
                team=teams[name];line=line[len(name):].strip();last=None
                records[(day,match,team)]={'submitted':True,'players':[]}
                break
        if team is None or day is None or match is None:continue
        record=records[(day,match,team)]
        if 'NOT YET SUBMITTED' in line.upper():record['submitted']=False;continue
        player=re.match(r'(.+?),\s+(.+?)\s+(Out|Doubtful|Questionable|Probable|Available)\b\s*(.*)',line)
        if player:
            last={'name':player[2]+' '+player[1],'status':player[3],'reason':player[4]};record['players'].append(last)
        elif last is not None and line:last['reason']+=' '+line
    return published,records


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data',default='data/nba/history-enriched.jsonl')
    parser.add_argument('--cache',default='data/nba/injury-pdfs')
    parser.add_argument('--out',default='data/nba/pregame-injuries.json')
    parser.add_argument('--extractor',default='/tmp/cbb-nba-pdftext')
    parser.add_argument('--scheduled-scoreboard',help='optional prospective scoreboard capture to include future games')
    parser.add_argument('--observed-at',help='required UTC/offset timestamp of that capture; never fetch information published later')
    parser.add_argument('--latest',action='store_true',help='prospective: use recent quarter-hour reports instead of the conservative historical snapshot')
    parser.add_argument('--identity-catalog',help='optional current roster identities; JSON list with name and player_id')
    parser.add_argument('--only-scheduled',action='store_true',help='use history for identity matching but fetch reports only for the captured future schedule')
    args=parser.parse_args();cache=Path(args.cache);cache.mkdir(parents=True,exist_ok=True)
    if args.only_scheduled and not args.scheduled_scoreboard:parser.error('--only-scheduled needs --scheduled-scoreboard')
    games={};catalog=defaultdict(set);initials=defaultdict(set)
    with open(args.data) as source:
        for line in source:
            row=json.loads(line)
            if not args.only_scheduled:games.setdefault(row['game_id'],row)
            catalog[normalized(row['name'])].add(row['player_id'])
            words=row['name'].split()
            if words:initials[(normalized(words[-1]),words[0][0].lower())].add(row['player_id'])
    if args.identity_catalog:
        for row in json.loads(Path(args.identity_catalog).read_text()):
            catalog[normalized(row['name'])].add(str(row['player_id']))
    if args.latest and not args.scheduled_scoreboard:parser.error('--latest requires a prospective captured schedule')
    observed=None
    if args.scheduled_scoreboard:
        if not args.observed_at:parser.error('a prospective schedule needs --observed-at')
        observed=dt.datetime.fromisoformat(args.observed_at.replace('Z','+00:00'))
        if observed.tzinfo is None:parser.error('--observed-at needs a timezone')
        payload=json.loads(Path(args.scheduled_scoreboard).read_text())
        for event in payload.get('events',[]):
            starts=dt.datetime.fromisoformat(event['date'].replace('Z','+00:00'))
            if starts<=observed or event.get('status',{}).get('type',{}).get('state')!='pre':continue
            sides=event.get('competitions',[{}])[0].get('competitors',[])
            if len(sides)!=2:continue
            games[str(event['id'])]={'starts_at':starts.isoformat(),'team':str(sides[0]['team']['id']),'opponent':str(sides[1]['team']['id'])}
    teams={}
    for path in Path('data/nba/raw').glob('schedule-*.json'):
        payload=json.loads(path.read_text())
        for event in payload.get('events',[]):
            for competition in event.get('competitions',[]):
                for side in competition.get('competitors',[]):
                    t=side.get('team',{})
                    if t.get('displayName'):teams[t['displayName']]=str(t['id'])
    for alias,canonical in {'LA Clippers':'Los Angeles Clippers','LA Lakers':'Los Angeles Lakers'}.items():
        if canonical in teams:teams[alias]=teams[canonical]
    slates=defaultdict(list)
    for game,row in games.items():slates[dt.datetime.fromisoformat(row['starts_at']).astimezone(NY).date()].append((game,row))
    def fetch_slate(item):
        day,rows=item
        first=min(dt.datetime.fromisoformat(r['starts_at']).astimezone(NY) for _,r in rows)
        cutoff=min(first-dt.timedelta(hours=1),dt.datetime.combine(day,dt.time(17),NY)).replace(minute=0,second=0,microsecond=0)
        if observed is not None:cutoff=min(cutoff,(observed.astimezone(NY)-dt.timedelta(hours=1)).replace(minute=0,second=0,microsecond=0))
        if args.latest:
            cutoff=min(first-dt.timedelta(minutes=15),observed.astimezone(NY)-dt.timedelta(minutes=15))
            cutoff=cutoff.replace(minute=(cutoff.minute//15)*15,second=0,microsecond=0)
        moments=[cutoff-dt.timedelta(hours=hours) for hours in range(4)]
        if args.latest:
            # Morning feeds can have no report in the last two hours. Search
            # farther back for the newest available official snapshot.
            moments=[cutoff-dt.timedelta(minutes=15*i) for i in range(8)]
            hour=cutoff.replace(minute=0)
            moments+= [hour-dt.timedelta(hours=i) for i in range(2,25)]
        for moment in dict.fromkeys(moments):
            path,url=download(moment,cache)
            if path:return day,rows,moment,path,url
        return day,rows,None,None,None
    fetched=[]
    with ThreadPoolExecutor(max_workers=8) as executor:
        for i,item in enumerate(executor.map(fetch_slate,sorted(slates.items())),1):
            fetched.append(item)
            if i%50==0:print(f'Downloaded {i}/{len(slates)} slate snapshots',flush=True)
    paths=[str(item[3]) for item in fetched if item[3] and not Path(str(item[3])+'.txt').exists()]
    for start in range(0,len(paths),60):subprocess.run([args.extractor]+paths[start:start+60],check=True)
    output={};audit={'slates':len(slates),'snapshots_found':0,'missing_slates':[],'unmatched':[],'matches':0,'ambiguous':0,'timestamp_aliases':0,'invalid_timestamps':[]}
    for day,rows,moment,path,url in fetched:
        if not path:audit['missing_slates'].append(str(day));continue
        published,records=parse_report(Path(str(path)+'.txt').read_text(),teams)
        first=min(dt.datetime.fromisoformat(r['starts_at']) for _,r in rows)
        if published.date()!=moment.date() or published>=first or (observed is not None and published>observed):
            audit['invalid_timestamps'].append({'file':str(path),'published_at':published.isoformat()});continue
        audit['timestamp_aliases']+=int(published!=moment)
        audit['snapshots_found']+=1
        for game,row in rows:
            start=dt.datetime.fromisoformat(row['starts_at'])
            if published>=start:raise ValueError('non-pregame snapshot')
            sides={}
            for team in [row['team'],row['opponent']]:
                matches=[v for (d,_,t),v in records.items() if d==str(day) and t==team]
                if len(matches)!=1:sides[team]={'submitted':False,'players':[]};continue
                side={'submitted':matches[0]['submitted'],'players':[]}
                for player in matches[0]['players']:
                    ids=catalog.get(normalized(player['name']),set());method='exact_normalized'
                    if not ids:
                        words=player['name'].split();ids=initials.get((normalized(words[-1]),words[0][0].lower()),set()) if words else set();method='unique_family_initial'
                    if len(ids)!=1:
                        audit['unmatched'].append({'game':game,'team':team,**player});audit['ambiguous']+=int(len(ids)>1);continue
                    audit['matches']+=1;side['players'].append(dict(player,player_id=next(iter(ids)),match_method=method))
                sides[team]=side
            output[game]={'published_at':published.isoformat(),'starts_at':row['starts_at'],'url':url,'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'teams':sides}
    result={'reports':output,'audit':audit,'note':'One snapshot per slate, strictly before its first game; unsubmitted/missing teams and unmatched names are unknown, never healthy.'}
    path=Path(args.out);path.write_text(json.dumps(result,indent=2)+'\n')
    print('Saved',len(output),'game reports;',audit['snapshots_found'],'slates;',len(audit['missing_slates']),'missing slates;',len(audit['unmatched']),'unmatched entries',flush=True)


if __name__=='__main__':main()
