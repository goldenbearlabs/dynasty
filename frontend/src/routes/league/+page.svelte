<script lang="ts">
	// League overview: standings and the latest activity across sports.
	import { untrack } from 'svelte';
	import { getOdds, type Odds } from '#lib/api.ts';
	import { invalidate } from '$app/navigation';
	import Activity from '#lib/Activity.svelte';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Standings from '#lib/Standings.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let odds = $state<Odds>();
	let oddsDynasty = '';
	$effect(() => {
		const dynasty = data.dynasty?.id;
		void data.standings;
		// Keep the last odds visible during a score refresh so columns stay put.
		// A different dynasty must never inherit the previous one's odds.
		if (oddsDynasty !== (dynasty ?? '')) {
			oddsDynasty = dynasty ?? '';
			odds = undefined;
		}
		let active = true;
		if (dynasty) getOdds().then(value => { if (active) odds = value; }, () => {});
		return () => { active = false; };
	});
	let sport = $state('all');
	// Picking a league's standings filters the activity to it; Overall shows every sport.
	let standingsTab = $state('');
	$effect(() => {
		const tab = standingsTab;
		// Only a new tab resets the filter, not a score refresh.
		sport = untrack(() => data.dynasty?.leagues.some((l) => l.competition === tab)) ? tab : 'all';
	});
	let activityType = $state('all');
	const filtered = $derived(sport !== 'all' || activityType !== 'all');
	const activity = $derived(data.activity.filter(item =>
		(sport === 'all' || item.competition === sport) &&
		(activityType === 'all' || (activityType === 'roster'
			? ['add', 'drop', 'claim', 'sign', 'unsigned', 'move', 'graduated', 'released'].includes(item.kind)
			: item.kind === activityType))));

	function clearFilters() { sport = 'all'; activityType = 'all'; }

	// Refresh this page's scores without reloading shared league/session data.
	onScoresChange(
		() => data.dynasty?.leagues.map((l) => l.competition) ?? [],
		() => { void invalidate('app:league-overview'); }
	);
</script>

<svelte:head><title>League · {data.dynasty?.name ?? 'Crossover Dynasty'}</title></svelte:head>

{#if data.dynasty}
	<div class="stack overview">
		<header>
			<p class="eyebrow">League overview</p>
			<h1>{data.dynasty.name}</h1>
		</header>

		<section class="stack tight" aria-labelledby="standings-heading">
			<h2 id="standings-heading">Standings</h2>
			<Standings
				identities={data.dynasty.team_identities}
				tables={data.standings}
				overall={data.overall}
				{odds}
				bind:shown={standingsTab}
				franchises={data.dynasty.franchises}
				leagues={data.dynasty.leagues}
			/>
		</section>

		<section class="stack tight" aria-labelledby="activity-heading">
			<div class="spread activity-header">
				<h2 id="activity-heading">Recent activity</h2>
				<div class="filters">
					<select aria-label="Filter activity by sport" bind:value={sport}>
						<option value="all">All sports</option>
						{#each data.dynasty.leagues as league (league.id)}
							<option value={league.competition}>{league.competition.toUpperCase()}</option>
						{/each}
					</select>
					<select aria-label="Filter activity by type" bind:value={activityType}>
						<option value="all">All activity</option>
						<option value="trade">Trades</option>
						<option value="roster">Roster moves</option>
						<option value="settings">Rule changes</option>
					</select>
					{#if filtered}<button class="quiet small" onclick={clearFilters}>Clear filters</button>{/if}
				</div>
			</div>
			{#if filtered && activity.length === 0}
				<Empty icon="clock" title="No matching activity">Try another sport or activity type.</Empty>
			{:else}
				<!-- svelte-ignore a11y_no_noninteractive_tabindex: the feed scrolls with the keyboard too -->
				<div class="activity-scroll" role="region" aria-label="Recent activity" tabindex="0"><Activity items={activity} /></div>
			{/if}
		</section>
	</div>
{/if}

<style>
	header { display: grid; gap: 0.25rem; }
	.overview { gap: 1.5rem; }
	.activity-header { gap: 0.65rem; }
	.filters { display: flex; align-items: center; flex-wrap: wrap; gap: 0.4rem; }
	.filters select { padding: 0.35rem 0.55rem; font-size: 0.8rem; }
	/* The feed scrolls in its own box rather than running down the page. */
	.activity-scroll { max-height: min(30rem, 70dvh); overflow-y: auto; overscroll-behavior: contain; border: 1px solid var(--rule); border-radius: var(--radius); scrollbar-width: thin; }
	@media (max-width: 640px) {
		.activity-header { align-items: flex-start; }
		.filters { width: 100%; }
		.filters select { flex: 1; min-width: 0; }
	}
</style>
