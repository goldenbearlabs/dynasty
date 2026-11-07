#!/usr/bin/env python3
"""Normalize and audit historical CBB player/team/schedule archives.

Target-game starts, minutes, box scores, scores and period counts are outcomes.
Polls, standings records, tournament seeds and end-season ratings are not inputs.
"""
import argparse
from collections import Counter,defaultdict
import datetime as dt
import hashlib
import json
import math
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

STATS=['pts','reb','ast','stl','blk','tov','tpm','fgm','fga','ftm','fta','dd','td','pts40','pts50']
FIELDS={'points':'pts','rebounds':'reb','assists':'ast','steals':'stl','blocks':'blk','turnovers':'tov',
        'three_point_field_goals_made':'tpm','three_point_field_goals_attempted':'tpa','field_goals_made':'fgm','field_goals_attempted':'fga',
        'free_throws_made':'ftm','free_throws_attempted':'fta','offensive_rebounds':'oreb','defensive_rebounds':'dreb','fouls':'pf'}
D1_CONFERENCES={1,62,3,2,46,8,4,5,6,7,9,10,11,45,12,13,14,16,18,44,19,20,21,22,23,24,25,26,49,27,30,29}
ELIGIBLE={8,23,7,2,4,44,3,21}
NY=dt.timezone(dt.timedelta(hours=-5))
from zoneinfo import ZoneInfo
EASTERN=ZoneInfo('America/New_York')


def number(value):
    if value is None:return None
    try:
        value=float(value)
        return value if math.isfinite(value) else None
    except (TypeError,ValueError):return None


def sports_day(timestamp):
    return (timestamp.astimezone(EASTERN)-dt.timedelta(hours=5)).date().isoformat()


def validate_stats(stats):
    if any(not math.isfinite(v) or v<0 for v in stats.values()):return False
    if abs(stats['pts']-(2*stats['fgm']+stats['tpm']+stats['ftm']))>.01:return False
    return stats['fgm']<=stats['fga'] and stats['tpm']<=stats['tpa']<=stats['fga'] and stats['tpm']<=stats['fgm'] and stats['ftm']<=stats['fta']


