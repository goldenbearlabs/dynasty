#!/usr/bin/env python3
"""Build a standalone, interactive game review from harness prediction JSONL."""
import argparse
import datetime as dt
import json
from pathlib import Path
from zoneinfo import ZoneInfo


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--predictions", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--baseline-errors", help="optional compact errors JSONL from a frozen model on the identical cohort")
    args = parser.parse_args()
    baseline = {}
    if args.baseline_errors:
        with open(args.baseline_errors) as source:
            for line in source:
                r = json.loads(line)
                baseline[(r["game"], r["player"])] = r
    rows = []
    with open(args.predictions) as source:
        for line in source:
            item = json.loads(line)
            moment = dt.datetime.fromisoformat(item["target"]["starts_at"].replace("Z", "+00:00"))
            day = (moment.astimezone(ZoneInfo("America/New_York")) - dt.timedelta(hours=5)).date().isoformat()
            rows.append({"game": item["target"]["game_id"], "date": item["target"]["starts_at"],
                         "day": day,
                         "player": item["name"], "player_id": item["target"]["player_id"],
                         "position": item["features"]["position"],
                         "actual": item["actual_points"], "model": item["prediction"]["fantasy_points_if_active"],
                         "proxy": item["proxy"], "proxy_if_active": item["proxy_if_active"], "recent_10": item["recent_10"],
                         "minutes": item["prediction"]["minutes"], "actual_minutes": item["actual_minutes"],
                         "stats": item["prediction"]["stats"], "actual_stats": item["actual_stats"]})
            if args.baseline_errors:
                b = baseline[(item["target"]["game_id"], item["target"]["player_id"])]
                if b["actual"] != item["actual_points"]:
                    raise ValueError("baseline outcomes differ")
                rows[-1]["baseline_model"] = b["predicted"]
    if not rows:
        raise ValueError("no predictions")
    payload = json.dumps(rows, separators=(",", ":")).replace("</", "<\\/")
    output = Path(args.out)
    output.parent.mkdir(parents=True, exist_ok=True)
    option = '<option value="baseline_model">Frozen previous model</option>' if args.baseline_errors else ""
    output.write_text(HTML.replace("__DATA__", payload).replace("__BASELINE_OPTION__", option))
    print(f"Wrote {output} ({len(rows)} player-games)")


