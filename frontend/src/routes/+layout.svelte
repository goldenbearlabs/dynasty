<script lang="ts">
	// The app shell: a rail of navigation beside the page on wide screens,
	// and a tab bar under it on narrow ones.
	import '@fontsource-variable/bricolage-grotesque';
	import '@fontsource-variable/instrument-sans';
	import '../app.css';

	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { logOut } from '#lib/api.ts';
	import favicon from '#lib/assets/favicon.svg';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import Toaster from '#lib/ui/Toaster.svelte';
	import LiveScores from '#lib/LiveScores.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	type Link = { href: string; label: string; icon: IconName; count?: number };

	// Offers waiting on this manager's answer.
	const toAnswer = $derived(
		data.trades.filter(
			(t) => t.status === 'proposed' && t.parties.some((p) => p.franchise_id === data.me?.id && !p.accepted_at)
		).length
	);
	const usesWaivers = $derived(data.dynasty?.leagues.some((l) => l.settings.waivers.mode !== 'none') ?? false);
	const links = $derived<Link[]>([
		...(data.me ? [{ href: `/franchise/${data.me.slug}`, label: 'My team', icon: 'shield' } as Link] : []),
		{ href: '/league', label: 'League', icon: 'home' },
		{ href: '/scores', label: 'Scores', icon: 'scores' },
		{ href: '/players', label: 'Players', icon: 'players' },
		{ href: '/research', label: 'Research', icon: 'search' },
		...(usesWaivers ? [{ href: '/waivers', label: 'Waivers', icon: 'clock' } as Link] : []),
		{ href: '/drafts', label: 'Drafts', icon: 'draft' },
		{ href: '/trades', label: 'Trades', icon: 'trade', count: toAnswer },
		{ href: '/info', label: 'Rules', icon: 'info' },
		...(data.me?.is_commissioner ? [{ href: '/commissioner', label: 'Commissioner', icon: 'settings' } as Link] : [])
	]);

	const path = $derived(page.url.pathname);
	const current = (href: string) => path === href || path.startsWith(href + '/');

	// The landing page, the setup wizard and invite links stand alone.
	const bare = $derived(!data.dynasty || path === '/' || path.startsWith('/join/'));

	// A draft in progress is the most time-sensitive thing in the league.
	const liveDraft = $derived(data.drafts.find((d) => d.status === 'live' || d.status === 'paused'));

	async function leave() {
		await logOut();
		await invalidateAll();
		await goto('/');
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

{#if bare || !data.dynasty}
	<main class="bare">{@render children()}</main>
{:else}
	<div class="shell">
		<aside class="rail">
			<a class="brand" href="/league">
				<span class="mark" aria-hidden="true">CD</span>
				<span class="name">{data.dynasty.name}</span>
			</a>

			<nav aria-label="Main">
				{#each links as link (link.href)}
					<a href={link.href} aria-current={current(link.href) ? 'page' : undefined}>
						<Icon name={link.icon} />
						<span>{link.label}</span>
						{#if link.count}<span class="count">{link.count}</span>{/if}
					</a>
				{/each}
			</nav>

			{#if data.me}
				<div class="me">
					<Crest src={data.me.image_url} name={data.me.name} size={34} />
					<div class="who">
						<strong>{data.me.name}</strong>
						<span class="muted small-text">{data.me.manager_name}</span>
					</div>
					<button class="quiet small" aria-label="Sign out" title="Sign out" onclick={leave}><Icon name="out" size={16} /></button>
				</div>
			{:else}
				<p class="me"><a class="button primary" href="/">Sign in</a></p>
			{/if}
		</aside>

		<div class="page">
			{#if !path.startsWith('/draft/')}
				<LiveScores sports={data.dynasty.leagues.map((l) => l.competition)} rosters={data.myTeam?.rosters ?? []} signedIn={!!data.me} />
			{/if}
			{#if liveDraft && !path.startsWith('/draft/')}
				<a class="live" href="/draft/{liveDraft.id}">
					<span class="dot" aria-hidden="true"></span>
					<strong>{liveDraft.name}</strong>
					{liveDraft.status === 'live' ? 'is live' : 'is paused'}
					<span class="enter">Enter the draft room <Icon name="right" size={15} /></span>
				</a>
			{/if}
			<main>{@render children()}</main>
		</div>
	</div>
{/if}

<Toaster />

<style>
	.bare {
		max-width: 52rem;
		margin: 0 auto;
		padding: 2.5rem 1rem 4rem;
	}

	.shell {
		display: grid;
		grid-template-columns: 12.5rem minmax(0, 1fr);
		min-height: 100dvh;
	}

	/* ---- rail ---- */
	.rail {
		position: sticky;
		top: 0;
		height: 100dvh;
		display: flex;
		flex-direction: column;
		gap: 1.2rem;
		padding: 1rem 0.65rem;
		background: var(--surface);
		border-right: 1px solid var(--rule);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		padding: 0 0.4rem;
		color: var(--ink);
	}
	.brand:hover {
		text-decoration: none;
	}
	.mark {
		display: grid;
		place-items: center;
		width: 2.1rem;
		height: 2.1rem;
		border-radius: 8px;
		background: var(--ink);
		color: var(--surface);
		font: 800 0.9rem/1 var(--display);
		letter-spacing: -0.04em;
	}
	.name {
		font: 750 1.05rem/1.15 var(--display);
		letter-spacing: -0.015em;
	}

	nav {
		display: grid;
		gap: 0.15rem;
	}
	nav a {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding: 0.55rem 0.65rem;
		border-radius: var(--radius-small);
		color: var(--ink-soft);
		font-weight: 600;
	}
	nav a:hover {
		background: var(--surface-2);
		color: var(--ink);
		text-decoration: none;
	}
	nav a[aria-current='page'] {
		background: var(--brand-soft);
		color: var(--brand);
	}
	.count {
		margin-left: auto;
		min-width: 1.3rem;
		padding: 0.1rem 0.35rem;
		border-radius: 999px;
		background: var(--bad);
		color: var(--surface);
		font: 700 0.7rem/1.3 var(--mono);
		text-align: center;
	}

	.me {
		margin-top: auto;
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.7rem 0.4rem 0;
		border-top: 1px solid var(--rule);
	}
	.who {
		display: grid;
		min-width: 0;
		flex: 1;
		line-height: 1.25;
	}
	.who strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* ---- page ---- */
	.page {
		min-width: 0;
	}
	main {
		padding: 1.25rem 1.5rem 3rem;
	}
	.live {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.35rem 0.55rem;
		padding: 0.5rem 1.5rem;
		background: var(--gold-soft);
		color: var(--ink);
		border-bottom: 1px solid var(--rule);
	}
	.live:hover {
		text-decoration: none;
	}
	.dot {
		width: 0.55rem;
		height: 0.55rem;
		border-radius: 50%;
		background: var(--bad);
		animation: pulse 1.6s ease-in-out infinite;
	}
	.enter {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		margin-left: auto;
		font-weight: 650;
		color: var(--gold);
	}
	@keyframes pulse {
		50% {
			opacity: 0.35;
		}
	}

	/* ---- narrow screens: the rail becomes a tab bar ---- */
	@media (max-width: 860px) {
		.shell {
			display: block;
		}
		.rail {
			position: fixed;
			z-index: 20;
			inset: auto 0 0 0;
			height: auto;
			padding: 0.3rem 0.4rem calc(0.3rem + env(safe-area-inset-bottom));
			border-right: none;
			border-top: 1px solid var(--rule);
		}
		.brand,
		.me {
			display: none;
		}
		nav {
			grid-auto-flow: column;
			grid-auto-columns: minmax(4.2rem, 1fr);
			overflow-x: auto; /* more sections than fit: the bar scrolls */
			scrollbar-width: none;
		}
		nav a {
			flex-direction: column;
			gap: 0.15rem;
			padding: 0.4rem 0.2rem;
			font-size: 0.68rem;
			position: relative;
		}
		.count {
			position: absolute;
			top: 0.15rem;
			left: 50%;
			margin-left: 0.5rem;
		}
		main {
			padding: 1rem 0.85rem calc(5rem + env(safe-area-inset-bottom));
		}
		.live {
			padding: 0.6rem 1rem;
		}
	}
</style>
