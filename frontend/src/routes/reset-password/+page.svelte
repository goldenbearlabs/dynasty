<!-- Requests a reset email or consumes its one-use link to change an account password. -->
<script lang="ts">
	import { page } from '$app/state';
	import { completePasswordReset, requestPasswordReset } from '#lib/api.ts';

	let email = $state('');
	let password = $state('');
	let busy = $state(false);
	let done = $state(false);
	let message = $state('');
	const token = $derived(page.url.searchParams.get('token'));

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		message = '';
		try {
			if (token) {
				await completePasswordReset(token, password);
				message = 'Your password has been changed. Sign in again on all your devices.';
			} else {
				await requestPasswordReset(email);
				message = 'If that email has an account, a reset link has been sent.';
			}
			done = true;
		} catch (error) {
			message = (error as Error).message;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Reset password · Mosaic Fantasy</title></svelte:head>

<div class="card stack" style="max-width: 28rem; margin: 3rem auto; padding: 1.5rem;">
	<h1>{token ? 'Choose a new password' : 'Reset your password'}</h1>
	{#if !done}
		<form class="stack" onsubmit={submit}>
			{#if token}
				<label class="field">New password <input type="password" autocomplete="new-password" minlength="8" maxlength="72" bind:value={password} required /></label>
			{:else}
				<label class="field">Email <input type="email" autocomplete="email" bind:value={email} required /></label>
			{/if}
			<button class="primary" disabled={busy}>{busy ? 'Please wait…' : token ? 'Save new password' : 'Email reset link'}</button>
		</form>
	{/if}
	{#if message}<p role="status">{message}</p>{/if}
	<a href="/">Back to sign in</a>
</div>
