<script lang="ts">
	import { tick } from 'svelte';
	import type { DraftPick, Franchise, PickReaction } from '#lib/api.ts';
	import Headshot from '#lib/ui/Headshot.svelte';
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import PresenceDot from '#lib/ui/PresenceDot.svelte';
	import { layoutDraftBoard } from '#lib/draftBoard.ts';

	let {
		picks,
		franchises,
		onClockId,
		myId,
		autoIds,
		onlineIds = [],
		reactions = [],
		onreact,
		showSport,
		changingId,
		onselect,
		onchange
	}: {
		picks: DraftPick[];
		franchises: Franchise[];
		onClockId: string | null;
		myId?: string;
		/** Franchises with auto pick on. */
		autoIds: string[];
		/** Managers currently connected to this draft room. */
		onlineIds?: string[];
		/** Managers' emoji on made picks. */
		reactions?: PickReaction[];
		/** Given to a signed-in manager: adds or takes back their emoji on a pick. */
		onreact?: (pickId: string, emoji: string) => void;
		showSport: boolean;
		/** The made pick the commissioner is changing. */
		changingId?: string;
		onselect?: (id: string) => void;
		/** Given only to a commissioner: offers to change a made pick. */
		onchange?: (pick: DraftPick) => void;
	} = $props();
	const name = (id: string) => franchises.find((f) => f.id === id)?.name ?? 'Unknown team';
	const layout = $derived(layoutDraftBoard(picks));
	const columns = $derived(Math.max(1, layout.columns.length));
	const mine = $derived(
		picks
			.toSorted((a, b) => a.position - b.position)
			.filter((p) => p.current_franchise_id === myId && !p.player_id)
	);
	const next = $derived(mine.find((p) => !p.skipped_at && !p.passed_at) ?? mine[0]);
	// The emoji a manager can react with.
	const emojis = ['🔥', '💎', '🐐', '👏', '😂', '😭', '😬', '🤔', '💀', '🥀', '🤡', '🍆', '🗑️'];
	// Each pick's reactions, one chip per emoji in the order they first came.
	const reacted = $derived.by(() => {
		const byPick = new Map<string, { emoji: string; ids: string[] }[]>();
		for (const r of reactions) {
			const chips = byPick.get(r.pick_id) ?? [];
			const chip = chips.find((c) => c.emoji === r.emoji);
			if (chip) chip.ids.push(r.franchise_id);
			else chips.push({ emoji: r.emoji, ids: [r.franchise_id] });
			byPick.set(r.pick_id, chips);
		}
		return byPick;
	});
	let choosingFor = $state<string>(); // the pick whose emoji palette is open
	function react(pickId: string, emoji: string) {
		choosingFor = undefined;
		onreact?.(pickId, emoji);
	}
	// A chip pops when it appears and each time its count changes, for everyone in the room.
	const pop = (count: number) => (chip: HTMLElement) => {
		if (count && !matchMedia('(prefers-reduced-motion: reduce)').matches)
			chip.animate(
				[{ transform: 'scale(1)' }, { transform: 'scale(1.3) rotate(-6deg)' }, { transform: 'scale(1)' }],
				{ duration: 320, easing: 'cubic-bezier(.3, 1.6, .5, 1)' }
			);
	};
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

<svelte:window onkeydown={(e) => e.key === 'Escape' && (choosingFor = undefined)} />