HTML = r'''<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>NBA projection review</title>
<style>
body{margin:32px auto;padding:0 24px;max-width:1250px;font:15px system-ui;color:#17263b;background:#f6f8fc}h1{font-size:28px;margin-bottom:8px}p{line-height:1.55}select,input{padding:9px;border:1px solid #bcc8dc;border-radius:5px;background:white;font:inherit}label{display:inline-flex;gap:8px;align-items:center;margin:8px 14px 8px 0}.cards{display:flex;gap:12px;flex-wrap:wrap;margin:20px 0}.card{background:white;border:1px solid #dce3ee;border-radius:8px;padding:16px;min-width:155px}.card strong{font-size:25px;display:block}.card span{color:#54647c}.review{display:grid;grid-template-columns:420px 1fr;gap:24px}svg{width:100%;height:auto;background:white;border:1px solid #dce3ee;border-radius:8px}.scroll{overflow:auto;max-height:620px;border:1px solid #dce3ee;border-radius:8px}table{border-collapse:collapse;background:white;width:100%;font-variant-numeric:tabular-nums}th,td{padding:10px;text-align:right;white-space:nowrap;border-bottom:1px solid #edf0f6}th{position:sticky;top:0;background:#eaf0f9}th:first-child,td:first-child{text-align:left}tbody tr{cursor:pointer}tbody tr:hover{background:#eef5ff}.positive{color:#a03615}.negative{color:#176675}#detail{padding:18px;background:white;border-radius:8px;margin-top:16px}small{color:#54647c}@media(max-width:900px){.review{grid-template-columns:1fr}body{margin:20px auto}}
</style>
<h1>NBA projection review</h1>
<p>Compare pregame projections with actual production. All numbers are <b>conditional on playing positive minutes</b>; DNPs are excluded. The proxy is reconstructed without archived pregame odds or injury snapshots. Positive error means the projection was too high.</p>
<label>Game <select id="game"><option value="">All games</option></select></label>
<label>Player <input id="search" placeholder="Search by name"></label>
<label>Comparison <select id="method"><option value="model">Minutes × stat rates</option>__BASELINE_OPTION__<option value="proxy">Existing proxy (neutral market)</option><option value="proxy_if_active">Proxy if active</option><option value="recent_10">Last 10 average</option></select></label>
<label>Sort <select id="sort"><option value="error">Largest absolute error</option><option value="actual">Actual points</option><option value="player">Player name</option></select></label>
<div class="cards" id="cards"></div>
<div class="review"><div><svg id="plot" viewBox="0 0 420 420" role="img" aria-label="Actual versus projected fantasy points"></svg><p><small>Each dot is a player-game. Dots on the diagonal match the actual score. Select a row to inspect its stat predictions.</small></p></div><div class="scroll"><table><thead><tr><th>Player</th><th>Actual FP</th><th>Projected</th><th>Error</th><th>Min proj.</th><th>Min actual</th></tr></thead><tbody id="body"></tbody></table></div></div>
<div id="detail">Select a player-game to review its stat projections.</div>
<script type="application/json" id="data">__DATA__</script>
<script>
const data=JSON.parse(document.getElementById('data').textContent), $=id=>document.getElementById(id), fmt=n=>n.toFixed(2);
for(const id of [...new Set(data.map(x=>x.game))]){const opt=document.createElement('option');opt.value=id;const r=data.find(x=>x.game===id);opt.textContent=r.day+' · '+id;$('game').append(opt)}
function cell(row,text){const td=document.createElement('td');td.textContent=text;row.append(td);return td}
function detail(r){$('detail').replaceChildren();const title=document.createElement('h2');title.textContent=r.player+' · '+r.game;$('detail').append(title);const table=document.createElement('table'),head=document.createElement('tr');for(const x of ['Stat','Actual','Model']){const th=document.createElement('th');th.textContent=x;head.append(th)}table.append(head);for(const stat of Object.keys(r.stats)){const tr=document.createElement('tr');cell(tr,stat);cell(tr,fmt(r.actual_stats[stat]||0));cell(tr,fmt(r.stats[stat]));table.append(tr)}$('detail').append(table)}
function node(tag,attrs,text){const n=document.createElementNS('http://www.w3.org/2000/svg',tag);for(const [k,v]of Object.entries(attrs))n.setAttribute(k,v);if(text!==undefined)n.textContent=text;return n}
function render(){const key=$('method').value,query=$('search').value.toLowerCase(),game=$('game').value;let rows=data.filter(x=>(!game||x.game===game)&&x.player.toLowerCase().includes(query));const n=rows.length,err=rows.map(r=>r[key]-r.actual),mae=n?err.reduce((s,x)=>s+Math.abs(x),0)/n:0,rmse=n?Math.sqrt(err.reduce((s,x)=>s+x*x,0)/n):0,bias=n?err.reduce((s,x)=>s+x,0)/n:0;$('cards').replaceChildren();for(const [label,value]of [['Player-games',String(n)],['Mean absolute error',fmt(mae)],['Root mean square error',fmt(rmse)],['Bias',fmt(bias)]]){const card=document.createElement('div');card.className='card';const strong=document.createElement('strong');strong.textContent=value;const span=document.createElement('span');span.textContent=label;card.append(strong,span);$('cards').append(card)}
const order=$('sort').value;rows.sort((a,b)=>order==='player'?a.player.localeCompare(b.player):order==='actual'?b.actual-a.actual:Math.abs(b[key]-b.actual)-Math.abs(a[key]-a.actual));$('body').replaceChildren();for(const r of rows){const tr=document.createElement('tr');cell(tr,r.player);cell(tr,fmt(r.actual));cell(tr,fmt(r[key]));const error=cell(tr,fmt(r[key]-r.actual));error.className=r[key]>r.actual?'positive':'negative';cell(tr,fmt(r.minutes));cell(tr,fmt(r.actual_minutes));tr.onclick=()=>detail(r);$('body').append(tr)}
const svg=$('plot');svg.replaceChildren();const limit=Math.max(10,...rows.map(r=>Math.max(r.actual,r[key]))),p=v=>45+330*v/limit,y=v=>375-330*v/limit;svg.append(node('line',{x1:45,y1:375,x2:375,y2:375,stroke:'#8395af'}),node('line',{x1:45,y1:375,x2:45,y2:45,stroke:'#8395af'}),node('line',{x1:45,y1:375,x2:375,y2:45,stroke:'#8395af','stroke-dasharray':'5 5'}));for(let i=0;i<=4;i++){const v=limit*i/4;svg.append(node('text',{x:p(v),y:394,'font-size':11,'text-anchor':'middle'},v.toFixed(0)),node('text',{x:37,y:y(v)+4,'font-size':11,'text-anchor':'end'},v.toFixed(0)))}svg.append(node('text',{x:210,y:414,'font-size':12,'text-anchor':'middle'},'Actual fantasy points'),node('text',{x:210,y:22,'font-size':12,'text-anchor':'middle'},'Projected fantasy points'));for(const r of rows){const circle=node('circle',{cx:p(r.actual),cy:y(r[key]),r:3.2,fill:'#286fca',opacity:.5});circle.append(node('title',{},r.player+': actual '+fmt(r.actual)+', projected '+fmt(r[key])));circle.onclick=()=>detail(r);svg.append(circle)}}
for(const id of ['game','search','method','sort'])$(id).addEventListener('input',render);render();
</script></html>'''


if __name__ == "__main__":
    main()
