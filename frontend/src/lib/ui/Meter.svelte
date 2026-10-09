<script lang="ts">
	// How full something is: "9 / 14" over a bar. Turns red when over.
	let { label, value, max }: { label: string; value: number; max: number } = $props();

	const over = $derived(value > max);
	const fill = $derived(max > 0 ? Math.min(100, (value / max) * 100) : 0);
</script>

<div class="meter" class:over>
	<div class="spread">
		<span class="eyebrow">{label}</span>
		<span class="count"><strong>{value}</strong> / {max}</span>
	</div>
	<div class="track" role="meter" aria-label={label} aria-valuemin="0" aria-valuemax={max} aria-valuenow={value}>
		<div class="fill" style:width="{fill}%"></div>
	</div>
</div>

<style>
	.meter {
		display: grid;
		gap: 0.35rem;
	}
	.count {
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
		color: var(--ink-soft);
	}
	.count strong {
		color: var(--ink);
	}
	.track {
		height: 6px;
		border-radius: 3px;
		background: var(--surface-2);
		border: 1px solid var(--rule);
		overflow: hidden;
	}
	.fill {
		height: 100%;
		background: var(--sport, var(--brand));
		transition: width 0.25s;
	}
	.over .fill {
		background: var(--bad);
	}
	.over .count strong {
		color: var(--bad);
	}
</style>