def normalize_season(folder,year,through):
    schedule=pq.read_table(folder/f'mbb_schedule_{year}.parquet').to_pylist()
    schedules={};audit=Counter();issues=[]
    for row in schedule:
        if row.get('status_type_completed') is not True or row.get('season_type') not in [2,3]:continue
        start=dt.datetime.fromisoformat(row['date'].replace('Z','+00:00'))
        if sports_day(start)>through:continue
        game=int(row['game_id']);schedules[game]=row
    team_cols=['game_id','team_id','opponent_team_id','team_score','field_goals_attempted','free_throws_attempted','offensive_rebounds','total_turnovers','turnovers']
    teams={(int(r['game_id']),int(r['team_id'])):r for r in pq.read_table(folder/f'team_box_{year}.parquet',columns=team_cols).to_pylist()}
    table=pq.read_table(folder/f'player_box_{year}.parquet')
    cols=['game_id','athlete_id','athlete_display_name','team_id','minutes','starter','did_not_play','athlete_position_abbreviation']+list(FIELDS)
    players=defaultdict(list)
    for row in table.select(cols).to_pylist():
        if row['game_id'] is not None:players[int(row['game_id'])].append(row)
    output=[];games=[]
    for game,s in sorted(schedules.items()):
        home,away=int(s['home_id']),int(s['away_id']);sides=[home,away]
        if any((game,t) not in teams for t in sides) or not players.get(game):audit['missing_boxscore_games']+=1;continue
        start=dt.datetime.fromisoformat(s['date'].replace('Z','+00:00'));day=sports_day(start)
        if s.get('neutral_site') is None:audit['unknown_neutral_site_games']+=1
        confs={home:int(number(s.get('home_conference_id')) or 0),away:int(number(s.get('away_conference_id')) or 0)}
        neutral=bool(s.get('neutral_site'));duration=40+5*max(0,int(number(s.get('status_period')) or 2)-2)
        game_rows=[];seen=set();bad=False
        for row in players[game]:
            if row.get('athlete_id') is None:audit['unidentified_player_rows']+=1;continue
            player=int(row['athlete_id']);team=int(row['team_id'])
            if team not in sides or player in seen:bad=True;audit['identity_conflict_rows']+=1;break
            seen.add(player);opponent=away if team==home else home
            stats={stat:number(row.get(name)) for name,stat in FIELDS.items()}
            minutes=number(row.get('minutes'));played=not bool(row.get('did_not_play')) and minutes is not None and minutes>0
            if played and (any(v is None for v in stats.values()) or not validate_stats(stats) or not 0<minutes<=duration+1):bad=True;audit['invalid_playing_rows']+=1;break
            if not played:
                audit['dnp_zero_or_unknown_minutes']+=1
                if minutes is None:audit['unknown_minutes_rows']+=1
                stats={k:float(v or 0) for k,v in stats.items()}
            tens=sum(stats.get(k,0)>=10 for k in ['pts','reb','ast','stl','blk'])
            stats.update(dd=float(tens>=2),td=float(tens>=3),pts40=float(stats['pts']>=40),pts50=float(stats['pts']>=50))
            result={'game_id':game,'player_id':player,'name':row['athlete_display_name'],'team':team,'opponent':opponent,'season':year,'starts_at':start.isoformat(),'day':day,
                    'position':row.get('athlete_position_abbreviation') or 'UNK','minutes':minutes or 0.,'played':played,'starter':bool(row.get('starter')),
                    'home':team==home,'neutral':neutral,'neutral_known':s.get('neutral_site') is not None,'conference':confs[team],'opponent_conference':confs[opponent],
                    'd1_matchup':all(c in D1_CONFERENCES for c in confs.values()),'eligible':confs[team] in ELIGIBLE,'season_type':int(s['season_type']),**stats}
            game_rows.append(result)
        if bad:issues.append({'game':game,'issue':'invalid player identity/stat line'});continue
        scores={t:float(number(teams[(game,t)]['team_score']) or 0) for t in sides}
        if any(abs(sum(r['pts'] for r in game_rows if r['team']==t)-scores[t])>.01 for t in sides):
            audit['player_team_score_mismatch_games']+=1;issues.append({'game':game,'issue':'player scores do not sum to team total'});continue
        game_teams=[]
        for team in sides:
            other=away if team==home else home;t=teams[(game,team)]
            values=[number(t.get(k)) for k in ['field_goals_attempted','free_throws_attempted','offensive_rebounds']]
            tov=number(t.get('total_turnovers'));tov=number(t.get('turnovers')) if tov is None else tov
            poss=values[0]+.475*values[1]-values[2]+tov if all(v is not None for v in values) and tov is not None else None
            game_teams.append({'team':team,'opponent':other,'score':scores[team],'possessions_raw':poss,'conference':confs[team]})
        if any(t['possessions_raw'] is None for t in game_teams):audit['missing_team_possession_inputs']+=1;continue
        possessions=sum(t['possessions_raw'] for t in game_teams)/2
        if not 25<possessions<180:audit['invalid_possessions_games']+=1;continue
        game_record={'game_id':game,'season':year,'starts_at':start.isoformat(),'day':day,'home':home,'away':away,'neutral':neutral,'neutral_known':s.get('neutral_site') is not None,
                     'conference_game':bool(s.get('conference_competition')),'season_type':int(s['season_type']),'duration':duration,'possessions':possessions,'pace':possessions*40/duration,'teams':game_teams,'d1_matchup':all(c in D1_CONFERENCES for c in confs.values())}
        if duration>40:audit['overtime_games']+=1
        for team in sides:
            minute_total=sum(r['minutes'] for r in game_rows if r['team']==team and r['played'])
            if abs(minute_total-5*duration)>5:audit['team_minute_sum_outside_rounding']+=1
        output.extend(game_rows);games.append(game_record);audit['retained_games']+=1;audit['retained_player_rows']+=len(game_rows);audit['positive_minute_appearances']+=sum(r['played'] for r in game_rows)
    audit['completed_scheduled_games']=len(schedules)
    return output,games,{'season':year,**dict(audit),'issues':issues}


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--raw',default='data/cbb/raw');p.add_argument('--out',default='data/cbb');p.add_argument('--seasons',default='2023,2024,2025,2026,2027');p.add_argument('--through',default='2026-11-05');args=p.parse_args()
    folder=Path(args.out);folder.mkdir(parents=True,exist_ok=True);audits=[];games=[];tables=[]
    for year in map(int,args.seasons.split(',')):
        rows,season_games,audit=normalize_season(Path(args.raw),year,args.through);audits.append(audit);games.extend(season_games);tables.append(pa.Table.from_pylist(rows));print(year,{k:v for k,v in audit.items() if k!='issues'},flush=True)
    table=pa.concat_tables(tables);indices=pa.compute.sort_indices(table,sort_keys=[('starts_at','ascending'),('game_id','ascending'),('player_id','ascending')]);table=table.take(indices)
    pq.write_table(table,folder/'history.parquet',compression='zstd')
    games.sort(key=lambda g:(g['starts_at'],g['game_id']));(folder/'games.json').write_text(json.dumps(games,separators=(',',':'))+'\n')
    manifest=json.loads((Path(args.raw)/'manifest.json').read_text())
    result={'through_sports_day':args.through,'seasons':audits,'games':len(games),'rows':table.num_rows,'source_manifest':manifest,'history_sha256':hashlib.sha256((folder/'history.parquet').read_bytes()).hexdigest(),'games_sha256':hashlib.sha256((folder/'games.json').read_bytes()).hexdigest(),
            'notes':['Exclusions are corruption/coverage checks, never forecast-error or outlier filtering.','DNP/unknown minutes retained but conditional-on-playing metrics exclude them.','Conference and neutral-site metadata come from each historical schedule; rankings, records, seeds and tournament outcome fields are not features.','A missing D1 conference id marks the matchup unverified, not necessarily a non-D1 opponent.']}
    (folder/'dataset-audit.json').write_text(json.dumps(result,indent=2)+'\n')


if __name__=='__main__':main()
