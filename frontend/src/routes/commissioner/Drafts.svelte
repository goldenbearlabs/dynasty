<script lang="ts">
	// Create a draft. Its pick order is then arranged in the draft room.
	import { untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { createDraft, createFutureDrafts, scheduleDraft, nextDrafts, type DraftSummary, type Dynasty, type NewDraft } from '#lib/api.ts';
	import DraftCard from '#lib/DraftCard.svelte';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { dynasty, drafts }: { dynasty: Dynasty; drafts: DraftSummary[] } = $props();

	// A startup draft is slow, so everyone can make every pick: hours a pick, not seconds.
	const startupClock = 4 * 3600;

	let draft = $state<NewDraft>({
		name: '',
		kind: 'startup',
		year: new Date().getFullYear(),
		league_ids: [],
		rounds: 0,
		order: 'snake',
		franchise_order: untrack(() => dynasty.franchises.map((f) => f.id)),
		pick_clock_seconds: startupClock
	});

	// Only what is coming next: the drafts for later years exist so their picks can be traded.
	const upcoming = $derived(nextDrafts(drafts));
	const franchise = (id: string) => dynasty.franchises.find((f) => f.id === id)!;
	const chosen = $derived(dynasty.leagues.filter((l) => draft.league_ids.includes(l.id)));
	// How many players each franchise can draft across the chosen leagues:
	// every spot on its main rosters and reserve lists.
	const capacity = $derived(chosen.reduce((sum, l) => sum + l.settings.roster.main + l.settings.roster.reserve, 0));

	function toggleLeague(id: string) {
		if (draft.kind === 'seasonal') {
			draft.league_ids = [id];
			// A seasonal draft starts from the league's own draft rules.
			const rules = dynasty.leagues.find((l) => l.id === id)!.settings.draft;
			draft.rounds = rules.rounds;
			draft.order = rules.order;
			draft.pick_clock_seconds = rules.pick_clock_seconds;
		} else if (draft.league_ids.includes(id)) {
			draft.league_ids = draft.league_ids.filter((l) => l !== id);
		} else {
			draft.league_ids.push(id);
		}
		if (draft.kind === 'startup') draft.rounds = capacity; // a full roster in every league
	}

	function setKind(kind: NewDraft['kind']) {
		draft.kind = kind;
		draft.league_ids = [];
		draft.rounds = 0;
		draft.pick_clock_seconds = kind === 'startup' ? startupClock : 0;
	}

	function move(i: number, by: number) {
		const order = draft.franchise_order;
		[order[i], order[i + by]] = [order[i + by], order[i]];
	}

	async function schedule(d: DraftSummary) {
		try { await scheduleDraft(d.id); await invalidateAll(); toast.good(`${d.name} is now listed on Drafts.`); }
		catch (e) { toast.error(e); }
	}
	async function createFuture() {
		try {
			const { created } = await createFutureDrafts();
			await invalidateAll();
			toast.good(created > 0 ? `Created ${created} future ${created === 1 ? 'draft' : 'drafts'}.` : 'Every future draft already exists.');
		} catch (e) {
			toast.error(e);
		}
	}

	async function create(event: SubmitEvent) {
		event.preventDefault();
		try {
			const created = await createDraft(draft);
			await invalidateAll();
			toast.good('Draft created. Arrange the picks, then start it.');
			await goto(`/draft/${created.id}`);
		} catch (e) {
			toast.error(e);
		}
	}
</script>

<section class="stack">
	<div class="card spread">
		<div>
			<strong>Future draft picks</strong>
			<p class="muted small-text">
				Each league's rookie drafts for the coming years are created automatically; this fills in any that are missing (as many as its rules allow trading ahead), so their picks
				exist and can be traded.
			</p>
		</div>
		<button onclick={createFuture}>Create future drafts</button>
	</div>

	{#if upcoming.length > 0}
		<div class="list">
			{#each upcoming as d (d.id)}<div class="stack tight"><DraftCard draft={d} />{#if d.is_placeholder && d.status === 'scheduled'}<button class="small" onclick={() => schedule(d)}>Schedule draft</button>{/if}</div>{/each}
		</div>
		{#if drafts.length > upcoming.length}
			<p class="muted small-text">
				Showing the next draft for each league. Future picks remain available for trading. Schedule a draft when you’re ready to list it for managers.
			</p>
		{/if}
	{/if}

	<form class="card stack" onsubmit={create}>
		<h2>New draft</h2>

		<div class="row end">
			<label class="field grow">Name <input bind:value={draft.name} placeholder="2026 Startup Draft" required /></label>
			<label class="field">
				Kind
				<select value={draft.kind} onchange={(e) => setKind(e.currentTarget.value as NewDraft['kind'])}>
					<option value="startup">Startup: everyone is available</option>
					<option value="seasonal">Rookie draft: one league's yearly draft</option>
				</select>
			</label>
			<label class="field">Year <input type="number" bind:value={draft.year} /></label>
		</div>

		<div class="stack tight">
			<span class="eyebrow">
				{draft.kind === 'startup' ? 'Leagues: pick several for one combined draft' : 'League'}
			</span>
			<div class="row">
				{#each dynasty.leagues as league (league.id)}
					<button
						type="button"
						class="league"
						class:on={draft.league_ids.includes(league.id)}
						aria-pressed={draft.league_ids.includes(league.id)}
						data-sport={league.competition}
						onclick={() => toggleLeague(league.id)}
					>
						<SportBadge sport={league.competition} solid={draft.league_ids.includes(league.id)} />
						{league.name}
					</button>
				{/each}
			</div>
		</div>

		<div class="row end">
			<label class="field">Rounds <input type="number" min="1" max={capacity || undefined} bind:value={draft.rounds} /></label>
			<label class="field">
				Order
				<select bind:value={draft.order}>
					<option value="snake">Snake</option>
					<option value="linear">Same order every round</option>
				</select>
			</label>
			{#if draft.kind === 'startup'}
				<label class="field">
					Hours per pick (0 = no clock)
					<input type="number" min="0" step="any" bind:value={() => draft.pick_clock_seconds / 3600, (hours) => (draft.pick_clock_seconds = Math.round((hours ?? 0) * 3600))} />
				</label>
			{:else}
				<label class="field">Seconds per pick (0 = no clock) <input type="number" min="0" bind:value={draft.pick_clock_seconds} /></label>
			{/if}
			{#if capacity > 0}
				<span class="muted small-text hint">
					{#if draft.kind === 'startup'}
						{capacity} rounds fill every main roster and reserve list in {chosen.length === 1 ? 'this league' : 'these leagues'}. Each franchise sets its reserve list after the draft.
					{:else}
						Each franchise has {capacity} roster spots in this league.
					{/if}
				</span>
			{/if}
		</div>

		<div class="stack tight">
			<span class="eyebrow">First-round order</span>
			<ol class="order">
				{#each draft.franchise_order as id, i (id)}
					<li>
						<span class="n">{i + 1}</span>
						<Crest src={franchise(id).image_url} name={franchise(id).name} size={26} />
						<strong>{franchise(id).name}</strong>
						<span class="moves">
							<button type="button" class="quiet small" aria-label="Move up" disabled={i === 0} onclick={() => move(i, -1)}>
								<Icon name="up" size={15} />
							</button>
							<button
								type="button"
								class="quiet small"
								aria-label="Move down"
								disabled={i === draft.franchise_order.length - 1}
								onclick={() => move(i, 1)}
							>
								<Icon name="down" size={15} />
							</button>
						</span>
					</li>
				{/each}
			</ol>
			<p class="muted small-text">You can reassign, reorder, add and remove individual picks in the draft room before starting.</p>
		</div>

		<div><button class="primary" disabled={draft.league_ids.length === 0 || draft.rounds < 1}>Create draft</button></div>
	</form>
</section>

<style>
	.list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 0.8rem;
	}
	.grow {
		flex: 1 1 14rem;
	}
	.hint {
		padding-bottom: 0.55rem;
	}
	.league.on {
		border-color: var(--sport);
	}
	.order {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.3rem;
		max-width: 26rem;
	}
	.order li {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.3rem 0.4rem 0.3rem 0.7rem;
		background: var(--surface-2);
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
	}
	.n {
		width: 1.4rem;
		font: 650 0.8rem var(--mono);
		color: var(--ink-faint);
	}
	.moves {
		margin-left: auto;
		white-space: nowrap;
	}
</style>
