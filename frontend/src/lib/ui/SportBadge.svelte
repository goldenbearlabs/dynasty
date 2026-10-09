<script lang="ts">
	// The sport's short name in the sport's colour.
	let { sport, solid = false }: { sport: string; solid?: boolean } = $props();
</script>

<span class="badge" class:solid data-sport={sport}>{sport}</span>

<style>
	.badge {
		display: inline-block;
		font: 700 0.66rem/1 var(--mono);
		letter-spacing: 0.08em;
		text-transform: uppercase;
		padding: 0.28rem 0.42rem;
		border-radius: 5px;
		color: var(--sport);
		background: color-mix(in srgb, var(--sport) 14%, transparent);
	}
	.solid {
		color: var(--surface);
		background: var(--sport);
	}
</style>
