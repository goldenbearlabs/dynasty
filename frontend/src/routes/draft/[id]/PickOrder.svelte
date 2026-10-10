<script lang="ts">
	// The commissioner's editor for a draft that has not started: reassign,
	// reorder, add and remove picks. Nothing changes until Save.
	import { untrack } from 'svelte';
	import { setDraftPicks, type DraftPick, type Franchise, type PickSlot } from '#lib/api.ts';
	import { fillPickOrder, inferPickOrder, roundTeams } from '#lib/pickOrder.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { draftId, picks, franchises }: { draftId: string; picks: DraftPick[]; franchises: Franchise[] } = $props();

	const toSlots = (list: DraftPick[]): PickSlot[] =>
		list.map((p) => ({
			id: p.id,
			round: p.round,
			original_franchise_id: p.original_franchise_id,
			current_franchise_id: p.current_franchise_id
		}));

	let slots = $state<PickSlot[]>([]);
	let dirty = $state(false);
	let saving = $state(false);
	let teams = $state<string[]>([]);
	let order = $state<'snake' | 'linear'>('snake');
	const automatic = $derived(roundTeams(slots) !== null);
	const rounds = $derived(Math.max(0,...slots.map(p => p.round)));
	const teamName = (id: string) => franchises.find(f => f.id === id)?.name ?? 'Unknown team';
	function fill() {
		slots = fillPickOrder(slots,teams,order);
		dirty = true;
	}
	function moveTeam(i: number, by: number) {
		[teams[i],teams[i+by]] = [teams[i+by],teams[i]];
		fill();
	}

	function discard() {
		slots = toSlots(picks);
		teams = roundTeams(slots) ?? [];
		order = inferPickOrder(slots);
		dirty = false;
	}
	function edited() {
		dirty = true;
		teams = roundTeams(slots) ?? [];
		order = inferPickOrder(slots);
	}

	// Follow the saved order until the commissioner starts editing.
	$effect(() => {
		const saved = toSlots(picks);
		if (!untrack(() => dirty)) {
			slots = saved;
			teams = roundTeams(saved) ?? [];
			order = inferPickOrder(saved);
		}
	});

	function move(i: number, by: number) {
		[slots[i], slots[i + by]] = [slots[i + by], slots[i]];
		edited();
	}

	function add() {
		const last = slots.at(-1);
		const owner = franchises[0].id;
		slots.push({ round: last?.round ?? 1, original_franchise_id: owner, current_franchise_id: owner });
		edited();
	}

	async function save() {
		if (saving) return;
		saving = true;
		try {
			await setDraftPicks(draftId, slots);
			dirty = false;
			toast.good('Pick order saved.');
		} catch (e) {
			toast.error(e);
		} finally { saving = false; }
	}
</script>

<div class="stack tight">
	<div class="spread">
		<p class="muted">
			Arrange the teams in round one. Every remaining round follows that order, reversed on even rounds for a snake draft.
		</p>
		<div class="row">
			{#if dirty}<button class="quiet" disabled={saving} onclick={discard}>Discard changes</button>{/if}
			<button class="primary" disabled={!dirty || saving} onclick={save}>Save pick order</button>
		</div>
	</div>

	{#if automatic}
		<div class="order-layout">
			<section class="card stack tight">
				<div class="spread"><h3>First-round order</h3><label class="row small-text">Draft format <select bind:value={order} onchange={fill} disabled={saving}><option value="snake">Snake</option><option value="linear">Same order every round</option></select></label></div>
				<ol class="team-order">
					{#each teams as id, i (id)}
						<li><span class="n">{i+1}</span><strong>{teamName(id)}</strong><button class="quiet small" aria-label="Move {teamName(id)} up" disabled={saving || i===0} onclick={() => moveTeam(i,-1)}><Icon name="up" size={15} /></button><button class="quiet small" aria-label="Move {teamName(id)} down" disabled={saving || i===teams.length-1} onclick={() => moveTeam(i,1)}><Icon name="down" size={15} /></button></li>
					{/each}
				</ol>
			</section>
			<section class="card stack tight">
				<h3>How the rounds will run</h3>
				<p class="muted small-text">{rounds} rounds · {slots.length} picks. Traded picks keep their current owners.</p>
				{#each [1,2,3].filter(r => r <= rounds) as round}
					<div class="round-preview"><strong>Round {round}</strong><p class="muted small-text">{(order === 'snake' && round%2===0 ? [...teams].reverse() : teams).map(teamName).join(' → ')}</p></div>
				{/each}
				<p class="muted small-text">{order === 'snake' ? 'Odd rounds follow round one. Even rounds reverse it.' : 'Every round follows the same team order.'}</p>
			</section>
		</div>
	{:else}
		<p class="muted small-text">This draft has custom picks. Use individual edits below; automatic ordering requires one pick per team in each round.</p>
	{/if}

	<details class="individual" open={!automatic}>
		<summary>Advanced: edit individual picks</summary>
		<p class="muted small-text">Reassign traded picks, add or remove picks, or customize individual rounds. “Originally” identifies the team whose pick it was.</p>
	<fieldset disabled={saving}>
	<div class="card flush scroll">
		<table>
			<thead><tr><th class="num">Pick</th><th>Round</th><th>Belongs to</th><th>Originally</th><th></th></tr></thead>
			<tbody>
				{#each slots as slot, i (slot.id ?? `new-${i}`)}
					<tr>
						<td class="num n">{i + 1}</td>
						<td><input type="number" min="1" aria-label="Round" bind:value={slot.round} oninput={edited} /></td>
						<td>
							<select aria-label="Belongs to" bind:value={slot.current_franchise_id} onchange={edited}>
								{#each franchises as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
							</select>
						</td>
						<td>
							<select aria-label="Originally" bind:value={slot.original_franchise_id} onchange={edited}>
								{#each franchises as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
							</select>
						</td>
						<td class="actions">
							<button class="quiet small" aria-label="Move up" disabled={i === 0} onclick={() => move(i, -1)}><Icon name="up" size={15} /></button>
							<button class="quiet small" aria-label="Move down" disabled={i === slots.length - 1} onclick={() => move(i, 1)}>
								<Icon name="down" size={15} />
							</button>
							<button
								class="quiet small danger"
								aria-label="Remove pick"
								onclick={() => {
									slots.splice(i, 1);
									edited();
								}}
							>
								<Icon name="x" size={15} />
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div><button disabled={!franchises.length} onclick={add}><Icon name="plus" size={16} /> Add a pick</button></div>
	</fieldset>
	</details>
</div>

<style>
	.order-layout{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,22rem),1fr));gap:1rem;align-items:start}
	.team-order{list-style:none;padding:0;margin:0}.team-order li{display:flex;align-items:center;gap:.5rem;padding:.4rem 0;border-bottom:1px solid var(--rule)}.team-order strong{flex:1}
	.round-preview{display:grid;gap:.2rem;padding:.6rem 0;border-bottom:1px solid var(--rule)}
	.individual > summary{cursor:pointer;font-weight:600;padding:.7rem 0}.individual > p{padding-bottom:.7rem}
	fieldset{border:0;padding:0;margin:0;min-width:0;display:grid;gap:.7rem}

	.n {
		font: 700 0.85rem var(--mono);
		color: var(--ink-faint);
		width: 3.5rem;
	}
	td input[type='number'] {
		width: 4.2rem;
	}
	.spread p {
		max-width: 38rem;
	}
</style>
