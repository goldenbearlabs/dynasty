"""Small serializable CBB minutes × stat-rate model and chronological metrics."""
from collections import defaultdict
import json
import math
import numpy as np
from scipy.special import ndtr
from scipy.stats import rankdata
from cbb_features import STATS,NAMES,GROUPS,COMMON,WEIGHTS

VARIANTS={
 'player_only':[], 'strength':['strength'], 'tempo':['tempo'], 'matchups':['matchups'],
 'role':['role'],'rotation':['rotation'],'opportunities':['opportunities'],
 'combined':['strength','tempo','matchups','role','rotation','opportunities'],
 'combined_without_strength':['tempo','matchups','role','rotation','opportunities'],
 'combined_without_tempo':['strength','matchups','role','rotation','opportunities'],
}


def columns(groups,minutes=False):
    names=list(COMMON) if minutes else list(GROUPS['baseline'])
    for group in groups:
        if group=='opportunities' or (minutes and group=='matchups'):continue
        names+=GROUPS[group]
    return [NAMES.index(name) for name in names]


class Moments:
    """One training-only Gram matrix supports every nested feature ablation."""
    def __init__(self,x,y,weights):
        weight=weights.astype(np.float64);self.total=weight.sum();self.mean=np.average(x,axis=0,weights=weight)
        self.offset=np.average(y,axis=0,weights=weight)
        centered=(x-self.mean).astype(np.float32);self.scale=np.maximum(np.sqrt(np.average(centered*centered,axis=0,weights=weight)),1e-3)
        z=(centered/self.scale).astype(np.float32)*np.sqrt(weight/self.total).astype(np.float32)[:,None]
        target=(y-self.offset).astype(np.float32)*np.sqrt(weight/self.total).astype(np.float32)[:,None]
        self.gram=(z.T@z).astype(np.float64);self.xy=(z.T@target).astype(np.float64)

    def fit(self,cols,penalty):
        cov=self.gram[np.ix_(cols,cols)]+penalty*np.eye(len(cols));coef=np.linalg.solve(cov,self.xy[cols])
        coef/=self.scale[cols,None]
        offset=self.offset-self.mean[cols]@coef
        return {'columns':cols,'coefficients':coef.tolist(),'offset':offset.tolist(),'ridge':penalty}


def training_moments(cache,a,mask):
    idx=np.flatnonzero(mask);x=cache['x'][idx];minutes=a['minutes'][idx];season=a['season'][idx];latest=int(season.max())
    w=np.power(.7,latest-season)
    my=((minutes-cache['x'][idx,0]*40)/10)[:,None]
    ry=(cache['stats'][idx]/minutes[:,None]-cache['base'][idx])/cache['scales'][idx]
    return Moments(x,my,w),Moments(x,ry,w*np.minimum(minutes,40)/25)


def head_predict(head,x):
    return x[:,head['columns']]@np.asarray(head['coefficients'])+np.asarray(head['offset'])


def predict(model,cache,idx,frozen=False,actual_minutes=None):
    prefix='frozen_' if frozen else '';x=cache[prefix+'x'][idx];base=cache[prefix+'base'][idx];scale=cache[prefix+'scale' if frozen else 'scales'][idx]
    minutes=np.clip(x[:,0]*40+10*head_predict(model['minutes'],x)[:,0],.5,40) if actual_minutes is None else np.asarray(actual_minutes)
    rates=np.maximum(0,base+scale*head_predict(model['rates'],x));stats=rates*minutes[:,None]
    if 'opportunities' in model['groups']:
        shooting=cache[prefix+'shoot'][idx];share,p2,p3,pf=shooting.T
        fga=stats[:,STATS.index('fga')];fta=stats[:,STATS.index('fta')];two=fga*(1-share)*p2;three=fga*share*p3
        stats[:,STATS.index('fgm')]=two+three;stats[:,STATS.index('tpm')]=three;stats[:,STATS.index('ftm')]=fta*pf;stats[:,STATS.index('pts')]=2*two+3*three+fta*pf
    else:
        stats[:,STATS.index('fgm')]=np.minimum(stats[:,STATS.index('fgm')],stats[:,STATS.index('fga')])
        stats[:,STATS.index('tpm')]=np.minimum(stats[:,STATS.index('tpm')],stats[:,STATS.index('fgm')])
        stats[:,STATS.index('ftm')]=np.minimum(stats[:,STATS.index('ftm')],stats[:,STATS.index('fta')])
    for stat in ['dd','td','pts40','pts50']:stats[:,STATS.index(stat)]=np.minimum(1,stats[:,STATS.index(stat)])
    stats[:,STATS.index('td')]=np.minimum(stats[:,STATS.index('td')],stats[:,STATS.index('dd')]);stats[:,STATS.index('pts50')]=np.minimum(stats[:,STATS.index('pts50')],stats[:,STATS.index('pts40')])
    return minutes,stats,stats@WEIGHTS


