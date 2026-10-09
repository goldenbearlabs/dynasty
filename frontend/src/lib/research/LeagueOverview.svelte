<script lang="ts">
 import type { Competition, ResearchPage, ResearchChartPoint } from '#lib/api.ts';
 import ResearchChart from './ResearchChart.svelte';
 import SportBadge from '#lib/ui/SportBadge.svelte';
 import { formatValue } from './columns';
 let { result, competitions }: { result: ResearchPage; competitions: Competition[] } = $props();
 let preset = $state('position');
 const name = (key: string) => competitions.find((c) => c.key === key)?.name ?? key.toUpperCase();
 const presets = {
  position: { title: 'Where each league is strongest', subtitle: 'Position average on the League+ scale. 100 is the league-season average. Multi-position players appear in each eligible position.', y: 'Position average League+' },
  scoring: { title: 'What drives the scoring', subtitle: 'Each stat’s signed share of absolute weighted scoring contributions. This compares scoring makeup, not raw point totals.', y: 'Weighted scoring share %' },
  concentration: { title: 'How concentrated is the production?', subtitle: 'Share of positive qualified fantasy points produced by the top ten players. Smaller imported populations naturally have greater concentration.', y: 'Top 10 production share %' },
  trends: { title: 'Compare season scoring environments', subtitle: 'Median FP/game by league-season, using current scoring rules. Compare seasons within a sport; raw point scales differ between sports.', y: 'Median FP / game' }
 };
 const selected = $derived(presets[preset as keyof typeof presets]);
 const points = $derived.by((): ResearchChartPoint[] => {
  if (preset === 'position') return result.benchmarks.filter((b) => b.position).map((b) => {
   const base = result.benchmarks.find((a) => a.competition === b.competition && a.season === b.season && !a.position);
   const group = `${name(b.competition)} ${b.season} · ${b.position}`;
   return { key: group, label: group, group, x: b.players, y: base && base.sd > 1e-9 && b.players >= 2 ? 100+15*(b.mean-base.mean)/base.sd : null };
  });
  if (preset === 'scoring') return result.analysis.flatMap((a) => {
   const total = Object.values(a.contributions).reduce((n,v) => n+Math.abs(v),0);
   return Object.entries(a.contributions).map(([key,value]) => {
    const stat = competitions.find((c) => c.key === a.competition)?.stats.find((s) => s.key === key)?.label ?? key;
    const group = `${name(a.competition)} ${a.season} · ${stat}`;
    return { key: group, label: group, group, x: null, y: total > 0 ? 100*value/total : null };
   });
  });
  return result.analysis.map((a) => { const group = `${name(a.competition)} ${a.season}`; return { key: group, label: group, group, x: a.qualified, y: a.qualified ? preset === 'concentration' ? a.top_ten_share : a.median : null }; });
 });
</script>
<div class="stack">
 <div class="spread"><div><h2>League-wide analysis</h2><p class="muted">Explore league depth, position strengths and scoring patterns across your selected datasets.</p></div><label class="control"><span>Analysis preset</span><select aria-label="Analysis preset" bind:value={preset}><option value="position">Position strengths</option><option value="scoring">Scoring drivers</option><option value="concentration">Production concentration</option><option value="trends">Season scoring trends</option></select></label></div>
 <p class="notice small-text">These profiles use the complete selected datasets, independently of player search and display filters. Imported coverage may include only players known to the feeds, especially in older seasons.</p>
 <p class="muted small-text">{selected.subtitle}</p>
 <ResearchChart {points} kind="bar" aggregation="mean" title={selected.title} xLabel="Qualified players" yLabel={selected.y} />
 <div class="profiles">
  {#each result.analysis as a (`${a.competition}-${a.season}`)}
   {@const b = result.benchmarks.find((b) => b.competition === a.competition && b.season === a.season && !b.position)}
   <article class="card profile"><div class="row"><SportBadge sport={a.competition} /><h3>{a.season}</h3><span class="pill">{a.scoring_source === 'league' ? 'League rules' : 'Sport defaults'}</span></div>
    <dl><div><dt>Imported players</dt><dd>{a.players.toLocaleString()}</dd></div><div><dt>Benchmark players</dt><dd>{a.qualified.toLocaleString()}</dd></div><div><dt>Median FP/game</dt><dd>{a.qualified ? formatValue(a.median) : '—'}</dd></div><div><dt>Top 10 share</dt><dd>{a.qualified ? `${formatValue(a.top_ten_share)}%` : '—'}</dd></div><div><dt>Average FP/game</dt><dd>{formatValue(b?.mean)}</dd></div><div><dt>90th percentile FP/game</dt><dd>{a.qualified ? formatValue(a.p90) : '—'}</dd></div></dl>
    <p class="muted small-text">{a.scored} / {a.players} imported players have stats matching the scoring rules.</p>
   </article>
  {/each}
 </div>
 <details class="card depth"><summary>Inspect position depth and replacement baselines</summary><div class="scroll"><table><thead><tr><th>League-season</th><th>Position</th><th class="num">Qualified players</th><th class="num">Average FP/game</th><th class="num">Replacement rank</th><th class="num">Replacement FP/game</th></tr></thead><tbody>
  {#each result.benchmarks.filter((b) => b.position) as b (`${b.competition}-${b.season}-${b.position}`)}<tr><td>{name(b.competition)} {b.season}</td><td>{b.position}</td><td class="num">{b.players}</td><td class="num">{formatValue(b.mean)}</td><td class="num">{b.replacement_rank ?? '—'}</td><td class="num">{formatValue(b.replacement_rate)}</td></tr>{/each}
 </tbody></table></div></details>
</div>
<style>
 h2 { font-size: 1.2rem; }
 h3 { font-size: 1rem; }
 .control { display: grid; gap: 0.25rem; font-size: 0.75rem; color: var(--ink-soft); }
 .notice { padding: 0.65rem 0.8rem; background: var(--surface-2); border-radius: var(--radius-small); color: var(--ink-soft); }
 .profiles { display: grid; grid-template-columns: repeat(auto-fit,minmax(18rem,1fr)); gap: 0.8rem; }
 .profile, .depth { padding: 1rem; }
 dl { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-block: 1rem; }
 dt { font-size: 0.7rem; color: var(--ink-soft); }
 dd { margin: 0.2rem 0 0; font: 700 1.25rem var(--display); }
 summary { font-weight: 600; cursor: pointer; }
 .depth .scroll { margin-top: 1rem; }
 th.num { text-align: right; }
</style>
