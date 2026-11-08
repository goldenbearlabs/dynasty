#!/usr/bin/env python3
"""Audit public official CBB availability archives without inventing timestamps.

Game dates and policy deadlines are NOT publication times. An archive row is
never eligible for historical features unless it has an explicit timestamp
before both tipoff and the forecast information cutoff.
"""
import argparse
import datetime as dt
from email.utils import parsedate_to_datetime
import hashlib
import json
from pathlib import Path
import re
import time
import urllib.request

ORIGIN='https://par.hdintelligence-app.com'
ARCHIVES={'sec':'ArCGiVsecmbb','acc':'ArCGiVcAcmbb','ncaa':'NCAAArchive','big12':'bIoArHIUembb'}


def eligible_timestamp(record,starts_at,information_cutoff):
    stamp=record.get('published_at') or record.get('observed_at')
    if not stamp:return False,'missing_publication_or_observation_timestamp'
    try:published=dt.datetime.fromisoformat(stamp.replace('Z','+00:00'))
    except (ValueError,TypeError):return False,'invalid_timestamp'
    if published.tzinfo is None:return False,'timezone_required'
    if published>=starts_at:return False,'report_not_before_tipoff'
    if published>information_cutoff:return False,'report_after_information_cutoff'
    return True,None


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--out',default='data/cbb/context');p.add_argument('--cached-only',action='store_true');args=p.parse_args();root=Path(args.out);raw=root/'raw';raw.mkdir(parents=True,exist_ok=True);captured=dt.datetime.now(dt.timezone.utc);entries=[];files=[]
    def fetch(url,path,headers=None):
        if not path.exists():
            if args.cached_only:raise ValueError('Missing cached source '+str(path))
            request=urllib.request.Request(url,headers={'User-Agent':'mosaic-cbb-research/1.0',**(headers or {})})
            with urllib.request.urlopen(request,timeout=60) as response:body=response.read()
            path.write_bytes(body);time.sleep(.3)
        body=path.read_bytes();files.append({'url':url,'sha256':hashlib.sha256(body).hexdigest(),'bytes':len(body)});return body
    home=fetch(ORIGIN+'?source=SECarchiveMBB',raw/'sec_hdi.html').decode();main_js=re.search(r'<script[^>]+src="([^"]+)"',home)[1];bundle=fetch(ORIGIN+main_js,raw/'sec_hdi.js').decode()
    for source,prefix in ARCHIVES.items():
        asset=re.search(r'\./('+re.escape(prefix)+r'-[^"\s]+\.js)',bundle)
        if not asset:continue
        script=fetch(ORIGIN+'/assets/'+asset[1],raw/asset[1]).decode();route=re.search(r'\.get\("([^\"]+)"',script)[1]
        # This is the public application's shared browser header. Keep it in
        # memory only; do not log it or include it in provenance/output.
        public_header=re.search(r'"x-api-key":"([^\"]+)"',script)[1]
        data=json.loads(fetch(ORIGIN+route,raw/('availability-'+prefix+'.json'),{'x-api-key':public_header}));rows=list(data.values()) if isinstance(data,dict) else data
        for record in rows:
            day=None
            if record.get('Week'):
                try:day=parsedate_to_datetime(record['Week']).date().isoformat()
                except (ValueError,TypeError):pass
            entries.append({'source':source,'team_name':record.get('Team'),'player_name':record.get('Player'),'opponent_name':record.get('Opponent'),'game_reference_day':day,'round':record.get('Round'),'initial_status':record.get('InitialStatus'),'gameday_status':record.get('GamedayStatus'),'published_at':None,'historically_usable':False,'excluded_reason':'archive_has_no_report_publication_timestamp'})
        print(source,'archive rows',len(rows),'timestamp-qualified',0,flush=True)
    result={'captured_at':captured.isoformat(),'records':entries,'sources':files,'audit':{'rows':len(entries),'timestamp_qualified_historical_rows':0,'excluded_missing_publication_timestamp':len(entries)},'note':'Capture time now is not a historical publication time. Initial/Gameday labels and nominal game dates do not prove when each historical report was available. No archive status is supplied to training.'}
    (root/'availability-audit.json').write_text(json.dumps(result,indent=2)+'\n')


if __name__=='__main__':main()
