#!/usr/bin/env python3
"""Create an interactive CBB forecast review using the shared basketball view."""
import argparse
import datetime as dt
import json
from pathlib import Path
from zoneinfo import ZoneInfo
from nba_report import HTML


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--predictions',required=True);p.add_argument('--baseline-errors',required=True);p.add_argument('--out',required=True);args=p.parse_args();baseline={}
    for line in open(args.baseline_errors):
        row=json.loads(line);baseline[(row['game'],row['player'])]=row
    rows=[]
    for line in open(args.predictions):
        item=json.loads(line);target=item['target'];before=baseline[(target['game_id'],target['player_id'])]
        if before['actual']!=item['actual_points']:raise ValueError('Baseline outcomes differ')
        day=(dt.datetime.fromisoformat(target['starts_at'].replace('Z','+00:00')).astimezone(ZoneInfo('America/New_York'))-dt.timedelta(hours=5)).date().isoformat()
        rows.append({'game':target['game_id'],'date':target['starts_at'],'day':day,'player':item['name'],'player_id':target['player_id'],'position':item['features']['position'],'actual':item['actual_points'],'model':item['prediction']['fantasy_points_if_active'],'proxy':item['proxy'],'proxy_if_active':item['proxy_if_active'],'recent_10':item['recent_10'],'minutes':item['prediction']['minutes'],'actual_minutes':item['actual_minutes'],'stats':item['prediction']['stats'],'actual_stats':item['actual_stats'],'baseline_model':before['predicted']})
    if not rows:raise ValueError('No review rows')
    payload=json.dumps(rows,separators=(',',':')).replace('</','<\\/')
    text=HTML.replace('NBA projection review','CBB projection review').replace('__DATA__',payload).replace('__BASELINE_OPTION__','<option value="baseline_model">Player-only model</option>')
    text=text.replace('Compare pregame projections with actual production.','Eligible-conference, verified D1 matchups. Compare pregame projections with actual production.').replace('The proxy is reconstructed without archived pregame odds or injury snapshots.','The conditional proxy uses the application’s two-season weights without archived odds or injuries. The attendance-discount variant uses an approximate season length.')
    Path(args.out).write_text(text);print('CBB review:',len(rows),'player-games',flush=True)


if __name__=='__main__':main()