def metrics(actual,predicted):
    delta=np.asarray(predicted)-np.asarray(actual)
    return {'n':len(delta),'mae':float(np.abs(delta).mean()),'rmse':float(np.sqrt((delta*delta).mean())),'bias':float(delta.mean())} if len(delta) else {'n':0,'mae':None,'rmse':None,'bias':None}


def masks(cache,a,idx):
    eligible=a['eligible'][idx]&a['d1_matchup'][idx];x=cache['x'][idx]
    return {'all_verified_d1':a['d1_matchup'][idx], 'eligible_conferences':eligible,
            'eligible_prior20_minutes':eligible&(x[:,0]*40>=20), 'eligible_cold_start':eligible&(cache['history'][idx]==0),
            'eligible_first3_season_games':eligible&(cache['season_n'][idx]<3),'eligible_transfer':eligible&(x[:,NAMES.index('transfer')]>0),
            'eligible_neutral_site':eligible&a['neutral'][idx],'eligible_postseason':eligible&(a['season_type'][idx]==3),
            'eligible_predicted_mismatch':eligible&(np.abs(x[:,NAMES.index('expected_margin')]*20)>=20),
            'unverified_opponent':~a['d1_matchup'][idx]}


def evaluate(model,cache,a,idx):
    minutes,stats,points=predict(model,cache,idx);actual=cache['stats'][idx]@WEIGHTS
    groups=masks(cache,a,idx)
    result={'games':len(set(a['game_id'][idx])),'model':metrics(actual,points),'minutes':metrics(a['minutes'][idx],minutes),
            'proxy_if_active':metrics(actual,cache['proxy'][idx,0]),'proxy_attendance_discount':metrics(actual,cache['proxy'][idx,1]),'recent10':metrics(actual,cache['prior_mean'][idx]),
            'groups':{key:{'model':metrics(actual[m],points[m]),'proxy_if_active':metrics(actual[m],cache['proxy'][idx,0][m]),'recent10':metrics(actual[m],cache['prior_mean'][idx][m]),'minutes':metrics(a['minutes'][idx][m],minutes[m])} for key,m in groups.items()},
            'stats':{s:metrics(cache['stats'][idx,j],stats[:,j]) for j,s in enumerate(STATS)}}
    main=groups['eligible_conferences'];result['eligible_games']=len(set(a['game_id'][idx][main]));return result,(minutes,stats,points)


def paired_bootstrap(a,idx,actual,base,selected,mask,repetitions=1500):
    games=a['game_id'][idx][mask];gain=np.abs(base[mask]-actual[mask])-np.abs(selected[mask]-actual[mask]);ids,inverse=np.unique(games,return_inverse=True)
    totals=np.bincount(inverse,weights=gain);counts=np.bincount(inverse);rng=np.random.default_rng(20261106)
    samples=[]
    for _ in range(repetitions):
        selected_games=rng.integers(0,len(ids),len(ids));samples.append(float(totals[selected_games].sum()/counts[selected_games].sum()))
    return {'mae_reduction':float(gain.mean()),'game_bootstrap_95_interval':list(map(float,np.quantile(samples,[.025,.975]))),'games':len(ids),'appearances':len(gain)}


