<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import Tabs from '#lib/ui/Tabs.svelte';
	import type { PageProps } from './$types';
	import Drafts from './Drafts.svelte';
	import Feeds from './Feeds.svelte';
	import Invites from './Invites.svelte';
	import PlayerPool from './PlayerPool.svelte';
	import Review from './Review.svelte';
	import Rules from './Rules.svelte';
	import Seasons from './Seasons.svelte';

	let { data }: PageProps = $props();

	type Section = 'franchises' | 'rules' | 'seasons' | 'drafts' | 'players' | 'review' | 'feeds';
	// Other pages can link straight to a section: /commissioner?section=drafts
	let section = $state<Section>(untrack(() => (page.url.searchParams.get('section') as Section) ?? 'franchises'));
	const tabs: { value: Section; label: string }[] = [
		{ value: 'franchises', label: 'Franchises' },
		{ value: 'rules', label: 'Rules' },
		{ value: 'seasons', label: 'Seasons' },
		{ value: 'drafts', label: 'Drafts' },
		{ value: 'players', label: 'Player pool' },
		{ value: 'review', label: 'Review' },
		{ value: 'feeds', label: 'Data feeds' }
	];
</script>

<svelte:head><title>Commissioner</title></svelte:head>

{#if !data.me?.is_commissioner || !data.dynasty}
	<p role="alert">Only a commissioner can open this page.</p>
{:else}
	<div class="stack">
		<header class="stack tight">
			<p class="eyebrow">League office</p>
			<h1>Commissioner</h1>
			<Tabs {tabs} bind:value={section} label="Section" />
		</header>

		{#if section === 'franchises'}
			<Invites />
		{:else if section === 'rules'}
			<Rules dynasty={data.dynasty} competitions={data.competitions} />
		{:else if section === 'seasons'}
			<Seasons dynasty={data.dynasty} competitions={data.competitions} />
		{:else if section === 'drafts'}
			<Drafts dynasty={data.dynasty} drafts={data.drafts} />
		{:else if section === 'review'}
			<Review dynasty={data.dynasty} />
		{:else if section === 'players'}
			<PlayerPool competitions={data.competitions} />
		{:else}
			<Feeds competitions={data.competitions} />
		{/if}
	</div>
{/if}
