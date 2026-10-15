<script lang="ts">
	let { active = false }: { active?: boolean } = $props();
</script>

{#if active}
	<span class="presence" role="img" aria-label="Connected to draft room" title="Connected to draft room"></span>
{/if}

<style>
	.presence {
		display: inline-block;
		width: 7px;
		height: 7px;
		flex: none;
		border-radius: 50%;
		background: var(--good, #22c55e);
	}
</style>