<div class="board-tools">
	<div class="legend">
		<span class="key mine-key"></span>Your picks <span class="key clock-key"></span>On clock
		<span class="key auto-key"></span>Auto pick on
		<PresenceDot active />In draft room
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
		<thead>
			<tr>
				<th scope="col" class="corner">Rd</th>
				{#each layout.columns as column (column.key)}
					<th scope="col" class="team-heading" title={name(column.franchiseId)}>
						<span class="team-name"><PresenceDot active={onlineIds.includes(column.franchiseId)} /><span>{name(column.franchiseId)}</span></span>
						<span class="manager">{franchises.find((f) => f.id === column.franchiseId)?.manager_name ?? ''}</span>
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each layout.rounds as { round, picks: roundPicks } (round)}
				<tr
					><th scope="row">R{round}</th>
					{#each roundPicks as pick, column (layout.columns[column].key)}
						{#if pick}
						<td
							data-pick={pick.id}
							class:clock={pick.id === onClockId}
							class:mine={pick.current_franchise_id === myId}
							class:made={!!pick.player_id}
							class:changing={pick.id === changingId}
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
										>{#if pick.player_id}<ProspectRank id={pick.player_id} />{/if}
									</button>
									<span class="meta"
										>{pick.player_positions.join('/')}{#if showSport}
											· {pick.competition.toUpperCase()}{/if}{#if pick.auto_picked}
											· Auto{/if}</span
									>{#if onchange}<button
											class="change"
											title={`Change pick #${pick.position}`}
											onclick={() => onchange(pick)}>Change</button
										>{/if}
									{@const chips = reacted.get(pick.id) ?? []}
									{#if chips.length || onreact}<div class="reactions">
											{#each chips as chip (chip.emoji)}<button
													class="chip"
													class:own={!!myId && chip.ids.includes(myId)}
													disabled={!onreact}
													aria-pressed={!!myId && chip.ids.includes(myId)}
													title={chip.ids.map(name).join(', ')}
													onclick={() => react(pick.id, chip.emoji)}
													{@attach pop(chip.ids.length)}>{chip.emoji}<span>{chip.ids.length}</span></button
												>{/each}{#if onreact}<button
													class="add"
													class:open={choosingFor === pick.id}
													aria-expanded={choosingFor === pick.id}
													title="React to this pick"
													onclick={() => (choosingFor = choosingFor === pick.id ? undefined : pick.id)}
													>+</button
												>{/if}
										</div>{#if choosingFor === pick.id}<div
												class="palette"
												role="group"
												aria-label="React with"
											>
												{#each emojis as emoji (emoji)}<button
														onclick={() => react(pick.id, emoji)}
														title={`React with ${emoji}`}>{emoji}</button
													>{/each}
											</div>{/if}{/if}
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
						{:else}<td class="empty-slot" aria-label="No pick in this round"></td>{/if}
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
	thead th {
		top: 0;
		left: auto;
		z-index: 2;
		width: auto;
		border-bottom: 1px solid var(--rule-strong);
	}
	thead .corner {
		left: 0;
		width: 32px;
		z-index: 3;
	}
	.team-heading {
		padding: 0.4rem;
	}
	.team-name {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.3rem;
		font-size: 0.65rem;
	}
	.team-name > span,
	.manager {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.manager {
		display: block;
		font-size: 0.58rem;
		font-weight: 400;
		color: var(--ink-soft);
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
	.change {
		justify-self: start;
		padding: 0;
		border: 0;
		background: transparent;
		font-size: 0.58rem;
		color: var(--ink-soft);
		text-decoration: underline;
		visibility: hidden;
	}
	td:hover .change,
	td.changing .change,
	.change:focus-visible {
		visibility: visible;
	}
	.change:hover {
		color: var(--brand);
	}
	td.changing,
	tbody tr:hover td.changing {
		background: var(--gold-soft);
		box-shadow: inset 0 0 0 2px var(--gold);
	}
	@media (hover: none) {
		.change {
			visibility: visible;
		}
	}
	.reactions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.2rem;
	}
	.reactions button,
	.palette button {
		padding: 0 0.25rem;
		border: 1px solid var(--rule);
		border-radius: 999px;
		background: var(--surface);
		font-size: 0.68rem;
		line-height: 1.5;
	}
	.chip span {
		margin-left: 0.15rem;
		font: 600 0.55rem var(--mono);
		color: var(--ink-soft);
	}
	.chip.own {
		border-color: var(--brand);
		background: var(--brand-soft);
	}
	.chip.own span {
		color: var(--brand);
	}
	.chip:disabled {
		opacity: 1;
		cursor: default;
	}
	.add {
		color: var(--ink-soft);
		visibility: hidden;
	}
	td:hover .add,
	.add.open,
	.add:focus-visible {
		visibility: visible;
	}
	.add:hover,
	.add.open {
		color: var(--brand);
		border-color: var(--brand);
	}
	.palette {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.15rem;
		padding: 0.2rem;
		border: 1px solid var(--rule-strong);
		border-radius: 6px;
		background: var(--surface-2);
		animation: open 140ms ease-out;
	}
	.palette button {
		padding: 0.1rem 0;
		border-color: transparent;
		background: transparent;
		font-size: 0.85rem;
		transition: transform 120ms;
	}
	.palette button:hover {
		transform: scale(1.25);
	}
	@keyframes open {
		from {
			opacity: 0;
			transform: translateY(-3px) scale(0.96);
		}
	}
	@media (hover: none) {
		.add {
			visibility: visible;
		}
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
