<script lang="ts">
	// One league's lineup on a page of its own. The editing is done by the
	// same lineup editor the team page uses.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { teamIdentity } from '#lib/identity.ts';
 import Crest from '#lib/ui/Crest.svelte';
 import LineupEditor from '#lib/LineupEditor.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const sport = $derived(page.params.sport!);
	const league = $derived(data.dynasty?.leagues.find((l) => l.competition === sport));
	// Whose lineup: ?franchise=<slug>, or the signed-in manager's.
	const franchise = $derived(
		data.dynasty?.franchises.find((f) => f.slug === (page.url.searchParams.get('franchise') ?? data.me?.slug))
	);
	const mine = $derived(franchise !== undefined && franchise.id === data.me?.id);

	// The day is kept in the address, so a lineup for a given day can be linked to.
	function go(to: string | undefined) {
		const query = new URLSearchParams(page.url.search);
		if (to) query.set('day', to);
		else query.delete('day');
		goto(`?${query}`, { replaceState: true });
	}
</script>

<svelte:head><title>Lineup</title></svelte:head>

{#if !league || !franchise}
	<Empty icon="shield" title="No lineup to show"><a href="/">Sign in</a> to set your lineup.</Empty>
{:else}
	<div class="stack" data-sport={sport}>
		<header>
			<p class="row eyebrow"><SportBadge {sport} solid /> <a href="/franchise/{franchise.slug}?view={sport}">{teamIdentity(franchise,data.dynasty?.team_identities,league.id).name}</a></p>
			<h1 class="row"><Crest name={teamIdentity(franchise,data.dynasty?.team_identities,league.id).name} src={teamIdentity(franchise,data.dynasty?.team_identities,league.id).image_url} size={44} />{teamIdentity(franchise,data.dynasty?.team_identities,league.id).name} lineup</h1>
		</header>
		{#key league.id + franchise.id}
			<LineupEditor
				{league}
				{franchise}
				canEdit={mine || data.me?.is_commissioner === true}
				commissioner={data.me?.is_commissioner === true}
				bind:day={() => page.url.searchParams.get('day') ?? undefined, go}
			/>
		{/key}
	</div>
{/if}

<style>
	.eyebrow {
		gap: 0.5rem;
		margin-bottom: 0.35rem;
	}
</style>
