<script lang="ts">
	// One tap from wherever a player is listed: put your own player on the
	// trade block, or mark someone else's as a target so his manager knows.
	// Your own player also shows who is after him.
	import { page } from '$app/state';
	import type { Franchise } from '#lib/api.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';

	let {
		leagueId,
		playerId,
		ownerId,
		name = 'He'
	}: { leagueId: string; playerId: string; ownerId: string | null | undefined; name?: string } = $props();

	const me = $derived(page.data.me as Franchise | null | undefined);
	const mine = $derived(!!me && ownerId === me.id);
	const blocked = $derived(tradeMarket.onBlock(leagueId, playerId));
	const targeted = $derived(tradeMarket.targeted(leagueId, playerId));
	const interest = $derived(mine ? tradeMarket.interestIn(leagueId, playerId) : []);
	const franchiseName = (id: string) =>
		(page.data.dynasty?.franchises as Franchise[] | undefined)?.find((f) => f.id === id)?.name ?? 'A manager';
</script>

{#if me && ownerId}
	<span class="flags">
		{#if mine}
			<button
				class="flag"
				class:on={!!blocked}
				aria-pressed={!!blocked}
				disabled={tradeMarket.busy}
				title={blocked ? `On your trade block${blocked.note ? `: ${blocked.note}` : ''}. Tap to take him off.` : 'Put him on your trade block'}
				onclick={() => tradeMarket.block(leagueId, playerId, !blocked, blocked?.note ?? '', name)}
			>
				<Icon name="trade" size={12} />{blocked ? 'On the block' : 'Trade block'}
			</button>
			{#if interest.length}
				<span class="interest" title="Targeted by {interest.map(franchiseName).join(', ')}">
					<Icon name="target" size={12} />{interest.length} interested
				</span>
			{/if}
		{:else}
			<button
				class="flag target"
				class:on={targeted}
				aria-pressed={targeted}
				disabled={tradeMarket.busy}
				title={targeted ? 'You are after him: his manager can see it. Tap to drop the target.' : 'Mark him as a target: his manager sees you are interested'}
				onclick={() => tradeMarket.target(leagueId, playerId, !targeted, name)}
			>
				<Icon name="target" size={12} />{targeted ? 'Targeted' : 'Target'}
			</button>
		{/if}
	</span>
{/if}

<style>
	.flags {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		vertical-align: middle;
	}
	.flag,
	.interest {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.15rem 0.45rem;
		border: 1px solid var(--rule-strong);
		border-radius: 999px;
		background: transparent;
		color: var(--ink-soft);
		font: 600 0.68rem/1.3 var(--body);
		white-space: nowrap;
	}
	.flag:hover {
		border-color: var(--gold);
		color: var(--ink);
	}
	.flag.on {
		border-color: var(--gold);
		background: color-mix(in srgb, var(--gold) 16%, transparent);
		color: var(--gold);
	}
	.flag.target.on {
		border-color: var(--bad);
		background: color-mix(in srgb, var(--bad) 14%, transparent);
		color: var(--bad);
	}
	.interest {
		border-color: transparent;
		background: color-mix(in srgb, var(--bad) 14%, transparent);
		color: var(--bad);
	}
</style>
