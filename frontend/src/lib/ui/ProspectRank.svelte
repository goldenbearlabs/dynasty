<script lang="ts">
	// A small tag beside a ranked prospect's name: his place on the MLB Pipeline Top 100.
	import { prospectRank } from '#lib/prospects.svelte.ts';

	let { id }: { id: string } = $props();
	const r = $derived(prospectRank(id));
</script>

{#if r}
	<!-- A plain tag, not a link, so it can sit inside a clickable row. -->
	<span class="pill gold prospect" title="MLB Pipeline Top 100 prospect · #{r.rank} in {r.year}{r.eta ? ` · MLB ETA ${r.eta}` : ''}">#{r.rank}</span>
{/if}

<style>
	.prospect {
		padding: 0.12rem 0.32rem;
		font-size: 0.6rem;
		vertical-align: middle;
	}
</style>
