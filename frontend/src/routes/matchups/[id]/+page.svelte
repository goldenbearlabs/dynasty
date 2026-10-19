<script lang="ts">
	import { page } from '$app/state';
	import { getMatchup, type MatchupDetail } from '#lib/api.ts';
	import MatchupHub from '#lib/MatchupHub.svelte';
	import MatchupView from '#lib/MatchupView.svelte';
	import { getMatchups } from '#lib/api.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let matchup = $state<MatchupDetail>();
	let currentSeason = $state(true);
	let error = $state('');
	let loading = $state(true);
	const league = $derived(data.dynasty?.leagues.find(l => l.competition === matchup?.competition));
	$effect(() => {
		const id = page.params.id!;
		let active = true;
		loading = true;
		getMatchup(id).then(async found => {
			const league = data.dynasty?.leagues.find(l => l.competition === found.competition);
			const list = league ? await getMatchups(league.id, found.period.seq) : undefined;
			if (active) { matchup = found; currentSeason = list?.period?.id === found.period.id; loading = false; error = ''; }
		}).catch(e => { if(active) { error=String(e); loading=false; } });
		return () => { active=false; };
	});
</script>

<svelte:head><title>Matchup · {league?.name ?? 'Crossover Dynasty'}</title></svelte:head>
<div class="stack matchup-page">
	<header class="spread"><h1>{league?.name ?? 'Matchup'}</h1><a href="/matchups{league ? `?sport=${league.competition}` : ''}">All matchups</a></header>
	{#if loading}<p class="muted" role="status">Loading matchup…</p>
	{:else if error}<p role="alert">{error}</p>
	{:else if league && matchup}
		{#if currentSeason}
			{#key matchup.id}<MatchupHub {league} franchises={data.dynasty?.franchises ?? []} identities={data.dynasty?.team_identities} mine={data.me?.id} editable={data.me?.is_commissioner} initialId={matchup.id} initialPeriod={matchup.period.seq} />{/key}
		{:else}
			<MatchupView id={matchup.id} {league} franchises={data.dynasty?.franchises ?? []} identities={data.dynasty?.team_identities} />
		{/if}
	{/if}
</div>

<style>
 .matchup-page { min-width: 0; grid-template-columns: minmax(0,1fr); }
</style>
