<script lang="ts">
	// A matchup as two lineups facing each other, for a phone: one team down
	// the left, the other down the right, each slot's players side by side
	// with the slot between them. Names sit on the outside and points on the
	// inside; under each name are the stats that scored and where his game
	// stands.
	import type { GameSummary } from '#lib/api.ts';
	import { gameSummary, rowStats, type MatchupRow } from '#lib/matchupView.ts';
	import { scoringStats } from '#lib/statLine.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { points } from '#lib/ui/time.ts';

	type Side = { rows: MatchupRow[]; empty: boolean };
	// slots: the league's slot names in lineup order, which pick each tag's colour.
	let { left, right, scoring, slots = [], scores = {}, picking = true }: {
		left: Side; right: Side; scoring: Record<string, number>; slots?: string[]; scores?: Record<string, GameSummary>; picking?: boolean;
	} = $props();
	// One colour per slot, in the league's order: QB, RB, WR, TE… or F, D, G.
	const palette = ['#e0457b', '#1fa971', '#3b82f6', '#f08c00', '#8b5cf6', '#0ea5b7', '#c99700', '#e8590c', '#5c7cfa', '#2f9e44'];
	const tone = (slot: string) => {
		const i = slots.indexOf(slot);
		return i < 0 ? '#64748b' : palette[i % palette.length];
	};
	let benchOpen = $state(false);

	// Pairs the two sides row by row: starters slot by slot, then the bench.
	const pair = (a: MatchupRow[], b: MatchupRow[]) =>
		Array.from({ length: Math.max(a.length, b.length) }, (_, i) => ({ a: a[i], b: b[i], slot: a[i]?.slot ?? b[i]?.slot ?? '' }));
	const starters = $derived(pair(left.rows.filter((r) => !r.bench), right.rows.filter((r) => !r.bench)));
	const bench = $derived(pair(left.rows.filter((r) => r.bench), right.rows.filter((r) => r.bench)));
	const live = (row: MatchupRow) => row.games.some((g) => g.gameId && scores[g.gameId]?.status === 'live');
	// Over or under what was expected, once the games behind his points are over.
	const verdict = (row: MatchupRow) => (!row.settled || !row.pregame ? '' : row.points > row.pregame ? 'over' : row.points < row.pregame ? 'under' : '');
</script>

