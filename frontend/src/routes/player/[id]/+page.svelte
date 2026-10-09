<script lang="ts">
 import { page } from '$app/state';
 import { invalidateAll } from '$app/navigation';
 import { getPlayerProfile, getPlayerTimeline, getPlayerGameLog, changeRoster, type PlayerProfile, type PlayerEvent, type ProfileGame, type List } from '#lib/api.ts';
 import MetricHelp from '#lib/research/MetricHelp.svelte';
 import ColumnHeader from '#lib/research/ColumnHeader.svelte';
 import PlayerNickname from '#lib/PlayerNickname.svelte';
 import PlayerSearch from '#lib/PlayerSearch.svelte';
 import Headshot from '#lib/ui/Headshot.svelte';
 import Tabs from '#lib/ui/Tabs.svelte';
 import ResearchChart from '#lib/research/ResearchChart.svelte';
 import { advanced, basics, formatValue as fmt } from '#lib/research/columns.ts';
 import { toast } from '#lib/ui/toast.svelte.ts';
 import type { PageProps } from './$types';
 let { data }: PageProps = $props();
 let profile = $state<PlayerProfile>(), error = $state(''), loading = $state(true), retry = $state(0);
 let tab = $state('overview'), seasonKey = $state(''), rawMode = $state('totals');
 let chartMetric = $state('league_index'), chartKind = $state('scatter'), chartSport = $state('');
 let games = $state<ProfileGame[]>([]), gameTotal = $state(0), gamePage = $state(1), gameSport = $state('');
 let events = $state<PlayerEvent[]>([]), eventTotal = $state(0), eventPage = $state(1);
 let gamesBusy = $state(false), eventsBusy = $state(false), adding = $state(false);
 let historyFilter = $state(''), gamesError = $state(''), eventsError = $state('');
 let gameRequest = 0, eventRequest = 0;
 const id = $derived(page.params.id!);
 const season = $derived(profile?.seasons.find(s => s.key === seasonKey));
 const research = $derived(season?.research);
 const own = $derived(profile?.ownership.find(o => o.competition === profile?.player.competition));
 const waiver = $derived(profile?.waivers.find(w => w.competition === profile?.player.competition));
 const league = $derived(data.dynasty?.leagues.find(l => l.competition === profile?.player.competition));
 const nameSport = (key: string) => data.competitions.find(c => c.key === key)?.name ?? key.toUpperCase();
 const labelStat = (key: string, sport: string) => data.competitions.find(c => c.key === sport)?.stats.find(s => s.key === key)?.label ?? key.replaceAll('_',' ').toUpperCase();
 const date = (value: string | null | undefined) => value ? new Date(/^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T12:00:00` : value).toLocaleDateString(undefined,{year:'numeric',month:'short',day:'numeric'}) : '—';
 const age = $derived.by(() => { if (!profile?.player.birth_date) return null; const b = new Date(`${profile.player.birth_date.slice(0,10)}T12:00:00`), now = new Date(); return now.getFullYear()-b.getFullYear()-(now.getMonth()<b.getMonth() || now.getMonth()===b.getMonth() && now.getDate()<b.getDate() ? 1 : 0); });
 const seasonSports = $derived([...new Set(profile?.seasons.map(s => s.competition) ?? [])]);
 const chartOptions = $derived([...basics,...advanced,...Object.keys(profile?.seasons.filter(s => s.competition === chartSport).reduce((a,s) => ({...a,...s.stats}),{} as Record<string,number>) ?? {}).sort().map(key => ({key:`raw:${key}`,label:labelStat(key,chartSport),help:'Imported season total.'}))]);
 const chartPoints = $derived((profile?.seasons ?? []).filter(s => s.competition === chartSport).map(s => ({ key:s.key, label:`${s.season} · ${s.team || s.league || nameSport(s.competition)}`, group:`${s.season}${s.league ? ` · ${s.league}` : ''}`, x:s.year, y:chartMetric.startsWith('raw:') ? s.stats[chartMetric.slice(4)] ?? null : chartMetric === 'games' ? s.games : chartMetric === 'points' ? s.points : chartMetric === 'points_per_game' ? s.points_per_game : (s.research as unknown as Record<string, number | null> | null)?.[chartMetric] ?? null })));
 const drivers = $derived(Object.entries(season?.stats ?? {}).map(([key,value]) => ({key,value,weight:profile?.scoring_rules[season?.competition ?? '']?.[key] ?? 0,points:value*(profile?.scoring_rules[season?.competition ?? '']?.[key] ?? 0)})).filter(s => s.weight !== 0).sort((a,b) => Math.abs(b.points)-Math.abs(a.points)));
 const biggestDriver = $derived(Math.max(1,...drivers.map(s => Math.abs(s.points))));
 const rawKeys = $derived(Object.keys(season?.stats ?? {}).sort());
 const gameKeys = $derived([...new Set(games.flatMap(g => Object.keys(g.stats)))].sort());
 const assessment = $derived(research?.league_index == null ? 'Comparative ranking unavailable' : research.league_index >= 130 ? 'Exceptional season production' : research.league_index >= 115 ? 'Strong season production' : research.league_index >= 100 ? 'Above-average season production' : 'Below-average season production');
 const visibleEvents = $derived(events.filter(e => !historyFilter || e.kind === historyFilter));
 const eventKinds = $derived([...new Set(events.map(e => e.kind))].sort());
 const human = (value: string) => value.replaceAll('_',' ').replace(/^./,s => s.toUpperCase());
 function eventDetail(e: PlayerEvent) {
  const d = e.detail ?? {}; const parts: string[] = [];
  if (d.from && d.to) parts.push(`${human(String(d.from))} → ${human(String(d.to))}`);
  else if (d.list) parts.push(`${human(String(d.list))} roster`);
  if (e.kind === 'claim' && typeof d.bid === 'number') parts.push(`Winning bid: ${d.bid}`);
  if (d.reason) parts.push(String(d.reason)); if (d.reversed) parts.push('Reversed');
  if (e.draft_name) parts.push(`${e.draft_name} · Round ${e.pick_round}, pick ${e.pick_position}`);
  return parts.join(' · ');
 }
 function apply(p: PlayerProfile) {
  profile = p; seasonKey = p.seasons.find(s => s.competition === p.player.competition)?.key ?? p.seasons[0]?.key ?? '';
  chartSport = p.seasons.some(s => s.competition === p.player.competition) ? p.player.competition : p.seasons[0]?.competition ?? p.player.competition;
  games = p.game_log.games; gameTotal = p.game_log.total; gamePage = 1; gameSport = ''; gamesBusy = false; gamesError = '';
  events = p.timeline.events; eventTotal = p.timeline.total; eventPage = 1; eventsBusy = false; eventsError = ''; historyFilter = '';
 }
 $effect(() => {
  const playerID = id; retry; let active = true;
  gameRequest++; eventRequest++; profile = undefined; error = ''; loading = true; tab = 'overview';
  getPlayerProfile(playerID).then(p => { if (active) apply(p); }).catch(e => { if (active) error = e instanceof Error ? e.message : 'Could not load this player.'; }).finally(() => { if (active) loading = false; });
  return () => { active = false; };
 });
 async function loadGames(reset = false) {
  const playerID = id, request = ++gameRequest, next = reset ? 1 : gamePage + 1, sport = gameSport;
  gamesBusy = true; gamesError = ''; if (reset) { games = []; gameTotal = 0; }
  try { const result = await getPlayerGameLog(playerID,next,sport); if (request === gameRequest && playerID === id) { games = reset ? result.games : [...games,...result.games]; gamePage = result.page; gameTotal = result.total; } }
  catch (e) { if (request === gameRequest) gamesError = e instanceof Error ? e.message : 'Game log could not load.'; }
  finally { if (request === gameRequest) gamesBusy = false; }
 }
 async function loadEvents() {
  const playerID = id, request = ++eventRequest; eventsBusy = true; eventsError = '';
  try { const result = await getPlayerTimeline(playerID,eventPage+1); if (request === eventRequest && playerID === id) { events = [...events,...result.events]; eventPage = result.page; eventTotal = result.total; } }
  catch (e) { if (request === eventRequest) eventsError = e instanceof Error ? e.message : 'History could not load.'; }
  finally { if (request === eventRequest) eventsBusy = false; }
 }
 async function add(list: List) {
  if (!league || !data.me) return; const playerID = id; adding = true;
  try { await changeRoster(league.id,'add',{player_id:playerID,list,franchise_id:data.me.id}); toast.good('Player added.'); const p = await getPlayerProfile(playerID); if (id===playerID) apply(p); await invalidateAll(); }
  catch(e) { toast.error(e); } finally { adding = false; }
 }
 function exportStats() {
  if (!profile) return;
  const keys = [...new Set(profile.seasons.flatMap(s => Object.keys(s.stats)))].sort();
  const cell = (v: unknown) => `"${String(v ?? '').replaceAll('"','""')}"`;
  const rows = [['Player','League','Season','Team','Games','Fantasy points','FP/game','League+','Above replacement',...keys],...profile.seasons.map(s => [profile!.player.full_name,s.league || nameSport(s.competition),s.season,s.team,s.games,s.points,s.points_per_game,s.research?.league_index,s.research?.points_above_replacement,...keys.map(k => s.stats[k])])];
  const url = URL.createObjectURL(new Blob([rows.map(r => r.map(cell).join(',')).join('\n')],{type:'text/csv;charset=utf-8'}));
  const a = document.createElement('a'); a.href = url; a.download = `${profile.player.full_name}-career.csv`; a.click(); setTimeout(() => URL.revokeObjectURL(url),1000);
 }
