<script lang="ts">
	// The landing page: sign in, or start a dynasty if this server has none.
	import { invalidateAll } from '$app/navigation';
	import { logIn } from '#lib/api.ts';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		busy = true;
		try {
			await logIn({ email, password });
			await invalidateAll(); // now signed in, so this page sends them to their team
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{data.dynasty?.name ?? 'Crossover Dynasty'}</title></svelte:head>

<div class="landing">
	<section class="pitch">
		<p class="eyebrow">Crossover Dynasty</p>
		<h1>{data.dynasty?.name ?? 'One franchise. Every sport.'}</h1>
		<p class="lede">
			A dynasty league across sports. Draft a high schooler and keep him through college into the pros, and trade an NBA
			first-round pick for an NHL goalie.
		</p>
		<div class="row sports">
			{#each data.dynasty ? data.dynasty.leagues.map((l) => l.competition) : data.competitions.map((c) => c.key) as sport (sport)}
				<SportBadge {sport} solid />
			{/each}
		</div>
	</section>

	{#if data.dynasty}
		<form class="card stack" onsubmit={submit}>
			<h2>Sign in</h2>
			<label class="field">Email <input type="email" autocomplete="username" bind:value={email} required /></label>
			<label class="field">
				Password
				<input type="password" autocomplete="current-password" bind:value={password} required />
			</label>
			{#if error}<p role="alert">{error}</p>{/if}
			<button class="primary" disabled={busy}>Sign in</button>
			<p class="muted small-text">
				New here? Open the invite link from your commissioner to create your account. Forgot your password? Ask them for a new
				link.
			</p>
			<p class="small-text"><a href="/league">Look around without signing in</a></p>
		</form>
	{:else}
		<div class="card stack">
			<h2>No dynasty here yet</h2>
			<p class="muted">Set one up: pick your sports and rules, and invite your friends. It takes a few minutes.</p>
			<a class="button primary" href="/setup">Set up a dynasty</a>
		</div>
	{/if}
</div>

<style>
	.landing {
		display: grid;
		grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
		align-items: center;
		gap: 2.5rem;
		min-height: calc(100dvh - 8rem);
	}
	@media (max-width: 760px) {
		.landing {
			grid-template-columns: 1fr;
			align-items: start;
			gap: 1.75rem;
		}
	}
	.pitch {
		display: grid;
		gap: 1rem;
	}
	h1 {
		font-size: clamp(2.4rem, 7vw, 3.8rem);
		font-weight: 800;
		line-height: 1;
		letter-spacing: -0.03em;
	}
	.lede {
		font-size: 1.1rem;
		color: var(--ink-soft);
		max-width: 30rem;
	}
	.sports {
		gap: 0.35rem;
	}
	.card {
		padding: 1.5rem;
	}
</style>
