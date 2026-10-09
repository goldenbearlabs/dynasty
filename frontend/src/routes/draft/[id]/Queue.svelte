<script lang="ts">
	// A manager's ranked wish list. If their clock runs out, the first
	// player on it who is still available is drafted for them.
	import type { QueuedPlayer } from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';

	type Props = {
		queue: QueuedPlayer[];
		/** Called with the new order of player ids. */
		onchange: (playerIds: string[]) => void;
		onselect?: (id: string) => void;
		editable?: boolean;
		disabled?: boolean;
	};
	let { queue, onchange, onselect, editable = true, disabled = false }: Props = $props();

	const ids = $derived(queue.map((p) => p.player_id));

	function move(i: number, by: number) {
		const next = [...ids];
		[next[i], next[i + by]] = [next[i + by], next[i]];
		onchange(next);
	}
</script>

{#if queue.length === 0}
	<Empty icon="queue" title="Your queue is empty">
		Add players from the pool. When your clock expires, the first available player is drafted for
		you.
	</Empty>
{:else}
	<ol>
		{#each queue as player, i (player.player_id)}
			<li>
				<span class="n">{i + 1}</span>
				<Headshot name={player.full_name} src={player.headshot_url} size={24} />
				<div class="who">
					{#if onselect}<button class="name" onclick={() => onselect(player.player_id)}
							>{player.full_name}</button
						>{:else}<strong>{player.full_name}</strong>{/if}
					<span class="muted small-text row">
						<SportBadge sport={player.competition} />
						{player.positions.join('/')}
						{player.team_abbrev}
					</span>
				</div>
				{#if editable}<span class="moves">
						<button
							class="quiet small"
							aria-label="Move up"
							disabled={disabled || i === 0}
							onclick={() => move(i, -1)}><Icon name="up" size={12} /></button
						>
						<button
							class="quiet small"
							aria-label="Move down"
							disabled={disabled || i === queue.length - 1}
							onclick={() => move(i, 1)}
						>
							<Icon name="down" size={15} />
						</button>
						<button
							class="quiet small danger"
							{disabled}
							aria-label="Remove from queue"
							onclick={() => onchange(ids.filter((id) => id !== player.player_id))}
						>
							<Icon name="x" size={15} />
						</button>
					</span>{/if}
			</li>
		{/each}
	</ol>
{/if}

<style>
	ol {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.4rem 0.25rem;
		font-size: 0.72rem;
		border-bottom: 1px solid var(--rule);
	}
	li:last-child {
		border-bottom: none;
	}
	.n {
		width: 1.3rem;
		font: 700 0.8rem var(--mono);
		color: var(--ink-faint);
	}
	.who {
		display: grid;
		flex: 1;
		line-height: 1.3;
		min-width: 0;
	}
	.who .row {
		gap: 0.4rem;
	}
	.moves {
		white-space: nowrap;
	}
	.moves button {
		padding: 0.15rem;
	}
	.name {
		padding: 0;
		border: 0;
		background: transparent;
		font-size: 0.72rem;
		text-align: left;
		white-space: normal;
		justify-content: start;
	}
	.name:hover {
		color: var(--brand);
	}
</style>
