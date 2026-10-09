<script lang="ts">
	// One draft in a list: what it is, how far along, and a way in.
	import type { DraftSummary } from '#lib/api.ts';
	import SportBadge from '#lib/ui/SportBadge.svelte';

	let { draft }: { draft: DraftSummary } = $props();

	const status = {
		scheduled: { label: 'Not started', tone: '' },
		live: { label: 'Live', tone: 'bad' },
		paused: { label: 'Paused', tone: 'gold' },
		complete: { label: 'Complete', tone: 'good' }
	} as const;
</script>

<a class="card" href="/draft/{draft.id}">
	<div class="spread">
		<span class="row sports">
			{#each draft.competitions as sport (sport)}<SportBadge {sport} solid />{/each}
		</span>
		<span class="pill {status[draft.status].tone}">{status[draft.status].label}</span>
	</div>
	<strong class="name">{draft.name}</strong>
	<span class="muted small-text">
		{draft.kind === 'startup' ? 'Startup' : 'Rookie draft'} · {draft.year} · {draft.picks_made} of {draft.picks} picks made
	</span>
</a>

<style>
	a {
		display: grid;
		gap: 0.45rem;
		color: var(--ink);
	}
	a:hover {
		text-decoration: none;
		border-color: var(--rule-strong);
	}
	.sports {
		gap: 0.3rem;
	}
	.name {
		font: 700 1.1rem/1.2 var(--display);
	}
</style>
