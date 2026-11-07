#!/usr/bin/env python3
"""Build sequential and frozen-week CBB features without outcome leakage."""
import argparse
from collections import defaultdict,deque
import copy
import datetime as dt
import hashlib
import json
from pathlib import Path

import numpy as np
import pyarrow.parquet as pq
from cbb_dataset import STATS,D1_CONFERENCES
from cbb_strength import TeamStrength,day_number

SCORING={'pts':.5,'reb':1.,'ast':1.,'stl':2.,'blk':2.,'tov':-1.,'tpm':.5,'dd':1.,'td':2.,'pts40':2.,'pts50':2.}
WEIGHTS=np.array([SCORING.get(s,0) for s in STATS])
FALLBACK=np.array([.45,.18,.085,.025,.018,.055,.045,.17,.38,.065,.095,.0015,.0001,.0003,.00003])
FLOORS=np.array([.1,.03,.02,.008,.006,.015,.015,.04,.08,.025,.025,.001,.0001,.0001,.00001])
COMMON=['base_minutes','recent3_minutes','recent10_minutes','season_minutes','previous_minutes','start_share','season_experience','history_experience','transfer','days_rest','home','neutral','guard','forward','center','postseason']
GROUPS={
 'baseline':COMMON+['base_'+s for s in STATS]+['recent_'+s for s in STATS]+['season_'+s for s in STATS],
 'strength':['own_adj_off','own_adj_def','opponent_adj_off','opponent_adj_def','expected_margin','absolute_margin','favorite_blowout','underdog_blowout','starter_margin','opponent_defense_change','transfer_offense_change','opponent_unverified','prior_unverified_share'],
 'tempo':['expected_pace','own_pace','opponent_pace','pace_change'],
 'matchups':['opponent_position_'+s for s in STATS]+['previous_meeting_'+s for s in STATS],
 'role':['median_minutes_5','median_shift','minutes_mad','short_foul_share','start_shift'],
 'rotation':['expected_team_minutes','same_position_competition','missing_prior_teammate_minutes','own_recent_participation','known_rotation','team_rest','team_games7'],
}
NAMES=sum(GROUPS.values(),[])
MINUTE_BASE=list(range(len(COMMON)))


class Dataset:
    def __init__(self,folder):
        self.folder=Path(folder);self.table=pq.read_table(self.folder/'history.parquet');self.a={}
        for key in ['game_id','player_id','team','opponent','season','minutes','played','starter','home','neutral','conference','opponent_conference','d1_matchup','eligible','season_type','pf','tpa']:
            self.a[key]=self.table[key].to_numpy(zero_copy_only=False)
        self.a['day']=np.array(self.table['day'].to_pylist(),dtype='datetime64[D]').astype(np.int32)
        self.stats=np.column_stack([self.table[s].to_numpy() for s in STATS]).astype(np.float32)
        self.positions=np.array([{'G':1,'PG':1,'SG':1,'F':2,'SF':2,'PF':2,'C':3}.get(p,0) for p in self.table['position'].to_pylist()],dtype=np.int8)
        self.games=json.loads((self.folder/'games.json').read_text());self.gmap={g['game_id']:g for g in self.games}
        self.byday=defaultdict(list)
        for g in self.games:self.byday[day_number(g['day'])].append(g)
        self.positive=np.flatnonzero(self.a['played'] & (self.a['minutes']>0))
        self.n=len(self.a['day'])


