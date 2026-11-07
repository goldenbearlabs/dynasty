#!/usr/bin/env python3
"""Download public SportsDataverse/ESPN CBB season archives with provenance.

Bulk releases avoid thousands of individual game requests. No application data
is modified. Normalization/auditing lives in cbb_dataset.py.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import datetime as dt
import hashlib
import json
from pathlib import Path
import time
import urllib.request

BASE='https://github.com/sportsdataverse/sportsdataverse-data/releases/download/'
FAMILIES={'players':('espn_mens_college_basketball_player_boxscores','player_box_'),
          'teams':('espn_mens_college_basketball_team_boxscores','team_box_'),
          'schedule':('espn_mens_college_basketball_schedules','mbb_schedule_')}


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--seasons',default='2023,2024,2025,2026,2027')
    p.add_argument('--out',default='data/cbb/raw')
    args=p.parse_args();folder=Path(args.out);folder.mkdir(parents=True,exist_ok=True)
    jobs=[(int(year),name,tag,prefix) for year in args.seasons.split(',') for name,(tag,prefix) in FAMILIES.items()]
    def download(job):
        year,name,tag,prefix=job;filename=f'{prefix}{year}.parquet';path=folder/filename;url=BASE+tag+'/'+filename
        observed=None
        if not path.exists():
            request=urllib.request.Request(url,headers={'User-Agent':'mosaic-cbb-model/1.0'})
            for attempt in range(3):
                try:
                    with urllib.request.urlopen(request,timeout=90) as response:body=response.read()
                    if not body.startswith(b'PAR1') or not body.endswith(b'PAR1'):raise ValueError('invalid parquet: '+url)
                    temporary=path.with_suffix('.tmp');temporary.write_bytes(body);temporary.replace(path)
                    observed=dt.datetime.now(dt.timezone.utc).isoformat();break
                except Exception:
                    if attempt==2:raise
                    time.sleep(2**attempt)
        body=path.read_bytes()
        print(year,name,len(body),'bytes',flush=True)
        return {'season':year,'family':name,'file':str(path),'url':url,'sha256':hashlib.sha256(body).hexdigest(),'bytes':len(body),'downloaded_at':observed,'source':'SportsDataverse bulk archive of ESPN observations; revisions may occur'}
    with ThreadPoolExecutor(max_workers=3) as workers:entries=list(workers.map(download,jobs))
    (folder/'manifest.json').write_text(json.dumps({'captured_at':dt.datetime.now(dt.timezone.utc).isoformat(),'files':entries},indent=2)+'\n')


if __name__=='__main__':main()
