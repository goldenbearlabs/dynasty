<!-- Guides signed-in accounts without a league while league creation is deferred on the website. -->
<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { logOut, resendVerification } from '#lib/api.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let notice = $state('');
	let busy = $state(false);
	const sectionNames: Record<string, string> = {
		team: 'My team', league: 'League', matchups: 'Matchups', players: 'Players',
		watchlist: 'Watchlist', research: 'Research', waivers: 'Waivers',
		drafts: 'Drafts', trades: 'Trades', rules: 'Rules'
	};
	const sectionTitle = $derived(sectionNames[page.url.searchParams.get('section') ?? '']);

	async function resend() {
		busy = true;
		try {
			await resendVerification(data.me!.email);
			notice = 'If your account needs verification, a new link has been sent.';
		} catch (error) {
			notice = (error as Error).message;
		} finally {
			busy = false;
		}
	}

	async function leave() {
		await logOut();
		await invalidateAll();
		await goto('/');
	}
</script>

<svelte:head><title>{sectionTitle ?? 'Your account'} · Mosaic Fantasy</title></svelte:head>

<div class="stack" style="max-width: 48rem; margin: 0 auto;">
	<p class="eyebrow">Mosaic Fantasy</p>
	<h1>{sectionTitle ?? 'Your account'}</h1>
	<section class="card stack" style="padding: 1.5rem;">
		<h2>{sectionTitle ? `Join a league to use ${sectionTitle}` : 'You are signed in'}</h2>
		<p>Your account ({data.me?.email}) is not connected to a playable league yet. Ask a commissioner for an invite link to join one.</p>
		<p>League creation on the website is coming later. Any setup you previously saved remains stored.</p>
	</section>
	{#if !data.me?.verified}
		<section class="card stack" style="padding: 1.5rem;">
			<p>Verify your email before using an invite link. Check your inbox for the verification message.</p>
			<button disabled={busy} onclick={resend}>Resend verification email</button>
			{#if notice}<p role="status">{notice}</p>{/if}
		</section>
	{/if}
	<button class="quiet" style="align-self: start;" onclick={leave}>Sign out</button>
</div>
