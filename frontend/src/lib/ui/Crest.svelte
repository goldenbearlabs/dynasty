<script lang="ts">
	// A franchise's monogram on a colour derived from its name, so each
	// franchise is recognisable at a glance without anyone uploading a logo.
	let { name, size = 40 }: { name: string; size?: number } = $props();

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

<span class="crest" style:--hue={hue} style:--size="{size}px" aria-hidden="true">{initials}</span>

<style>
	.crest {
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
