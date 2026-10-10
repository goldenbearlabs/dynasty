<script lang="ts">
	import { tick } from 'svelte';
	import type { DraftPick, Franchise } from '#lib/api.ts';
	import Headshot from '#lib/ui/Headshot.svelte';

	let {
		picks,
		franchises,
		onClockId,
		myId,
		autoIds,
		showSport,
		onselect
	}: {
		picks: DraftPick[];
		franchises: Franchise[];
		onClockId: string | null;
		myId?: string;
		/** Franchises with auto pick on. */
		autoIds: string[];
		showSport: boolean;
		onselect?: (id: string) => void;
	} = $props();
	const name = (id: string) => franchises.find((f) => f.id === id)?.name ?? 'Unknown team';
	const rounds = $derived(
		Map.groupBy(
			picks.toSorted((a, b) => a.position - b.position),
			(p) => p.round
		)
	);
	const columns = $derived(Math.max(1, ...Array.from(rounds.values(), (r) => r.length)));
	const mine = $derived(
		picks
			.toSorted((a, b) => a.position - b.position)
			.filter((p) => p.current_franchise_id === myId && !p.player_id)
	);
	const next = $derived(mine.find((p) => !p.skipped_at && !p.passed_at) ?? mine[0]);
	let board: HTMLDivElement;
	let focused = false;
	$effect(() => {
		const current = onClockId;
		if (focused || !current) return;
		focused = true;
		void tick().then(() => jump(current));
	});
	function jump(id: string | null | undefined) {
		if (!id) return;
		const cell = board?.querySelector<HTMLElement>(`[data-pick="${CSS.escape(id)}"]`);
		if (!cell) return;
		// Scroll only this pane; keep search and rosters in view.
		board.scrollTo({
			left: Math.max(0, cell.offsetLeft - board.clientWidth / 2 + cell.offsetWidth / 2),
			top: Math.max(0, cell.offsetTop - board.clientHeight / 2),
			behavior: 'smooth'
		});
	}
</script>

<div class="board-tools">
	<div class="legend">
		<span class="key mine-key"></span>Your picks <span class="key clock-key"></span>On clock
		<span class="key auto-key"></span>Auto pick on
	</div>
	<div class="row">
		<button class="quiet small" disabled={!onClockId} onclick={() => jump(onClockId)}
			>Current pick</button
		>{#if myId}<button class="quiet small" disabled={!next} onclick={() => jump(next?.id)}
				>My next pick</button
			>{/if}
	</div>
</div>
<div class="board" bind:this={board} role="region" aria-label="Draft board by round">
	<table style:--columns={columns}>
		<tbody>
			{#each rounds as [round, roundPicks] (round)}
				<tr
					><th scope="row">R{round}</th>
					{#each roundPicks as pick (pick.id)}
						<td
							data-pick={pick.id}
							class:clock={pick.id === onClockId}
							class:mine={pick.current_franchise_id === myId}
							class:made={!!pick.player_id}
							class:auto={!pick.player_id && !pick.passed_at && autoIds.includes(pick.current_franchise_id)}
						>
							<div class="cell">
								<div class="owner">
									<span class="n">#{pick.position}</span><span
										title={name(pick.current_franchise_id)}>{name(pick.current_franchise_id)}</span
									>
								</div>
								{#if pick.player_id}
									<button
										class="player"
										onclick={() => onselect?.(pick.player_id!)}
										title={`Research ${pick.player_name}`}
									>
										<Headshot name={pick.player_name} src={pick.player_headshot} size={22} /><strong
											>{pick.player_name}</strong
										>
									</button>
									<span class="meta"
										>{pick.player_positions.join('/')}{#if showSport}
											· {pick.competition.toUpperCase()}{/if}{#if pick.auto_picked}
											· Auto{/if}</span
									>
								{:else}<span class="pending"
										>{pick.id === onClockId
											? 'On the clock'
											: pick.passed_at
												? 'Passed'
												: pick.skipped_at
													? 'Skipped · owed'
													: autoIds.includes(pick.current_franchise_id)
														? 'Auto pick'
														: 'Upcoming'}</span
									>{/if}
								{#if pick.original_franchise_id !== pick.current_franchise_id}<span
										class="via"
										title={`Traded from ${name(pick.original_franchise_id)}`}
										>via {name(pick.original_franchise_id)}</span
									>{/if}
							</div>
						</td>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.board-tools {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		padding: 0.25rem 0.65rem;
		border-bottom: 1px solid var(--rule);
	}
	.legend {
		display: flex;
		gap: 0.3rem;
		align-items: center;
		color: var(--ink-soft);
		font-size: 0.65rem;
	}
	.key {
		width: 7px;
		height: 7px;
		margin-left: 0.35rem;
	}
	.mine-key {
		background: var(--brand-soft);
		border: 1px solid var(--brand);
	}
	.clock-key {
		background: var(--brand);
	}
	.auto-key {
		background: var(--gold-soft);
		border: 1px solid var(--gold);
	}
	.board {
		overflow: auto;
		min-height: 0;
		flex: 1;
		position: relative;
	}
	table {
		table-layout: fixed;
		width: max(100%, calc(var(--columns) * 106px + 32px));
	}
	th {
		position: sticky;
		left: 0;
		z-index: 1;
		width: 32px;
		padding: 0.3rem;
		background: var(--surface-2);
		border-right: 1px solid var(--rule);
		text-align: center;
		letter-spacing: 0;
	}
	td {
		padding: 0;
		vertical-align: top;
		border-right: 1px solid var(--rule);
		border-bottom: 1px solid var(--rule);
	}
	tbody tr:last-child td {
		border-bottom: 1px solid var(--rule);
	}
	tbody tr:hover td {
		background: var(--surface);
	}
	td.mine,
	tbody tr:hover td.mine {
		background: var(--brand-soft);
	}
	td.auto,
	tbody tr:hover td.auto {
		background: var(--gold-soft);
	}
	td.auto .pending {
		color: var(--gold);
	}
	td.clock,
	tbody tr:hover td.clock {
		background: var(--brand-soft);
		box-shadow: inset 0 0 0 2px var(--brand);
	}
	.cell {
		display: grid;
		align-content: start;
		gap: 0.2rem;
		padding: 0.4rem 0.45rem;
		min-height: 72px;
	}
	.owner {
		display: flex;
		gap: 0.3rem;
		font-size: 0.58rem;
		line-height: 1.2;
		color: var(--ink-soft);
	}
	.owner span:last-child {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.n {
		font-family: var(--mono);
		flex: none;
	}
	.player {
		display: flex;
		justify-content: start;
		gap: 0.3rem;
		padding: 0;
		border: 0;
		background: transparent;
		border-radius: 0;
		text-align: left;
		white-space: normal;
		line-height: 1.15;
	}
	.player strong {
		font-size: 0.7rem;
	}
	.player:hover strong {
		color: var(--brand);
	}
	.meta,
	.via {
		font-size: 0.58rem;
		color: var(--ink-soft);
	}
	.via {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.pending {
		font-size: 0.68rem;
		color: var(--ink-faint);
		padding-top: 0.5rem;
	}
	.clock .pending {
		color: var(--brand);
		font-weight: 700;
	}
	@media (max-width: 640px) {
		.legend {
			display: none;
		}
		.board-tools {
			justify-content: end;
		}
	}
</style>
