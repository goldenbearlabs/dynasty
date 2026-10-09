<script lang="ts">
	// One player or draft pick, as it appears anywhere in a trade.
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';

	type Props = {
		/** A player's name, or empty for a pick. */
		player?: string;
		positions?: string[];
		headshot?: string;
		sport?: string;
		/** For a pick: its draft and round, and whose it was if not the holder's. */
		draft?: string;
		round?: number;
		via?: string;
		note?: string;
	};
	let { player = '', positions = [], headshot = '', sport = '', draft = '', round = 0, via = '', note = '' }: Props = $props();
</script>

<span class="asset">
	{#if player}
		<Headshot name={player} src={headshot} size={30} />
		<span class="text">
			<strong>{player}</strong>
			<span class="muted small-text">{positions.join('/')} {note}</span>
		</span>
		{#if sport}<SportBadge {sport} />{/if}
	{:else}
		<span class="pick" aria-hidden="true">{round}</span>
		<span class="text">
			<strong>Round {round} pick</strong>
			<span class="muted small-text">{draft}{via ? ` · via ${via}` : ''} {note}</span>
		</span>
		{#if sport}<SportBadge {sport} />{/if}
	{/if}
</span>

<style>
	.asset {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		min-width: 0;
	}
	.text {
		display: grid;
		flex: 1;
		min-width: 0;
		line-height: 1.25;
	}
	.pick {
		flex: none;
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border-radius: 8px;
		background: var(--gold-soft);
		color: var(--gold);
		font: 750 0.95rem var(--display);
	}
</style>
