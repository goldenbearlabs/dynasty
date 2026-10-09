<script lang="ts">
	// Every trade this viewer may see: open offers, completed trades, and
	// offers that went nowhere.
	import { invalidateAll } from '$app/navigation';
	import TradeCard from '#lib/TradeCard.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type View = 'open' | 'done' | 'closed';
	const statuses: Record<View, string[]> = {
		open: ['proposed', 'accepted'],
		done: ['executed', 'reversed'],
		closed: ['rejected', 'cancelled']
	};
	const of = (view: View) => data.trades.filter((t) => statuses[view].includes(t.status));

	let view = $state<View>('open');
	const tabs = $derived([
		{ value: 'open' as View, label: 'Open offers', count: of('open').length },
		{ value: 'done' as View, label: 'Completed', count: of('done').length },
		{ value: 'closed' as View, label: 'Declined and withdrawn', count: of('closed').length }
	]);
	const empty = {
		open: 'Offers you make or receive show up here until everyone has answered.',
		done: 'Trades that have gone through are listed here for the whole league to see.',
		closed: 'Offers that were declined or withdrawn are kept here.'
	};
</script>

<svelte:head><title>Trades</title></svelte:head>

<div class="stack">
	<div class="spread">
		<h1>Trades</h1>
		{#if data.me}<a class="button primary" href="/trades/new"><Icon name="plus" size={16} /> New trade</a>{/if}
	</div>
	<Tabs {tabs} bind:value={view} label="Trades" />

	{#each of(view) as trade (trade.id)}
		<TradeCard {trade} franchises={data.dynasty?.franchises ?? []} me={data.me} onchange={invalidateAll} />
	{:else}
		<Empty icon="trade" title="Nothing here">{empty[view]}</Empty>
	{/each}
</div>
