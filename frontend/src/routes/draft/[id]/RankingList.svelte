<script lang="ts">
	// One of the manager's pre-draft lists, read in the draft room: its
	// players in the order he ranked them, with anyone already taken greyed
	// out, so he can queue from it as the draft goes.
	import type { Snippet } from 'svelte';
	import { getRanking, type RankedPlayer } from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';

	type Props = {
		rankingId: string;
		/** Players drafted so far, by id, with who took each. */
		taken: Map<string, string>;
		showSport: boolean;
		/** What to show beside a player who is still available. */
		action?: Snippet<[RankedPlayer]>;
		onselect?: (id: string) => void;
		selectedId?: string;
	};
	let { rankingId, taken, showSport, action, onselect, selectedId }: Props = $props();

	let players = $state<RankedPlayer[]>();
	let error = $state('');
	let hideTaken = $state(false);
	$effect(() => {
		const wanted = rankingId;
		players = undefined;
		getRanking(wanted).then(
			(r) => {
				if (wanted === rankingId) [players, error] = [r.players, ''];
			},
			(e: Error) => {
				if (wanted === rankingId) error = e.message;
			}
		);
	});
	// Taken in this draft, or already on a roster before it began.
	const owner = (p: RankedPlayer) => taken.get(p.player_id) ?? p.owner_name;
	const left = $derived((players ?? []).filter((p) => !owner(p)).length);
</script>

{#if error}
	<p class="muted small-text">{error}</p>
{:else if !players}
	<p class="muted small-text">Loading…</p>
{:else if players.length === 0}
	<Empty icon="queue" title="This list is empty">Add players to it on the Drafts page.</Empty>
{:else}
	<label class="row small-text muted tools">
		<input type="checkbox" bind:checked={hideTaken} />
		Hide taken · {left} of {players.length} left
	</label>
	<ol>
		{#each players as player, i (player.player_id)}
			{@const by = owner(player)}
			{#if !by || !hideTaken}
				<li class:taken={!!by} class:selected={player.player_id === selectedId}>
					<span class="n">{i + 1}</span>
					<Headshot name={player.full_name} src={player.headshot_url} size={24} />
					<div class="who">
						<button class="name" onclick={() => onselect?.(player.player_id)}>{player.full_name}</button>
						<span class="muted small-text row">
							{#if showSport}<SportBadge sport={player.competition} />{/if}
							{player.positions.join('/')}
							{player.team_abbrev}
						</span>
					</div>
					{#if by}<span class="by">{by}</span>{:else}{@render action?.(player)}{/if}
				</li>
			{/if}
		{/each}
	</ol>
{/if}

<style>
	.tools {
		gap: 0.35rem;
		padding-bottom: 0.3rem;
	}
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
	li.selected {
		background: var(--brand-soft);
	}
	li.taken {
		opacity: 0.45;
	}
	li.taken .name {
		text-decoration: line-through;
	}
	.n {
		width: 1.6rem;
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
	.by {
		font-size: 0.65rem;
		color: var(--ink-soft);
		white-space: nowrap;
	}
</style>