</script>
<svelte:head><title>{profile?.player.full_name ?? 'Player profile'} · Research</title></svelte:head>
<div class="stack profile">
 <nav class="row small-text"><a href="/research">← Research lab</a><span class="muted">/ Player profile</span></nav>
 <PlayerSearch competitions={data.competitions} />
 {#if loading}<div class="card"><p role="status">Loading player career and fantasy history…</p></div>
 {:else if error}<div class="card"><p role="alert">{error}</p><button onclick={() => retry++}>Try again</button><a href="/players">Browse players</a></div>
 {:else if profile}
  {@const p = profile.player}
  <header class="hero card">
   <div class="identity"><Headshot name={p.full_name} src={p.headshot_url} size={88} /><div class="stack tight"><span class="eyebrow">Player dossier · {nameSport(p.competition)}</span><h1>{p.full_name}</h1>{#if data.me}{#key data.me.id+p.id}<PlayerNickname franchiseID={data.me.id} playerID={p.id} playerName={p.full_name} load />{/key}{/if}<div class="row muted">{p.positions?.join(' / ')} · {p.team_abbrev || 'Team unavailable'}{#if age !== null} · Age {age}{/if}{#if p.class} · {p.class}{/if}<span class="pill">{human(p.status)}</span></div></div></div>
   <div class="status"><span class="eyebrow">Fantasy status</span>{#if own}<a href="/franchise/{own.franchise_slug}"><strong>{own.franchise_name}</strong></a><span>{human(own.list)} roster{own.slot ? ` · Starting at ${own.slot}` : ' · Not currently starting'}</span>{:else}<strong>{waiver ? 'On waivers' : 'Unrostered'}</strong><span class="muted small-text">{league ? 'Subject to league acquisition rules' : 'No dynasty league for this sport'}</span>{/if}
    {#if data.me && league}{#if !own && waiver}<a href="/waivers?competition={p.competition}">Review waiver claim →</a>{:else if !own}<div class="row"><button class="small" disabled={adding} onclick={() => add('main')}>Add to main</button><button class="small" disabled={adding} onclick={() => add('reserve')}>Add to reserve</button></div>{:else if own.franchise_id !== data.me.id}<a href="/trades/new">Build a trade →</a>{:else}<a href="/franchise/{own.franchise_slug}">Manage on your roster →</a>{/if}{/if}
   </div>
  </header>
  <div class="notice"><strong>{profile.starter_eligible ? 'Meets starting rules' : 'Not eligible to start'}</strong><span>{profile.eligibility_note}</span>{#if p.note}<span>{p.note}</span>{/if}</div>
  <Tabs tabs={[{value:'overview',label:'Overview'},{value:'stats',label:'Stats & charts'},{value:'games',label:'Game log',count:profile.game_log.total},{value:'fantasy',label:'Fantasy history',count:eventTotal}]} bind:value={tab} label="Player profile view" />
  {#if tab === 'overview' || tab === 'stats'}
   <div class="spread"><label class="season-label">Explore a season <select aria-label="Explore a season" bind:value={seasonKey}>{#each profile.seasons as s (s.key)}<option value={s.key}>{s.season} · {s.league || nameSport(s.competition)} · {s.team || 'Team unavailable'}</option>{/each}</select></label><button class="small" onclick={exportStats} disabled={!profile.seasons.length}>Export career CSV</button></div>
   {#if season}
    <p class="muted small-text">{season.games} appearances · {season.scoring_source === 'league' ? 'Current fantasy league scoring' : 'Sport default scoring'} · Stats synced {date(season.synced_at)}. Season totals include every recorded appearance.</p>
    {#if season.research_note}<p class="notice" role="status">{season.research_note}</p>{/if}
    <div class="metrics">{#each [...basics,...advanced.filter(c => ['league_index','points_above_replacement','percentile'].includes(c.key))] as c}<div class="metric"><MetricHelp label={c.label} help={c.help} /><strong>{fmt(c.key === 'games' ? season.games : c.key === 'points' ? season.points : c.key === 'points_per_game' ? season.points_per_game : (research as unknown as Record<string,unknown> | null)?.[c.key])}</strong></div>{/each}</div>
    {#if tab === 'overview'}
     <div class="two-columns"><section class="card stack"><span class="eyebrow">At a glance</span><h2>{assessment}</h2><p>{#if research?.league_index != null}League+ {fmt(research.league_index)} compares their total season fantasy production with all qualified {nameSport(season.competition)} players in {season.season}. Average is 100.{:else}Raw production is available below. A comparative ranking needs an eligible season and enough qualified peers.{/if}</p>{#if research?.points_above_replacement != null}<p>{fmt(research.points_above_replacement)} fantasy points above the estimated {research.metric_position} replacement baseline, at {fmt(research.replacement_rate)} FP/game.</p>{/if}<p class="muted small-text">Above replacement uses this league’s point scale and season workload. Use League+ to compare different scoring systems. These are descriptive research metrics.</p><button class="small" onclick={() => tab='stats'}>Explore the numbers →</button></section>
     <section class="card stack"><span class="eyebrow">Where the points come from</span><h2>Scoring breakdown</h2>{#each drivers.slice(0,6) as d}<div class="driver"><div class="spread"><span>{labelStat(d.key,season.competition)}</span><strong>{fmt(d.points)} FP</strong></div><div class="track"><span class:negative={d.points<0} style:width="{Math.abs(d.points)/biggestDriver*100}%"></span></div><small class="muted">{fmt(d.value)} × {fmt(d.weight,3)} points</small></div>{:else}<p class="muted">No scoring stats have been imported for this season.</p>{/each}<button class="small" onclick={() => tab='stats'}>View every raw stat →</button></section></div>
     <section class="card stack"><div class="spread"><div><span class="eyebrow">Career snapshot</span><h2>Season history</h2></div><button class="small" onclick={() => tab='stats'}>Build a career chart →</button></div><div class="table-wrap"><table><thead><tr><th>Season</th><th>League / team</th>{#each [...basics,advanced[0]] as c}<ColumnHeader label={c.label} help={c.help} />{/each}</tr></thead><tbody>{#each profile.seasons as s (s.key)}<tr><td><button class="text-button" onclick={() => seasonKey=s.key}>{s.season}</button></td><td>{s.league || nameSport(s.competition)}<span class="muted small-text"> · {s.team}</span></td><td>{s.games}</td><td>{fmt(s.points)}</td><td>{fmt(s.points_per_game)}</td><td>{fmt(s.research?.league_index)}</td></tr>{/each}</tbody></table></div></section>
    {:else}
     <section class="card stack"><div class="spread"><div><span class="eyebrow">Full stat sheet</span><h2>Raw stats & scoring formula</h2></div><Tabs tabs={[{value:'totals',label:'Totals'},{value:'per-game',label:'Per appearance'}]} bind:value={rawMode} label="Raw stat values" /></div><p class="muted small-text">Includes stats that score zero fantasy points. Per appearance divides by imported games; it is not a per-minute or per-start rate.</p><div class="table-wrap"><table><thead><tr><th>Stat</th><th>Raw value</th><ColumnHeader label="Weight" help="Current points awarded per unit of this stat." /><ColumnHeader label="Fantasy contribution" help="Raw value multiplied by the current scoring weight." /></tr></thead><tbody>{#each rawKeys as key}<tr><td>{labelStat(key,season.competition)} <small class="muted">({key})</small></td><td>{fmt(season.stats[key]/(rawMode === 'per-game' ? season.games || 1 : 1),3)}</td><td>{fmt(profile.scoring_rules[season.competition]?.[key] ?? 0,3)}</td><td>{fmt(season.stats[key]*(profile.scoring_rules[season.competition]?.[key] ?? 0)/(rawMode === 'per-game' ? season.games || 1 : 1),3)}</td></tr>{/each}</tbody></table></div></section>
     <section class="card stack"><h2>Advanced research</h2><div class="research-grid">{#each advanced as c}<div><MetricHelp label={c.label} help={c.help} /><strong>{fmt((research as unknown as Record<string,unknown> | null)?.[c.key])}</strong><p class="muted small-text">{c.help}</p></div>{/each}</div></section>
    {/if}
   {:else}<p class="card muted">No career season stats have been imported yet. Fantasy history and recorded game logs remain available in their tabs.</p>{/if}
   {#if tab === 'stats'}<section class="card stack"><h2>Build a career chart</h2><p class="muted small-text">Choose a metric and sport to explore changes between seasons. Missing and unqualified measurements are omitted. Raw fantasy points are comparable within the same scoring rules.</p><div class="row"><label>Sport <select bind:value={chartSport}>{#each seasonSports as sport}<option value={sport}>{nameSport(sport)}</option>{/each}</select></label><label>Metric <select aria-label="Career chart metric" bind:value={chartMetric}>{#each chartOptions as c}<option value={c.key}>{c.label}</option>{/each}</select></label><label>Chart <select aria-label="Career chart type" bind:value={chartKind}><option value="scatter">Season trend</option><option value="bar">Season comparison</option></select></label></div><ResearchChart points={chartPoints} kind={chartKind} title="{p.full_name} · {chartOptions.find(c => c.key === chartMetric)?.label ?? chartMetric}" xLabel="Season ending year" yLabel={chartOptions.find(c => c.key === chartMetric)?.label ?? chartMetric} /></section>{/if}
  {:else if tab === 'games'}
   <section class="card stack"><div class="spread"><div><h2>Recorded game log</h2><p class="muted small-text">Final games with imported stats, valued under current scoring. Your fantasy lineup and weekly limits determine which games actually count.</p></div><label>Sport <select bind:value={gameSport} onchange={() => loadGames(true)}><option value="">All career sports</option>{#each data.competitions as c}<option value={c.key}>{c.name}</option>{/each}</select></label></div>
    <div class="table-wrap"><table><thead><tr><th>Date</th><th>League</th><th>Game</th><ColumnHeader label="Fantasy points" help="Current scoring value of the recorded stat line. Starting lineup rules determine which games count toward actual fantasy totals." />{#each gameKeys as key}<ColumnHeader label={key.toUpperCase()} help={data.competitions.flatMap(c => c.stats).find(s => s.key === key)?.label ?? key} />{/each}</tr></thead><tbody>{#each games as g (g.id)}<tr><td>{date(g.day)}</td><td>{g.competition.toUpperCase()}</td><td><a href="/game/{g.id}">{g.away_abbrev || '?'} @ {g.home_abbrev || '?'}</a>{#if g.scoring_source === 'defaults'}<small class="muted"> · Default scoring</small>{/if}</td><td>{fmt(g.points)}</td>{#each gameKeys as key}<td>{fmt(g.stats[key],3)}</td>{/each}</tr>{/each}</tbody></table></div>
    {#if !games.length && !gamesBusy}<p class="muted">No final game stat lines have been imported for this selection.</p>{/if}{#if gamesError}<p role="alert">{gamesError}</p><button onclick={() => loadGames(!games.length)}>Retry</button>{/if}<div class="spread"><span class="muted small-text">{games.length} of {gameTotal} recorded games</span>{#if games.length<gameTotal}<button disabled={gamesBusy} onclick={() => loadGames()}>{gamesBusy ? 'Loading…' : 'Load more games'}</button>{/if}</div>
   </section>
  {:else if tab === 'fantasy'}
   <section class="card stack"><span class="eyebrow">Points that counted</span><h2>Fantasy scoring history</h2><p class="muted small-text">Production from saved starting lineups in fantasy seasons, including weekly game and pitcher-start limits. Recalculated under current league rules, matching standings. Research totals include every appearance instead.</p>{#if profile.fantasy_production.length}<div class="table-wrap"><table><thead><tr><th>Fantasy season</th><th>League</th><th>Fantasy team</th><ColumnHeader label="Counted games" help="Recorded games counted by saved starting lineups and weekly limits." /><ColumnHeader label="Counted fantasy points" help="Points contributed to this franchise in the fantasy season under current league scoring rules." /></tr></thead><tbody>{#each profile.fantasy_production as f}<tr><td>{f.year}</td><td>{nameSport(f.competition)}</td><td><a href="/franchise/{f.franchise_slug}">{f.franchise_name}</a></td><td>{f.games}</td><td>{fmt(f.points)}</td></tr>{/each}</tbody></table></div>{:else}<p class="muted">No counted fantasy production is recorded in configured fantasy seasons.</p>{/if}</section>
   <div class="two-columns"><section class="card stack"><span class="eyebrow">Roster & lineup</span><h2>Current ownership</h2>{#each profile.ownership as o}<div class="history-item"><a href="/franchise/{o.franchise_slug}"><strong>{o.franchise_name}</strong></a><p>{o.league_name} · {human(o.list)} roster · {o.slot ? `Starting: ${o.slot}` : 'Not currently starting'}</p><p class="muted small-text">Acquired {date(o.acquired_at)} via {human(o.acquired_via)}</p>{#if profile.reserve_locked_until[o.league_id] && new Date(profile.reserve_locked_until[o.league_id]) > new Date()}<p class="small-text">Reserve lock ends {date(profile.reserve_locked_until[o.league_id])}</p>{/if}</div>{:else}<p class="muted">Not on a fantasy roster.</p>{/each}{#each profile.waivers as w}<p>{nameSport(w.competition)} waivers clear {date(w.clears_at)}.</p>{/each}<p class="muted small-text">{profile.eligibility_note}</p></section>
    <section class="card stack"><span class="eyebrow">Draft pedigree</span><h2>Draft history</h2>{#each profile.drafts as d}<div class="history-item"><a href="/draft/{d.draft_id}"><strong>{d.draft_name}</strong></a><p>Round {d.round} · Pick {d.position}{d.auto_picked ? ' · Auto-picked' : ''}</p><p class="muted small-text">{d.franchise_name} · {date(d.picked_at)} · {human(d.kind)}</p></div>{:else}<p class="muted">No recorded fantasy draft selections.</p>{/each}</section></div>
   <section class="card stack"><div class="spread"><div><span class="eyebrow">Player movement</span><h2>Transaction timeline</h2></div><label>Event <select bind:value={historyFilter}><option value="">All events</option>{#each eventKinds as kind}<option value={kind}>{human(kind)}</option>{/each}</select></label></div><p class="muted small-text">Recorded acquisitions, drops, roster moves, draft selections, and trades. Event filtering applies to the history loaded below.</p><ol class="timeline">{#each visibleEvents as e (e.id)}<li><div class="spread"><strong>{human(e.kind)}</strong><time>{date(e.created_at)}</time></div><p>{e.franchise_name || 'League'}{e.competition ? ` · ${nameSport(e.competition)}` : ''}</p>{#if eventDetail(e)}<p class="muted small-text">{eventDetail(e)}</p>{/if}{#if e.trade_id}<a class="small-text" href="/trades">View trades →</a>{/if}{#if e.draft_id}<a class="small-text" href="/draft/{e.draft_id}">View draft →</a>{/if}</li>{:else}<li class="muted">No recorded transactions match this view.</li>{/each}</ol>{#if eventsError}<p role="alert">{eventsError}</p>{/if}<div class="spread"><span class="muted small-text">{events.length} of {eventTotal} transactions loaded</span>{#if events.length<eventTotal}<button disabled={eventsBusy} onclick={loadEvents}>{eventsBusy ? 'Loading…' : 'Load more history'}</button>{/if}</div></section>
   <section class="card stack"><h2>Completed trade history</h2>{#each profile.trades as t}<div class="history-item"><div class="spread"><strong>{t.from_name} → {t.to_name}</strong><span class="pill">{human(t.status)}</span></div><p class="muted small-text">{nameSport(t.competition)} · Proposed {date(t.created_at)}{t.resolved_at ? ` · Resolved ${date(t.resolved_at)}` : ''}</p>{#if t.note}<p>{t.note}</p>{/if}<a href="/trades" class="small-text">View trades →</a></div>{:else}<p class="muted">No recorded completed trades.</p>{/each}</section>
  {/if}
 {/if}
</div>
<style>
 .profile{gap:1.25rem}.hero{display:flex;justify-content:space-between;gap:1.5rem;padding:1.5rem}.identity{display:flex;align-items:center;gap:1.1rem}.status{display:flex;flex-direction:column;gap:.5rem;min-width:220px}.eyebrow{font-size:.7rem;text-transform:uppercase;letter-spacing:.09em;font-weight:700;color:var(--ink-soft)}h1{font-size:clamp(1.7rem,4vw,2.4rem)}h2{font-size:1.2rem}.notice{padding:.8rem 1rem;border:1px solid var(--rule);border-radius:8px;background:var(--brand-soft);display:flex;flex-direction:column;gap:.3rem;font-size:.9rem}.metrics{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:.6rem}.metric{border:1px solid var(--rule);border-radius:10px;padding:1rem;display:flex;flex-direction:column;gap:.6rem}.metric strong{font-size:1.7rem;font-variant-numeric:tabular-nums}.two-columns{align-items:start;display:grid;grid-template-columns:1fr 1fr;gap:1rem}.card{padding:1.2rem}.season-label{display:flex;align-items:center;gap:.7rem}.season-label select{max-width:100%}.track{height:6px;background:var(--rule);border-radius:6px;margin:.4rem 0}.track span{display:block;height:100%;background:var(--brand);border-radius:6px}.track .negative{background:#c2255c}.research-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem}.research-grid>div{border:1px solid var(--rule);padding:1rem;border-radius:8px}.research-grid strong{display:block;font-size:1.6rem;margin:.4rem 0}.table-wrap{overflow:auto}table{width:100%;white-space:nowrap}th,td{text-align:left;padding:.65rem .75rem}th{font-size:.8rem}td{font-variant-numeric:tabular-nums}.text-button{background:none;border:none;padding:0;color:var(--brand);cursor:pointer}.history-item{border-bottom:1px solid var(--rule);padding:.8rem 0}.timeline{list-style:none;padding:0;margin:0}.timeline li{border-left:2px solid var(--brand);padding:.5rem 1rem;margin:.6rem 0}.timeline time{color:var(--ink-soft);font-size:.8rem}@media(max-width:900px){.metrics{grid-template-columns:repeat(3,minmax(0,1fr))}.hero{flex-direction:column}.two-columns{grid-template-columns:1fr}}@media(max-width:600px){.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.research-grid{grid-template-columns:1fr}.identity{align-items:flex-start}.season-label{flex-direction:column;align-items:flex-start}.spread{flex-wrap:wrap;gap:.7rem}}
</style>
