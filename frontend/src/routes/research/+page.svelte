<script lang="ts">
 import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
 import PlayerSearch from '#lib/PlayerSearch.svelte';
 import { onMount, untrack } from 'svelte';
 import { page as route } from '$app/state';
 import { getResearch, type ResearchPage, type ResearchPool, type ResearchPlayer } from '#lib/api.ts';
 import Tabs from '#lib/ui/Tabs.svelte';
 import Headshot from '#lib/ui/Headshot.svelte';
 import SportBadge from '#lib/ui/SportBadge.svelte';
 import Icon from '#lib/ui/Icon.svelte';
 import Empty from '#lib/ui/Empty.svelte';
 import PlayerResearch from '../draft/[id]/Research.svelte';
 import ColumnHeader from '#lib/research/ColumnHeader.svelte';
 import ResearchChart from '#lib/research/ResearchChart.svelte';
 import LeagueOverview from '#lib/research/LeagueOverview.svelte';
 import { basics, advanced, formatValue, type Column } from '#lib/research/columns.ts';
 import type { PageProps } from './$types';

 let { data }: PageProps = $props();
 let pools = $state<ResearchPool[]>([]);
 let addCompetition = $state('nhl');
 let addSeason = $state('');
 let workspace = $state('players');
 let view = $state('overview');
 let rawMode = $state('totals');
 let competition = $state(untrack(() => route.url.searchParams.get('competition') ?? ''));
 let status = $state('');
 let search = $state('');
 let query = $state('');
 let selectedPositions = $state<string[]>([]);
 let team = $state('');
 let owner = $state('');
 let minGames = $state<number | undefined>();
 let maxGames = $state<number | undefined>();
 let minRate = $state<number | undefined>();
 let maxRate = $state<number | undefined>();
 let minIndex = $state<number | undefined>();
 let maxIndex = $state<number | undefined>();
 let minPAR = $state<number | undefined>();
 let statKey = $state('');
 let minStat = $state<number | undefined>();
 let maxStat = $state<number | undefined>();
 let qualifiedOnly = $state(false);
 let missingOnly = $state(false);
 let aboveReplacement = $state(false);
 let benchmarkGames = $state<number | undefined>(5);
 let pitcherWorkloadPercent = $state<number | undefined>(25);
 let replacementRank = $state<number | undefined>(0);
 let sort = $state('league_index');
 let ascending = $state(false);
 let page = $state(1);
 let selectedId = $state<string>();
 let result = $state<ResearchPage>();
 let loading = $state(true);
 let error = $state('');
 let retry = $state(0);
 let restored = $state(false);
 let chartKind = $state('scatter');
 let chartX = $state('games');
 let chartY = $state('league_index');
 let chartGroup = $state('cohort');
 let chartAggregation = $state('mean');
 let chartBins = $state<number | undefined>(15);
 let chartTitle = $state('Production and opportunity');

 onMount(() => {
  try {
   const saved = JSON.parse(localStorage.getItem('research-pools') ?? '[]');
   if (Array.isArray(saved)) pools = saved.filter((p) => typeof p?.season === 'string' && data.competitions.some((c) => c.key === p?.competition)).slice(0,24);
  } catch { /* Start from the default pool when saved settings are invalid. */ }
  if (!data.competitions.some((c) => c.key === addCompetition)) addCompetition = data.competitions[0]?.key ?? '';
  restored = true;
 });
 $effect(() => { if (restored) { try { localStorage.setItem('research-pools',JSON.stringify(pools)); } catch { /* Storage is optional. */ } } });
 const sportName = (key: string) => data.competitions.find((c) => c.key === key)?.name ?? key.toUpperCase();
 const datasetCompetitions = $derived(data.competitions.filter((c) => !pools.length || pools.some((p) => p.competition === c.key)));
 const positions = $derived([...new Set(datasetCompetitions.flatMap((c) => c.positions))].sort());
 const seasonChoices = $derived(result?.catalog.filter((c) => c.competition === addCompetition) ?? []);
 const rawColumns = $derived.by((): Column[] => {
  const columns = new Map<string,Column>();
  for (const c of datasetCompetitions.filter((c) => !competition || c.key===competition)) for (const stat of c.stats) {
   columns.set(stat.key, { key: stat.key, label: stat.key.replaceAll('_',' ').toUpperCase(), help: `${stat.label}. Imported raw season ${rawMode === 'totals' ? 'total' : 'total divided by games played'}. This stat is visible even when it does not earn fantasy points.` });
  }
  for (const key of result?.raw_stat_keys ?? []) if (!columns.has(key)) columns.set(key, { key, label: key.replaceAll('_',' ').toUpperCase(), help: `Imported ${key} season stat. Missing values are shown as a dash.` });
  return [...columns.values()];
 });
 const metricColumns = $derived(view === 'raw' ? [] : view === 'overview' ? advanced.filter((c) => ['league_index','percentile','points_above_replacement'].includes(c.key)) : advanced);
 const chartMetrics = $derived([...basics,...advanced,...rawColumns.map((c) => ({...c,label:`${c.label} · raw total`}))]);
 const metricLabel = (key: string) => chartMetrics.find((c) => c.key === key)?.label ?? key;
 const qualifiedCount = $derived(result?.benchmarks.filter((b) => !b.position).reduce((n,b) => n+b.players,0) ?? 0);
 const pages = $derived(result ? Math.max(1,Math.ceil(result.total/result.per_page)) : 1);
 const activeFilters = $derived([competition,status,query,selectedPositions.length>0,team,owner,minGames!==undefined,maxGames!==undefined,minRate!==undefined,maxRate!==undefined,minIndex!==undefined,maxIndex!==undefined,minPAR!==undefined,statKey,qualifiedOnly,missingOnly,aboveReplacement].filter(Boolean).length);
 const poolCount = $derived(pools.length || result?.analysis.length || 0);
 function addPool() {
  if (pools.length >= 24 || pools.some((p) => p.competition === addCompetition && p.season === addSeason)) return;
  pools = [...pools,{competition:addCompetition,season:addSeason}];
  competition = ''; team = ''; selectedPositions = []; selectedId = undefined;
 }
 function removePool(index: number) { pools = pools.filter((_,i) => i !== index); competition = ''; team = ''; selectedPositions = []; }
 function clearFilters() {
  competition = ''; status = ''; search = ''; query = ''; selectedPositions = []; team = ''; owner = '';
  minGames = undefined; maxGames = undefined; minRate = undefined; maxRate = undefined;
  minIndex = undefined; maxIndex = undefined; minPAR = undefined; statKey = ''; minStat = undefined; maxStat = undefined;
  qualifiedOnly = false; missingOnly = false; aboveReplacement = false;
 }
 function sortBy(key: string) {
  if (sort === key) ascending = !ascending;
  else { sort = key; ascending = ['name','team','competition','season','owner'].includes(key); }
 }
 function quickPreset(kind: string) {
  clearFilters(); view = 'overview'; ascending = false;
  if (kind === 'free') { owner = 'available'; qualifiedOnly = true; aboveReplacement = true; sort = 'points_above_replacement'; }
  else if (kind === 'durable') { qualifiedOnly = true; sort = 'availability'; }
  else { qualifiedOnly = true; sort = 'league_index'; }
 }
 function rawValue(player: ResearchPlayer, key: string) {
  const value = player.stats[key];
  return value === undefined || rawMode === 'per_game' && player.games <= 0 ? '—' : formatValue(rawMode === 'per_game' ? value/player.games : value);
 }
 $effect(() => {
  const value = search.trim(); const timer = setTimeout(() => query = value,250); return () => clearTimeout(timer);
 });
 let previousFilters = '';
 $effect.pre(() => {
  const key = JSON.stringify([pools,competition,status,query,selectedPositions,team,owner,minGames,maxGames,minRate,maxRate,minIndex,maxIndex,minPAR,statKey,minStat,maxStat,qualifiedOnly,missingOnly,aboveReplacement,benchmarkGames,pitcherWorkloadPercent,replacementRank,sort,ascending,rawMode]);
  if (key !== previousFilters) { previousFilters = key; page = 1; }
 });
 $effect(() => {
  void retry;
  let active = true; loading = true; error = '';
  getResearch({ pools:pools.length ? JSON.stringify(pools) : undefined, competition, status, q:query, sort, page,
   position:selectedPositions.join(','), team, owner, min_games:minGames, max_games:maxGames, min_rate:minRate, max_rate:maxRate,
   min_index:minIndex, max_index:maxIndex, min_par:minPAR, stat_key:statKey, min_stat:minStat, max_stat:maxStat,
   qualified_only:String(qualifiedOnly), missing_only:String(missingOnly), above_replacement:String(aboveReplacement),
   benchmark_games:benchmarkGames ?? 5, pitcher_workload_percent:pitcherWorkloadPercent ?? 25, replacement_rank:replacementRank ?? 0, ascending:String(ascending),
   raw_per_game:String((view==='raw' || view==='all') && rawMode==='per_game'), include_chart:String(workspace === 'charts'), chart_x:chartX, chart_y:chartY, chart_group:chartGroup })
   .then((r) => { if (active) { result = r; page = r.page; } })
   .catch((e: Error) => { if (active) error = e.message; })
   .finally(() => { if (active) loading = false; });
  return () => active = false;
 });
