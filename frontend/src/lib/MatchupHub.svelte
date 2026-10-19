<script lang="ts">
	import { untrack } from 'svelte';
	import { getMatchups, type Franchise, type League, type Matchups, type TeamIdentity } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import { teamIdentity } from '#lib/identity.ts';
	import MatchupView from '#lib/MatchupView.svelte';
	import MatchupEditor from '#lib/MatchupEditor.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';

	let { league, franchises, identities = [], mine, editable = false, initialId, initialPeriod }: {
		league: League; franchises: Franchise[]; identities?: TeamIdentity[]; mine?: string;
		editable?: boolean; initialId?: string; initialPeriod?: number;
	} = $props();
	let seq = $state<number | undefined>(untrack(() => initialPeriod));
	let selected = $state(untrack(() => initialId ?? ''));
	let view = $state<Matchups>();
	let error = $state('');
	let loading = $state(true);
	let editing = $state(false);
	let request = 0;
	async function load() {
		const token = ++request;
		try {
			const found = await getMatchups(league.id, seq);
			if (token !== request) return;
			view = found;
			if (!found.matchups.some(m => m.id === selected)) {
				selected = found.matchups.find(m => m.home_franchise_id === mine || m.away_franchise_id === mine)?.id ?? found.matchups[0]?.id ?? '';
			}
			error = '';
		} catch (e) { if (token === request) error = String(e); }
		finally { if (token === request) loading = false; }
	}
	$effect(() => { void [league.id, seq]; loading = true; load(); return () => { request++; }; });
	onScoresChange(() => [league.competition], load);
	const period = $derived(view?.period);
	const matchups = $derived(view?.matchups ?? []);
	const index = $derived(matchups.findIndex(m => m.id === selected));
	const identity = (id: string | null) => id ? teamIdentity(franchises.find(f => f.id === id), identities, league.id) : { name: 'Bye', image_url: '' };
	function move(by: number) {
		const next = matchups[index + by];
		if (next) selected = next.id;
	}
	function changePeriod(value: number) { seq = value; editing = false; }
	let picker = $state<HTMLElement>();
	$effect(() => {
		void selected;
		const nav = picker;
		if (!nav) return;
		const button = nav.querySelector<HTMLElement>('[aria-pressed="true"]');
		if (button) nav.scrollTo({ left: button.offsetLeft - (nav.clientWidth-button.clientWidth)/2, behavior: 'auto' });
	});
	let touch: { x: number; y: number } | undefined;
	function touchStart(event: TouchEvent) {
		if (event.touches.length !== 1 || (event.target as HTMLElement).closest('a,button,input,select')) { touch = undefined; return; }
		touch = { x: event.touches[0].clientX, y: event.touches[0].clientY };
	}
	function touchEnd(event: TouchEvent) {
		if (!touch || !event.changedTouches.length) return;
		const dx = event.changedTouches[0].clientX - touch.x;
		const dy = event.changedTouches[0].clientY - touch.y;
		touch = undefined;
		if (Math.abs(dx) > 65 && Math.abs(dx) > Math.abs(dy) * 1.5) { event.preventDefault(); move(dx < 0 ? 1 : -1); }
	}
</script>

