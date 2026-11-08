#!/usr/bin/env python3
"""Additional pregame recruiting, transfer and observed-roster context.

Recruiting attributes describe completed high-school classes. Roster/depth
features use only closed prior games, including their DNP roster entries.
Final season rosters, target participants and target starts are never inputs.
"""
import argparse
from collections import defaultdict
import hashlib
import json
import math
from pathlib import Path

import numpy as np
from cbb_features import Dataset,NAMES as BASE_NAMES
from cbb_context_sources import normalized
from cbb_strength import day_number

GROUPS={
 'recruiting':['recruit_known','recruit_grade','recruit_log_rank','recruit_top10','recruit_top25','recruit_top100','recruit_years_since_class','recruit_height','recruit_guard','recruit_forward','recruit_center','recruit_grade_new_history','recruit_grade_new_season','recruit_rank_new_history','incoming_top100','incoming_grade_sum','own_vs_incoming_grade'],
 'transfer_roles':['persistent_transfer','old_team_minutes','old_team_points_per40','old_team_usage_per40','old_team_start_share','current_team_games','current_team_minutes','current_team_vs_old_minutes','old_team_exposure_share','previous_college_seasons','new_team_no_appearances','prior_minutes_new_season','prior_points_new_season'],
 'observed_depth':['current_roster_known','own_prior_roster_known','observed_roster_size','returning_minutes_share','incoming_transfer_minutes','expected_depth_minutes','own_prior_minutes_rank','own_prior_usage_share','position_competition','current_team_start_share','roster_position_known','roster_guard','roster_forward','roster_center'],
 'participation':['team_season_games','own_team_appearance_share','team_games_since_last_play','own_missed_last3','prior_teammate_missing_minutes','current_role_new_season','calendar_opening','calendar_days_into_season'],
}
NAMES=sum(GROUPS.values(),[])


def position(value):return {'G':1,'PG':1,'SG':1,'F':2,'SF':2,'PF':2,'C':3}.get(value,0)


class Recruiting:
    def __init__(self,path,names):
        self.players=defaultdict(list);self.teams=defaultdict(list);self.audit=defaultdict(int)
        self.records=json.load(open(path))['records']
        for record in self.records:
            player=record.get('player_id');grade=float(record.get('grade') or 0);rank=float(record.get('national_rank') or 0)
            for team in record.get('signed_team_ids',[]):self.teams[(int(record['class_year']),int(team))].append(record)
            if player is None:self.audit['no_college_id']+=1;continue
            if player not in names:self.audit['outside_identity_catalog']+=1;continue
            if record['normalized_name'] not in names[player]:self.audit['name_conflict']+=1;continue
            self.players[int(player)].append(record);self.audit['validated_records']+=1
        for values in self.players.values():values.sort(key=lambda r:r['class_year'])

    def get(self,player,season,first_college_season=None):
        limit=min(season,first_college_season or season)
        candidates=[r for r in self.players.get(int(player),[]) if int(r['class_year'])<limit]
        return candidates[-1] if candidates else None

    def features(self,player,team,season,n_history,n_season,first_college_season=None):
        r=self.get(player,season,first_college_season);grade=float(r.get('grade') or 0) if r else 0;rank=float(r.get('national_rank') or 0) if r else 0;known=bool(r and (grade>0 or rank>0));g=(grade-80)/20 if grade>0 else 0
        rank_value=1/math.log2(rank+1) if rank>0 else 0;pos=position(r.get('position')) if r else 0;height=float(r.get('height_inches') or 0) if r else 0
        incoming=self.teams.get((season-1,int(team)),[]);top=sum(0<float(v.get('national_rank') or 0)<=100 for v in incoming);grade_sum=sum(max(0,float(v.get('grade') or 0)-80) for v in incoming)/100
        strongest=max([float(v.get('grade') or 0) for v in incoming]+[0]);relative=(grade-strongest)/20 if grade>0 and strongest>0 else 0
        return [float(known),g,rank_value,float(0<rank<=10),float(0<rank<=25),float(0<rank<=100),min(8,season-1-int(r['class_year']))/4 if r else 0,(height-75)/10 if 60<=height<=100 else 0,float(pos==1),float(pos==2),float(pos==3),g/(1+n_history/5),g/(1+n_season),rank_value/(1+n_history/5),top/5,grade_sum,relative]


