<script lang="ts">
 import { onMount } from 'svelte';
 import { invalidateAll } from '$app/navigation';
 import { getGameCounts, type GameDayCount, getSchedule, setSchedule, generateSchedule, type League, type Season, type Franchise, type SchedulePeriod, type CustomPeriod } from '#lib/api.ts';
 import { dayLabel, sportsToday } from '#lib/ui/time.ts';
 import { toast } from '#lib/ui/toast.svelte.ts';

 let { league, season, teams }: { league: League; season: Season; teams: Franchise[] } = $props();
 let periods = $state<SchedulePeriod[]>([]);
 let draft = $state<CustomPeriod[]>([]);
 let loading = $state(true);
 let busy = $state(false);
 let dirty = $state(false);
 let length = $state(7);
 let playoffStart = $state('');
 let gameDays = $state<GameDayCount[]>([]);
 let countsLoading = $state(true);
 let countsError = $state(false);
 const sportName = $derived(league.competition.toUpperCase());
 const gameCount = (period: CustomPeriod) => gameDays.reduce((total, day) => total + (day.day >= period.starts_on && day.day <= period.ends_on ? day.games : 0), 0);
 // Doubling only matters where a slot counts some games a week, or pitcher starts are capped.
 const canDouble = $derived(league.settings.lineup.slots.some((s) => s.games_per_week > 0) || league.settings.lineup.pitcher_starts_per_week > 0);
 const weekly = $derived(league.settings.lineup.period === 'week');
 const weekdays = ['sunday','monday','tuesday','wednesday','thursday','friday','saturday'];
 const misaligned = (p: CustomPeriod) => p.double_games && weekly && !!p.starts_on && !!p.ends_on &&
  (weekdays[new Date(`${p.starts_on}T12:00:00Z`).getUTCDay()] !== league.settings.lineup.week_start || duration(p) % 7 !== 0);
 const doubleNote = $derived([
  league.settings.lineup.slots.some((s) => s.games_per_week > 0) && 'each starter counts twice the games',
  league.settings.lineup.pitcher_starts_per_week > 0 && `pitchers get ${league.settings.lineup.pitcher_starts_per_week * 2} starts`
 ].filter(Boolean).join(' and '));
 const duration = (period: CustomPeriod) => Math.round((Date.parse(period.ends_on) - Date.parse(period.starts_on)) / 86400000) + 1;
 async function loadCounts() {
  countsLoading = true;
  countsError = false;
  try { gameDays = await getGameCounts(league.id, season.starts_on, season.ends_on); }
  catch { countsError = true; }
  finally { countsLoading = false; }
 }

 const today = sportsToday;
 const kept = $derived(periods.filter((p) => p.starts_on <= today()));
 const locked = $derived(kept.some((p) => p.is_playoff));
 const bracketRounds = (n: number) => n < 2 ? 0 : Math.ceil(Math.log2(n));
 const rounds = $derived(Math.max(bracketRounds(Math.min(league.settings.format.playoff_teams, teams.length)), league.settings.format.consolation !== false ? bracketRounds(Math.max(0,teams.length-league.settings.format.playoff_teams)) : 0));
 const addDays = (day: string, count: number) => {
  const date = new Date(`${day}T12:00:00Z`);
  date.setUTCDate(date.getUTCDate() + count);
  return date.toISOString().slice(0,10);
 };
 const first = () => [season.starts_on, addDays(today(),1), kept.length ? addDays(kept.at(-1)!.ends_on,1) : season.starts_on].sort().at(-1)!;
 const name = (id: string | null) => teams.find((t) => t.id === id)?.name ?? 'Bye';
 const duplicate = (p: CustomPeriod) => {
  const picked = (p.matchups ?? []).flatMap((m) => [m.home_franchise_id, m.away_franchise_id]).filter(Boolean);
  return new Set(picked).size !== picked.length;
 };
 const invalidSlots = (period: CustomPeriod) => {
  if (!period.is_playoff || !period.matchups) return false;
  const next: Record<string,number> = {};
  const used = new Set<string>();
  for (const pair of period.matchups) {
   const bracket=pair.bracket ?? 'championship';
   const slot=pair.bracket_slot ?? next[bracket] ?? 0;
   const key=`${bracket}/${slot}`;
   if (!Number.isInteger(slot) || slot < 0 || slot > 63 || used.has(key)) return true;
   used.add(key); next[bracket]=Math.max(next[bracket] ?? 0, slot+1);
  }
  return false;
 };
 const stageInvalid = $derived(draft.filter((p) => p.is_playoff).length !== rounds || (!kept.length && !draft.some((p) => !p.is_playoff)) || draft.some((p,i) => !p.is_playoff && draft.slice(0,i).some((prior) => prior.is_playoff)));
 const invalid = $derived(draft.some((p,i) => !p.starts_on || !p.ends_on || p.starts_on > p.ends_on || p.starts_on < first() || p.ends_on > season.ends_on || (i > 0 && p.starts_on <= draft[i-1].ends_on) || duplicate(p) || invalidSlots(p) || p.matchups?.some((m) => !m.home_franchise_id) || p.matchups?.length === 0 || misaligned(p)));

 async function load() {
  periods = await getSchedule(league.id);
  draft = periods.filter((p) => p.starts_on > today()).map((p) => ({ starts_on: p.starts_on, ends_on: p.ends_on, is_playoff: p.is_playoff, double_games: p.double_games,
   matchups: p.by_hand ? p.matchups.map((m) => ({ home_franchise_id: m.home_franchise_id, away_franchise_id: m.away_franchise_id, bracket: m.bracket, bracket_slot: m.bracket_slot })) : null }));
  length = league.settings.format.matchup_days;
  playoffStart = draft.find((p) => p.is_playoff)?.starts_on ?? '';
  dirty = false;
 }
 onMount(() => {
  load().catch(toast.error).finally(() => loading = false);
  void loadCounts();
 });

 function build() {
  if (!Number.isInteger(length) || length < 1 || length > 31 || !playoffStart || playoffStart <= first() || playoffStart > season.ends_on) {
   toast.error('Choose 1–31 days per matchup and a playoff start after the first future matchup.'); return;
  }
  const next: CustomPeriod[] = [];
  for (let start = first(); start < playoffStart;) {
   const end = [addDays(start,length-1),addDays(playoffStart,-1)].sort()[0];
   next.push({starts_on:start,ends_on:end,is_playoff:false,double_games:false,matchups:null});
   start = addDays(end,1);
  }
  let start = playoffStart;
  for (let i=0; i<rounds; i++) {
   const end = i === rounds-1 ? season.ends_on : addDays(start,length-1);
   if (start > end || end > season.ends_on) { toast.error('The season needs more days for all playoff rounds.'); return; }
   next.push({starts_on:start,ends_on:end,is_playoff:true,double_games:false,matchups:null});
   start=addDays(end,1);
  }
  draft=next; dirty=true;
 }
 async function save() {
  busy=true;
  try { await setSchedule(season.id,draft); await load(); await invalidateAll(); toast.good('Custom schedule saved.'); }
  catch (err) { toast.error(err); }
  finally { busy=false; }
 }
 async function automatic() {
  if (!confirm('Rebuild future dates using the league rules? Custom pairings are kept where the dates still match.')) return;
  busy=true;
  try { await generateSchedule(season.id); await load(); await invalidateAll(); toast.good('Automatic schedule restored.'); }
  catch (err) { toast.error(err); }
  finally { busy=false; }
 }
 function customize(period: CustomPeriod) {
  const original = periods.find((p) => p.starts_on === period.starts_on && p.ends_on === period.ends_on);
  period.matchups = original?.matchups.length ? original.matchups.map((m) => ({home_franchise_id:m.home_franchise_id,away_franchise_id:m.away_franchise_id,bracket:m.bracket,bracket_slot:m.bracket_slot})) : [{home_franchise_id:'',away_franchise_id:null}];
  dirty=true;
 }
 function addPeriod() {
  const start=draft.length ? addDays(draft.at(-1)!.ends_on,1) : first();
  draft.push({starts_on:start,ends_on:addDays(start,length-1),is_playoff:false,double_games:false,matchups:null}); dirty=true;
 }
