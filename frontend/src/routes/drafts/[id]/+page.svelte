<script lang="ts">
	// A finished draft's board, to look back on: who went where, and nothing
	// to change.
	import { goto } from '$app/navigation';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel } from '#lib/ui/time.ts';
	import Board from '../../draft/[id]/Board.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const draft = $derived(data.state.draft);
	const sports = $derived((data.dynasty?.leagues ?? []).filter((l) => data.state.league_ids.includes(l.id)).map((l) => l.competition));
	const made = $derived(data.state.picks.filter((p) => p.player_id).length);
</script>

<svelte:head><title>{draft.name} · Draft history</title></svelte:head>

<div class="stack history">
	<a class="small-text back" href="/drafts?tab=history"><Icon name="left" size={14} /> Draft history</a>
	<header class="spread">
		<div class="stack tight">
			<span class="row sports">{#each sports as sport (sport)}<SportBadge {sport} solid />{/each}</span>
			<h1>{draft.name}</h1>
			<p class="muted small-text">
				{draft.kind === 'startup' ? 'Startup draft' : 'Rookie draft'} · {draft.year} · {made} of {data.state.picks.length} picks made{#if draft.completed_at} · finished {dayLabel(draft.completed_at.slice(0, 10))}{/if}
			</p>
		</div>
		<span class="pill">View only</span>
	</header>
	<section class="panel board" aria-label="Draft board">
		<Board
			picks={data.state.picks}
			franchises={data.dynasty?.franchises ?? []}
			onClockId={null}
			myId={data.me?.id}
			autoIds={[]}
			reactions={data.state.reactions}
			showSport={sports.length > 1}
			onselect={(id) => goto(`/player/${id}`)}
			readonly
		/>
	</section>
</div>

<style>
	.history {
		gap: 0.75rem;
		min-width: 0;
		grid-template-columns: minmax(0, 1fr);
	}
	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
	}
	.sports {
		gap: 0.3rem;
	}
	.board {
		min-width: 0;
		overflow: auto;
	}
</style>
