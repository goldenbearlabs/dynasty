<!-- Shows a signed-in account's leagues, including a useful empty state before joining one. -->
<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { createMyLeague, logOut, resendVerification } from '#lib/api.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let notice = $state('');
	let busy = $state(false);
	let creating = $state(false);
	let creationError = $state('');
	let leagueName = $state('');
	let franchiseName = $state('');
	let managerName = $state('');
	const sectionNames: Record<string, string> = {
		team: 'My team', league: 'League', matchups: 'Matchups', players: 'Players',
		watchlist: 'Watchlist', research: 'Research', waivers: 'Waivers',
		drafts: 'Drafts', trades: 'Trades', rules: 'Rules'
	};
	const section = $derived(data.me?.id ? '' : page.url.searchParams.get('section') ?? '');
	const sectionTitle = $derived(sectionNames[section]);
	const hasSavedSetup = $derived(data.myLeagues.some((league) => !league.ready));

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

	async function create(event: SubmitEvent) {
		event.preventDefault();
		creationError = '';
		creating = true;
		try {
			const created = await createMyLeague({name: leagueName, franchise_name: franchiseName, manager_name: managerName});
			await invalidateAll();
			await goto(`/leagues/${created.id}`);
		} catch (error) {
			creationError = (error as Error).message;
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head><title>{sectionTitle ?? 'My Leagues'} · Mosaic Fantasy</title></svelte:head>

<div class="stack" style="max-width: 48rem; margin: 0 auto;">
	<p class="eyebrow">Mosaic Fantasy</p>
	<h1>{sectionTitle ?? 'My Leagues'}</h1>
	{#if sectionTitle}
		<section class="card stack" style="padding: 1.5rem;">
			<h2>{hasSavedSetup ? `${sectionTitle} is not ready yet` : `Join a league to use ${sectionTitle}`}</h2>
			<p>{hasSavedSetup ? 'Your league setup is saved privately. Gameplay features will appear after their API routes are scoped to each league.' : 'This area depends on a league and its franchises. Your account is ready, but no active league is connected yet.'}</p>
			<p>You can save a league setup from My Leagues or open a commissioner invite link to join the existing league. Shared join codes are still being built.</p>
			<a href="/leagues">Back to My Leagues</a>
		</section>
	{:else}
		{#each data.myLeagues as league (league.id)}
			<a class="card stack" href={league.id === data.dynasty?.id ? '/league' : `/leagues/${league.id}`} style="padding: 1.5rem;">
				<h2>{league.name}</h2>
				<p>{league.sports.length} sports · {league.franchise_name}{league.ready ? '' : ' · Setup saved'}</p>
				<span>Open league →</span>
			</a>
		{:else}
			<section class="card stack" style="padding: 1.5rem;">
				<h2>No leagues yet</h2>
				<p>You're signed in as {data.me?.email}. When you join or create a league, it will appear here.</p>
			</section>
		{/each}
		{#if !data.me?.verified}
			<section class="card stack" style="padding: 1.5rem;">
				<p>Verify your email before creating or joining a league. Check your inbox for the link.</p>
				<button disabled={busy} onclick={resend}>Resend verification email</button>
				{#if notice}<p role="status">{notice}</p>{/if}
			</section>
		{:else}
			<form class="card stack" style="padding: 1.5rem;" onsubmit={create}>
				<h2>Create a league</h2>
				<p>Save a private league setup using the current league's sports and rules. Gameplay becomes available after the multi-league API is finished.</p>
				<label class="field">League name <input bind:value={leagueName} maxlength="100" required /></label>
				<label class="field">Your franchise name <input bind:value={franchiseName} maxlength="100" required /></label>
				<label class="field">Your manager name <input bind:value={managerName} maxlength="100" required /></label>
				{#if creationError}<p role="alert">{creationError}</p>{/if}
				<button class="primary" disabled={creating}>{creating ? 'Saving…' : 'Create league setup'}</button>
			</form>
			<p>Have an invite link from a commissioner? Open it to join an existing league. Shared join codes are still being built.</p>
		{/if}
	{/if}
	<button class="quiet" style="align-self: start;" onclick={leave}>Sign out</button>
</div>
