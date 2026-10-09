<script lang="ts">
	// The league's recent moves, newest first.
	import type { Activity } from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { ago } from '#lib/ui/time.ts';

	let { items }: { items: Activity[] } = $props();

	const listName = (a: Activity) => (a.detail.list === 'reserve' ? 'reserve list' : 'main roster');

	// The words between the franchise and the player, and after the player.
	function describe(a: Activity): [string, string] {
		switch (a.kind) {
			case 'add':
				return ['added', `to the ${listName(a)}`];
			case 'claim':
				return ['claimed', a.detail.bid ? `off waivers for ${a.detail.bid}` : 'off waivers'];
			case 'pick':
				return ['drafted', ''];
			case 'drop':
				return ['dropped', ''];
			case 'move':
				return ['moved', `to the ${listName(a)}`];
			case 'graduated':
				return ['kept', `as he moved up from ${a.detail.from}`];
			case 'released':
				return ['lost', 'who moved on to another league'];
			case 'undo_pick':
				return ['had their pick of', 'undone'];
			default:
				return [a.kind, ''];
		}
	}
</script>

{#if items.length === 0}
	<Empty icon="clock" title="Nothing has happened yet">Adds, drops, picks and rule changes will show up here.</Empty>
{:else}
	<ol class="card flush">
		{#each items as a (a.id)}
			<li>
				{#if a.competition}<SportBadge sport={a.competition} />{/if}
				<span class="what">
					{#if a.kind === 'settings'}
						League rules changed
					{:else if a.kind === 'trade'}
						<a href="/franchise/{a.franchise_slug}">{a.franchise_name}</a>
						{a.detail.reversed ? 'got back' : 'acquired'}
						<strong>{a.player_name || `a round ${a.pick_round} pick in the ${a.draft_name}`}</strong>
						from {a.detail.from}
						<span class="pill {a.detail.reversed ? '' : 'brand'}">{a.detail.reversed ? 'Trade reversed' : 'Trade'}</span>
					{:else}
						{@const [verb, tail] = describe(a)}
						<a href="/franchise/{a.franchise_slug}">{a.franchise_name}</a>
						{verb} <strong>{a.player_name}</strong>
						{tail}
						{#if a.detail.forced}<span class="pill gold">Commissioner</span>{/if}
					{/if}
				</span>
				<time class="muted small-text" datetime={a.created_at}>{ago(a.created_at)}</time>
			</li>
		{/each}
	</ol>
{/if}

<style>
	ol {
		list-style: none;
		margin: 0;
	}
	li {
		display: flex;
		align-items: baseline;
		gap: 0.7rem;
		padding: 0.65rem 1rem;
		border-bottom: 1px solid var(--rule);
	}
	li:last-child {
		border-bottom: none;
	}
	.what {
		flex: 1;
	}
	time {
		white-space: nowrap;
	}
</style>
