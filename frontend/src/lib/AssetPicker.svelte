<script lang="ts">
	// Everything one franchise could put in a trade: its players in every
	// sport and its draft picks. Sport is a filter here, never a boundary.
	import type { SvelteSet } from 'svelte/reactivity';
	import type { Franchise, FranchiseDetail } from '#lib/api.ts';
	import TradeAsset from '#lib/TradeAsset.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { assetsOf } from './tradeAssets.ts';

	type Props = {
		detail: FranchiseDetail;
		franchises: Franchise[];
		/** Keys of the assets chosen; toggled in place. */
		selected: SvelteSet<string>;
	};
	let { detail, franchises, selected }: Props = $props();

	const assets = $derived(assetsOf(detail, franchises));
	let filter = $state('');
	const tabs = $derived([
		{ value: '', label: 'All' },
		...detail.rosters
			.filter((r) => r.players.length > 0)
			.map((r) => ({ value: r.competition, label: r.competition.toUpperCase(), sport: r.competition })),
		...(detail.picks.length > 0 ? [{ value: 'picks', label: 'Picks' }] : [])
	]);
	const shown = $derived(
		assets.filter((a) => filter === '' || (filter === 'picks' ? a.pickId !== undefined : a.sport === filter && !a.pickId))
	);

	const toggle = (key: string) => (selected.has(key) ? selected.delete(key) : selected.add(key));
</script>

{#if assets.length === 0}
	<Empty icon="shield" title="Nothing to trade">{detail.franchise.name} has no players or picks yet.</Empty>
{:else}
	<div class="stack tight">
		<Tabs {tabs} bind:value={filter} label="Show" />
		<ul class="card flush">
			{#each shown as asset (asset.key)}
				<li>
					<label class:chosen={selected.has(asset.key)} class:locked={!asset.tradeable}>
						<input type="checkbox" checked={selected.has(asset.key)} disabled={!asset.tradeable} onchange={() => toggle(asset.key)} />
						<TradeAsset {...asset.display} />
					</label>
				</li>
			{/each}
		</ul>
	</div>
{/if}

<style>
	ul {
		list-style: none;
		margin: 0;
		max-height: 28rem;
		overflow-y: auto;
	}
	li + li {
		border-top: 1px solid var(--rule);
	}
	label {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.55rem 0.9rem;
		cursor: pointer;
	}
	label:hover {
		background: var(--surface-2);
	}
	label.chosen {
		background: var(--brand-soft);
	}
	label.locked {
		opacity: 0.55;
		cursor: default;
	}
</style>