class State:
    def __init__(self,data):
        self.data=data;self.history=defaultdict(list);self.pos=defaultdict(lambda:np.zeros(18));self.defense=defaultdict(lambda:np.zeros((2,len(STATS))));self.team_games=defaultdict(list);self.roster={};self.team_roster=defaultdict(set);self.context_cache={}
        self.prior_opp_def=np.zeros(data.n,dtype=np.float32);self.prior_own_off=np.zeros(data.n,dtype=np.float32)
        self.strength=TeamStrength();self.ratings={};self.league=(105.,68.);self.origin=None;self.week_snapshot=None
        self.pool_squared=36.;self.pool_n=1
        self.game_counts=defaultdict(lambda:defaultdict(int))

    def freeze(self,day):
        self.origin=day
        self.week_snapshot={'pos':{k:v.copy() for k,v in self.pos.items()},'defense':{k:v.copy() for k,v in self.defense.items()},'ratings':dict(self.ratings),'league':self.league,'roster':dict(self.roster),'team_roster':{k:set(v) for k,v in self.team_roster.items()},'pool':(self.pool_squared,self.pool_n)}

    def player_history(self,player,origin=None):
        h=self.history.get(int(player),[])
        if origin is not None:h=[i for i in h if self.data.a['day'][i]<origin]
        return h

    def priors(self,season,position,frozen):
        source=self.week_snapshot['pos'] if frozen else self.pos
        total=np.zeros(18)
        for year in [season-2,season-1,season]:
            v=source.get((year,position),np.zeros(18))
            if v[1]<200:v=source.get((year,0),np.zeros(18))
            total+=v*2**(year-season)
        if total[1]<=0:return 20.,FALLBACK.copy(),.5,.33,.7
        rates=total[2:17]/total[1]
        two_made=total[2+STATS.index('fgm')]-total[2+STATS.index('tpm')];two_att=total[2+STATS.index('fga')]-total[17]
        return total[1]/max(1,total[0]),rates,two_made/max(1,two_att),total[2+STATS.index('tpm')]/max(1,total[17]),total[2+STATS.index('ftm')]/max(1,total[2+STATS.index('fta')])

    def rotation(self,team,day,position,player,frozen):
        origin=self.origin if frozen else None;key=(int(team),int(position),bool(frozen))
        if key not in self.context_cache:
            games=[g for g in self.team_games.get(int(team),[]) if origin is None or day_number(g['day'])<origin][-5:]
            roster=self.week_snapshot['roster'] if frozen else self.roster
            expected=same=missing=0.;parts={};known=0
            roster_teams=self.week_snapshot['team_roster'] if frozen else self.team_roster
            for who in roster_teams.get(int(team),set()):
                which,seen=roster[who]
                if which!=team or day-seen>30:continue
                h=self.player_history(who,origin)
                h=[i for i in h if self.data.a['team'][i]==team]
                if not h:continue
                recent=h[-10:];minutes=float(np.mean(self.data.a['minutes'][recent]));game_set={g['game_id'] for g in games}
                p=(sum(self.data.a['game_id'][i] in game_set for i in recent)+.5)/(len(games)+1) if games else .5;p=min(1,p)
                expected+=p*minutes;missing+=(1-p)*minutes;known+=1
                if self.data.positions[h[-1]]==position:same+=p*minutes
                parts[who]=(p,minutes)
            rest=min(14,day-day_number(games[-1]['day'])) if games else 7
            games7=sum(day-day_number(g['day'])<=7 for g in self.team_games.get(int(team),[]) if origin is None or day_number(g['day'])<origin)
            self.context_cache[key]=(expected,same,missing,parts,known,rest,games7)
        expected,same,missing,parts,known,rest,games7=self.context_cache[key];p,minutes=parts.get(int(player),(.5,20))
        return [expected/200,max(0,same-p*minutes)/100,missing/100,p,known/12,rest/7,games7/4]

    def build(self,index,day,game,frozen=False,target=None):
        a=self.data.a
        if target is None:player,team,opponent,season=int(a['player_id'][index]),int(a['team'][index]),int(a['opponent'][index]),int(a['season'][index])
        else:player,team,opponent,season=(int(target[k]) for k in ['player_id','team','opponent','season'])
        info_day=self.origin if frozen else day
        origin=self.origin if frozen else None;h=self.player_history(player,origin);h=[i for i in h if a['season'][i]>=season-2]
        position=int(self.data.positions[h[-1]]) if h else 0
        prior_m,prior_rate,p2,p3,pf=self.priors(season,position,frozen);scale=np.maximum(prior_rate,FLOORS)
        if h:
            ix=np.asarray(h);mins=a['minutes'][ix];ys=self.data.stats[ix];ages=np.arange(len(h)-1,-1,-1)
            w=np.exp2(-ages/10)*np.power(.35,season-a['season'][ix]);weighted_m=float(w@mins)
            base_minutes=(2*prior_m+float(w@mins))/(2+float(w.sum()));base_rates=(120*prior_rate+w@ys)/(120+weighted_m)
            recent=h[-10:];rmin=float(a['minutes'][recent].sum());recent_rate=self.data.stats[recent].sum(axis=0)/max(1,rmin)
            current=ix[a['season'][ix]==season];previous=ix[a['season'][ix]==season-1]
            season_m=float(a['minutes'][current].mean()) if len(current) else base_minutes;previous_m=float(a['minutes'][previous].mean()) if len(previous) else base_minutes
            season_rate=self.data.stats[current].sum(axis=0)/max(1,float(a['minutes'][current].sum())) if len(current) else base_rates
            start_share=float(a['starter'][recent].mean());rest=min(30,info_day-int(a['day'][h[-1]]));transfer=float(a['team'][h[-1]]!=team)
            recent_fp=self.data.stats[recent]@WEIGHTS;recent_points=float(recent_fp.mean());variance=float(np.sum((recent_fp-recent_points)**2));n=len(recent)
        else:
            base_minutes=prior_m;base_rates=prior_rate;recent_rate=base_rates;season_rate=base_rates;season_m=previous_m=base_minutes;start_share=.5;rest=7;transfer=0.;current=[];recent=[];variance=0.;n=0;recent_points=float(base_minutes*(base_rates@WEIGHTS))
        base_minutes=float(np.clip(base_minutes,1,40))
        home=0 if game['neutral'] else (1 if team==game['home'] else -1)
        common=[base_minutes/40,float(a['minutes'][h[-3:]].mean())/40 if h else base_minutes/40,float(a['minutes'][recent].mean())/40 if h else base_minutes/40,season_m/40,previous_m/40,start_share,min(len(current),15)/15,min(len(h),60)/60,transfer,rest/14,home,float(game['neutral']),float(position==1),float(position==2),float(position==3),float(game['season_type']==3)]
        base=common+list(base_rates/scale)+list((recent_rate-base_rates)/scale)+list((season_rate-base_rates)/scale)
        ratings=self.week_snapshot['ratings'] if frozen else self.ratings;league=self.week_snapshot['league'] if frozen else self.league
        c=self.strength.context(game,ratings,league)[team];lo,lp=league
        old_def=float(np.mean(self.prior_opp_def[recent])) if recent else c['opp_def'];old_off=float(np.mean(self.prior_own_off[recent])) if recent else c['own_off']
        unknown=float(np.mean(~a['d1_matchup'][recent])) if recent else 0.
        if target is None:opponent_verified=int(a['opponent_conference'][index]) in D1_CONFERENCES
        else:opponent_verified=int(target.get('opponent_conference',0)) in D1_CONFERENCES
        strength=[(c['own_off']-lo)/15,(c['own_def']-lo)/15,(c['opp_off']-lo)/15,(c['opp_def']-lo)/15,c['margin']/20,abs(c['margin'])/20,max(0,c['margin']-15)/20,max(0,-c['margin']-15)/20,abs(c['margin'])*start_share/20,(c['opp_def']-old_def)/15,(c['own_off']-old_off)/15,float(not opponent_verified),unknown]
        prior_paces=[self.data.gmap[int(a['game_id'][i])]['pace'] for i in recent]
        previous_pace=float(np.mean(prior_paces)) if prior_paces else lp
        tempo=[(c['pace']-lp)/10,(c['own_pace']-lp)/10,(c['opp_pace']-lp)/10,(c['pace']-previous_pace)/10]
        source=self.week_snapshot['defense'] if frozen else self.defense;dv=source.get((season,opponent,position),np.zeros((2,len(STATS))))
        factors=np.clip((dv[0]+500*scale)/(dv[1]+500*scale),.5,1.5)-1
        meeting=next((i for i in reversed(h) if a['opponent'][i]==opponent and day-a['day'][i]<=365),None)
        h2h=(self.data.stats[meeting]/a['minutes'][meeting]-base_rates)/scale*a['minutes'][meeting]/(a['minutes'][meeting]+120) if meeting is not None else np.zeros(len(STATS))
        if h:
            m5=a['minutes'][h[-5:]];old=a['minutes'][h[-10:-5]];median=float(np.median(m5));shift=(median-float(np.median(old)))/40 if len(old) else 0
            short=float(np.mean((a['pf'][h[-5:]]>=4)&(m5<base_minutes-6)));spread=float(np.mean(np.abs(m5-median)));oldstarts=float(a['starter'][h[-10:-5]].mean()) if len(old) else start_share
            role=[median/40,shift,spread/10,short,float(a['starter'][h[-5:]].mean())-oldstarts]
            ix=np.asarray(h);ages=np.arange(len(h)-1,-1,-1);volume_w=np.exp2(-ages/10)*np.power(.35,season-a['season'][ix]);conv_w=np.exp2(-ages/50)*np.power(.5,season-a['season'][ix]);ys=self.data.stats[ix];tp=a['tpa'][ix]
            share=(volume_w@tp+50*.35)/(volume_w@ys[:,STATS.index('fga')]+50)
            two=(conv_w@(ys[:,STATS.index('fgm')]-ys[:,STATS.index('tpm')])+100*p2)/(conv_w@(ys[:,STATS.index('fga')]-tp)+100)
            three=(conv_w@ys[:,STATS.index('tpm')]+150*p3)/(conv_w@tp+150)
            free=(conv_w@ys[:,STATS.index('ftm')]+50*pf)/(conv_w@ys[:,STATS.index('fta')]+50)
        else:role=[base_minutes/40,0,0,0,0];share=.35;two=p2;three=p3;free=pf
        rotation=self.rotation(team,info_day,position,player,frozen)
        pool_sq,pool_n=self.week_snapshot['pool'] if frozen else (self.pool_squared,self.pool_n)
        sd=float(np.sqrt((variance+20*pool_sq/max(1,pool_n))/(max(0,n-1)+20)))
        x=np.asarray(base+strength+tempo+list(factors)+list(h2h)+role+rotation,dtype=np.float32)
        assert len(x)==len(NAMES)
        return x,base_rates.astype(np.float32),scale.astype(np.float32),np.clip([share,two,three,free],0,1).astype(np.float32),{'history':len(h),'season_games':len(current),'recent_points':recent_points,'prior_sd':max(1.,sd),'position':position,'opp_def':c['opp_def'],'own_off':c['own_off'],'origin_known':bool(h and (self.week_snapshot['roster'] if frozen else self.roster).get(player,(None,None))[0]==team)}

    def observe(self,indices,games,base_by_index):
        a=self.data.a;self.context_cache={}
        for i in indices:
            player,team,season=int(a['player_id'][i]),int(a['team'][i]),int(a['season'][i]);old_team=self.roster.get(player,(None,None))[0]
            if old_team is not None and old_team!=team:self.team_roster[old_team].discard(player)
            self.roster[player]=(team,int(a['day'][i]));self.team_roster[team].add(player)
            if not a['played'][i] or a['minutes'][i]<=0:continue
            h=self.history[player]
            if len(h)>=5:
                residual=float(self.data.stats[i]@WEIGHTS-np.mean(self.data.stats[h[-10:]]@WEIGHTS));self.pool_squared+=residual*residual;self.pool_n+=1
            h.append(int(i))
            if len(h)>100:del h[0]
            if int(a['conference'][i]) in D1_CONFERENCES:
                position=int(self.data.positions[i]);total=np.r_[1.,a['minutes'][i],self.data.stats[i],a['tpa'][i]]
                for pos in {0,position}:self.pos[(season,pos)]+=total
                if a['d1_matchup'][i]:
                    d=self.defense[(season,int(a['opponent'][i]),position)];d[0]+=self.data.stats[i];d[1]+=base_by_index[int(i)]*a['minutes'][i]
        for g in games:
            for side in g['teams']:
                self.team_games[side['team']].append(g)
                if side['conference'] in D1_CONFERENCES:self.game_counts[g['season']][side['team']]+=1
        self.strength.observe(games)

    def proxy(self,index):
        a=self.data.a;h=self.history.get(int(a['player_id'][index]),[])
        if not self.game_counts:return 0.,0.
        latest=max(self.game_counts);lengths={y:float(np.mean(list(counts.values()))) for y,counts in self.game_counts.items() if y>=latest-1 and counts}
        total=np.zeros(len(STATS));n=0.
        for i in h:
            year=int(a['season'][i])
            if year not in lengths:continue
            w=1. if year==latest else .25;total+=w*self.data.stats[i];n+=w
        if n==0:return 0.,0.
        conditional=float(total@WEIGHTS/n);length=sum((1. if y==latest else .25)*v for y,v in lengths.items())
        return conditional,conditional*min(1,n/max(1,length))


