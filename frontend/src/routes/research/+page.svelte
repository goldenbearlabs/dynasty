<script lang="ts">
	import { untrack } from 'svelte';
	import { page as route } from '$app/state';
	import { getResearch, type ResearchPage } from '#lib/api.ts';
	import Tabs from '#lib/ui/Tabs.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import PlayerResearch from '../draft/[id]/Research.svelte';
	import { points } from '#lib/ui/time.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let competition = $state(untrack(() => route.url.searchParams.get('competition') ?? ''));
	let season = $state('');
	let status = $state('');
	let search = $state('');
	let query = $state('');
	let availableOnly = $state(false);
	let sort = $state('points');
	let page = $state(1);
	let selectedId = $state<string>();
	let result = $state<ResearchPage>();
	let error = $state('');
	let loading = $state(true);
	let retry = $state(0);

	const tabs = $derived([
		{ value: '', label: 'All leagues' },
		...data.competitions.map((c) => ({ value: c.key, label: c.name, sport: c.key }))
	]);
	const league = $derived(data.dynasty?.leagues.find((l) => l.competition === competition));
	const shownCompetitions = $derived(data.competitions.filter((c) => !competition || c.key === competition));
	// Use only stats with a nonzero scoring weight. Across leagues the union
	// preserves each sport's columns; unavailable stats are shown as a dash.
	const stats = $derived.by(() => {
		const columns = new Map<string, { key: string; label: string }>();
		for (const c of shownCompetitions) {
			const rules = data.dynasty?.leagues.find((l) => l.competition === c.key)?.settings.scoring ?? {};
			for (const [key, weight] of Object.entries(rules)) {
				if (weight !== 0 && !columns.has(key)) columns.set(key, c.stats.find((s) => s.key === key) ?? { key, label: key });
			}
		}
		return [...columns.values()];
	});
	const pages = $derived(result ? Math.max(1, Math.ceil(result.total / result.per_page)) : 1);
	const number = (value: number | undefined) => value === undefined ? '—' : value.toLocaleString(undefined, { maximumFractionDigits: 1 });
	const short = (key: string) => key.replaceAll('_', ' ').toUpperCase();

	$effect(() => {
		const value = search.trim();
		const timer = setTimeout(() => { query = value; }, 250);
		return () => clearTimeout(timer);
	});
	let previousCompetition = untrack(() => competition);
	$effect.pre(() => {
		if (competition !== previousCompetition) {
			previousCompetition = competition;
			season = '';
			availableOnly = false;
			sort = 'points';
			selectedId = undefined;
		}
	});
	let previousFilters = '';
	$effect.pre(() => {
		const filters = JSON.stringify([competition, season, status, query, availableOnly, sort]);
		if (filters !== previousFilters) { previousFilters = filters; page = 1; }
	});
	$effect(() => {
		void retry;
		let active = true;
		loading = true;
		error = '';
		getResearch({ competition, season, status, q: query, sort, page, available_in: availableOnly ? league?.id : undefined })
			.then((r) => { if (active) result = r; })
			.catch((e: Error) => { if (active) error = e.message; })
			.finally(() => { if (active) loading = false; });
		return () => { active = false; };
	});
</script>

<svelte:head><title>Research</title></svelte:head>

