<script lang="ts">
	// What to show where a list would be when there is nothing in it.
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	let { icon, title, children }: { icon: IconName; title: string; children?: Snippet } = $props();
</script>

<div class="empty">
	<Icon name={icon} size={26} />
	<strong>{title}</strong>
	{#if children}<p class="muted small-text">{@render children()}</p>{/if}
</div>

<style>
	.empty {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: center;
		gap: 0.2rem 0.75rem;
		padding: 1rem 0;
		color: var(--ink-faint);
		border-top: 1px solid var(--rule);
	}
	.empty :global(svg) {
		grid-row: 1 / span 2;
	}
	strong {
		color: var(--ink);
	}
	p {
		grid-column: 2;
		max-width: 34rem;
	}
</style>