def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--data',default='data/cbb');p.add_argument('--out',default='data/cbb/features.npz');args=p.parse_args()
    data=Dataset(args.data);state=State(data);a=data.a;positive=data.positive;n=len(positive);lookup={int(i):k for k,i in enumerate(positive)}
    x=np.zeros((n,len(NAMES)),dtype=np.float32);frozen_x=np.zeros_like(x);base=np.zeros((n,len(STATS)),dtype=np.float32);frozen_base=np.zeros_like(base);scales=np.zeros_like(base);frozen_scale=np.zeros_like(base);shoot=np.zeros((n,4),np.float32);frozen_shoot=np.zeros_like(shoot)
    history=np.zeros(n,np.int16);season_n=np.zeros(n,np.int16);prior_mean=np.zeros(n,np.float32);prior_sd=np.zeros(n,np.float32);pos=np.zeros(n,np.int8);frozen_known=np.zeros(n,bool);proxy=np.zeros((n,2),np.float32)
    previous_week=None;days,starts=np.unique(a['day'],return_index=True);ends=np.r_[starts[1:],data.n]
    for step,(day,first,last) in enumerate(zip(days,starts,ends)):
        games=data.byday[int(day)];season=int(a['season'][first]);teams={side['team'] for g in games for side in g['teams']}
        state.ratings=state.strength.fit(season,int(day),teams);state.league=state.strength.league;state.context_cache={}
        week=int(day)-(int(day)+3)%7
        if week!=previous_week:state.freeze(int(day));previous_week=week
        base_by_index={}
        for i in range(int(first),int(last)):
            if i not in lookup:continue
            k=lookup[i];game=data.gmap[int(a['game_id'][i])]
            x[k],base[k],scales[k],shoot[k],meta=state.build(i,int(day),game)
            state.prior_opp_def[i]=meta['opp_def'];state.prior_own_off[i]=meta['own_off'];base_by_index[i]=base[k]
            history[k]=meta['history'];season_n[k]=meta['season_games'];prior_mean[k]=meta['recent_points'];prior_sd[k]=meta['prior_sd'];pos[k]=meta['position'];proxy[k]=state.proxy(i)
            if day>=day_number('2024-11-01'):
                frozen_x[k],frozen_base[k],frozen_scale[k],frozen_shoot[k],fm=state.build(i,int(day),game,True);frozen_known[k]=fm['origin_known']
        state.observe(range(int(first),int(last)),games,base_by_index)
        if step%40==0:print('Features',step+1,'/',len(days),'days;',str(np.datetime64(int(day),'D')),flush=True)
    np.savez_compressed(args.out,x=x,frozen_x=frozen_x,base=base,frozen_base=frozen_base,scales=scales,frozen_scale=frozen_scale,shoot=shoot,frozen_shoot=frozen_shoot,history=history,season_n=season_n,prior_mean=prior_mean,prior_sd=prior_sd,position=pos,frozen_known=frozen_known,proxy=proxy,rows=positive,stats=data.stats[positive])
    source=''.join(Path(__file__).read_text()+Path('tools/cbb_strength.py').read_text()+Path('tools/cbb_dataset.py').read_text())
    manifest={'features':NAMES,'groups':GROUPS,'feature_source_sha256':hashlib.sha256(source.encode()).hexdigest(),'dataset_sha256':json.loads((Path(args.data)/'dataset-audit.json').read_text())['history_sha256'],'rows':n,'notes':['Player identity and cohort are conditional on positive-minute appearances. No target starts/minutes/position/stats supplied to Build.','Frozen-week features use the same prior-day origin for every remaining game; unknown/traded origin roster membership excluded from ordering.','Team ratings rebuilt from prior completed D1 games; current-season ranking snapshots are never replayed onto earlier games.']}
    Path(args.out).with_suffix('.json').write_text(json.dumps(manifest,indent=2)+'\n')


if __name__=='__main__':main()
