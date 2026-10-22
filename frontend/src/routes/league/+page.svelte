<script lang="ts">
	// League overview: standings and the latest activity across sports.
	import { invalidateAll } from '$app/navigation';
	import Activity from '#lib/Activity.svelte';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Standings from '#lib/Standings.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let sport = $state('all');
	let activityType = $state('all');
	const filtered = $derived(sport !== 'all' || activityType !== 'all');
	const activity = $derived(data.activity.filter(item =>
		(sport === 'all' || item.competition === sport) &&
		(activityType === 'all' || (activityType === 'roster'
			? ['add', 'drop', 'claim', 'sign', 'unsigned', 'move', 'graduated', 'released'].includes(item.kind)
			: item.kind === activityType))));

	function clearFilters() { sport = 'all'; activityType = 'all'; }

	// Standings and activity refresh as games are played.
	onScoresChange(() => data.dynasty?.leagues.map((l) => l.competition) ?? [], invalidateAll);
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
				odds={data.odds}
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
				<Activity items={activity} />
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
	@media (max-width: 640px) {
		.activity-header { align-items: flex-start; }
		.filters { width: 100%; }
		.filters select { flex: 1; min-width: 0; }
	}
</style>