<!-- One side's cells: name then points on the left, points then name on the right. -->
{#snippet player(row: MatchupRow | undefined, side: 'a' | 'b', byeSide: boolean)}
	{#if side === 'b'}{@render fp(row, side)}{/if}
	<div class="who {side}">
		{#if !row}<span class="muted">{byeSide ? '' : '—'}</span>
		{:else if row.empty}<span class="muted">Empty</span>
		{:else}
			{@const stats = rowStats(row)}
			{@const line = stats.played ? scoringStats(stats.line, scoring) : []}
			<a href="/player/{row.id}" class="name">{row.name}</a>
			{#if line.length}<span class="line">{#each line as stat (stat.key)}<span class="stat"><b>{stat.value}</b>{stat.label}</span>{/each}</span>{/if}
			<span class="state" class:live={live(row)}>{gameSummary(row, scores, false, picking)}{#if row.earlier} · Earlier{/if}</span>
		{/if}
	</div>
	{#if side === 'a'}{@render fp(row, side)}{/if}
{/snippet}

{#snippet fp(row: MatchupRow | undefined, side: 'a' | 'b')}
	<div class="fp {side}" class:none={!row || row.empty || (!row.started && !row.points && !row.upcoming)}>
		{#if row && !row.empty}
			{#if !row.started && !row.points && row.upcoming}
				<!-- Nothing of his counts yet: what his next start is projected for, not a zero. -->
				<span class="ahead" title="Projected for his next start">{points(row.upcoming)}</span>
			{:else if !row.started && !row.points}
				– <!-- no game of his in view -->
			{:else}
				<span class={verdict(row)} title={verdict(row) ? `Projected ${points(row.pregame)} before the game` : undefined}>{points(row.points)}</span>
				{#if row.settled && row.pregame}<span class="proj" title="Projected before the game">proj {points(row.pregame)}</span>{/if}
				{#if row.projected}<span class="proj" title="Projected for games still to come">+{points(row.projected)}</span>{/if}
			{/if}
		{:else if row?.empty}–{/if}
	</div>
{/snippet}

{#snippet rows(list: typeof starters, onBench: boolean)}
	{#each list as r, i (i)}
		<li class="pair" class:bench={onBench}>
			{@render player(r.a, 'a', left.empty)}
			<span class="slot" style:--tone={onBench ? undefined : tone(r.slot)}>{r.slot}</span>
			{@render player(r.b, 'b', right.empty)}
		</li>
	{/each}
{/snippet}

<ol class="faceoff">
	{@render rows(starters, false)}
	{#if bench.length}
		<li class="bench-head">
			<button aria-expanded={benchOpen} onclick={() => (benchOpen = !benchOpen)}>
				<Icon name={benchOpen ? 'down' : 'right'} size={11} /> Bench
			</button>
		</li>
		{#if benchOpen}{@render rows(bench, true)}{/if}
	{/if}
</ol>

<style>
	.faceoff {
		container-type: inline-size;
		margin: 0;
		padding: 0;
		list-style: none;
		background: var(--surface);
		border: 1px solid var(--rule);
		border-radius: var(--radius);
		overflow: hidden;
	}
	/* Name, points, slot, points, name. */
	.pair {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 2.9rem 2.1rem 2.9rem minmax(0, 1fr);
		align-items: center;
		gap: 0 0.3rem;
		padding: 0.35rem 0.45rem;
	}
	.pair + .pair,
	.bench-head + .pair {
		border-top: 1px solid var(--rule);
	}
	.who {
		display: grid;
		min-width: 0;
		line-height: 1.25;
	}
	.who.b {
		text-align: right;
		justify-items: end;
	}
	.who > * {
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	/* Three tiers: the name, the stats that scored, where his game stands. */
	.name {
		color: var(--ink);
		font: 700 0.84rem/1.3 var(--body);
		letter-spacing: -0.005em;
	}
	.line {
		display: flex;
		flex-wrap: wrap;
		gap: 0 0.5rem;
		margin-top: 0.1rem;
		white-space: normal;
	}
	.who.b .line {
		justify-content: flex-end;
	}
	.stat {
		white-space: nowrap;
		font: 500 0.6rem/1.45 var(--body);
		letter-spacing: 0.03em;
		color: var(--ink-soft);
	}
	.stat b {
		margin-right: 0.15rem;
		font: 700 0.74rem var(--body);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0;
		color: var(--ink);
	}
	.state {
		margin-top: 0.1rem;
		font-size: 0.64rem;
		color: var(--ink-faint);
	}
	.state.live {
		color: var(--bad);
		font-weight: 600;
	}
	/* Wide enough: the stats sit on the name's line. */
	@container (min-width: 46rem) {
		.who {
			display: flex;
			flex-wrap: wrap;
			align-items: baseline;
			column-gap: 0.7rem;
		}
		.who.b {
			flex-direction: row-reverse;
		}
		.who > .name {
			flex: none;
			max-width: 60%;
		}
		.line {
			flex: 1 1 0;
			min-width: 0;
			margin-top: 0;
		}
		.state {
			flex-basis: 100%;
		}
	}
	.fp {
		font: 750 0.85rem var(--mono);
		font-variant-numeric: tabular-nums;
	}
	.fp.a {
		text-align: right;
	}
	.fp.none {
		color: var(--ink-faint);
		font-weight: 500;
	}
	.fp > span {
		display: block;
	}
	.over {
		color: var(--good);
	}
	.under {
		color: var(--bad);
	}
	/* A projection standing in for points not yet scored. */
	.ahead {
		font-style: italic;
		font-weight: 550;
		color: var(--ink-soft);
	}
	.proj {
		font: italic 500 0.6rem var(--body);
		color: var(--ink-soft);
	}
	.slot {
		justify-self: center;
		min-width: 1.9rem;
		padding: 0.3rem 0.2rem;
		border-radius: 999px;
		background: var(--tone, #64748b);
		color: #fff;
		font: 800 0.6rem/1 var(--mono);
		letter-spacing: 0.02em;
		text-align: center;
		box-shadow: 0 1px 0 rgb(0 0 0 / 0.15);
	}
	.bench .slot {
		background: var(--surface-2);
		color: var(--ink-faint);
		box-shadow: inset 0 0 0 1px var(--rule-strong);
	}
	/* More room on a wider screen. */
	@media (min-width: 641px) {
		.pair {
			grid-template-columns: minmax(0, 1fr) 4rem 2.8rem 4rem minmax(0, 1fr);
			gap: 0 0.6rem;
			padding: 0.45rem 0.9rem;
		}
		.name {
			font-size: 0.95rem;
		}
		.stat {
			font-size: 0.66rem;
		}
		.stat b {
			font-size: 0.82rem;
		}
		.state {
			font-size: 0.7rem;
		}
		.fp {
			font-size: 1rem;
		}
		.proj {
			font-size: 0.68rem;
		}
		.slot {
			min-width: 2.5rem;
			padding: 0.38rem 0.3rem;
			font-size: 0.7rem;
		}
		.bench-head button {
			padding: 0.4rem 0.9rem;
			font-size: 0.68rem;
		}
	}
	.bench .fp {
		font-weight: 550;
		opacity: 0.8;
	}
	.bench-head {
		border-top: 1px solid var(--rule);
		background: var(--surface-2);
	}
	.bench-head button {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		width: 100%;
		padding: 0.35rem 0.5rem;
		border: 0;
		border-radius: 0;
		background: transparent;
		font: 600 0.62rem var(--mono);
		text-transform: uppercase;
		color: var(--ink-soft);
	}
</style>
