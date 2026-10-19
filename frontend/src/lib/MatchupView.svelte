<script lang="ts">
	import { getLineup, getMatchup, type Franchise, type League, type Lineup, type MatchupDetail, type TeamIdentity } from '#lib/api.ts';
	import { matchupRows } from '#lib/matchupView.ts';
	import MatchupStats from '#lib/MatchupStats.svelte';
	import { teamIdentity } from '#lib/identity.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel, points, shiftDay, sportsToday } from '#lib/ui/time.ts';

	let { id, league, franchises, identities = [] }: {
		id: string; league: League; franchises: Franchise[]; identities?: TeamIdentity[];
	} = $props();
	let matchup = $state<MatchupDetail>();
	let lineups = $state<(Lineup | undefined)[]>([]);
	let day = $state('');
	let wholePeriod = $state(false);
	let showOther = $state(false);
	let loading = $state(true);
	let loadingLineups = $state(false);
	let error = $state('');
	let lineupError = $state('');
	let revision = $state(0);
	let attempt = $state(0);
	const weekly = $derived(league.settings.lineup.period === 'week');
	const pickedGames = $derived(league.settings.lineup.slots.some(s => (s.games_per_week ?? 0) > 0));

	$effect(() => {
		const key = id;
		void attempt;
		let active = true;
		loading = true;
		getMatchup(key).then(m => {
			if (!active) return;
			matchup = m;
			lineups = [];
			const today = sportsToday();
			day = today < m.period.starts_on ? m.period.starts_on : today > m.period.ends_on ? m.period.ends_on : today;
			wholePeriod = false;
			error = '';
			loading = false;
		}, e => { if (active) { error = String(e); loading = false; } });
		return () => { active = false; };
	});
	$effect(() => {
		const current = matchup;
		const selected = day;
		const leagueId = league.id;
		void revision;
		if (!current || current.id !== id || !selected) return;
		let active = true;
		loadingLineups = true;
		Promise.all([current.home_franchise_id, current.away_franchise_id].map(f =>
			f ? getLineup(leagueId, f, selected) : undefined)).then(found => {
			if (active) { lineups = found; loadingLineups = false; lineupError = ''; }
		}, e => { if (active) { loadingLineups = false; lineupError = String(e); } });
		return () => { active = false; };
	});
	onScoresChange(() => [league.competition], async () => {
		const key = id;
		try { const found = await getMatchup(key); if (key === id) { matchup = found; revision++; } }
		catch { /* Keep the last score until the connection recovers. */ }
	});
	const sides = $derived(matchup ? [
		{ id: matchup.home_franchise_id, total: matchup.home_points, projected: matchup.home_projected, scored: matchup.home_players, lineup: lineups[0] },
		{ id: matchup.away_franchise_id, total: matchup.away_points, projected: matchup.away_projected, scored: matchup.away_players, lineup: lineups[1] }
	].map(side => ({ ...side,
		identity: side.id ? teamIdentity(franchises.find(f => f.id === side.id), identities, league.id) : { name: 'Bye', image_url: '' },
		rows: matchupRows(side.scored, side.lineup, matchup!.period, wholePeriod, day, weekly)
	})) : []);
	const windowStart = $derived(weekly ? lineups.find(Boolean)?.day ?? day : day);
	const windowEnd = $derived(weekly ? lineups.find(Boolean)?.last_day ?? day : day);
	const dates = $derived.by(() => {
		if (!matchup || weekly) return [];
		const days: string[] = [];
		for (let d = matchup.period.starts_on; d <= matchup.period.ends_on; d = shiftDay(d, 1)) days.push(d);
		return days;
	});
	function step(direction: number) {
		if (!matchup) return;
		const next = shiftDay(direction < 0 ? windowStart : windowEnd, direction);
		day = next < matchup.period.starts_on ? matchup.period.starts_on : next > matchup.period.ends_on ? matchup.period.ends_on : next;
		wholePeriod = false;
	}
</script>

