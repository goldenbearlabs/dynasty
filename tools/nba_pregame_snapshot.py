#!/usr/bin/env python3
"""Capture current ESPN NBA injuries/schedule and pregame summaries prospectively.

Every observation is timestamped when fetched. Starter flags in a summary are
preserved as raw source data, never labelled confirmed by this collector.
"""
import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
import time
import urllib.request

BASE='https://site.api.espn.com/apis/site/v2/sports/basketball/nba'


def pregame_events(payload,observed_at):
    selected=[]
    for event in payload.get('events',[]):
        starts=dt.datetime.fromisoformat(event['date'].replace('Z','+00:00'))
        state=event.get('status',{}).get('type',{}).get('state')
        if state=='pre' and starts>observed_at:selected.append((str(event['id']),starts.isoformat()))
    return selected


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--date',required=True,help='scheduled game date YYYY-MM-DD')
    parser.add_argument('--out',default='data/nba/live-snapshots')
    parser.add_argument('--summaries',action='store_true')
    args=parser.parse_args();day=dt.date.fromisoformat(args.date)
    started=dt.datetime.now(dt.timezone.utc)
    folder=Path(args.out)/started.strftime('%Y%m%dT%H%M%S%fZ');folder.mkdir(parents=True)
    captured=[]
    def fetch(name,url):
        requested=dt.datetime.now(dt.timezone.utc)
        request=urllib.request.Request(url,headers={'User-Agent':'crossover-dynasty/0.1'})
        with urllib.request.urlopen(request,timeout=30) as response:
            body=response.read();server_date=response.headers.get('Date')
        received=dt.datetime.now(dt.timezone.utc)
        path=folder/(name+'.json');path.write_bytes(body)
        captured.append({'file':path.name,'url':url,'requested_at':requested.isoformat(),'observed_at':received.isoformat(),
                         'http_date':server_date,'sha256':hashlib.sha256(body).hexdigest()})
        time.sleep(.25)
        return json.loads(body),received
    schedule,observed=fetch('scoreboard',BASE+'/scoreboard?dates='+day.strftime('%Y%m%d'))
    fetch('injuries',BASE+'/injuries')
    events=pregame_events(schedule,observed)
    if args.summaries:
        for game,starts in events:
            if dt.datetime.fromisoformat(starts)<=dt.datetime.now(dt.timezone.utc):continue
            fetch('summary-'+game,BASE+'/summary?event='+game)
    manifest={'requested_game_date':args.date,'capture_started_at':started.isoformat(),'captures':captured,
              'pregame_event_ids':[g for g,_ in events],
              'notes':['Current observations only; these files must not be backdated into historical forecasts.',
                       'All injury statuses are preserved, including questionable/probable; no status is inferred from eventual participation.',
                       'Raw summary starter flags are not asserted to be confirmed starting lineups.']}
    (folder/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print('Saved prospective snapshot:',folder,'pregame events:',len(events))


if __name__=='__main__':main()
