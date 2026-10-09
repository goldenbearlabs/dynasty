<script lang="ts">
	// The commissioner's own matchups for one period, in place of the
	// schedule's. A side left as a bye, or a franchise left out, sits out.
	import { untrack } from 'svelte';
	import { resetMatchups, setMatchups, type Matchup, type Period } from '#lib/api.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { period, matchups, teams, ondone }: {
		period: Period;
		matchups: Matchup[];
		teams: { id: string; name: string }[];
		ondone: (changed: boolean) => void;
	} = $props();

	// A draft of the period as it stood when editing began.
	let pairs = $state(untrack(() => matchups.map((m) => ({ home: m.home_franchise_id, away: m.away_franchise_id ?? '' }))));
	const picked = $derived(pairs.flatMap((p) => [p.home, p.away]).filter(Boolean));
	const twice = $derived(teams.filter((t) => picked.filter((id) => id === t.id).length > 1));
	const out = $derived(teams.filter((t) => !picked.includes(t.id)));
	const ready = $derived(pairs.length > 0 && pairs.every((p) => p.home) && twice.length === 0);

	const names = (list: typeof teams) => list.map((t) => t.name).join(', ');
	const run = (action: Promise<void>, done: string) =>
		action.then(() => (toast.good(done), ondone(true)), toast.error);
	const save = () =>
		run(setMatchups(period.id, pairs.map((p) => ({ home_franchise_id: p.home, away_franchise_id: p.away || null }))), 'Matchups saved.');
</script>

<div class="stack tight">
	{#each pairs as pair, i (i)}
		<div class="row">
			<select aria-label="Home side" bind:value={pair.home}>
				<option value="" disabled>Choose…</option>
				{#each teams as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
			</select>
			<span class="muted">vs</span>
			<select aria-label="Away side" bind:value={pair.away}>
				<option value="">Bye</option>
				{#each teams as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
			</select>
			<button class="quiet small" aria-label="Remove matchup" title="Remove" onclick={() => pairs.splice(i, 1)}><Icon name="x" size={14} /></button>
		</div>
	{/each}
	{#if twice.length}
		<p class="small-text bad">{names(twice)} {twice.length === 1 ? 'is' : 'are'} in more than one matchup.</p>
	{:else if out.length}
		<p class="muted small-text">Sitting out: {names(out)}.</p>
	{/if}
	<div class="row">
		<button class="small" onclick={() => pairs.push({ home: '', away: '' })}>Add matchup</button>
		<button class="small primary" disabled={!ready} onclick={save}>Save matchups</button>
		{#if period.by_hand}
			<button class="small" onclick={() => run(resetMatchups(period.id), 'Matchups are back to the schedule.')}>Back to automatic</button>
		{/if}
		<button class="small quiet" onclick={() => ondone(false)}>Cancel</button>
	</div>
</div>

<style>
	.bad {
		color: var(--bad);
	}
</style>
