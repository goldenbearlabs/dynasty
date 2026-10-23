<!-- Introduces a newly created private league while gameplay is being tenant-scoped. -->
<script lang="ts">
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	const sectionNames: Record<string, string> = {
		team: 'My team', league: 'League', matchups: 'Matchups', players: 'Players',
		watchlist: 'Watchlist', research: 'Research', waivers: 'Waivers',
		drafts: 'Drafts', trades: 'Trades', rules: 'Rules'
	};
	const sectionTitle = $derived(sectionNames[page.url.searchParams.get('section') ?? '']);
</script>

<svelte:head><title>{sectionTitle ?? data.league.name} · Mosaic Fantasy</title></svelte:head>

<div class="stack" style="max-width: 48rem; margin: 0 auto;">
	<p class="eyebrow">My Leagues</p>
	<h1>{sectionTitle ?? data.league.name}</h1>
	{#if sectionTitle}
		<section class="card stack" style="padding: 1.5rem;">
			<h2>{data.league.name}</h2>
			<p>{sectionTitle} is not available in this league yet. The gameplay API is being updated to keep each league's data separate.</p>
			<a href="/leagues/{data.league.id}">Back to league overview</a>
		</section>
	{:else}
		<section class="card stack" style="padding: 1.5rem;">
			<h2>League setup saved</h2>
			<p>Your franchise will be {data.league.franchise_name}. This private setup has its own copy of the current sports and rule settings.</p>
			<p>Sports: {data.league.sports.join(', ')}</p>
			<p>This setup is not yet a playable league. Rosters, drafts, trades, and commissioner controls will become available after their API routes are updated for multiple leagues.</p>
		</section>
	{/if}
	<a href="/leagues">Back to My Leagues</a>
</div>
