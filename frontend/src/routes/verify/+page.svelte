<!-- Consumes a one-use email link and returns the new account to sign-in. -->
<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { verifyAccount } from '#lib/api.ts';

	let message = $state('Verifying your email…');
	let verified = $state(false);

	onMount(() => {
		const token = page.url.searchParams.get('token');
		if (!token) {
			message = 'This verification link is missing its token.';
			return;
		}
		void (async () => {
			try {
				await verifyAccount(token);
				await invalidateAll();
				verified = true;
				message = 'Your email is verified. You can now create or join a league.';
			} catch (error) {
				message = (error as Error).message;
			}
		})();
	});
</script>

<svelte:head><title>Verify email · Mosaic Fantasy</title></svelte:head>

<div class="card stack" style="max-width: 28rem; margin: 3rem auto; padding: 1.5rem;">
	<h1>Email verification</h1>
	<p role="status">{message}</p>
	{#if verified}<a class="button primary" href="/">Continue to Mosaic Fantasy</a>{:else}<a href="/">Back to sign in</a>{/if}
</div>
