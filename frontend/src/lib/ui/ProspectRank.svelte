<script lang="ts">
	import type { ProspectRankingFields } from '#lib/api.ts';
	import { prospectRank } from '#lib/prospects.svelte.ts';

	let { id, ranking }: { id: string; ranking?: ProspectRankingFields } = $props();
	// Player search already includes the rank; other lists share the catalog.
	const r = $derived(
		ranking?.prospect_rank != null && ranking.prospect_rank_source === 'mlb_pipeline'
			? { rank: ranking.prospect_rank, year: ranking.prospect_rank_year, eta: ranking.prospect_eta }
			: prospectRank(id)
	);
</script>

{#if r && r.rank >= 1 && r.rank <= 100}
	<!-- A plain badge can also sit inside a clickable player row. -->
	<span
		class="pill gold prospect"
		title="MLB Pipeline Top 100 prospect · #{r.rank}{r.year ? ` in ${r.year}` : ''}{r.eta ? ` · MLB ETA ${r.eta}` : ''}"
	>MLB <strong>#{r.rank}</strong></span>
{/if}

<style>
	.prospect {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.18rem 0.38rem;
		font-size: 0.64rem;
		line-height: 1.2;
		border-color: color-mix(in srgb, var(--gold) 35%, transparent);
		vertical-align: middle;
		flex-shrink: 0;
	}
	strong { font-weight: 800; }
</style>
