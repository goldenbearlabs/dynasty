<script lang="ts">
 import type { ResearchChartPoint } from '#lib/api.ts';
 import { formatValue } from './columns';
 let { points, kind = 'scatter', aggregation = 'mean', bins = 15, title = 'Custom research chart', xLabel = 'Games', yLabel = 'League+', loading = false }: {
  points: ResearchChartPoint[]; kind?: string; aggregation?: string; bins?: number; title?: string; xLabel?: string; yLabel?: string; loading?: boolean;
 } = $props();
 let svg = $state<SVGSVGElement>();
 let hover = $state('Hover over a mark to inspect its values.');
 const width = 920, height = 430, left = 85, right = 25, top = 30, bottom = 80;
 const palette = ['#2f4fe0', '#0b7285', '#c2255c', '#6741d9', '#d9510a', '#2b8a3e', '#a16a0a', '#8b91a3'];
 const valid = $derived(points.filter((p) => p.y !== null && Number.isFinite(p.y) && (kind !== 'scatter' || p.x !== null && Number.isFinite(p.x))));
 const groups = $derived([...new Set(valid.map((p) => p.group || 'Unknown'))].sort());
 const color = (group: string) => palette[Math.max(0, groups.indexOf(group || 'Unknown')) % palette.length];
 const bars = $derived.by(() => {
  if (kind === 'histogram') {
   const values = valid.map((p) => p.y!);
   const low = values.length ? Math.min(...values) : 0;
   const high = values.length ? Math.max(...values) : 1;
   const step = (high - low || 1) / Math.max(2, bins);
   const buckets = Array.from({ length: Math.max(2, bins) }, (_, i) => ({ label: `${formatValue(low + i*step)}–${formatValue(low + (i+1)*step)}`, value: 0, group: '', start: low + i*step }));
   for (const value of values) buckets[Math.min(buckets.length-1, Math.floor((value-low)/step))].value++;
   return buckets;
  }
  const map = new Map<string, number[]>();
  for (const p of valid) { const key = p.group || 'Unknown'; const values = map.get(key) ?? []; values.push(p.y!); map.set(key,values); }
  return [...map].map(([group, values]) => ({ group, label: group, start: 0, value: aggregation === 'count' ? values.length : values.reduce((a,b) => a+b,0) / (aggregation === 'mean' ? values.length : 1) })).sort((a,b) => b.value-a.value).slice(0,30);
 });
 const limits = $derived.by(() => {
  const xs = valid.map((p) => p.x!);
  const ys = kind === 'scatter' ? valid.map((p) => p.y!) : bars.map((b) => b.value);
  let xmin = kind === 'scatter' && xs.length ? Math.min(...xs) : 0;
  let xmax = kind === 'scatter' && xs.length ? Math.max(...xs) : Math.max(1,bars.length);
  let ymin = ys.length ? kind === 'scatter' ? Math.min(...ys) : Math.min(0,...ys) : 0;
  let ymax = ys.length ? Math.max(...ys) : 1;
  if (xmin === xmax) { xmin -= 0.5; xmax += 0.5; }
  if (ymin === ymax) { ymin -= 0.5; ymax += 0.5; }
  if (kind === 'scatter') { const pad=(ymax-ymin)*0.04; ymin-=pad; ymax+=pad; }
  return { xmin, xmax, ymin, ymax };
 });
 const sx = (value: number) => left + (value-limits.xmin)/(limits.xmax-limits.xmin)*(width-left-right);
 const sy = (value: number) => height-bottom - (value-limits.ymin)/(limits.ymax-limits.ymin)*(height-top-bottom);
 const sampled = $derived(valid.filter((_,i) => i % Math.max(1,Math.ceil(valid.length/2000)) === 0));
 const yTitle = $derived(kind === 'histogram' ? 'Player-season count' : kind === 'bar' ? `${aggregation === 'mean' ? 'Average' : aggregation === 'sum' ? 'Total' : 'Count'} ${aggregation === 'count' ? 'player-seasons' : yLabel}` : yLabel);
 function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = filename; a.click(); setTimeout(() => URL.revokeObjectURL(url),1000);
 }
 function exportSVG() {
  if (!svg) return;
  const clone = svg.cloneNode(true) as SVGSVGElement;
  clone.setAttribute('xmlns','http://www.w3.org/2000/svg');
  const original = svg.querySelectorAll('*'), copied = clone.querySelectorAll('*');
  original.forEach((node,i) => { const cs = getComputedStyle(node); for (const key of ['fill','stroke','stroke-dasharray','stroke-width','font-family','font-size','font-weight','opacity']) copied[i].setAttribute(key, cs.getPropertyValue(key)); });
  const bg = document.createElementNS('http://www.w3.org/2000/svg','rect'); bg.setAttribute('width',String(width));bg.setAttribute('height',String(height));bg.setAttribute('fill',getComputedStyle(svg).backgroundColor);clone.prepend(bg);
  download(new Blob([clone.outerHTML],{type:'image/svg+xml'}),'research-chart.svg');
 }
 function exportCSV() {
  const quote = (value: string) => `"${value.replaceAll('"','""')}"`;
  const rows = [['Player-season','Group',xLabel,yLabel].map(quote).join(','),...points.map((p) => [quote(p.label),quote(p.group),p.x ?? '',p.y ?? ''].join(','))];
  download(new Blob([rows.join('\n')],{type:'text/csv'}),'research-chart.csv');
 }
