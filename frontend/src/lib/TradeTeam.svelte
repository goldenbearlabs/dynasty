<script lang="ts">
	// One team's side of a trade being built: everything it could send, one
	// thin row each, so a whole roster fits on screen. A sport shows that
	// league's players and its draft picks together. Ticking a row sends it;
	// in a trade of three or more teams the row then asks where it goes.
	import type { SvelteMap } from 'svelte/reactivity';
	import type { Franchise, FranchiseDetail } from '#lib/api.ts';
	import { assetsOf, type Asset, type Section } from '#lib/tradeAssets.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import SportBadge from '#lib/ui/SportBadge.svelte';

	type Props = {
		detail: FranchiseDetail;
		franchises: Franchise[];
		/** The sport to show, or '' for every one. */
		sport: string;
		/** Every team in the trade, this one included. */
		parties: Franchise[];
		/** The assets chosen in the whole trade, by key: who sends each and who gets it. */
		chosen: SvelteMap<string, { from: string; to: string }>;
		/** Where an asset this team ticks goes first. */
		defaultTo: string;
		/** The signed-in manager. */
		me: string;
	};
	let { detail, franchises, sport, parties, chosen, defaultTo, me }: Props = $props();

	const id = $derived(detail.franchise.id);
	const assets = $derived(assetsOf(detail, franchises));
	const sections: { key: Section; label: string }[] = [
		{ key: 'main', label: 'Main roster' },
		{ key: 'reserve', label: 'Reserve' },
		{ key: 'rights', label: 'Unsigned picks' },
		{ key: 'picks', label: 'Draft picks' }
	];
	// Sport by sport (a pick in a draft shared by several leagues under its
	// own heading), then list by list.
	const groups = $derived.by(() => {
		const shown = assets.filter((a) => !sport || a.sports.includes(sport));
		const keyOf = (a: Asset) => (sport ? sport : a.sports.length === 1 ? a.sports[0] : 'shared');
		const sports = [...new Set(shown.map(keyOf))];
		return sports.map((s) => ({
			sport: s,
			sections: sections
				.map((sec) => ({ ...sec, assets: shown.filter((a) => keyOf(a) === s && a.section === sec.key) }))
				.filter((sec) => sec.assets.length > 0)
		}));
	});

	const others = $derived(parties.filter((p) => p.id !== id));
	const toggle = (asset: Asset) => (chosen.has(asset.key) ? chosen.delete(asset.key) : chosen.set(asset.key, { from: id, to: defaultTo }));

	// What the trade market says: on the block, a target of yours, or (for
	// your own players) wanted by someone in this trade.
	function marks(asset: Asset): { label: string; tone: string; title?: string }[] {
		if (!asset.leagueId || !asset.playerId) return [];
		const out = [];
		const block = tradeMarket.onBlock(asset.leagueId, asset.playerId);
		if (block) out.push({ label: 'Block', tone: 'gold', title: block.note || 'On the trade block' });
		if (id !== me && tradeMarket.targeted(asset.leagueId, asset.playerId)) out.push({ label: 'Target', tone: 'bad', title: 'One of your targets' });
		if (id === me) {
			const wanting = tradeMarket.interestIn(asset.leagueId, asset.playerId).filter((f) => others.some((o) => o.id === f));
			if (wanting.length) out.push({ label: 'Wanted', tone: 'bad', title: `Wanted by ${wanting.map((f) => franchises.find((x) => x.id === f)?.name).join(', ')}` });
		}
		return out;
	}
	const count = $derived([...chosen.values()].filter((c) => c.from === id).length);
</script>

{#if assets.length === 0}
	<p class="muted small-text pad">{detail.franchise.name} has no players or picks yet.</p>
{:else if groups.length === 0}
	<p class="muted small-text pad">Nothing in {sport.toUpperCase()}.</p>
{:else}
	<div class="team" aria-label="{detail.franchise.name}: players and picks to trade{count ? `, ${count} chosen` : ''}">
		{#each groups as group (group.sport)}
			{#if !sport}
				<h4 class="sport">{#if group.sport === 'shared'}Shared drafts{:else}<SportBadge sport={group.sport} solid />{/if}</h4>
			{/if}
			{#each group.sections as sec (sec.key)}
				<h5 class="section">{sec.label} <span>{sec.assets.length}</span></h5>
				<ul>
					{#each sec.assets as asset (asset.key)}
						{@const pick = chosen.get(asset.key)}
						<li class:on={!!pick} class:locked={!asset.tradeable}>
							<label>
								<input type="checkbox" checked={!!pick} disabled={!asset.tradeable} onchange={() => toggle(asset)} />
								<span class="name" title={asset.name}>{asset.name}</span>
								<span class="detail">{asset.detail}</span>
								{#each marks(asset) as m (m.label)}<span class="mark {m.tone}" title={m.title}>{m.label}</span>{/each}
							</label>
							{#if pick && others.length > 1}
								<select class="to" aria-label="Who gets {asset.name}" bind:value={() => pick.to, (to) => chosen.set(asset.key, { from: id, to })}>
									{#each others as o (o.id)}<option value={o.id}>→ {o.id === me ? 'You' : o.name}</option>{/each}
								</select>
							{/if}
						</li>
					{/each}
				</ul>
			{/each}
		{/each}
	</div>
{/if}

<style>
	.pad {
		padding: 0.6rem 0.75rem;
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.sport {
		position: sticky;
		top: 0;
		z-index: 2;
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin: 0;
		padding: 0.3rem 0.6rem;
		background: var(--surface);
		border-bottom: 1px solid var(--rule-strong);
		font: 700 0.7rem var(--mono);
		text-transform: uppercase;
	}
	.section {
		display: flex;
		justify-content: space-between;
		margin: 0;
		padding: 0.18rem 0.6rem;
		background: var(--surface-2);
		border-bottom: 1px solid var(--rule);
		font: 600 0.6rem var(--mono);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		border-bottom: 1px solid var(--rule);
		padding-right: 0.35rem;
	}
	li:hover {
		background: var(--surface-2);
	}
	li.on {
		background: var(--brand-soft);
	}
	li.locked {
		opacity: 0.5;
	}
	/* One thin line: tick, name, what he is, any market marks. */
	label {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		flex: 1;
		min-width: 0;
		min-height: 1.85rem;
		padding: 0.1rem 0 0.1rem 0.6rem;
		cursor: pointer;
		font-size: 0.8rem;
	}
	li.locked label {
		cursor: default;
	}
	input {
		flex: none;
		margin: 0;
	}
	.name {
		flex: 0 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}
	.detail {
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.68rem;
		color: var(--ink-faint);
	}
	.mark {
		flex: none;
		padding: 0.05rem 0.35rem;
		border-radius: 999px;
		font: 650 0.58rem/1.4 var(--body);
	}
	.mark.gold {
		background: color-mix(in srgb, var(--gold) 16%, transparent);
		color: var(--gold);
	}
	.mark.bad {
		background: color-mix(in srgb, var(--bad) 14%, transparent);
		color: var(--bad);
	}
	.to {
		flex: none;
		max-width: 8.5rem;
		padding: 0.1rem 0.3rem;
		font-size: 0.7rem;
	}
	@media (pointer: coarse) {
		/* Thin, but still a comfortable tap. */
		label {
			min-height: 2.3rem;
		}
	}
</style>
