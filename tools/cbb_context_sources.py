#!/usr/bin/env python3
"""Fetch historical pre-college recruiting context and public availability.

Public browser headers are discovered from official linked archive components;
never stored in the normalized output or source. No accounts are created.
"""
import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import time
import urllib.request
import unicodedata

CORE='https://sports.core.api.espn.com/v2/sports/basketball/leagues/mens-college-basketball'


def normalized(value):
    value=''.join(c for c in unicodedata.normalize('NFKD',value) if not unicodedata.combining(c))
    return ''.join(p for p in re.findall('[a-z0-9]+',value.lower()) if p not in ['jr','sr','ii','iii','iv'])


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--out',default='data/cbb/context');p.add_argument('--classes',default='2018,2019,2020,2021,2022,2023,2024,2025,2026');p.add_argument('--recruits-only',action='store_true');args=p.parse_args();root=Path(args.out);raw=root/'raw';raw.mkdir(parents=True,exist_ok=True);manifest=[]
    def fetch(url,path,headers=None):
        if not path.exists():
            request=urllib.request.Request(url,headers={'User-Agent':'mosaic-cbb-research/1.0',**(headers or {})})
            with urllib.request.urlopen(request,timeout=60) as response:body=response.read()
            temporary=path.with_suffix('.tmp');temporary.write_bytes(body);temporary.replace(path);time.sleep(.3)
        body=path.read_bytes();manifest.append({'url':url,'file':str(path),'bytes':len(body),'sha256':hashlib.sha256(body).hexdigest()});return body
    recruits=[]
    for year in map(int,args.classes.split(',')):
        page=1;pages=1;before=len(recruits)
        while page<=pages:
            url=f'{CORE}/recruiting/{year}/athletes?limit=500&page={page}';d=json.loads(fetch(url,raw/f'recruits-{year}-{page}.json'));pages=d['pageCount']
            for item in d.get('items',[]):
                athlete=item.get('athlete',{});alternate=athlete.get('alternateId');attributes={v.get('name'):v.get('value') for v in item.get('attributes',[])}
                recruits.append({'player_id':int(alternate) if alternate else None,'recruit_id':str(athlete['id']),'class_year':year,'name':athlete.get('fullName',athlete.get('displayName','')),'normalized_name':normalized(athlete.get('fullName',athlete.get('displayName',''))),
                    'grade':item.get('grade'),'national_rank':attributes.get('rank'),'position_rank':attributes.get('positionRank'),'height_inches':athlete.get('height'),'weight_pounds':athlete.get('weight'),'position':athlete.get('position',{}).get('abbreviation'),'signed_team_ids':[int(re.search(r'/teams/(\d+)',school['team']['$ref'])[1]) for school in item.get('schools',[]) if school.get('status',{}).get('id')==3 and re.search(r'/teams/(\d+)',school.get('team',{}).get('$ref',''))],'source_url':url})
            page+=1
        print('Recruit class',year,'linked',len(recruits)-before,flush=True)
    (root/'recruiting.json').write_text(json.dumps({'records':recruits,'notes':['Explicit ESPN alternateId supplies the college athlete id; names must also validate at join time.','Only classes strictly before the ending season can be used. Missing/unranked is not a zero rating.','Class grades/ranks describe pre-college recruiting, but these responses were downloaded now and are not original historical HTTP captures. No signed-school destination is used.']},indent=2)+'\n')
    (root/'sources.json').write_text(json.dumps({'captured_at':dt.datetime.now(dt.timezone.utc).isoformat(),'files':manifest},indent=2)+'\n')


if __name__=='__main__':main()