{#if loading}<p class="muted" role="status">Loading matchup…</p>
{:else if error}<p role="alert">{error}</p><button class="small" onclick={() => attempt++}>Try again</button>
{:else if matchup}
	<div class="stack matchup-view" data-sport={league.competition}>
		<section class="scoreboard card" aria-label="Matchup totals">
			{#each sides as side, index (index)}
				<div class="score-team" class:away={index === 1} class:leader={side.total > sides[1-index].total}>
					<Crest src={side.identity.image_url} name={side.identity.name} size={24} />
					<strong>{side.identity.name}</strong>
					<span class="score" title="Full matchup fantasy points">{side.id ? points(side.total) : '—'}{#if side.projected != null}<small class="proj" title="Points so far plus what the starters are projected to add, from recent seasons and the betting lines">Proj {points(side.projected)}</small>{/if}</span>
				</div>
				{#if index === 0}<div class="score-state"><span class="pill" class:good={!matchup.final && sportsToday() >= matchup.period.starts_on}>{matchup.final ? 'Final' : sportsToday() < matchup.period.starts_on ? 'Upcoming' : 'In progress'}</span>{#if matchup.period.double_games}<small>Double games</small>{/if}</div>{/if}
			{/each}
		</section>

		<div class="spread view-controls">
			<div class="row modes" aria-label="Stats view">
				<button aria-pressed={!wholePeriod} onclick={() => wholePeriod = false}>{weekly ? pickedGames ? 'Selected games' : 'Weekly lineup' : 'Daily stats'}</button>
				<button aria-pressed={wholePeriod} onclick={() => wholePeriod = true}>Matchup totals</button>
			</div>
			<label class="row small-text"><input type="checkbox" bind:checked={showOther} /> Uncounted games</label>

		{#if !wholePeriod}
			<nav class="row date-controls" aria-label={weekly ? 'Lineup week' : 'Scoring day'}>
				<button class="small" aria-label={weekly ? 'Previous lineup week' : 'Previous scoring day'} disabled={loadingLineups || windowStart <= matchup.period.starts_on} onclick={() => step(-1)}><Icon name="left" size={16} /></button>
				{#if weekly}
					<strong>{dayLabel(windowStart < matchup.period.starts_on ? matchup.period.starts_on : windowStart)} – {dayLabel(windowEnd > matchup.period.ends_on ? matchup.period.ends_on : windowEnd)}</strong>
				{:else}
					<select aria-label="Scoring day" bind:value={day}>{#each dates as d (d)}<option value={d}>{dayLabel(d)}{d === sportsToday() ? ' · Today' : ''}</option>{/each}</select>
				{/if}
				<button class="small" aria-label={weekly ? 'Next lineup week' : 'Next scoring day'} disabled={loadingLineups || windowEnd >= matchup.period.ends_on} onclick={() => step(1)}><Icon name="right" size={16} /></button>
			</nav>
		{/if}
		</div>
		{#if lineupError}<p role="alert">{lineupError}</p><button class="small" onclick={() => revision++}>Retry lineups</button>{/if}
		{#if loadingLineups}<p class="muted small-text" role="status">Loading lineups…</p>{/if}
		<div class="teams" hidden={loadingLineups} aria-busy={loadingLineups}>
			{#each sides as side, index (index)}
				<section class="card flush team-panel">
					<header class="spread"><strong>{side.identity.name}</strong><span class="scope-score">{side.id ? points(side.rows.reduce((n,p) => n+p.points,0)) : '—'} <small>{wholePeriod ? 'total FP' : weekly ? side.lineup?.doubled ? 'matchup FP' : 'week FP' : 'day FP'}</small></span></header>
					{#if side.lineup?.starts && !wholePeriod}<p class="cap small-text">Pitcher starts: {side.lineup.starts.made.filter(s => s.counts).length} / {side.lineup.starts.limit}{side.lineup.doubled ? ' · shared across double matchup' : ' this week'}</p>{/if}
					{#if !side.id}<p class="empty muted">No opponent this round.</p>
					{:else if side.rows.length === 0}<p class="empty muted">No starters or scored contributions for this view.</p>
					{:else}
						<MatchupStats rows={side.rows} sport={league.competition} scoring={league.settings.scoring} {showOther} picking={pickedGames} />
					{/if}
				</section>
			{/each}
		</div>
		<p class="muted scoring-note">FP = counted fantasy points. Expand a player for individual games and box scores.</p>
	</div>
{/if}

<style>
	.matchup-view { min-width: 0; grid-template-columns: minmax(0,1fr); gap: .45rem; }
	.scoreboard { display: grid; grid-template-columns: minmax(0,1fr) auto minmax(0,1fr); gap: .75rem; align-items: center; padding: .65rem .75rem; border-top: 2px solid var(--sport); }
	.score-team { display: flex; align-items: center; gap: .45rem; min-width: 0; }
	.score-team.away { flex-direction: row-reverse; text-align: right; }
	.score-team strong { flex: 1; min-width: 0; font-size: .85rem; overflow-wrap: anywhere; }
	.score { font: 800 1.65rem/1.1 var(--display); font-variant-numeric: tabular-nums; }
	.leader .score { color: var(--sport); }
	.score .proj { display: block; font: italic 500 .65rem var(--body); color: var(--ink-soft); }
	.score-state { display: grid; gap: .2rem; text-align: center; justify-items: center; }
	.score-state .pill { font-size: .55rem; }
	.score-state small { font-size: .6rem; color: var(--ink-soft); }
	.view-controls { gap: .4rem .75rem; }
	.view-controls label { margin-left: auto; font-size: .67rem; gap: .3rem; }
	.modes { gap: .2rem; }
	.modes button { font-size: .7rem; padding: .3rem .45rem; }
	.modes button[aria-pressed='true'] { background: var(--brand-soft); color: var(--brand); border-color: var(--brand); }
	.date-controls { gap: .3rem; font-size: .72rem; margin-left: auto; }
	.date-controls select { font-size: .72rem; padding: .3rem .4rem; }
	.date-controls button { padding: .25rem .35rem; }
	.teams { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: .5rem; align-items: start; }
	.teams[hidden] { display: none; }
	.team-panel { min-width: 0; }
	.team-panel header { padding: .45rem .5rem; border-bottom: 1px solid var(--rule); background: var(--surface-2); font-size: .78rem; }
	.scope-score { font-weight: 750; font-variant-numeric: tabular-nums; }
	.scope-score small { color: var(--ink-soft); font-weight: 500; font-size: .6rem; }
	.cap, .empty { padding: .4rem .5rem; font-size: .7rem; }
	.scoring-note { font-size: .65rem; }
	@media(max-width: 800px) {
		.teams { grid-template-columns: minmax(0,1fr); }
		.scoreboard { padding: .5rem; gap: .4rem; }
		.score-team { flex-wrap: wrap; gap: .2rem .35rem; }
		.score-team strong { font-size: .75rem; }
		.score { width: 100%; font-size: 1.4rem; }
		.score-team.away .score { text-align: right; }
		.score-team:first-child .score { text-align: left; }
		.date-controls { margin-left: 0; }
	}
</style>
