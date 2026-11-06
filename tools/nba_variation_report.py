#!/usr/bin/env python3
"""Create a standalone per-player forecast-variation and uncertainty review."""
import argparse
import json
from pathlib import Path


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--study",default="data/nba/variation-study")
    parser.add_argument("--out",default="data/nba/variation-study/player-review.html")
    args=parser.parse_args()
    root=Path(args.study)
    results=json.loads((root/'results.json').read_text())
    payload={}
    for fold in ['historical','current']:
        with (root/f'distribution-predictions-{fold}.jsonl').open() as source:
            payload[fold]={'rows':[json.loads(line) for line in source], 'calibration':results['folds'][fold]['distribution']['calibration_bins']}
    body=json.dumps(payload,separators=(',',':')).replace('</','<\\/')
    path=Path(args.out);path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(HTML.replace('__DATA__',body))
    print('Wrote',path)


HTML=r'''<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>NBA player variation</title><style>
body{font:15px system-ui;background:#f6f8fc;color:#17263b;max-width:1150px;margin:32px auto;padding:0 22px}p{line-height:1.6}h1{font-size:28px}select{font:inherit;padding:8px;margin:8px;border:1px solid #b9c8dd;border-radius:5px;background:white}.cards{display:flex;gap:12px;flex-wrap:wrap;margin:18px 0}.card{background:white;border:1px solid #dce3ee;padding:14px;border-radius:8px;min-width:125px}.card strong{display:block;font-size:23px}.card span{font-size:13px;color:#54647c}svg{display:block;width:100%;background:white;border:1px solid #dce3ee;border-radius:8px;margin:15px 0}.legend span{margin-right:18px}table{border-collapse:collapse;background:white;width:100%;font-variant-numeric:tabular-nums}th,td{padding:9px;text-align:right;border-bottom:1px solid #dce3ee}th:first-child,td:first-child{text-align:left}.scroll{overflow:auto}small{color:#54647c}
</style><h1>NBA player-by-player variation</h1>
<p>Separate predictable changes in a player's expected production from the much larger variation in his actual results. Forecasts below were issued before each individual game. The typical-score calibration is experimental and has not replaced the expected-score model. The written study separately evaluates weekly matchup choices using forecasts frozen at one shared origin.</p>
<label>Window<select id="fold"><option value="historical">Historical: Jan–Apr 2026</option><option value="current">Current: Oct 30–Nov 4</option></select></label><label>Player<select id="player"></select></label>
<div id="cards" class="cards"></div>
<p class="legend"><span style="color:#286fca">● Expected score</span><span style="color:#a352c5">● Typical calibrated score</span><span style="color:#bc6319">● Actual score</span><span style="color:#8395af">— Prior usual score</span><span style="color:#286fca">▧ 80% prediction range</span></p>
<svg id="chart" viewBox="0 0 1100 380" role="img" aria-label="Actual scores, forecasts and prediction ranges by game"></svg>
<p class="legend"><span style="color:#286fca">● Probability of beating usual score</span><span style="color:#bc6319">● Actual: above usual (100%) or not (0%)</span><span style="color:#8395af">● Actual percentile against prior history</span></p>
<svg id="probability" viewBox="0 0 1100 250" role="img" aria-label="Predicted probabilities and actual within-player outcomes"></svg>
<p><small>Usual score is the median of the prior 20 appearances. Actual percentile uses the prior 60 appearances. These references exclude the target game and every other game on the same sports day. Ranges are conditional on playing; they do not include DNP risk. Current-season player samples are very small.</small></p>
<h2>Probability calibration across all players in this window</h2><p>If games assigned a 60% chance beat the player's usual score about 60% of the time, the probability is calibrated.</p>
<div class="scroll"><table><thead><tr><th>Probability group</th><th>Games</th><th>Mean forecast</th><th>Actual above usual</th><th>Mean actual percentile</th></tr></thead><tbody id="calibration"></tbody></table></div>
<script type="application/json" id="data">__DATA__</script><script>
const data=JSON.parse(document.getElementById('data').textContent),$=id=>document.getElementById(id),fmt=x=>x.toFixed(2),pct=x=>(100*x).toFixed(1)+'%';
function node(tag,attrs,text){const e=document.createElementNS('http://www.w3.org/2000/svg',tag);for(const[k,v]of Object.entries(attrs))e.setAttribute(k,v);if(text!==undefined)e.textContent=text;return e}
function sd(values){const m=values.reduce((a,b)=>a+b,0)/values.length;return Math.sqrt(values.reduce((a,b)=>a+(b-m)**2,0)/values.length)}
function players(){const old=$('player').value,groups=new Map;for(const r of data[$('fold').value].rows){if(!groups.has(r.player))groups.set(r.player,{name:r.name,n:0});groups.get(r.player).n++}$('player').replaceChildren();for(const[id,p]of [...groups.entries()].sort((a,b)=>a[1].name.localeCompare(b[1].name))){const o=document.createElement('option');o.value=id;o.textContent=p.name+' ('+p.n+' games)';$('player').append(o)}if(groups.has(old))$('player').value=old;else{const eligible=data[$('fold').value].rows.filter(r=>groups.get(r.player).n>=3);if(eligible.length){const top=eligible.reduce((a,b)=>a.expected>b.expected?a:b);$('player').value=top.player}}render()}
function draw(id,rows,series,low,high,height){const svg=$(id);svg.replaceChildren();const left=55,right=1075,top=20,bottom=height-40,x=i=>left+(right-left)*i/Math.max(1,rows.length-1),y=v=>bottom-(bottom-top)*(v-low)/(high-low);for(let i=0;i<=4;i++){const v=low+(high-low)*i/4;svg.append(node('line',{x1:left,y1:y(v),x2:right,y2:y(v),stroke:'#edf0f6'}),node('text',{x:45,y:y(v)+4,'text-anchor':'end','font-size':12},id==='probability'?pct(v):v.toFixed(0)))}if(id==='chart'){const points=rows.map((r,i)=>x(i)+','+y(r.low80)).concat(rows.map((r,i)=>x(i)+','+y(r.high80)).reverse()).join(' ');svg.append(node('polygon',{points,fill:'#286fca',opacity:.1}))}for(const s of series){if(s.line)svg.append(node('polyline',{points:rows.map((r,i)=>x(i)+','+y(r[s.key])).join(' '),fill:'none',stroke:s.color,'stroke-width':s.key==='actual'?1:2,'stroke-dasharray':s.key==='usual_score'?'5 4':''}));rows.forEach((r,i)=>{const circle=node('circle',{cx:x(i),cy:y(r[s.key]),r:s.key==='actual'?3.3:2.5,fill:s.color});circle.append(node('title',{},r.day+' · '+s.label+': '+(id==='probability'?pct(r[s.key]):fmt(r[s.key]))));svg.append(circle)})}const step=Math.max(1,Math.ceil(rows.length/9));rows.forEach((r,i)=>{if(i%step===0||i===rows.length-1)svg.append(node('text',{x:x(i),y:height-15,'font-size':11,'text-anchor':'middle'},r.day.slice(5)))})}
function cell(tr,text){const c=document.createElement('td');c.textContent=text;tr.append(c)}
function render(){const rows=data[$('fold').value].rows.filter(r=>r.player===$('player').value).map(r=>({...r,event_above_usual:Number(r.actual>r.usual_score)}));if(!rows.length)return;let wins=0,pairs=0;for(let i=0;i<rows.length;i++)for(let j=i+1;j<rows.length;j++){const a=rows[i].actual-rows[j].actual,p=rows[i].expected-rows[j].expected;if(a!==0){pairs++;wins+=Math.abs(p)<1e-12?.5:Number(a*p>0)}}const mae=rows.reduce((n,r)=>n+Math.abs(r.expected-r.actual),0)/rows.length,coverage=rows.reduce((n,r)=>n+Number(r.actual>=r.low80-1e-9&&r.actual<=r.high80+1e-9),0)/rows.length;$('cards').replaceChildren();for(const[label,value]of [['Games',String(rows.length)],['Expected-score MAE',fmt(mae)],['Actual score SD',fmt(sd(rows.map(r=>r.actual)))],['Forecast SD',fmt(sd(rows.map(r=>r.expected)))],['Pair ranking',pairs?pct(wins/pairs):'n/a'],['80% range coverage',pct(coverage)]]){const d=document.createElement('div');d.className='card';const b=document.createElement('strong');b.textContent=value;const s=document.createElement('span');s.textContent=label;d.append(b,s);$('cards').append(d)}const low=Math.min(-2,...rows.map(r=>r.low80),...rows.map(r=>r.actual))-1,high=Math.max(...rows.map(r=>r.high80),...rows.map(r=>r.actual))+2;draw('chart',rows,[{key:'usual_score',color:'#8395af',label:'Usual score',line:true},{key:'expected',color:'#286fca',label:'Expected',line:true},{key:'typical',color:'#a352c5',label:'Typical',line:true},{key:'actual',color:'#bc6319',label:'Actual',line:true}],low,high,380);draw('probability',rows,[{key:'probability_above_usual',color:'#286fca',label:'Probability above usual',line:true},{key:'event_above_usual',color:'#bc6319',label:'Actual above usual',line:false},{key:'actual_prior_percentile',color:'#8395af',label:'Actual prior percentile',line:false}],0,1,250);$('calibration').replaceChildren();for(const b of data[$('fold').value].calibration){const tr=document.createElement('tr');cell(tr,(b.bin*10)+'–'+((b.bin+1)*10)+'%');cell(tr,b.n);cell(tr,pct(b.mean_probability));cell(tr,pct(b.actual_above_usual_fraction));cell(tr,pct(b.actual_mean_prior_percentile));$('calibration').append(tr)}}
$('fold').onchange=players;$('player').onchange=render;players();
</script></html>'''


if __name__=='__main__':main()
