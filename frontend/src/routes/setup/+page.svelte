<script lang="ts">
	// First run, in three steps: name the dynasty, pick its sports and rules,
	// list the managers.
	import { untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { createDynasty, type Competition } from '#lib/api.ts';
	import LeagueSettingsForm from '#lib/LeagueSettingsForm.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const steps = ['Dynasty', 'Sports and rules', 'Franchises'];
	let step = $state(0);

	// Every sport starts ticked, with its default rules.
	const startingLeagues = (competitions: Competition[]) =>
		competitions.map((competition) => ({
			competition,
			included: true,
			editing: false,
			settings: structuredClone(competition.defaults)
		}));

	let name = $state('');
	let overallTitle = $state(true);
	let overallPoints = $state('10, 7, 5, 3, 2, 1');
	let leagues = $state(untrack(() => startingLeagues(data.competitions)));
	let franchises = $state([
		{ name: '', manager_name: '' },
		{ name: '', manager_name: '' }
	]);

	let account = $state({ email: '', password: '' });

	let error = $state('');
	let saving = $state(false);

	const included = $derived(leagues.filter((l) => l.included));
	const othersFor = (key: string) => included.map((l) => l.competition).filter((c) => c.key !== key);

	const summary = (l: (typeof leagues)[number]) =>
		`${l.settings.roster.main} main · ${l.settings.roster.reserve} reserve · ` +
		(l.settings.format.type === 'head_to_head' ? 'head-to-head' : 'total points');

	// Each step must be complete before the next.
	const ready = $derived(
		[
			name.trim() !== '',
			included.length > 0,
			franchises[0].name.trim() !== '' && franchises[0].manager_name.trim() !== '' && account.email !== '' && account.password.length >= 8
		][step]
	);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (step < steps.length - 1) {
			step++;
			return;
		}
		error = '';
		saving = true;
		const keys = included.map((l) => l.competition.key);
		try {
			await createDynasty({
				name,
				settings: {
					overall_title: {
						enabled: overallTitle,
						points_by_finish: overallPoints
							.split(',')
							.map((p) => Number(p.trim()))
							.filter((p) => !Number.isNaN(p))
					}
				},
				leagues: included.map((l) => ({
					competition: l.competition.key,
					// Players cannot carry over into a sport that was left out.
					settings: {
						...l.settings,
						continuity: l.settings.continuity && keys.includes(l.settings.continuity.into) ? l.settings.continuity : null
					}
				})),
				franchises: franchises.filter((f) => f.name.trim() || f.manager_name.trim()),
				account
			});
			await invalidateAll();
			await goto('/commissioner');
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>Set up your dynasty</title></svelte:head>

{#if data.dynasty}
	<div class="stack tight">
		<h1>{data.dynasty.name} is already set up</h1>
		<p><a href="/league">Go to the league</a></p>
	</div>
{:else}
	<form class="stack" onsubmit={submit}>
		<header class="stack tight">
			<p class="eyebrow">Crossover Dynasty</p>
			<h1>Set up your dynasty</h1>
			<p class="muted">Every rule here can be changed later from the commissioner page.</p>
		</header>

		<ol class="steps">
			{#each steps as label, i (label)}
				<li class:done={i < step} aria-current={i === step ? 'step' : undefined}>
					<button type="button" class="quiet" disabled={i > step} onclick={() => (step = i)}>
						<span class="n">{#if i < step}<Icon name="check" size={14} />{:else}{i + 1}{/if}</span>
						{label}
					</button>
				</li>
			{/each}
		</ol>

		{#if step === 0}
			<section class="card stack">
				<label class="field">
					Dynasty name
					<!-- svelte-ignore a11y_autofocus -->
					<input bind:value={name} placeholder="The Group Chat Dynasty" required autofocus />
				</label>
				<div class="stack tight">
					<label class="check">
						<input type="checkbox" bind:checked={overallTitle} />
						<span><strong>Crown an overall champion</strong> across all sports, as well as one per sport</span>
					</label>
					{#if overallTitle}
						<label class="field">
							Points for finishing 1st, 2nd, 3rd… in each sport
							<input bind:value={overallPoints} />
						</label>
					{/if}
				</div>
			</section>
		{:else if step === 1}
			<section class="stack tight">
				{#each leagues as league (league.competition.key)}
					<div class="card sport" class:off={!league.included} data-sport={league.competition.key}>
						<div class="spread">
							<label class="check">
								<input type="checkbox" bind:checked={league.included} />
								<SportBadge sport={league.competition.key} solid={league.included} />
								<strong class="sport-name">{league.competition.name}</strong>
							</label>
							{#if league.included}
								<div class="row">
									<span class="muted small-text">{summary(league)}</span>
									<button type="button" class="small" onclick={() => (league.editing = !league.editing)}>
										{league.editing ? 'Done' : 'Edit rules'}
									</button>
								</div>
							{/if}
						</div>
						{#if league.included && league.editing}
							<div class="rules">
								<LeagueSettingsForm
									bind:settings={league.settings}
									competition={league.competition}
									others={othersFor(league.competition.key)}
								/>
							</div>
						{/if}
					</div>
				{/each}
			</section>
		{:else}
			<section class="card stack">
				<h2>Your account</h2>
				<p class="muted">You will sign in with this, and you will be the commissioner.</p>
				<div class="row end">
					<label class="field grow">Email <input type="email" autocomplete="username" bind:value={account.email} required /></label>
					<label class="field grow">
						Password (at least 8 characters)
						<input type="password" autocomplete="new-password" minlength="8" bind:value={account.password} required />
					</label>
				</div>
			</section>
			<section class="card stack">
				<h2>Franchises</h2>
				<p class="muted">
					The first franchise is yours. Each other manager gets an invite link to create their own account. You can add more
					franchises later.
				</p>
				{#each franchises as franchise, i (i)}
					<div class="row end">
						<label class="field grow">
							{i === 0 ? 'Your franchise name' : 'Franchise name'}
							<input bind:value={franchise.name} required={i === 0} />
						</label>
						<label class="field grow">
							{i === 0 ? 'Your name' : 'Manager'}
							<input bind:value={franchise.manager_name} required={i === 0} />
						</label>
						<button type="button" class="quiet danger" disabled={i === 0} aria-label="Remove franchise" onclick={() => franchises.splice(i, 1)}>
							<Icon name="x" />
						</button>
					</div>
				{/each}
				<div>
					<button type="button" onclick={() => franchises.push({ name: '', manager_name: '' })}>
						<Icon name="plus" size={16} /> Add a franchise
					</button>
				</div>
			</section>
		{/if}

		{#if error}<p role="alert">{error}</p>{/if}

		<div class="spread">
			<button type="button" class="quiet" disabled={step === 0} onclick={() => step--}>Back</button>
			<button class="primary" disabled={!ready || saving}>
				{step < steps.length - 1 ? 'Continue' : 'Create dynasty'}
			</button>
		</div>
	</form>
{/if}

<style>
	.steps {
		list-style: none;
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem 0.5rem;
		margin: 0;
		padding: 0;
	}
	.steps button {
		padding-left: 0.4rem;
		opacity: 1;
	}
	.n {
		display: grid;
		place-items: center;
		width: 1.5rem;
		height: 1.5rem;
		border-radius: 50%;
		border: 1px solid var(--rule-strong);
		font: 650 0.75rem var(--mono);
	}
	.steps [aria-current='step'] button {
		color: var(--ink);
	}
	.steps [aria-current='step'] .n {
		background: var(--brand);
		border-color: var(--brand);
		color: var(--brand-ink);
	}
	.done .n {
		background: var(--good-soft);
		border-color: transparent;
		color: var(--good);
	}

	.sport.off {
		box-shadow: none;
		background: transparent;
	}
	.sport-name {
		font: 700 1.05rem var(--display);
	}
	.rules {
		margin-top: 1rem;
		padding-top: 0.6rem;
		border-top: 1px solid var(--rule);
	}
	.grow {
		flex: 1 1 12rem;
	}
</style>