</script>
<div class="chart" aria-busy={loading}>
 <div class="spread chart-heading"><div><h3>{title}</h3><p class="muted small-text">{valid.length.toLocaleString()} observations with values · {points.length-valid.length} missing values omitted</p></div><div class="row"><button class="small quiet" disabled={!valid.length} onclick={exportSVG}>Download SVG</button><button class="small quiet" disabled={!points.length} onclick={exportCSV}>Download CSV</button></div></div>
 {#if valid.length}
  <svg bind:this={svg} viewBox="0 0 {width} {height}" role="img" aria-label="{title}: {kind} chart of {yTitle}" class:loading>
   <title>{title}</title><desc>{valid.length} observations. {kind === 'scatter' ? `${xLabel} on the horizontal axis and ${yLabel} on the vertical axis.` : yTitle}.</desc>
   {#each Array.from({length: 6},(_,i) => limits.ymin+i*(limits.ymax-limits.ymin)/5) as tick (tick)}
    <line x1={left} x2={width-right} y1={sy(tick)} y2={sy(tick)} class="grid" /><text x={left-12} y={sy(tick)+4} text-anchor="end">{formatValue(tick)}</text>
   {/each}
   <line x1={left} x2={width-right} y1={kind === 'scatter' ? height-bottom : sy(0)} y2={kind === 'scatter' ? height-bottom : sy(0)} class="axis" />
   <text x="20" y={(height-bottom+top)/2} text-anchor="middle" transform="rotate(-90 20 {(height-bottom+top)/2})" class="axis-title">{yTitle}</text>
   {#if kind === 'scatter'}
    {#if ['League+','Position+'].includes(yLabel) && limits.ymin<100 && limits.ymax>100}<line x1={left} x2={width-right} y1={sy(100)} y2={sy(100)} class="reference" /><text x={width-right-4} y={sy(100)-7} text-anchor="end">Average · 100</text>{/if}
    {#each Array.from({length:6},(_,i) => limits.xmin+i*(limits.xmax-limits.xmin)/5) as tick (tick)}<text x={sx(tick)} y={height-bottom+22} text-anchor="middle">{formatValue(tick)}</text>{/each}
    {#each sampled as p (p.key)}<circle role="button" aria-label={p.label} onclick={() => hover = `${p.label} · ${xLabel}: ${formatValue(p.x)} · ${yLabel}: ${formatValue(p.y)}`} onkeydown={(e) => { if(e.key==='Enter' || e.key===' ') hover = `${p.label} · ${xLabel}: ${formatValue(p.x)} · ${yLabel}: ${formatValue(p.y)}`; }} tabindex="0" onfocus={() => hover = `${p.label} · ${xLabel}: ${formatValue(p.x)} · ${yLabel}: ${formatValue(p.y)}`} cx={sx(p.x!)} cy={sy(p.y!)} r="4.2" fill={color(p.group)} opacity="0.65" onmouseenter={() => hover = `${p.label} · ${xLabel}: ${formatValue(p.x)} · ${yLabel}: ${formatValue(p.y)}`}><title>{p.label}: {xLabel} {formatValue(p.x)}, {yLabel} {formatValue(p.y)}</title></circle>{/each}
    <text x={(width+left-right)/2} y={height-15} text-anchor="middle" class="axis-title">{xLabel}</text>
   {:else}
    {#each bars as b,i (i)}
     {@const bw = (width-left-right)/Math.max(1,bars.length)}
     <rect role="button" aria-label={b.label} onclick={() => hover = `${b.label} · ${yTitle}: ${formatValue(b.value)}`} onkeydown={(e) => { if(e.key==='Enter' || e.key===' ') hover = `${b.label} · ${yTitle}: ${formatValue(b.value)}`; }} tabindex="0" onfocus={() => hover = `${b.label} · ${yTitle}: ${formatValue(b.value)}`} x={left+i*bw+3} width={Math.max(1,bw-6)} y={Math.min(sy(0),sy(b.value))} height={Math.max(1,Math.abs(sy(b.value)-sy(0)))} fill={kind === 'histogram' ? palette[0] : color(b.group)} opacity="0.8" onmouseenter={() => hover = `${b.label} · ${yTitle}: ${formatValue(b.value)}`}><title>{b.label}: {formatValue(b.value)}</title></rect>
     <text x={left+(i+0.5)*bw} y={height-bottom+15} text-anchor="end" transform="rotate(-35 {left+(i+0.5)*bw} {height-bottom+15})">{b.label.length>22 ? `${b.label.slice(0,20)}…` : b.label}</text>
    {/each}
   {/if}
  </svg>
  <p class="inspection small-text">{hover}</p>
  {#if kind === 'scatter'}<div class="row legend">{#each groups.slice(0,24) as group (group)}<span><i style:background={color(group)}></i>{group}</span>{/each}</div>{/if}
  {#if kind === 'scatter' && sampled.length < valid.length}<p class="muted small-text">Dots use an even sample of {sampled.length} observations for readability. The CSV includes every returned chart observation.</p>{/if}
  {#if kind === 'bar' && groups.length>30}<p class="muted small-text">Showing the 30 highest groups.</p>{/if}
 {:else if loading}<div class="empty">Loading chart observations…</div>{:else}<div class="empty">No observations have values for these axes. Try Games or raw stats, or lower the benchmark minimum for advanced metrics.</div>{/if}
</div>
<style>
 .chart { padding: 1rem; border: 1px solid var(--rule); border-radius: var(--radius); background: var(--surface); }
 .chart-heading { margin-bottom: 1rem; }
 h3 { font-size: 1rem; }
 svg { display: block; width: 100%; background: var(--surface); min-height: 230px; }
 svg.loading { opacity: 0.45; }
 text { fill: var(--ink-soft); font: 11px var(--body); }
 .grid { stroke: var(--rule); stroke-dasharray: 3 5; }
 .reference { stroke: var(--ink-faint); stroke-dasharray: 6 5; }
 .axis { stroke: var(--ink-faint); }
 .axis-title { font-weight: 600; font-size: 12px; }
 circle:hover, rect:hover { opacity: 1; }
 .inspection { padding: 0.5rem 0.7rem; background: var(--surface-2); border-radius: var(--radius-small); color: var(--ink-soft); }
 .legend { margin-top: 0.7rem; gap: 0.5rem 1rem; font-size: 0.72rem; color: var(--ink-soft); }
 .legend span { display: inline-flex; align-items: center; gap: 0.35rem; }
 .legend i { width: 0.5rem; height: 0.5rem; border-radius: 50%; }
 .empty { padding: 4rem 1rem; color: var(--ink-soft); text-align: center; }
</style>
