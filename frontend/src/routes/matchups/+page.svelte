<script lang="ts">
	// Every head-to-head league, one sport after another: the matchups in
	// progress, or the whole season's schedule for one team or all of them.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import Matchups from '#lib/Matchups.svelte';
	import Schedule from '#lib/Schedule.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const leagues = $derived(data.dynasty?.leagues.filter((l) => l.settings.format.type === 'head_to_head') ?? []);
	const franchises = $derived(data.dynasty?.franchises ?? []);

	// A link can open the schedule on a team: ?view=schedule&team=<franchise id>.
	const asked = untrack(() => page.url.searchParams);
	let view = $state<'now' | 'schedule'>(asked.get('view') === 'schedule' ? 'schedule' : 'now');
	let team = $state(asked.get('team') ?? untrack(() => data.me?.id) ?? ''); // empty for every team
</script>

<svelte:head><title>Matchups</title></svelte:head>

<div class="stack">
	<header class="spread">
		<h1>Matchups</h1>
		{#if view === 'schedule'}
			<select aria-label="Team" bind:value={team}>
				<option value="">All teams</option>
				{#each franchises as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
			</select>
		{/if}
	</header>
	<Tabs tabs={[{ value: 'now', label: 'In progress' }, { value: 'schedule', label: 'Schedule' }]} bind:value={view} label="View" />

	{#each leagues as league (league.id)}
		<section class="stack tight" data-sport={league.competition}>
			<h2 class="row"><SportBadge sport={league.competition} solid /> {league.name}</h2>
			{#if view === 'schedule'}
				<Schedule {league} {franchises} identities={data.dynasty?.team_identities} {team} />
			{:else}
				<Matchups {league} {franchises} identities={data.dynasty?.team_identities} mine={data.me?.id} editable={data.me?.is_commissioner} />
			{/if}
		</section>
	{:else}
		<Empty icon="scores" title="No matchups">None of the leagues is played head to head.</Empty>
	{/each}
</div>