<div class="stack matchup-hub" data-sport={league.competition}>
	{#if error}<p role="alert">{error}</p><button class="small" onclick={load}>Try again</button>{/if}
	{#if loading}<p role="status" class="muted">Loading matchups…</p>
	{:else if period && view}
		<div class="spread period-controls">
			<nav class="row" aria-label="Matchup period">
				<button aria-label="Previous matchup period" disabled={period.seq <= 1} onclick={() => changePeriod(period.seq-1)}><Icon name="left" size={16} /></button>
				<select aria-label="Matchup period" value={period.seq} onchange={e => changePeriod(Number(e.currentTarget.value))}>
					{#each view.periods as p (p.id)}<option value={p.seq}>{p.is_playoff ? 'Playoffs' : 'Matchup'} {p.seq} · {dayLabel(p.starts_on)} – {dayLabel(p.ends_on)}{p.double_games ? ' · Double games' : ''}</option>{/each}
				</select>
				<button aria-label="Next matchup period" disabled={period.seq >= (view.periods.at(-1)?.seq ?? 0)} onclick={() => changePeriod(period.seq+1)}><Icon name="right" size={16} /></button>
			</nav>
			{#if editable && !matchups.some(m => m.final)}<button class="quiet small" onclick={() => editing = !editing}>{editing ? 'Close editor' : 'Edit pairings'}</button>{/if}
		</div>
		{#if editing}
			{#key period.id}<MatchupEditor {period} {matchups} teams={franchises.map(f => ({id:f.id,name:identity(f.id).name}))} ondone={changed => { editing=false; if(changed) load(); }} />{/key}
		{:else if matchups.length}
			<nav bind:this={picker} class="matchup-picker" aria-label="Choose matchup">
				{#each matchups as m, i (m.id)}
					<button class:selected={selected === m.id} aria-pressed={selected === m.id} onclick={() => selected = m.id}>
						<span class="pair"><span>{identity(m.home_franchise_id).name}</span><strong>{points(m.home_points)}</strong></span>
						<span class="pair"><span>{identity(m.away_franchise_id).name}</span><strong>{m.away_franchise_id ? points(m.away_points) : '—'}</strong></span>
						{#if mine && (m.home_franchise_id === mine || m.away_franchise_id === mine)}<small>Your matchup</small>{:else}<small>{period.is_playoff ? m.bracket === 'consolation' ? 'Consolation' : 'Championship' : `Matchup ${i+1}`}</small>{/if}
					</button>
				{/each}
			</nav>
			<div class="spread">
				<div class="row matchup-nav" aria-label="Browse matchups">
					<button aria-label="Previous matchup" disabled={index <= 0} onclick={() => move(-1)}><Icon name="left" size={16} /></button>
					<span class="small-text muted" aria-live="polite">{index+1} of {matchups.length} · Swipe to browse</span>
					<button aria-label="Next matchup" disabled={index >= matchups.length-1} onclick={() => move(1)}><Icon name="right" size={16} /></button>
				</div>
				<a class="small-text" href="/matchups/{selected}">Matchup link <Icon name="link" size={13} /></a>
			</div>
			<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions: This focusable carousel region supports arrow keys as an alternative to swiping. -->
			<div class="swipe-area" role="region" aria-label="Selected matchup; left and right arrow keys browse matchups" tabindex="0"
				ontouchstart={touchStart} ontouchend={touchEnd} ontouchcancel={() => touch = undefined}
				onkeydown={event => { if(event.target !== event.currentTarget) return; if(event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); move(event.key === 'ArrowLeft' ? -1 : 1); } }}>
				<MatchupView id={selected} {league} {franchises} {identities} />
			</div>
		{:else}<Empty icon="scores" title="Matchups not set yet">Playoff pairings appear when the previous round finishes.</Empty>{/if}
	{:else if !error}<Empty icon="scores" title="No matchups scheduled">The commissioner can start this league’s season and set its schedule.</Empty>{/if}
</div>

<style>
	.matchup-hub { min-width: 0; grid-template-columns: minmax(0,1fr); }
	.period-controls nav { min-width: 0; flex-wrap: nowrap; gap: .4rem; }
	.period-controls select { min-width: 0; max-width: 30rem; font-size: .8rem; }
	nav button, .matchup-nav button { padding: .4rem .5rem; }
	.matchup-picker { position: relative; display: flex; gap: .5rem; overflow-x: auto; padding-bottom: .35rem; scrollbar-width: thin; scroll-snap-type: x proximity; }
	.matchup-picker button { flex: 0 0 12.5rem; text-align: left; display: grid; gap: .25rem; padding: .65rem .75rem; scroll-snap-align: start; }
	.matchup-picker button.selected { border-color: var(--sport); background: var(--surface-2); box-shadow: inset 0 -2px var(--sport); }
	.pair { display: flex; justify-content: space-between; gap: .5rem; font-size: .8rem; }
	.pair span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pair strong { font-variant-numeric: tabular-nums; }
	.matchup-picker small { color: var(--ink-soft); font-weight: 500; font-size: .65rem; }
	.matchup-nav { gap: .5rem; }
	.swipe-area { min-width: 0; touch-action: pan-y pinch-zoom; }
	.swipe-area:focus-visible { outline: 2px solid var(--brand); outline-offset: 4px; border-radius: var(--radius); }
	@media(max-width:640px) { .period-controls nav { width: 100%; } .period-controls select { flex: 1; } }
</style>
