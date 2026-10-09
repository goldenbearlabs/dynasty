<script lang="ts">
	// Every draft, and for a signed-in manager their own pre-draft rankings.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import DraftCard from '#lib/DraftCard.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import Rankings from './Rankings.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let tab = $state<'drafts' | 'rankings'>(untrack(() => (page.url.searchParams.get('tab') === 'rankings' ? 'rankings' : 'drafts')));
	const tabs = [
		{ value: 'drafts' as const, label: 'Drafts' },
		{ value: 'rankings' as const, label: 'My rankings' }
	];
</script>

<svelte:head><title>Drafts</title></svelte:head>

<div class="stack">
	<div class="spread">
		<h1>Drafts</h1>
		{#if data.me?.is_commissioner}<a class="button primary" href="/commissioner?section=drafts">New draft</a>{/if}
	</div>
	{#if data.me && data.dynasty}<Tabs {tabs} bind:value={tab} label="Drafts section" />{/if}

	{#if tab === 'rankings' && data.me && data.dynasty}
		<Rankings dynasty={data.dynasty} drafts={data.drafts} />
	{:else if data.drafts.length === 0}
		<Empty icon="draft" title="No drafts yet">
			The commissioner creates drafts: a startup draft to fill the rosters, then one each year per league.
		</Empty>
	{:else}
		<div class="list">
			{#each data.drafts as draft (draft.id)}<DraftCard {draft} />{/each}
		</div>
	{/if}
</div>

<style>
	.list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 0.8rem;
	}
</style>
