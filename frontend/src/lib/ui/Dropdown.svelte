<script lang="ts">
	// A button that opens a small panel over the page: filters, column
	// pickers, admin controls. It closes from its ×, a click anywhere
	// outside it, or Escape.
	import type { Snippet } from 'svelte';
	import Icon from '#lib/ui/Icon.svelte';

	type Props = {
		/** What the button says. */
		label: string;
		/** The panel's heading; the label unless given. */
		title?: string;
		/** Which edge of the button the panel lines up with. */
		align?: 'left' | 'right';
		/** Classes for the outer element, styled by the caller with :global(). */
		class?: string;
		children: Snippet;
	};
	let { label, title, align = 'right', class: classes = '', children }: Props = $props();

	let root = $state<HTMLDetailsElement>();
	let open = $state(false);
	const close = (refocus = false) => {
		open = false;
		if (refocus) root?.querySelector('summary')?.focus();
	};
	$effect(() => {
		if (!open) return;
		const outside = (e: PointerEvent) => {
			if (root && !root.contains(e.target as Node)) close();
		};
		const escape = (e: KeyboardEvent) => {
			if (e.key === 'Escape') close(true);
		};
		document.addEventListener('pointerdown', outside);
		document.addEventListener('keydown', escape);
		return () => {
			document.removeEventListener('pointerdown', outside);
			document.removeEventListener('keydown', escape);
		};
	});
</script>

<details bind:this={root} bind:open class="dropdown {classes}">
	<summary>{label}</summary>
	<div class="panel" class:left={align === 'left'} role="group" aria-label={title ?? label}>
		<div class="head">
			<span>{title ?? label}</span>
			<button class="quiet close" aria-label="Close {title ?? label}" onclick={() => close(true)}><Icon name="x" size={16} /></button>
		</div>
		{@render children()}
	</div>
</details>

<style>
	.dropdown {
		position: relative;
	}
	summary {
		cursor: pointer;
		user-select: none;
	}
	.panel {
		position: absolute;
		z-index: 30;
		top: calc(100% + 0.35rem);
		right: 0;
		min-width: 14rem;
		max-width: calc(100vw - 2rem);
		max-height: min(70dvh, 28rem);
		overflow: auto;
		padding: 0.4rem 0.75rem 0.75rem;
		background: var(--surface);
		border: 1px solid var(--rule-strong);
		border-radius: var(--radius);
		box-shadow: 0 8px 24px #0003;
	}
	.panel.left {
		right: auto;
		left: 0;
	}
	.head {
		position: sticky;
		top: -0.4rem;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		margin: -0.4rem -0.75rem 0.5rem;
		padding: 0.3rem 0.35rem 0.3rem 0.75rem;
		background: var(--surface);
		border-bottom: 1px solid var(--rule);
		font: 600 0.62rem var(--mono);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.close {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		padding: 0;
		color: var(--ink-soft);
	}
</style>
