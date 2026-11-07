"""Pregame schedule-adjusted offense, defense and tempo, calculated internally.

Ratings use completed prior sports days, never end-season ranking downloads.
The estimator is an additive, regularized opponent-adjustment model, not a
replication of KenPom or Torvik. Only verified D1-v-D1 games fit these ratings.
"""
from collections import defaultdict
import datetime as dt
import numpy as np


def day_number(day):return int(np.datetime64(day,'D').astype(int))


class TeamStrength:
    def __init__(self):
        self.games=defaultdict(list)
        self.previous={}
        self.latest_season=None
        self.current={}
        self.league=(105.,68.)

    def fit(self,season,day,extra_teams=()):
        previous=self.previous.get(season-1,{})
        observations=[g for g in self.games[season] if day_number(g["day"])<day]
        teams=sorted(set(extra_teams)|set(previous)|{t['team'] for g in observations for t in g['teams']})
        if not teams:return {}
        ids={team:i for i,team in enumerate(teams)};n=len(teams)
        league_o=(sum(t['score'] for g in observations for t in g['teams'])/sum(2*g['possessions'] for g in observations)*100) if observations else 105.
        league_p=np.mean([g['pace'] for g in observations]) if observations else 68.
        self.league=(float(league_o),float(league_p))
        prior=np.array([[league_o+.35*(previous.get(t,(105.,105.,68.))[0]-105.),league_o+.35*(previous.get(t,(105.,105.,68.))[1]-105.),league_p+.35*(previous.get(t,(105.,105.,68.))[2]-68.)] for t in teams])
        prior[:,0:2]=np.clip(prior[:,0:2],80,135);prior[:,2]=np.clip(prior[:,2],55,85)
        rating=prior.copy();a=[];b=[];eff=[];pace=[];home=[];weight=[]
        for g in observations:
            age=day-day_number(g['day']);w=2**(-age/90)
            for side in g['teams']:
                a.append(ids[side['team']]);b.append(ids[side['opponent']]);eff.append(100*side['score']/g['possessions']);pace.append(g['pace']);home.append(0 if g['neutral'] else (1 if side['team']==g['home'] else -1));weight.append(w)
        if a:
            a=np.array(a);b=np.array(b);eff=np.array(eff);pace=np.array(pace);home=np.array(home);weight=np.array(weight)
            denom=6+np.bincount(a,weights=weight,minlength=n)
            for _ in range(14):
                own_off=eff-home*2-(rating[b,1]-league_o)
                # The opponent's observed offense is also this team's defense.
                own_def=eff-home*2-(rating[a,0]-league_o)
                tempo=pace-(rating[b,2]-league_p)
                updated=np.empty_like(rating)
                updated[:,0]=(6*prior[:,0]+np.bincount(a,weights=weight*own_off,minlength=n))/denom
                updated[:,1]=(6*prior[:,1]+np.bincount(b,weights=weight*own_def,minlength=n))/denom
                updated[:,2]=(6*prior[:,2]+np.bincount(a,weights=weight*tempo,minlength=n))/denom
                rating=.5*rating+.5*updated
        result={team:tuple(map(float,rating[i])) for i,team in enumerate(teams)}
        self.current=result;self.previous[season]=result
        return result

    def observe(self,games):
        for game in games:
            if game['d1_matchup']:self.games[game['season']].append(game)

    def context(self,game,ratings=None,league=None):
        ratings=self.current if ratings is None else ratings;lo,lp=self.league if league is None else league
        sides={side['team']:side for side in game['teams']};result={}
        for team,side in sides.items():
            opponent=side['opponent'];own=ratings.get(team,(lo,lo,lp));opp=ratings.get(opponent,(lo,lo,lp));home=0 if game['neutral'] else (1 if team==game['home'] else -1)
            pace=float(min(90,max(50,own[2]+opp[2]-lp)))
            own_eff=own[0]+opp[1]-lo+2*home;opp_eff=opp[0]+own[1]-lo-2*home
            result[team]={'own_off':own[0],'own_def':own[1],'opp_off':opp[0],'opp_def':opp[1],'own_pace':own[2],'opp_pace':opp[2],
                          'pace':pace,'margin':(own_eff-opp_eff)*pace/100,'league_off':lo,'league_pace':lp,'known':team in ratings and opponent in ratings}
        return result
