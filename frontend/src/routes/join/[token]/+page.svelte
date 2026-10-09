<script lang="ts">
	// An invite link lands here. The manager chooses the email and password
	// they will sign in with from then on.
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { claimInvite } from '#lib/api.ts';
	import Crest from '#lib/ui/Crest.svelte';
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
			await claimInvite(page.params.token!, { email, password });
			await invalidateAll();
			await goto('/', { replaceState: true }); // the front door sends them to their team
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Join {data.dynasty?.name ?? 'the league'}</title></svelte:head>

<div class="card stack">
	{#if data.invite}
		{@const franchise = data.invite.franchise}
		<div class="row who">
			<Crest src={franchise.image_url} name={franchise.name} size={52} />
			<div>
				<p class="eyebrow">{data.dynasty?.name}</p>
				<h1>{franchise.name}</h1>
			</div>
		</div>
		<p class="muted">
			{#if data.invite.reset}
				Choose a new email and password for this franchise. Anywhere you were signed in before will be signed out.
			{:else}
				Welcome, {franchise.manager_name}. Choose the email and password you will sign in with.
			{/if}
		</p>
		<form class="stack" onsubmit={submit}>
			<label class="field">Email <input type="email" autocomplete="username" bind:value={email} required /></label>
			<label class="field">
				Password (at least 8 characters)
				<input type="password" autocomplete="new-password" minlength="8" bind:value={password} required />
			</label>
			{#if error}<p role="alert">{error}</p>{/if}
			<button class="primary" disabled={busy}>{data.invite.reset ? 'Save and sign in' : 'Create my account'}</button>
		</form>
	{:else}
		<h1>That link did not work</h1>
		<p role="alert">{data.problem}</p>
		<p>Invite links work once. If you already have an account, <a href="/">sign in</a>.</p>
	{/if}
</div>

<style>
	.card {
		max-width: 28rem;
		margin: 3rem auto;
		padding: 1.5rem;
	}
	.who {
		gap: 0.9rem;
	}
	h1 {
		font-size: 1.6rem;
	}
</style>