def variation(cache,a,idx,points):
    actual=cache['stats'][idx]@WEIGHTS;groups=defaultdict(list)
    for k,player in enumerate(a['player_id'][idx]):groups[int(player)].append(k)
    correlations=[];pred_center=[];actual_center=[];trend_a=[];trend_p=[];spread=[]
    for ks in groups.values():
        av,pv=actual[ks],points[ks]
        if len(ks)>=5 and np.std(av)>0 and np.std(pv)>1e-8:
            correlations.append(float(np.corrcoef(rankdata(av),rankdata(pv))[0,1]));pred_center.extend(pv-pv.mean());actual_center.extend(av-av.mean());spread.append((float(np.std(pv)),float(np.std(av))))
        for start in range(0,len(ks)-2,3):trend_a.append(float(av[start:start+3].mean()));trend_p.append(float(pv[start:start+3].mean()))
    return {'within_player_spearman_mean':float(np.mean(correlations)) if correlations else None,'players_with_5_games':len(correlations),
            'within_player_centered_correlation':float(np.corrcoef(pred_center,actual_center)[0,1]) if pred_center else None,
            'median_player_forecast_sd':float(np.median([x[0] for x in spread])) if spread else None,'median_player_actual_sd':float(np.median([x[1] for x in spread])) if spread else None,
            'sequential_three_game_average':metrics(trend_a,trend_p)}


def ranking(model,cache,a,idx):
    unknown=int(np.sum((~cache['frozen_known'][idx])&a['eligible'][idx]&a['d1_matchup'][idx]))
    keep=cache['frozen_known'][idx]&a['eligible'][idx]&a['d1_matchup'][idx];idx=idx[keep]
    _,_,points=predict(model,cache,idx,True);actual=cache['stats'][idx]@WEIGHTS;groups=defaultdict(list)
    for k,(player,day) in enumerate(zip(a['player_id'][idx],a['day'][idx])):groups[(int(player),int(day)-(int(day)+3)%7)].append(k)
    correct=pairs=0.;regrets=[];chosen=[];counts=0
    for ks in groups.values():
        if len(ks)<2:continue
        counts+=1;best=max(ks,key=lambda i:points[i]);regrets.append(float(max(actual[ks])-actual[best]));chosen.append(float(actual[best]))
        for j,left in enumerate(ks):
            for right in ks[j+1:]:
                difference=actual[left]-actual[right]
                if abs(difference)<1e-8:continue
                pred=points[left]-points[right];pairs+=1;correct+=.5 if abs(pred)<1e-8 else float(pred*difference>0)
    return {'player_weeks':counts,'pairs':int(pairs),'pair_accuracy':correct/pairs if pairs else None,'regret':float(np.mean(regrets)) if regrets else None,'chosen_actual_mean':float(np.mean(chosen)) if chosen else None,'eligible_appearances':len(idx),'unknown_origin_excluded':unknown}


def calibration(validation_actual,validation_pred,validation_mean,validation_sd,actual,pred,prior_mean,prior_sd):
    residuals=np.sort((validation_actual-validation_pred)/validation_sd);q=np.quantile(residuals,[.025,.1,.9,.975])
    scaled=(prior_mean-pred)/prior_sd;probability=1-np.searchsorted(residuals,scaled,side='right')/len(residuals);outcome=actual>prior_mean
    base=float(np.mean(validation_actual>validation_mean));bins=[]
    for lo in np.arange(0,1,.1):
        m=(probability>=lo)&(probability<lo+.1 if lo<.9 else probability<=1)
        if m.any():bins.append({'lower':round(float(lo),1),'n':int(m.sum()),'predicted':float(probability[m].mean()),'actual_above_usual_rate':float(outcome[m].mean())})
    return {'interval80':{'coverage':float(np.mean((actual>=pred+q[1]*prior_sd)&(actual<=pred+q[2]*prior_sd))),'mean_width':float(np.mean((q[2]-q[1])*prior_sd))},
            'interval95':{'coverage':float(np.mean((actual>=pred+q[0]*prior_sd)&(actual<=pred+q[3]*prior_sd))),'mean_width':float(np.mean((q[3]-q[0])*prior_sd))},
            'above_usual_probability_brier':float(np.mean((probability-outcome)**2)),'fitted_base_rate_brier':float(np.mean((base-outcome)**2)),'validation_base_rate':base,'bins':bins,'scaled_residual_quantiles':q.tolist()}
