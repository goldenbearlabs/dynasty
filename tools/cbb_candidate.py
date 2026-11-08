"""Portable JSON prediction for the approved CBB linear + minutes-tree model."""
import numpy as np
from cbb_features import STATS,NAMES,WEIGHTS
from cbb_model import head_predict,predict as linear_predict


def export_trees(estimator,cols):
    trees=[]
    for stages in estimator._predictors:
        if len(stages)!=1:raise ValueError('expected one numeric regression tree per stage')
        nodes=stages[0].nodes
        if np.any(nodes['is_categorical']):raise ValueError('categorical trees are unsupported')
        trees.append({key:nodes[field].tolist() for key,field in {'value':'value','feature':'feature_idx','threshold':'num_threshold','left':'left','right':'right','leaf':'is_leaf','missing_left':'missing_go_to_left'}.items()})
    return {'format':'numeric_hist_gradient_boosting_v1','columns':cols,'baseline':float(estimator._baseline_prediction.ravel()[0]),'trees':trees}


def tree_predict(correction,x):
    values=np.asarray(x)[:,correction['columns']];total=np.full(len(values),correction['baseline'],dtype=float)
    for tree in correction['trees']:
        node=np.zeros(len(values),dtype=np.int32);leaf=np.asarray(tree['leaf'],bool);feature=np.asarray(tree['feature'],int);threshold=np.asarray(tree['threshold']);left=np.asarray(tree['left'],int);right=np.asarray(tree['right'],int);missing=np.asarray(tree['missing_left'],bool)
        for _ in range(len(leaf)):
            rows=np.flatnonzero(~leaf[node])
            if not len(rows):break
            n=node[rows];v=values[rows,feature[n]];go_left=np.where(np.isnan(v),missing[n],v<=threshold[n]);node[rows]=np.where(go_left,left[n],right[n])
        else:raise ValueError('invalid/cyclic tree')
        total+=np.asarray(tree['value'])[node]
    return total


def predict(model,cache,idx,frozen=False):
    if model.get('context_correction'):raise ValueError('Context model requires the context-aware predictor; refusing to ignore its corrections')
    if not model.get('minutes_correction'):return linear_predict(model,cache,idx,frozen)
    x=cache['frozen_x' if frozen else 'x'][idx]
    minutes=np.clip(x[:,0]*40+10*head_predict(model['minutes'],x)[:,0],.5,40)
    minutes=np.clip(minutes+tree_predict(model['minutes_correction'],x),.5,40)
    return linear_predict(model,cache,idx,frozen,actual_minutes=minutes)


def opening_phase(day):
    dates=np.asarray(day,dtype='datetime64[D]');months=dates.astype('datetime64[M]');month=months.astype(int)%12+1;dom=(dates-months.astype('datetime64[D]')).astype(int)+1
    return (month==11)&(dom<=15)


def fit_uncertainty(actual,points,prior_mean,sd,phase):
    levels=np.unique(np.r_[np.linspace(0,1,129),.025,.1,.9,.975]);result={}
    for name,mask in [('all',np.ones(len(points),bool)),('opening',phase),('established',~phase)]:
        if mask.sum()<250 and name!='all':continue
        z=(actual[mask]-points[mask])/sd[mask]
        result[name]={'n':int(mask.sum()),'levels':levels.tolist(),'quantiles':np.quantile(z,levels).tolist(),'above_usual_base_rate':float(np.mean(actual[mask]>prior_mean[mask]))}
    return result


def uncertainty(model,points,prior_mean,sd,phase):
    n=len(points);prob=np.zeros(n);base=np.zeros(n);lo80=np.zeros(n);hi80=np.zeros(n);lo95=np.zeros(n);hi95=np.zeros(n)
    for flag,name in [(True,'opening'),(False,'established')]:
        m=np.asarray(phase)==flag
        if not m.any():continue
        fit=model['uncertainty'].get(name,model['uncertainty']['all']);qs=np.asarray(fit['quantiles']);levels=np.asarray(fit['levels'])
        prob[m]=1-np.interp((prior_mean[m]-points[m])/sd[m],qs,levels);base[m]=fit['above_usual_base_rate']
        q=np.interp([.025,.1,.9,.975],levels,qs);lo95[m]=points[m]+q[0]*sd[m];lo80[m]=points[m]+q[1]*sd[m];hi80[m]=points[m]+q[2]*sd[m];hi95[m]=points[m]+q[3]*sd[m]
    return {'probability_above_usual':prob,'base_probability':base,'lower80':lo80,'upper80':hi80,'lower95':lo95,'upper95':hi95}