class ContextState:
    def __init__(self,data,recruits):
        self.data=data;self.recruits=recruits;self.history=defaultdict(list);self.roster_history=defaultdict(list);self.team_games=defaultdict(list);self.any_history=defaultdict(list);self.cache={};self.first_season={};self.team_minutes=defaultdict(float);self.closed_summaries={}

    def histories(self,player,cutoff):
        return [i for i in self.history.get(int(player),[]) if self.data.a['day'][i]<cutoff]

    def summary(self,player,season,cutoff,team=None):
        closed_key=(int(player),int(season))
        if team is None and closed_key in self.closed_summaries:return self.closed_summaries[closed_key]
        key=(int(player),int(season),int(cutoff),team)
        if key not in self.cache:
            a=self.data.a;h=[i for i in self.histories(player,cutoff) if int(a['season'][i])==season and (team is None or int(a['team'][i])==team)]
            if h:
                minutes=float(a['minutes'][h].sum());points=float(self.data.stats[h,0].sum());fga=float(self.data.stats[h,8].sum());fta=float(self.data.stats[h,10].sum());tov=float(self.data.stats[h,5].sum())
                teams=defaultdict(float)
                for i in h:teams[int(a['team'][i])]+=float(a['minutes'][i])
                dominant=max(teams,key=teams.get);result=(len(h),minutes/len(h),points/max(1,minutes)*40,(fga+.475*fta+tov)/max(1,minutes)*40,float(a['starter'][h].mean()),dominant,minutes)
            else:result=(0,0.,0.,0.,0.,0,0.)
            self.cache[key]=result
            if team is None:self.closed_summaries[closed_key]=result
        return self.cache[key]

    def roster(self,team,season,cutoff):
        last=next((v for v in reversed(self.roster_history.get(int(team),[])) if v[0]<cutoff),None)
        return last[2] if last and last[1]==season else {}

    def build(self,index,cutoff,target=None):
        a=self.data.a
        if target is None:player,team,season=(int(a[k][index]) for k in ['player_id','team','season'])
        else:player,team,season=(int(target[k]) for k in ['player_id','team','season'])
        h=self.histories(player,cutoff);current=[i for i in h if int(a['season'][i])==season];same=[i for i in current if int(a['team'][i])==team]
        previous=self.summary(player,season-1,cutoff);now=self.summary(player,season,cutoff,team);transfer=previous[0]>0 and previous[5]!=team
        old_share=sum(int(a['team'][i])!=team for i in h[-10:])/min(10,len(h)) if h else 0;seasons=len({int(a['season'][i]) for i in h});new_weight=1/(1+len(same))
        first=self.first_season.get(player);recruit=self.recruits.features(player,team,season,len(h),len(same),first)
        roles=[float(transfer),previous[1]/40,previous[2]/30,previous[3]/30,previous[4],min(15,len(same))/15,now[1]/40,(now[1]-previous[1])/40 if now[0] and previous[0] else 0,old_share,min(5,seasons)/5,float(len(same)==0),previous[1]/40*new_weight,previous[2]/30*new_weight]
        roster=self.roster(team,season,cutoff);known=bool(roster);own=player in roster;roster_pos=position(self.data.table['position'][roster[player]].as_py()) if own else 0
        depth_key=('depth',team,season,cutoff)
        if depth_key not in self.cache:
            prior={who:self.summary(who,season-1,cutoff) for who in roster};old_team_minutes=self.team_minutes.get((season-1,team),0.)
            returning=sum(p[6] for p in prior.values() if p[5]==team);incoming=sum(p[1] for p in prior.values() if p[0] and p[5]!=team);expected=sum(p[1] for p in prior.values());usage=sum(p[3] for p in prior.values())
            self.cache[depth_key]=(prior,returning/max(1,old_team_minutes),incoming,expected,usage)
        prior,returning,incoming,expected,usage=self.cache[depth_key]
        own_prior=prior.get(player,previous);rank=(sum(p[1]>previous[1] for p in prior.values())+1)/max(1,len(prior)) if known else 0
        competition=sum(p[1] for who,p in prior.items() if who!=player and position(self.data.table['position'][roster[who]].as_py())==roster_pos) if roster_pos else 0
        depth=[float(known),float(own),len(roster)/20,returning,incoming/200,expected/200,rank,own_prior[3]/max(1,usage),competition/100,now[4],float(roster_pos>0),float(roster_pos==1),float(roster_pos==2),float(roster_pos==3)]
        games=[g for g in self.team_games.get(team,[]) if g[0]<cutoff and g[1]==season];last_play=max([int(a['day'][i]) for i in same]+[-999999]);missed=sum(day>last_play for day,_,_ in games)
        actual_games={int(a['game_id'][i]) for i in same};missing=sum(gid not in actual_games for _,_,gid in games[-3:])/max(1,len(games[-3:]))
        missing_key=('missing',team,season,cutoff)
        if missing_key not in self.cache:
            recent={gid for _,_,gid in games[-3:]};lost=0.
            for who in roster:
                wh=[i for i in self.histories(who,cutoff) if int(a['team'][i])==team and cutoff-int(a['day'][i])<=30]
                if wh and recent:lost+=float(np.mean(a['minutes'][wh[-10:]]))*(1-sum(int(a['game_id'][i]) in recent for i in wh)/len(recent))
            self.cache[missing_key]=max(0,lost)
        month=__import__('datetime').date.fromisoformat(str(np.datetime64(int(cutoff),'D')));opening=month.month==11 and month.day<=15;nov=day_number(f'{season-1}-11-01');phase=max(0,min(200,cutoff-nov))/120
        participation=[min(40,len(games))/40,len(same)/max(1,len(games)),min(10,missed)/10,missing,self.cache[missing_key]/100,now[1]/40*new_weight,float(opening),phase]
        result=np.asarray(recruit+roles+depth+participation,dtype=np.float32)
        if len(result)!=len(NAMES) or not np.isfinite(result).all():raise ValueError('invalid context features')
        return result

    def observe(self,indices):
        a=self.data.a;rosters=defaultdict(dict)
        for i in indices:
            player,team,season=(int(a[k][i]) for k in ['player_id','team','season']);self.first_season.setdefault(player,season);self.any_history[player].append(i);rosters[(int(a['game_id'][i]),team)][player]=i
            if a['played'][i] and a['minutes'][i]>0:self.history[player].append(i);self.team_minutes[(season,team)]+=float(a['minutes'][i])
        for (game,team),roster in rosters.items():
            i=next(iter(roster.values()));day=int(a['day'][i]);season=int(a['season'][i]);self.roster_history[team].append((day,season,roster));self.team_games[team].append((day,season,game))
        self.cache={}


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--data',default='data/cbb');p.add_argument('--out',default='data/cbb/context');args=p.parse_args();root=Path(args.out);root.mkdir(exist_ok=True,parents=True);d=Dataset(args.data);names=defaultdict(set)
    for player,name in zip(d.a['player_id'],d.table['name'].to_pylist()):names[int(player)].add(normalized(name))
    recruits=Recruiting(root/'recruiting.json',names);s=ContextState(d,recruits);rows=d.positive;lookup={int(i):k for k,i in enumerate(rows)};x=np.zeros((len(rows),len(NAMES)),np.float32);fx=np.zeros_like(x)
    days,starts=np.unique(d.a['day'],return_index=True);ends=np.r_[starts[1:],d.n];week=None;origin=None
    for step,(day,start,end) in enumerate(zip(days,starts,ends)):
        w=int(day)-(int(day)+3)%7
        if w!=week:origin=int(day);week=w
        for i in range(int(start),int(end)):
            if i not in lookup:continue
            k=lookup[i];x[k]=s.build(i,int(day))
            if day>=day_number('2024-11-01'):fx[k]=s.build(i,origin)
        s.observe(range(int(start),int(end)))
        if step%50==0:print('Context features',step+1,'/',len(days),flush=True)
    np.savez_compressed(root/'features.npz',x=x,frozen_x=fx,rows=rows)
    source_files=['tools/cbb_context_features.py','tools/cbb_context_sources.py'];manifest={'names':NAMES,'groups':GROUPS,'source_sha256':{f:hashlib.sha256(Path(f).read_bytes()).hexdigest() for f in source_files},'recruiting_sha256':hashlib.sha256((root/'recruiting.json').read_bytes()).hexdigest(),'data_sha256':json.load(open(Path(args.data)/'dataset-audit.json'))['history_sha256'],'join_audit':dict(recruits.audit),'notes':['All roster features use prior completed games, not end-season/current target roster lists.','Freshman/recruit rank is a pre-college attribute; archived labels are downloaded now, not historical HTTP snapshots.','Availability archives lack report publication timestamps and are excluded from numeric training.']}
    (root/'features.json').write_text(json.dumps(manifest,indent=2)+'\n')


if __name__=='__main__':main()
