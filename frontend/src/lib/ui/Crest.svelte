<script lang="ts">
	// Custom organization/team logo, with a monogram if absent or unavailable.
	let { name, size = 40, src = '' }: { name: string; size?: number; src?: string } = $props();
 let failed = $state(false);
 $effect(() => { src; failed = false; });

	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((word) => word[0]!.toUpperCase())
			.join('')
	);
	const hue = $derived([...name].reduce((sum, ch) => (sum * 31 + ch.charCodeAt(0)) % 360, 7));
</script>

<span class="crest" style:--hue={hue} style:--size="{size}px" aria-hidden="true">{initials}{#if src && !failed}<img {src} alt="" width={size} height={size} loading="lazy" referrerpolicy="no-referrer" onerror={() => failed=true} />{/if}</span>

<style>
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; background: var(--surface); }
	.crest {
 position: relative; overflow: hidden;
		flex: none;
		display: inline-grid;
		place-items: center;
		width: var(--size);
		height: var(--size);
		border-radius: 28%;
		font: 750 calc(var(--size) * 0.4) / 1 var(--display);
		letter-spacing: -0.02em;
		color: #fff;
		background: linear-gradient(140deg, hsl(var(--hue) 62% 46%), hsl(calc(var(--hue) + 40) 58% 34%));
	}
</style>
