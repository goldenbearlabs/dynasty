<script lang="ts">
	// The commissioner's editor for a draft that has not started: reassign,
	// reorder, add and remove picks. Nothing changes until Save.
	import { untrack } from 'svelte';
	import { setDraftPicks, type DraftPick, type Franchise, type PickSlot } from '#lib/api.ts';
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

	// Follow the saved order until the commissioner starts editing.
	$effect(() => {
		const saved = toSlots(picks);
		if (!untrack(() => dirty)) slots = saved;
	});

	function move(i: number, by: number) {
		[slots[i], slots[i + by]] = [slots[i + by], slots[i]];
		dirty = true;
	}

	function add() {
		const last = slots.at(-1);
		const owner = franchises[0].id;
		slots.push({ round: last?.round ?? 1, original_franchise_id: owner, current_franchise_id: owner });
		dirty = true;
	}

	async function save() {
		try {
			await setDraftPicks(draftId, slots);
			dirty = false;
			toast.good('Pick order saved.');
		} catch (e) {
			toast.error(e);
		}
	}
</script>

<div class="stack tight">
	<div class="spread">
		<p class="muted">
			Set who owns each pick and the order they come in. “Originally” is whose pick it was, shown as “via” on the board.
		</p>
		<div class="row">
			{#if dirty}<button class="quiet" onclick={() => (dirty = false)}>Discard changes</button>{/if}
			<button class="primary" disabled={!dirty} onclick={save}>Save pick order</button>
		</div>
	</div>

	<div class="card flush scroll">
		<table>
			<thead><tr><th class="num">Pick</th><th>Round</th><th>Belongs to</th><th>Originally</th><th></th></tr></thead>
			<tbody>
				{#each slots as slot, i (slot.id ?? `new-${i}`)}
					<tr>
						<td class="num n">{i + 1}</td>
						<td><input type="number" min="1" aria-label="Round" bind:value={slot.round} oninput={() => (dirty = true)} /></td>
						<td>
							<select aria-label="Belongs to" bind:value={slot.current_franchise_id} onchange={() => (dirty = true)}>
								{#each franchises as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
							</select>
						</td>
						<td>
							<select aria-label="Originally" bind:value={slot.original_franchise_id} onchange={() => (dirty = true)}>
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
									dirty = true;
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

	<div><button onclick={add}><Icon name="plus" size={16} /> Add a pick</button></div>
</div>

<style>
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