</script>

<div class="stack tight schedule-editor">
 <h3>Schedule & matchups</h3>
 {#if league.competition === 'nhl' && league.settings.format.matchup_days === 7}<p class="muted small-text">NHL defaults to Monday–Sunday matchups, with daily lineups. Custom period dates below override that schedule.</p>{/if}
 <p class="muted small-text">Set dates for every matchup period, choose when playoffs begin, and set opponents or byes. Each period can have a different length. Saved custom dates stay in place until you restore the automatic schedule.</p>
 {#if loading}
  <p class="muted">Loading schedule…</p>
 {:else if locked}
  <p class="muted">The playoffs have begun. Schedule dates are locked; opponents can still be edited from <a href="/matchups">Matchups</a> until a period finishes.</p>
 {:else}
  <fieldset disabled={busy} class="stack tight">
   <div class="row end">
    <label class="field">Matchup length (days)<input type="number" min="1" max="31" bind:value={length} /></label>
    <label class="field">Playoffs start<input type="date" min={first()} max={season.ends_on} bind:value={playoffStart} /></label>
    <button class="small" onclick={build}>Build draft schedule</button>
   </div>
   <p class="muted small-text">{rounds} playoff rounds. Building a draft replaces unsaved dates and pairings. Adjust any period below before saving.</p>
   <p class="muted small-text">Game counts show real {sportName} games across all teams and update as you change dates. Counts use the currently loaded schedule; future dates may be incomplete.</p>
   {#if countsError}<p class="small-text" role="alert">Game counts couldn’t be loaded. <button class="small quiet" onclick={loadCounts}>Retry game counts</button></p>{/if}
   {#if kept.length}<p class="muted small-text">{kept.length} started periods are locked through {dayLabel(kept.at(-1)!.ends_on)}. To start fresh, change the season’s First day above or choose Start tomorrow.</p>{/if}
   {#each draft as period, i (i)}
    <div class="period stack tight" onchange={() => dirty=true}>
     <div class="row end">
      <strong>Period {kept.length+i+1}</strong>
      <label class="field">First day<input type="date" min={first()} max={season.ends_on} bind:value={period.starts_on} /></label>
      <label class="field">Last day<input type="date" min={period.starts_on} max={season.ends_on} bind:value={period.ends_on} /></label>
      <label class="field">Stage<select bind:value={period.is_playoff}><option value={false}>Regular season</option><option value={true}>Playoffs</option></select></label>
      {#if canDouble}<label class="double" title="For a long matchup, like two weeks over the holidays: {doubleNote}, over the whole matchup."><input type="checkbox" bind:checked={period.double_games} /> Double games</label>{/if}
      <button class="small quiet" onclick={() => { draft.splice(i,1); dirty=true; }}>Remove period</button>
     </div>
     {#if period.double_games}<p class="muted small-text">Double games: {doubleNote}, counted across the whole matchup rather than week by week.</p>{/if}
     {#if misaligned(period)}<p class="bad small-text" role="alert">A double-games matchup must run whole lineup weeks, starting on a {league.settings.lineup.week_start}.</p>{/if}

     <div class="row" aria-live="polite" aria-atomic="true">
      {#if countsLoading}<span class="muted small-text">Loading game count…</span>
      {:else if countsError}<span class="muted small-text">Game count unavailable</span>
      {:else if !period.starts_on || !period.ends_on || period.starts_on > period.ends_on}<span class="muted small-text">Choose valid dates to see game counts.</span>
      {:else}
       {@const count = gameCount(period)}
       <span class="pill" class:gold={count === 0}>{count.toLocaleString()} {sportName} {count === 1 ? 'game' : 'games'} loaded</span>
       <span class="muted small-text">{duration(period)} {duration(period) === 1 ? 'day' : 'days'}</span>
      {/if}
     </div>
     {#if period.matchups === null}
      <div class="row"><span class="muted small-text">{period.is_playoff ? 'Automatically set championship and consolation brackets from standings / previous round winners.' : 'Automatic round-robin opponents.'}</span>
       <button class="small" onclick={() => customize(period)}>Set opponents</button>
      </div>
     {:else}
      {#each period.matchups as pair, j (j)}
       <div class="row">
        {#if period.is_playoff}
         <label class="field">Bracket<select value={pair.bracket ?? 'championship'} onchange={(e) => { pair.bracket=e.currentTarget.value as 'championship' | 'consolation'; delete pair.bracket_slot; dirty=true; }}><option value="championship">Championship</option><option value="consolation">Consolation</option></select></label>
         <label class="field">Bracket position<input type="number" min="1" max="64" value={(pair.bracket_slot ?? period.matchups!.filter((m) => (m.bracket ?? 'championship') === (pair.bracket ?? 'championship')).indexOf(pair))+1} onchange={(e) => {pair.bracket_slot=Number(e.currentTarget.value)-1; dirty=true;}} /></label>
        {/if}
        <select aria-label="Home team for period {kept.length+i+1}, matchup {j+1}" bind:value={pair.home_franchise_id}>
         <option value="" disabled>Choose team…</option>{#each teams as team (team.id)}<option value={team.id}>{team.name}</option>{/each}
        </select>
        <span class="muted">vs</span>
        <select aria-label="Away team for period {kept.length+i+1}, matchup {j+1}" bind:value={pair.away_franchise_id}>
         <option value={null}>Bye</option>{#each teams as team (team.id)}<option value={team.id}>{team.name}</option>{/each}
        </select>
        <button class="small quiet" onclick={() => { period.matchups!.splice(j,1); dirty=true; }}>Remove matchup</button>
       </div>
      {/each}
      {#if duplicate(period)}<p class="bad small-text" role="alert">Each team can appear only once per period.</p>{/if}
      <div class="row">
       <button class="small" onclick={() => { period.matchups!.push({home_franchise_id:'',away_franchise_id:null}); dirty=true; }}>Add matchup</button>
       <button class="small quiet" onclick={() => { period.matchups=null; dirty=true; }}>Use automatic opponents</button>
      </div>
      <p class="muted small-text">Sitting out: {teams.filter((t) => !period.matchups!.some((m) => m.home_franchise_id===t.id || m.away_franchise_id===t.id)).map((t) => name(t.id)).join(', ') || 'None'}.</p>
     {/if}
    </div>
   {/each}
   {#if invalid}<p class="bad small-text" role="alert">Check dates, opponents and bracket positions. Periods must be in order, within the season, and cannot overlap. Each bracket position can be used only once.</p>{/if}
   {#if stageInvalid}<p class="bad small-text" role="alert">Include regular-season play followed by exactly {rounds} playoff rounds.</p>{/if}
   <div class="row">
    <button class="small" onclick={addPeriod}>Add period</button>
    <button class="primary" disabled={!dirty || invalid || stageInvalid || !draft.length} onclick={save}>{busy ? 'Saving…' : 'Save schedule & matchups'}</button>
    <button class="small quiet" onclick={automatic}>Restore automatic schedule</button>
    {#if dirty}<span class="pill gold">Unsaved changes</span>{/if}
   </div>
  </fieldset>
 {/if}
</div>

<style>
 .schedule-editor { border-top: 1px solid var(--rule); padding-top: 1rem; }
 fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
 .period { border: 1px solid var(--rule); border-radius: .5rem; padding: .8rem; }
 input[type='number'] { max-width: 8rem; }
 .bad { color: var(--bad); }
 .double { display: inline-flex; align-items: center; gap: .35rem; font-size: .85rem; font-weight: 600; white-space: nowrap; }
</style>