</script>

<svelte:head><title>Research lab</title></svelte:head>

<div class="stack research-page">
 <header class="spread"><div class="stack tight"><div class="row"><span class="eyebrow">Fantasy intelligence</span><span class="pill brand">{poolCount} datasets</span></div><h1>Research lab</h1><p class="muted">Compare player seasons, build your own charts, and discover where each league is strongest.</p></div></header>
 <PlayerSearch competitions={data.competitions} />

 <section class="card dataset-card" aria-label="Research datasets">
  <div class="spread"><div><h2><span class="step">1</span> Choose your research pool</h2><p class="muted small-text">Add league–season datasets to compare them side by side. Each keeps its own scoring and benchmarks.</p></div>{#if pools.length}<button class="quiet small" onclick={() => { pools = []; clearFilters(); }}>Use latest from all leagues</button>{/if}</div>
  <div class="row pool-builder">
   <label class="control"><span>League</span><select aria-label="Dataset league" bind:value={addCompetition} onchange={() => addSeason = ''}>{#each data.competitions as c (c.key)}<option value={c.key}>{c.name}</option>{/each}</select></label>
   <label class="control"><span>Season</span><select aria-label="Dataset season" bind:value={addSeason}><option value="">Latest imported season</option>{#each seasonChoices as s (`${s.year}-${s.label}`)}<option value={s.label}>{s.label} · {s.players} players</option>{/each}</select></label>
   <button class="primary add-dataset" onclick={addPool} disabled={pools.length>=24 || pools.some((p) => p.competition===addCompetition && p.season===addSeason)}><Icon name="plus" size={15} /> Add dataset</button>
  </div>
  <div class="row pool-chips">
   {#if pools.length}{#each pools as p,i (`${p.competition}-${p.season}`)}<span class="dataset-chip"><SportBadge sport={p.competition} /><strong>{p.season || 'Latest'}</strong><button class="quiet" aria-label="Remove {sportName(p.competition)} {p.season || 'latest'}" onclick={() => removePool(i)}><Icon name="x" size={13} /></button></span>{/each}
   {:else}<span class="pill">All leagues · latest imported seasons</span><span class="muted small-text">Adding your first dataset switches to a custom pool.</span>{/if}
  </div>
 </section>

 {#if result?.warnings.length}<details class="data-notes"><summary><Icon name="info" size={15} /> Scoring &amp; data notes ({result.warnings.length})</summary><div>{#each result.warnings as warning (warning)}<p>{warning}</p>{/each}</div></details>{/if}

 <section class="stack tight">
  {#if result?.starting_conferences?.cbb && (pools.length === 0 || pools.some((p) => p.competition === 'cbb'))}
   <p class="muted small-text"><strong>CBB starting conferences:</strong> {result.starting_conferences.cbb.length ? result.starting_conferences.cbb.map((id) => data.competitions.find((c) => c.key === 'cbb')?.conferences?.find((c) => c.id === id)?.name ?? id).join(', ') : 'All conferences'}. Research uses season-specific membership. Other conferences stay available for drafts and reserves. The commissioner can change this in league settings.</p>
  {/if}
  <div class="spread"><h2><span class="step">2</span> Explore your pool</h2><span class="muted small-text">{qualifiedCount.toLocaleString()} benchmark players · minimum {result?.benchmark_games ?? 5} games · pitchers also need {result?.pitcher_workload_percent ?? 25}% of their peer group’s appearances and innings</span></div>
  <Tabs tabs={[{value:'players',label:'Players'},{value:'charts',label:'Custom charts'},{value:'leagues',label:'League-wide analysis'}]} bind:value={workspace} label="Research workspace" />
 </section>

 {#if workspace !== 'leagues'}
  <section class="card filter-card stack tight" aria-label="Player filters">
   <div class="spread"><div class="row"><h3>Filter player seasons</h3>{#if activeFilters}<span class="pill brand">{activeFilters} active</span>{/if}</div><button class="quiet small" onclick={clearFilters}>Clear filters</button></div>
   <div class="row filters">
    <label class="search"><Icon name="search" size={16} /><input type="search" placeholder="Find a player…" aria-label="Search players" bind:value={search} /></label>
    <label class="control"><span>League</span><select bind:value={competition}><option value="">All selected leagues</option>{#each datasetCompetitions as c (c.key)}<option value={c.key}>{c.name}</option>{/each}</select></label>
    <details class="position-picker"><summary>Positions {selectedPositions.length ? `(${selectedPositions.length})` : '· any'}</summary><div class="position-options">{#each positions as pos (pos)}<label class="check"><input type="checkbox" value={pos} bind:group={selectedPositions} />{pos}</label>{/each}</div></details>
    <label class="control"><span>Roster</span><select bind:value={owner}><option value="">Any owner</option><option value="available">Available players</option><option value="rostered">Rostered players</option>{#each data.dynasty?.franchises ?? [] as f (f.id)}<option value={f.slug}>{f.name}</option>{/each}</select></label>
    <label class="control numeric"><span>Min games</span><input type="number" min="0" max="10000" placeholder="Any" bind:value={minGames} /></label>
   </div>
   <details class="more-filters"><summary>More filters <span class="muted">· team, status, production ranges &amp; raw stats</span></summary><div class="expanded-filters">
    <label class="control"><span>Team (season)</span><select bind:value={team}><option value="">Any team</option>{#each result?.teams ?? [] as t (t)}<option value={t}>{t}</option>{/each}</select></label>
    <label class="control"><span>Current player status</span><select bind:value={status}><option value="">Any status</option><option value="active">Active</option><option value="prospect">Prospect</option><option value="inactive">Inactive</option></select></label>
    <label class="control"><span>Max games</span><input type="number" min="0" max="10000" placeholder="No limit" bind:value={maxGames} /></label>
    <label class="control"><span>Min FP/game</span><input type="number" step="0.1" placeholder="No minimum" bind:value={minRate} /></label>
    <label class="control"><span>Max FP/game</span><input type="number" step="0.1" placeholder="No limit" bind:value={maxRate} /></label>
    <label class="control"><span>Min League+</span><input type="number" min="0" placeholder="No minimum" bind:value={minIndex} /></label>
    <label class="control"><span>Max League+</span><input type="number" min="0" placeholder="No limit" bind:value={maxIndex} /></label>
    <label class="control"><span>Min PAR/game</span><input type="number" step="0.1" placeholder="No minimum" bind:value={minPAR} /></label>
    <label class="control"><span>Raw stat to filter</span><select bind:value={statKey}><option value="">No raw stat filter</option>{#each rawColumns as c (c.key)}<option value={c.key}>{c.label}</option>{/each}</select></label>
    <label class="control"><span>Raw season total ≥</span><input type="number" step="any" disabled={!statKey} bind:value={minStat} placeholder="No minimum" /></label>
    <label class="control"><span>Raw season total ≤</span><input type="number" step="any" disabled={!statKey} bind:value={maxStat} placeholder="No limit" /></label>
    <div class="stack tight checks"><label class="check"><input type="checkbox" bind:checked={qualifiedOnly} />Qualified sample only</label><label class="check"><input type="checkbox" bind:checked={aboveReplacement} />Above replacement only</label><label class="check"><input type="checkbox" bind:checked={missingOnly} />Missing scoring stats only</label></div>
   </div></details>
  </section>
 {/if}

 {#if error}<div role="alert" class="row"><p>Could not load research: {error}</p><button onclick={() => retry++}>Retry</button></div>
 {:else if result}
  {#if workspace === 'leagues'}<LeagueOverview {result} competitions={data.competitions} />
  {:else if workspace === 'charts'}
   <section class="stack">
    <div><h2>Build a custom chart</h2><p class="muted small-text">Charts use every filtered player-season, across all table pages. Change the axes and grouping to ask your own question.</p></div>
    <div class="row presets"><span class="muted small-text">Start with</span><button class="small quiet" onclick={() => { chartKind='scatter';chartX='games';chartY='league_index';chartGroup='cohort';chartTitle='Production and opportunity'; }}>Opportunity vs production</button><button class="small quiet" onclick={() => {chartKind='histogram';chartY='points_per_game';chartTitle='How production is distributed';}}>Scoring distribution</button><button class="small quiet" onclick={() => {chartKind='bar';chartGroup='position';chartY='par_per_game';chartAggregation='mean';chartTitle='Replacement value by position';}}>Position value</button></div>
    <div class="card chart-controls">
     <label class="control"><span>Chart title</span><input bind:value={chartTitle} /></label>
     <label class="control"><span>Chart type</span><select aria-label="Chart type" bind:value={chartKind}><option value="scatter">Scatter plot</option><option value="bar">Grouped bars</option><option value="histogram">Distribution / histogram</option></select></label>
     {#if chartKind === 'scatter'}<label class="control"><span>Horizontal axis</span><select aria-label="Horizontal axis" bind:value={chartX}>{#each chartMetrics as c (c.key)}<option value={c.key}>{c.label}</option>{/each}</select></label>{/if}
     <label class="control"><span>{chartKind === 'histogram' ? 'Value to distribute' : 'Vertical axis'}</span><select aria-label="Vertical axis" bind:value={chartY}>{#each chartMetrics as c (c.key)}<option value={c.key}>{c.label}</option>{/each}</select></label>
     {#if chartKind !== 'histogram'}<label class="control"><span>{chartKind==='scatter' ? 'Color by' : 'Group by'}</span><select aria-label="Chart grouping" bind:value={chartGroup}><option value="cohort">League + season</option><option value="position">Comparison position</option><option value="team">Season team</option><option value="owner">Current roster owner</option></select></label>{/if}
     {#if chartKind === 'bar'}<label class="control"><span>Aggregate</span><select bind:value={chartAggregation}><option value="mean">Average</option><option value="sum">Total</option><option value="count">Player-season count</option></select></label>{/if}
     {#if chartKind === 'histogram'}<label class="control"><span>Number of bins</span><input type="number" min="2" max="40" bind:value={chartBins} /></label>{/if}
    </div>
    {#if result.chart_total>20000}<p class="muted small-text">Charts are limited to the first 20,000 filtered observations in the current sort order. Narrow your pool to include every observation.</p>{/if}
    <ResearchChart points={result.chart} kind={chartKind} aggregation={chartAggregation} bins={Math.max(2,Math.min(40,chartBins ?? 15))} title={chartTitle} xLabel={metricLabel(chartX)} yLabel={metricLabel(chartY)} {loading} />
   </section>
  {:else}
   <div class="spread table-toolbar"><div class="row"><h2>{result.total.toLocaleString()} player-seasons</h2><span class="muted small-text">{loading ? 'Updating…' : `Sorted by ${sort==='name' ? 'player name' : metricLabel(sort)} ${ascending ? '↑' : '↓'} · Hover to learn, click to sort.`}</span></div><label class="control"><span>Table view</span><select aria-label="Table view" bind:value={view}><option value="overview">Player overview</option><option value="advanced">Advanced metrics</option><option value="raw">Raw stats</option><option value="all">All columns</option></select></label></div>
   <div class="row presets"><span class="muted small-text">Quick looks</span><button class="small quiet" onclick={() => quickPreset('top')}>Top producers</button><button class="small quiet" onclick={() => quickPreset('free')}>Available above replacement</button><button class="small quiet" onclick={() => quickPreset('durable')}>Most games coverage</button>{#if view==='raw' || view==='all'}<label class="row small-text"><span>Raw stats</span><select aria-label="Raw stat units" bind:value={rawMode}><option value="totals">Season totals</option><option value="per_game">Per game</option></select></label>{/if}</div>
   {#if result.players.length===0}<Empty icon="search" title="No player-seasons match">Clear a filter, lower a minimum, or add another league-season dataset.</Empty>
   {:else}<div class="panel scroll research-table" class:loading aria-busy={loading}><table><thead><tr>
    <ColumnHeader sticky label="Player" help="One row per player, league and imported season. Click a name for current player history. The same player may appear in several seasons." active={sort==='name'} {ascending} onsort={() => sortBy('name')} />
    <ColumnHeader label="League / season" help="The competition where these season stats were recorded. League+ and replacement benchmarks are calculated separately for each league-season." active={sort==='competition'} {ascending} onsort={() => sortBy('competition')} />
    <ColumnHeader label="Position" help="Current eligible positions from the player feed. Historical position changes are not archived." />
    <ColumnHeader label="Team" help="Teams listed in this imported season. Players who changed teams have their totals combined. This may differ from their current team." active={sort==='team'} {ascending} onsort={() => sortBy('team')} />
    {#each basics as c (c.key)}<ColumnHeader label={c.label} help={c.help} active={sort===c.key} {ascending} numeric onsort={() => sortBy(c.key)} />{/each}
    {#each metricColumns as c (c.key)}<ColumnHeader label={c.label} help={c.help} active={sort===c.key} {ascending} numeric onsort={() => sortBy(c.key)} />{/each}
    {#if view==='advanced' || view==='all'}<ColumnHeader label="Compared at" help="Position used for Position+ and replacement value. With multiple eligible positions, the most favorable replacement advantage is used. Position filters restrict comparison to the selected eligible positions." />{/if}
    {#if view==='raw' || view==='all'}{#each rawColumns as c (c.key)}<ColumnHeader label={c.label} help={c.help} active={sort===c.key} {ascending} numeric onsort={() => sortBy(c.key)} />{/each}{/if}
    <ColumnHeader label="Roster owner" help="Current fantasy owner in the competition, rather than historical ownership in this season." active={sort==='owner'} {ascending} onsort={() => sortBy('owner')} />
   </tr></thead><tbody>
    {#each result.players as player (player.row_key)}<tr class:selected={selectedId===player.id}>
     <td class="identity"><div class="player"><Headshot name={player.full_name} src={player.headshot_url} size={32} /><div><a class="player-name" href="/player/{player.id}">{player.full_name}</a><InjuryBadge designation={player.injury_designation} /><button class="preview-link" onclick={() => selectedId=player.id} aria-label="Quick look at {player.full_name}">Quick look</button><div class="small-text muted">{#if !player.has_scoring_stats}<span>No scoring stats</span>{:else if !player.qualified}<span title="Minimum {player.benchmark_minimum_games} appearances and {formatValue(player.benchmark_minimum_innings)} innings">{player.qualification_note || `Small sample · ${player.games} games`}</span>{/if}{#if player.status!=='active'} · {player.status}{/if}</div></div></div></td>
     <td><div class="row tight"><SportBadge sport={player.competition} /><strong>{player.season || 'No season'}</strong></div>{#if player.scoring_source==='defaults'}<small class="muted">Sport default scoring</small>{/if}</td>
     <td>{player.positions.join('/') || '—'}</td><td class="muted">{player.team || '—'}</td>
     <td class="num">{player.season ? formatValue(player.games) : '—'}</td><td class="num">{player.has_scoring_stats ? formatValue(player.points) : '—'}</td><td class="num">{player.has_scoring_stats && player.games>0 ? formatValue(player.points_per_game) : '—'}</td>
     {#each metricColumns as c (c.key)}<td class="num" class:highlight={c.key==='league_index'} class:positive={['points_above_replacement','par_per_game','win_share_added'].includes(c.key) && Number(player[c.key as keyof ResearchPlayer])>0} class:negative={['points_above_replacement','par_per_game','win_share_added'].includes(c.key) && Number(player[c.key as keyof ResearchPlayer])<0}>{formatValue(player[c.key as keyof ResearchPlayer])}</td>{/each}
     {#if view==='advanced' || view==='all'}<td title="Replacement {formatValue(player.replacement_rate)} FP/game at rank {player.replacement_rank ?? '—'}">{player.metric_position || '—'}</td>{/if}
     {#if view==='raw' || view==='all'}{#each rawColumns as c (c.key)}<td class="num">{rawValue(player,c.key)}</td>{/each}{/if}
     <td>{#if player.owner_slug}<a href="/franchise/{player.owner_slug}">{player.owner_name}</a>{:else}<span class="muted">Available</span>{/if}</td>
    </tr>{/each}
   </tbody></table></div>
   {#if pages>1}<nav class="spread" aria-label="Research pages"><button disabled={loading || page<=1} onclick={() => page--}>Previous</button><span class="muted small-text">Page {page} of {pages.toLocaleString()}</span><button disabled={loading || page>=pages} onclick={() => page++}>Next</button></nav>{/if}
   {/if}
   <p class="muted small-text">A dash means missing data or an insufficient benchmark sample. League+ measures relative peer production, not projected performance. Above replacement uses raw league points and is not comparable across sports. Raw stats include stats that don’t earn fantasy points.</p>
  {/if}
 {:else}<p class="muted">Loading your research pool…</p>{/if}

 <details class="card method-guide"><summary>How the metrics work &amp; benchmark settings</summary><div class="stack guide">
  <div class="row"><label class="control numeric"><span>Benchmark minimum games</span><input type="number" min="1" max="10000" bind:value={benchmarkGames} /></label><label class="control numeric"><span>Pitcher workload minimum % (0 = off)</span><input aria-label="Pitcher workload minimum percent" type="number" min="0" max="100" bind:value={pitcherWorkloadPercent} /></label><label class="control numeric"><span>Replacement rank (0 = automatic)</span><input type="number" min="0" max="100000" bind:value={replacementRank} /></label></div>
  <p><strong>Pitcher sample requirements:</strong> By default, pitchers need at least 25% of both the highest appearance count and highest innings total among their pitching peers in the selected season, plus the benchmark minimum games. These are separate workload thresholds and scale as a season progresses. Small workloads keep their raw stats but have no comparative metrics. This is a ranking qualification rule, not a projection or confidence interval.</p>
  <p><strong>League+:</strong> 100 + 15 × (player season fantasy points − league-season average) / league-season standard deviation. All qualified positions and MLB roles share one benchmark. Percentile also ranks total season production. Availability affects these metrics; FP/game shows production rate separately. <strong>Position+:</strong> the same formula using FP/game among eligible positional peers. Benchmarks are independent of display filters. A 115 means one standard deviation above average, not 15% more points.</p>
  <p><strong>Above replacement:</strong> (FP/game − replacement FP/game) × games played. Automatic depth is ceil(managers × allocated starting slots) + 1. Flexible slots split demand equally across distinct eligible position pools. Basketball G/PG/SG and F/SF/PF aliases share a pool; MLB outfield aliases share OF. These are raw league fantasy points, so compare replacement totals within a sport and season; point scales and season lengths differ. A manual rank overrides this estimate. Too few qualified peers means no replacement estimate.</p>
  <p><strong>Win share added:</strong> GP × [Φ(PAR/game ÷ (league rate SD × √(2 × starters))) − 0.5]. This illustrative model assumes independent normal scores and one game per starter. It uses season-rate spread as a variance proxy and ignores schedules, actual lineup use, game-level variance and weekly caps. It is not measured fantasy wins or real-world Win Shares.</p>
  <p><strong>Historical scope:</strong> stats and teams belong to the selected season; eligibility, player status and fantasy ownership reflect today’s records. Imported history may cover only players known to the feed.</p>
 </div></details>

 {#if selectedId}<div class="history panel"><div class="spread history-heading"><h2>Player history</h2><button class="quiet small" onclick={() => selectedId=undefined}>Close</button></div><PlayerResearch {selectedId} competitions={data.competitions} action={playerAction} /></div>{/if}
</div>
{#snippet playerAction()}{/snippet}

<style>
 .preview-link { border: 0; background: none; color: var(--ink-soft); font-size: .7rem; padding: .2rem .4rem; }
 .preview-link:hover { color: var(--brand); }
 .research-page { gap: 1.15rem; }
 .eyebrow { color: var(--brand); font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.1em; font-weight: 700; }
 h1 { font-size: clamp(1.8rem,3vw,2.6rem); }
 h2 { font-size: 1.05rem; }
 h3 { font-size: 0.85rem; }
 .step { display: inline-grid; place-items: center; width: 1.4rem; height: 1.4rem; margin-right: 0.45rem; border-radius: 50%; background: var(--brand-soft); color: var(--brand); font: 700 0.75rem var(--body); }
 .dataset-card, .filter-card, .method-guide { padding: 1rem; }
 .pool-builder { align-items: flex-end; margin-block: 1rem; }
 .control { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.72rem; font-weight: 500; color: var(--ink-soft); }
 .control input, .control select { min-width: 0; }
 .control select { max-width: 16rem; }
 .numeric input { width: 8rem; }
 .pool-chips { gap: 0.5rem; }
 .dataset-chip { display: inline-flex; align-items: center; gap: 0.5rem; padding: 0.3rem 0.4rem 0.3rem 0.6rem; border: 1px solid var(--rule-strong); border-radius: var(--radius-small); font-size: 0.8rem; background: var(--surface-2); }
 .dataset-chip button { padding: 0.2rem; }
 .data-notes { padding: 0.65rem 0.8rem; border: 1px solid var(--rule); background: var(--gold-soft); border-radius: var(--radius-small); font-size: 0.78rem; }
 .data-notes summary { display: flex; gap: 0.4rem; align-items: center; cursor: pointer; font-weight: 600; }
 .data-notes div { margin-top: 0.6rem; display: grid; gap: 0.3rem; }
 .filters { align-items: flex-end; }
 .search { flex: 1 1 14rem; display: flex; align-items: center; gap: 0.5rem; padding-left: 0.7rem; background: var(--surface); border: 1px solid var(--rule-strong); border-radius: var(--radius-small); color: var(--ink-faint); }
 .search:focus-within { outline: 2px solid var(--brand); outline-offset: 1px; }
 .search input { min-width: 0; flex: 1; border: 0; background: transparent; padding-left: 0; outline: none; }
 .position-picker { position: relative; align-self: flex-end; }
 .position-picker summary { padding: 0.5rem 0.65rem; border: 1px solid var(--rule-strong); border-radius: var(--radius-small); cursor: pointer; font-size: 0.83rem; background: var(--surface); }
 .position-options { position: absolute; z-index: 10; top: calc(100% + 0.4rem); left: 0; min-width: 13rem; display: grid; grid-template-columns: 1fr 1fr; gap: 0.65rem; padding: 0.8rem; background: var(--surface); border: 1px solid var(--rule-strong); border-radius: var(--radius-small); box-shadow: 0 8px 24px #0002; }
 .more-filters { border-top: 1px solid var(--rule); padding-top: 0.7rem; }
 .more-filters summary, .method-guide summary { cursor: pointer; font-size: 0.8rem; font-weight: 600; }
 .expanded-filters, .chart-controls { display: grid; grid-template-columns: repeat(auto-fit,minmax(10rem,1fr)); gap: 0.8rem; }
 .expanded-filters { padding-top: 0.9rem; }
 .checks { justify-content: center; font-size: 0.76rem; }
 .table-toolbar { align-items: flex-end; }
 .presets { gap: 0.3rem 0.8rem; }
 .presets .quiet { border: 1px solid var(--rule); background: var(--surface); }
 .chart-controls { padding: 1rem; }
 .research-table { transition: opacity 0.15s; }
 .research-table.loading { opacity: 0.5; }
 td { white-space: nowrap; }
 .identity { position: sticky; left: 0; z-index: 2; background: var(--surface); min-width: 14rem; padding: 0.65rem; }
 .player { display: flex; align-items: center; gap: 0.55rem; }
 .player-name { padding: 0; border: 0; border-radius: 0; background: transparent; font-weight: 700; text-align: left; white-space: nowrap; }
 .player-name:hover { color: var(--brand); text-decoration: underline; }
 .player .small-text { font-size: 0.68rem; }
 .highlight { color: var(--brand); font-weight: 700; background: var(--brand-soft); }
 .positive { color: var(--good); }
 .negative { color: var(--bad); }
 tr.selected td { background: var(--brand-soft); }
 .guide { margin-top: 1rem; color: var(--ink-soft); font-size: 0.8rem; line-height: 1.6; }
 .history { max-width: 58rem; }
 .history-heading { padding: 0.8rem; border-bottom: 1px solid var(--rule); }
 @media (max-width:640px) { .pool-builder .control { flex: 1; } .more-filters summary .muted { display: none; } .filters .control { flex: 1; } }
</style>
