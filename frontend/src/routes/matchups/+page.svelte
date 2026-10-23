<script lang="ts">
	// Browse one league’s matchups, detailed scoring, and season schedule.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import MatchupHub from '#lib/MatchupHub.svelte';
	import Schedule from '#lib/Schedule.svelte';
	import Empty from '#lib/ui/Empty.svelte';
		import Tabs from '#lib/ui/Tabs.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const leagues = $derived(data.dynasty?.leagues.filter((l) => l.settings.format.type === 'head_to_head') ?? []);
	const franchises = $derived(data.dynasty?.franchises ?? []);

	// A link can open the schedule on a team: ?view=schedule&team=<franchise id>.
	const asked = untrack(() => page.url.searchParams);
	let view = $state<'now' | 'schedule'>(asked.get('view') === 'schedule' ? 'schedule' : 'now');
	let sport = $state(asked.get('sport') ?? untrack(() => data.defaultCompetition) ?? '');
	const league = $derived(leagues.find(l => l.competition === sport) ?? leagues[0]);
	let team = $state(asked.get('team') ?? untrack(() => data.me?.id) ?? ''); // empty for every team
</script>

<svelte:head><title>Matchups</title></svelte:head>

<div class="stack matchups-page">
	<header class="spread">
		<h1>Matchups</h1>
		<Tabs tabs={[{ value: 'now', label: 'Matchups' }, { value: 'schedule', label: 'Schedule' }]} bind:value={view} label="View" />
		{#if view === 'schedule'}
			<select aria-label="Team" bind:value={team}>
				<option value="">All teams</option>
				{#each franchises as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
			</select>
		{/if}
	</header>

	{#if league}
		<Tabs tabs={leagues.map(l => ({value:l.competition,label:l.competition.toUpperCase(),sport:l.competition}))} bind:value={sport} label="League" />
		<section class="stack tight" data-sport={league.competition} aria-label={league.name}>
			{#key league.id}
				{#if view === 'schedule'}
					<Schedule {league} {franchises} identities={data.dynasty?.team_identities} {team} />
				{:else}
					<MatchupHub {league} {franchises} identities={data.dynasty?.team_identities} mine={data.me?.id} />
				{/if}
			{/key}
		</section>
	{:else}
		<Empty icon="scores" title="No matchups">None of the leagues is played head to head.</Empty>
	{/if}
</div>

<style>
 .matchups-page { gap: .6rem; min-width: 0; grid-template-columns: minmax(0,1fr); }
 .matchups-page section { min-width: 0; grid-template-columns: minmax(0,1fr); }
 .matchups-page h1 { font-size: 1.6rem; }
</style>
