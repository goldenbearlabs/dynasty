"""Serializable context corrections layered on the frozen CBB champion."""
import numpy as np
from cbb_candidate import predict as baseline_predict,tree_predict
from cbb_model import head_predict
from cbb_features import STATS,WEIGHTS


def design(cache,context,idx,core_columns,frozen=False):
    name='frozen_x' if frozen else 'x'
    return np.column_stack([cache[name][idx][:,core_columns],context[name][idx]])


def predict(model,cache,context,idx,frozen=False):
    spec=model.get('context_correction')
    baseline={k:v for k,v in model.items() if k!='context_correction'} if spec else model
    minutes,stats,points=baseline_predict(baseline,cache,idx,frozen)
    if spec is None:return minutes,stats,points
    x=design(cache,context,idx,spec['core_columns'],frozen);rates=stats/minutes[:,None]
    correction=tree_predict(spec['minutes_trees'],x) if spec.get('minutes_trees') else 10*head_predict(spec['minutes'],x)[:,0]
    minutes=np.clip(minutes+correction,.5,40)
    scale=cache['frozen_scale' if frozen else 'scales'][idx]
    rates=np.maximum(0,rates+scale*head_predict(spec['rates'],x));stats=rates*minutes[:,None]
    shoot=cache['frozen_shoot' if frozen else 'shoot'][idx];share,p2,p3,pf=shoot.T
    fga=stats[:,STATS.index('fga')];fta=stats[:,STATS.index('fta')];two=fga*(1-share)*p2;three=fga*share*p3
    stats[:,STATS.index('fgm')]=two+three;stats[:,STATS.index('tpm')]=three;stats[:,STATS.index('ftm')]=fta*pf;stats[:,STATS.index('pts')]=2*two+3*three+fta*pf
    for stat in ['dd','td','pts40','pts50']:stats[:,STATS.index(stat)]=np.minimum(1,stats[:,STATS.index(stat)])
    stats[:,STATS.index('td')]=np.minimum(stats[:,STATS.index('td')],stats[:,STATS.index('dd')]);stats[:,STATS.index('pts50')]=np.minimum(stats[:,STATS.index('pts50')],stats[:,STATS.index('pts40')])
    return minutes,stats,stats@WEIGHTS
