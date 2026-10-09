<script lang="ts">
	// The dynasty's name and overall title, and each league's rules.
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { addLeague, updateDynasty, updateLeagueSettings, type Competition, type Dynasty } from '#lib/api.ts';
	import LeagueSettingsForm from '#lib/LeagueSettingsForm.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';

	let { dynasty, competitions }: { dynasty: Dynasty; competitions: Competition[] } = $props();

	// Working copies, taken once: nothing changes for the league until Save.
	const initial = untrack(() => structuredClone($state.snapshot(dynasty)));
	let name = $state(initial.name);
	let overall = $state(initial.settings.overall_title);
	let points = $state(initial.settings.overall_title.points_by_finish.join(', '));
	let leagues = $state(initial.leagues);

	let saved = $state(''); // id of what was last saved
	let failed = $state({ id: '', message: '' });

	const sport = (key: string) => competitions.find((c) => c.key === key)!;
	const othersFor = (key: string) => leagues.filter((l) => l.competition !== key).map((l) => sport(l.competition));

	async function save(id: string, work: () => Promise<void>) {
		saved = '';
		failed = { id: '', message: '' };
		try {
			await work();
			await invalidateAll();
			saved = id;
		} catch (e) {
			failed = { id, message: (e as Error).message };
		}
	}

	// Sports the dynasty does not play yet.
	const unplayed = $derived(competitions.filter((c) => !leagues.some((l) => l.competition === c.key)));
	let adding = $state('');

	// Replaces the working copy of a league's rules with its sport's
	// recommended ones; nothing changes for the league until Save.
	let loaded = $state('');
	function loadDefaults(league: (typeof leagues)[number]) {
		const continuity = league.settings.continuity;
		league.settings = { ...structuredClone($state.snapshot(sport(league.competition).defaults)), continuity };
		loaded = league.id;
	}

	const saveDynasty = () =>
		save('dynasty', () =>
			updateDynasty(name, {
				overall_title: {
					enabled: overall.enabled,
					points_by_finish: points
						.split(',')
						.map((p) => Number(p.trim()))
						.filter((p) => !Number.isNaN(p))
				}
			})
		);
</script>

{#snippet saveRow(id: string, label: string, onsave: () => void)}
	<div class="stack tight save">
		{#if failed.id === id}<p role="alert">{failed.message}</p>{/if}
		<div class="row">
			<button class="primary" onclick={onsave}>{label}</button>
			{#if saved === id}<span class="pill good">Saved</span>{/if}
		</div>
	</div>
{/snippet}

<section class="stack tight">
	<details class="card">
		<summary>Dynasty</summary>
		<div class="stack">
			<label class="field">Dynasty name <input bind:value={name} /></label>
			<label class="check">
				<input type="checkbox" bind:checked={overall.enabled} />
				Crown an overall champion across all sports
			</label>
			{#if overall.enabled}
				<label class="field">Points for finishing 1st, 2nd, 3rd… in each sport <input bind:value={points} /></label>
			{/if}
			{@render saveRow('dynasty', 'Save', saveDynasty)}
		</div>
	</details>

	{#each leagues as league (league.id)}
		<details class="card" data-sport={league.competition}>
			<summary><SportBadge sport={league.competition} solid /> {league.name} rules</summary>
			<div class="row recommended">
				<button type="button" class="small" onclick={() => loadDefaults(league)}>Load recommended settings</button>
				<span class="muted small-text">
					{loaded === league.id
						? 'Loaded below. Review them, then save.'
						: 'The lineup, roster sizes and scoring this sport was balanced with.'}
					<a href="/info">How they were chosen</a>
				</span>
			</div>
			<LeagueSettingsForm
				bind:settings={league.settings}
				competition={sport(league.competition)}
				others={othersFor(league.competition)}
			/>
			{@render saveRow(league.id, `Save ${league.name} rules`, () =>
				save(league.id, () => updateLeagueSettings(league.id, league.settings))
			)}
		</details>
	{/each}

	{#if unplayed.length > 0}
		<div class="card stack tight">
			<h3>Add a sport</h3>
			<p class="muted small-text">
				Every franchise joins the new league with an empty roster. It starts with that sport's default rules,
				which you can then change above.
			</p>
			{#if failed.id === 'league'}<p role="alert">{failed.message}</p>{/if}
			<div class="row">
				<select aria-label="Sport to add" bind:value={adding}>
					<option value="">Choose a sport</option>
					{#each unplayed as c (c.key)}
						<option value={c.key}>{c.name}</option>
					{/each}
				</select>
				<button class="primary" disabled={!adding} onclick={() => save('league', async () => void (await addLeague(adding)))}>
					Add league
				</button>
			</div>
		</div>
	{/if}
</section>

<style>
	.recommended {
		padding: 0.6rem 0 0.2rem;
	}
	summary {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		cursor: pointer;
		font: 700 1.05rem var(--display);
	}
	details[open] > summary {
		margin-bottom: 1.1rem;
		padding-bottom: 0.9rem;
		border-bottom: 1px solid var(--rule);
	}
	.save {
		padding-top: 0.4rem;
	}
</style>
