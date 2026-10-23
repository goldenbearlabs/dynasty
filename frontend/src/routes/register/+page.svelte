<!-- Lets a person create an account before they have a league invitation. -->
<script lang="ts">
	import { createAccount, resendVerification } from '#lib/api.ts';

	let email = $state('');
	let password = $state('');
	let busy = $state(false);
	let created = $state(false);
	let message = $state('');

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		message = '';
		try {
			const result = await createAccount({ email, password });
			created = true;
			message = result.email_sent ? 'Check your email for a verification link.' : 'Your account was created, but the email could not be sent. Try resending it below.';
		} catch (error) {
			message = (error as Error).message;
		} finally {
			busy = false;
		}
	}

	async function resend() {
		try {
			await resendVerification(email);
			message = 'If your account needs verification, a new link has been sent.';
		} catch (error) {
			message = (error as Error).message;
		}
	}
</script>

<svelte:head><title>Create an account · Mosaic Fantasy</title></svelte:head>

<div class="card stack" style="max-width: 28rem; margin: 3rem auto; padding: 1.5rem;">
	<h1>Create an account</h1>
	<p>Start with an email and password. After verifying your email, use a commissioner invite link to join a league.</p>
	{#if !created}
		<form class="stack" onsubmit={submit}>
			<label class="field">Email <input type="email" autocomplete="email" bind:value={email} required /></label>
			<label class="field">Password <input type="password" autocomplete="new-password" minlength="8" maxlength="72" bind:value={password} required /></label>
			<button class="primary" disabled={busy}>{busy ? 'Creating…' : 'Create account'}</button>
		</form>
	{/if}
	{#if message}<p role="status">{message}</p>{/if}
	{#if created}<button class="quiet" onclick={resend}>Resend verification email</button>{/if}
	<a href="/">Back to sign in</a>
</div>
