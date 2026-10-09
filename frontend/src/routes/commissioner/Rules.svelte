<script lang="ts">
	// The dynasty's name and overall title, and each league's rules.
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { updateDynasty, updateLeagueSettings, type Competition, type Dynasty } from '#lib/api.ts';
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
</section>

<style>
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