<div class="stack">
	<header class="stack tight">
		<h1>Research</h1>
		<p class="muted">Compare season stats and fantasy production using your league’s current scoring rules.</p>
		<Tabs {tabs} bind:value={competition} label="League" />
	</header>

	<div class="row filters">
		<label class="search"><Icon name="search" size={16} /><input type="search" placeholder="Search players" aria-label="Search players" bind:value={search} /></label>
		<select aria-label="Season" bind:value={season}>
			<option value="">Latest season per league</option>
			{#each result?.seasons ?? [] as s (`${s.year}-${s.label}`)}<option value={s.label}>{s.label}</option>{/each}
		</select>
		<select aria-label="Status" bind:value={status}>
			<option value="">Any status</option><option value="active">Active</option><option value="prospect">Prospects</option><option value="inactive">Inactive</option>
		</select>
		<label class="row"><span class="muted small-text">Sort by</span><select aria-label="Sort by" bind:value={sort}>
			<option value="points">Fantasy points</option><option value="points_per_game">FP per game</option><option value="games">Games played</option><option value="name">Player name</option>
			{#each stats as stat (stat.key)}<option value={stat.key}>{stat.label}</option>{/each}
		</select></label>
		{#if league}<label class="check"><input type="checkbox" bind:checked={availableOnly} />Available only</label>{/if}
	</div>

	<div class="spread small-text muted" aria-live="polite">
		<span>{loading ? 'Loading players…' : `${(result?.total ?? 0).toLocaleString()} players`} · Season totals</span>
		<span>FP = fantasy points · Click a player for history</span>
	</div>
	{#if !competition}<p class="muted small-text">Each league uses its own scoring rules and latest imported season. Select a league to focus on its scoring stats.</p>{/if}

	{#if error}
		<div role="alert" class="row"><p>Could not load research: {error}</p><button onclick={() => retry++}>Retry</button></div>
	{:else if result}
		{#if result.players.length === 0}
			<Empty icon="search" title="No players match">Try another search, league or status.</Empty>
		{:else}
			<div class="panel scroll" class:loading aria-busy={loading}>
				<table>
					<thead><tr>
						<th class="identity">Player</th><th>Pos</th><th>Team</th><th>Season</th>
						<th class="num">GP</th><th class="num fantasy">FP</th><th class="num fantasy">FP / GP</th>
						{#each stats as stat (stat.key)}<th class="num" title={stat.label}>{short(stat.key)}</th>{/each}
						<th>Owner</th>
					</tr></thead>
					<tbody>{#each result.players as player (player.id)}<tr class:selected={selectedId === player.id}>
						<td class="identity"><div class="player"><Headshot name={player.full_name} src={player.headshot_url} size={36} /><div>
							<button class="player-name" aria-pressed={selectedId === player.id} onclick={() => selectedId = player.id}>{player.full_name}</button>
							<div class="row detail">{#if !competition}<SportBadge sport={player.competition} />{/if}{#if player.status !== 'active'}<span class="pill">{player.status}</span>{/if}</div>
						</div></div></td>
						<td>{player.positions.join('/') || '—'}</td><td class="muted">{player.team || '—'}</td><td>{player.season || 'No stats'}</td>
						<td class="num">{player.season ? number(player.games) : '—'}</td>
						<td class="num fantasy">{player.season ? points(player.points) : '—'}</td>
						<td class="num fantasy">{player.games > 0 ? points(player.points_per_game) : '—'}</td>
						{#each stats as stat (stat.key)}<td class="num">{number(player.stats[stat.key])}</td>{/each}
						<td>{#if player.owner_slug}<a href="/franchise/{player.owner_slug}">{player.owner_name}</a>{:else}<span class="muted">Available</span>{/if}</td>
					</tr>{/each}</tbody>
				</table>
			</div>
			{#if pages > 1}<nav class="spread" aria-label="Research pages"><button disabled={loading || page <= 1} onclick={() => page--}>Previous</button><span class="muted small-text">Page {page} of {pages.toLocaleString()}</span><button disabled={loading || page >= pages} onclick={() => page++}>Next</button></nav>{/if}
			<p class="muted small-text">A dash means no imported stat. Players without season data stay visible; stats appear after the season history feed is synced.</p>
		{/if}
	{/if}

	{#if selectedId}
		<div class="history panel"><div class="spread history-header"><h2>Player details</h2><button class="quiet small" onclick={() => selectedId = undefined}>Close</button></div>
			<PlayerResearch {selectedId} competitions={data.competitions} action={playerAction} />
		</div>
	{/if}
</div>

{#snippet playerAction()}{/snippet}

<style>
	.search { flex: 1 1 14rem; display: flex; align-items: center; gap: 0.5rem; padding-left: 0.7rem; background: var(--surface); border: 1px solid var(--rule-strong); border-radius: var(--radius-small); color: var(--ink-faint); }
	.search:focus-within { outline: 2px solid var(--brand); outline-offset: 1px; }
	.search input { min-width: 0; flex: 1; border: none; background: transparent; padding-left: 0; outline: none; }
	.panel { transition: opacity 0.15s; }
	.loading { opacity: 0.55; }
	.player { display: flex; align-items: center; gap: 0.7rem; }
	.player-name { padding: 0; border: 0; background: transparent; font-weight: 700; text-align: left; white-space: nowrap; }
	.player-name:hover { color: var(--brand); text-decoration: underline; }
	.detail { gap: 0.3rem; font-size: 0.7rem; }
	th.num { text-align: right; }
	td, th { white-space: nowrap; }
	.fantasy { color: var(--brand); font-weight: 700; background: var(--brand-soft); }
	.identity { position: sticky; left: 0; z-index: 1; background: var(--surface); min-width: 14rem; }
	tr.selected td { background: var(--brand-soft); }
	.history { max-width: 58rem; }
	.history-header { padding: 0.75rem; border-bottom: 1px solid var(--rule); }
	.history-header h2 { font-size: 1rem; }
</style>
