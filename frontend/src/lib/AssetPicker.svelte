<script lang="ts">
	// Everything one franchise could put in a trade: its players in every
	// sport and its draft picks. Sport is a filter here, never a boundary.
	import type { SvelteSet } from 'svelte/reactivity';
	import type { Franchise, FranchiseDetail } from '#lib/api.ts';
	import TradeAsset from '#lib/TradeAsset.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import { assetsOf, type Asset } from './tradeAssets.ts';

	type Props = {
		detail: FranchiseDetail;
		franchises: Franchise[];
		/** Keys of the assets chosen; toggled in place. */
		selected: SvelteSet<string>;
		/** The other side of the trade, to show which of these players they want. */
		counterpart?: string;
	};
	let { detail, franchises, selected, counterpart }: Props = $props();

	// What the trade market says about a player: on his owner's block, a
	// target of yours, or one the other side has marked as a target.
	function marks(asset: Asset) {
		if (!asset.leagueId || !asset.playerId) return {};
		return {
			block: tradeMarket.data.block.find((b) => b.league_id === asset.leagueId && b.player_id === asset.playerId),
			yours: tradeMarket.targeted(asset.leagueId, asset.playerId),
			theirs: !!counterpart && tradeMarket.interestIn(asset.leagueId, asset.playerId).includes(counterpart)
		};
	}

	const assets = $derived(assetsOf(detail, franchises));
	let filter = $state('');
	const tabs = $derived([
		{ value: '', label: 'All' },
		...detail.rosters
			.filter((r) => r.players.length > 0)
			.map((r) => ({ value: r.competition, label: r.competition.toUpperCase(), sport: r.competition })),
		...(detail.picks.length > 0 ? [{ value: 'picks', label: 'Picks' }] : [])
	]);
	// Players the trade market has something to say about come first.
	const marked = (a: Asset) => {
		const m = marks(a);
		return m.block || m.yours || m.theirs ? 0 : 1;
	};
	const shown = $derived(
		assets
			.filter((a) => filter === '' || (filter === 'picks' ? a.pickId !== undefined : a.sport === filter && !a.pickId))
			.toSorted((a, b) => marked(a) - marked(b))
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
						{#if asset.playerId}
							{@const m = marks(asset)}
							<span class="marks">
								{#if m.block}<span class="mark block" title={m.block.note || 'On the trade block'}>On the block</span>{/if}
								{#if m.yours}<span class="mark want">Your target</span>{/if}
								{#if m.theirs}<span class="mark want">They want him</span>{/if}
							</span>
						{/if}
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
	.marks {
		display: flex;
		flex-wrap: wrap;
		justify-content: end;
		gap: 0.25rem;
		margin-left: auto;
	}
	.mark {
		padding: 0.1rem 0.45rem;
		border-radius: 999px;
		font: 600 0.65rem/1.4 var(--body);
		white-space: nowrap;
	}
	.mark.block {
		background: color-mix(in srgb, var(--gold) 16%, transparent);
		color: var(--gold);
	}
	.mark.want {
		background: color-mix(in srgb, var(--bad) 14%, transparent);
		color: var(--bad);
	}
	label.locked {
		opacity: 0.55;
		cursor: default;
	}
</style>
