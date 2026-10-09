<script lang="ts">
	// A player's photo over his initials, which show while it loads, when
	// there is none, or when it fails.
	let { name, src = '', size = 36 }: { name: string; src?: string; size?: number } = $props();

	let failed = $state(false);
	let loaded = $state(false);
	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.map((word) => word[0]!.toUpperCase())
			.slice(0, 2)
			.join('')
	);
</script>

<span class="headshot" style:--size="{size}px">
	{initials}
	{#if src && !failed}
		<img {src} alt="" loading="lazy" width={size} height={size} class:loaded onload={() => (loaded = true)} onerror={() => (failed = true)} />
	{/if}
</span>

<style>
	.headshot {
		flex: none;
		position: relative;
		display: inline-grid;
		place-items: center;
		width: var(--size);
		height: var(--size);
		border-radius: 50%;
		overflow: hidden;
		background: var(--surface-2);
		border: 1px solid var(--rule);
		font: 650 calc(var(--size) * 0.34) / 1 var(--body);
		color: var(--ink-faint);
	}
	img {
		position: absolute;
		inset: 0;
		background: var(--surface-2); /* covers the initials, since photos are often transparent */
		opacity: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		object-position: top;
	}
	img.loaded {
		opacity: 1;
	}
</style>
