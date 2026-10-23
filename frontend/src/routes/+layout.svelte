<script lang="ts">
	// The app shell: a rail of navigation beside the page on wide screens,
	// and a tab bar under it on narrow ones.
	import '@fontsource-variable/bricolage-grotesque';
	import '@fontsource-variable/instrument-sans';
	import '../app.css';

	import { afterNavigate, goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { rememberPlayerOrigin, restorePlayerOrigin } from '#lib/playerNavigation.svelte.ts';
	import { onMount, untrack } from 'svelte';
	import { inbox } from '#lib/inbox.svelte.ts';
	import { watchlist } from '#lib/watchlist.svelte.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import { logOut } from '#lib/api.ts';
	import favicon from '#lib/assets/favicon.svg';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import Toaster from '#lib/ui/Toaster.svelte';
	import NotificationCenter from '#lib/NotificationCenter.svelte';
	import LiveScores from '#lib/LiveScores.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	afterNavigate(({ from, to, type }) => {
		if (type === 'enter' && to) restorePlayerOrigin(to.url);
		else if (type !== 'popstate') rememberPlayerOrigin(from?.url ?? null, to?.url ?? null);
	});
	$effect(() => { const id = data.me?.id || ''; untrack(() => watchlist.setAccount(id)); });
	$effect(() => { const id = data.me?.id || ''; untrack(() => inbox.setAccount(id)); });
	$effect(() => { const id = data.me?.id || ''; untrack(() => tradeMarket.setAccount(id)); });
	const path = $derived(page.url.pathname);
	const viewingNewLeague = $derived(path.startsWith('/leagues/') && page.params.id !== data.dynasty?.id);

	let sidebarCollapsed = $state(false);
	let wideScreen = $state(false);
	const collapsed = $derived(wideScreen && sidebarCollapsed);
	onMount(() => {
		const refreshInbox = () => { if (document.visibilityState === 'visible') void inbox.refresh(); };
		const inboxTimer = setInterval(refreshInbox, 20000);
		window.addEventListener('focus', refreshInbox);
		document.addEventListener('visibilitychange', refreshInbox);
		const media = window.matchMedia('(min-width: 861px)');
		const resize = () => { wideScreen = media.matches; };
		resize();
		media.addEventListener('change', resize);
		try { sidebarCollapsed = localStorage.getItem('crossover.sidebar.collapsed') === 'true'; } catch { /* Storage may be unavailable. */ }
		return () => { media.removeEventListener('change', resize); clearInterval(inboxTimer); window.removeEventListener('focus', refreshInbox); document.removeEventListener('visibilitychange', refreshInbox); };
	});
	function toggleSidebar() {
		sidebarCollapsed = !sidebarCollapsed;
		try { localStorage.setItem('crossover.sidebar.collapsed', String(sidebarCollapsed)); } catch { /* Keep the session preference. */ }
	}

	type Link = { href: string; label: string; icon: IconName; count?: number };

	// Offers waiting on this manager's answer.
	const toAnswer = $derived(
		data.trades.filter(
			(t) => t.status === 'proposed' && t.parties.some((p) => p.franchise_id === data.me?.id && !p.accepted_at)
		).length
	);
	// Every league has waivers of a kind: at the least, released draft picks spend a day on them.
	const usesWaivers = $derived((data.dynasty?.leagues.length ?? 0) > 0);
	const links = $derived.by((): Link[] => {
		const accountLinks: Link[] = [{ href: '/leagues', label: 'My Leagues', icon: 'home' }];
		if (!data.me?.id || viewingNewLeague) {
			const sectionRoot = viewingNewLeague ? path : '/leagues';
			return [...accountLinks,
			{ href: `${sectionRoot}?section=team`, label: 'My team', icon: 'shield' },
			{ href: `${sectionRoot}?section=league`, label: 'League', icon: 'home' },
			{ href: `${sectionRoot}?section=matchups`, label: 'Matchups', icon: 'scores' },
			{ href: `${sectionRoot}?section=players`, label: 'Players', icon: 'players' },
			{ href: `${sectionRoot}?section=watchlist`, label: 'Watchlist', icon: 'star' },
			{ href: `${sectionRoot}?section=research`, label: 'Research', icon: 'search' },
			{ href: `${sectionRoot}?section=waivers`, label: 'Waivers', icon: 'clock' },
			{ href: `${sectionRoot}?section=drafts`, label: 'Drafts', icon: 'draft' },
			{ href: `${sectionRoot}?section=trades`, label: 'Trades', icon: 'trade' },
			{ href: `${sectionRoot}?section=rules`, label: 'Rules', icon: 'info' }
		];
		}
		return [...accountLinks,
		{ href: `/franchise/${data.me.slug}`, label: 'My team', icon: 'shield' },
		{ href: '/league', label: 'League', icon: 'home' },
		{ href: '/matchups', label: 'Matchups', icon: 'scores' },
		{ href: '/players', label: 'Players', icon: 'players' },
		{ href: '/watchlist', label: 'Watchlist', icon: 'star' },
		{ href: '/research', label: 'Research', icon: 'search' },
		...(usesWaivers ? [{ href: '/waivers', label: 'Waivers', icon: 'clock' } as Link] : []),
		{ href: '/drafts', label: 'Drafts', icon: 'draft' },
		{ href: '/trades', label: 'Trades', icon: 'trade', count: toAnswer },
		{ href: '/info', label: 'Rules', icon: 'info' },
		...(data.me.is_commissioner ? [{ href: '/commissioner', label: 'Commissioner', icon: 'settings' } as Link] : [])
		];
	});

	const current = (href: string) => {
		if (href === '/leagues' && path.startsWith('/leagues/')) return !page.url.search;
		if (href.startsWith('/leagues')) return path + page.url.search === href;
		return path === href || path.startsWith(href + '/');
	};

	// On a phone the tab bar holds the first few sections; the rest, and who
	// is signed in, open from "More".
	const TABS = 4;
	const more = $derived(links.slice(TABS));
	let moreOpen = $state(false);
	afterNavigate(() => { moreOpen = false; void inbox.refresh(); });

	// The landing page, the setup wizard and invite links stand alone.
	const bare = $derived(path === '/' || path === '/register' || path === '/verify' || path === '/reset-password' || path.startsWith('/join/') || path === '/setup');

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

{#if bare}
	<main class="bare">{@render children()}</main>
{:else}
	<div class="shell" class:collapsed>
		<button class="sidebar-toggle" aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
			title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} aria-expanded={!collapsed}
			aria-controls="sidebar-navigation" onclick={toggleSidebar}>
			<Icon name={collapsed ? 'right' : 'left'} size={15} />
		</button>
		<aside class="rail" id="sidebar-navigation" inert={collapsed}>
			<a class="brand" href="/leagues">
				<span class="mark" aria-hidden="true">MF</span>
				<span class="name">{data.dynasty?.name ?? 'Mosaic Fantasy'}</span>
			</a>

			<nav aria-label="Main">
				{#each links as link, i (link.href)}
					<a href={link.href} class:extra={i >= TABS} aria-current={current(link.href) ? 'page' : undefined}>
						<Icon name={link.icon} />
						<span>{link.label}</span>
						{#if link.count}<span class="count">{link.count}</span>{/if}
					</a>
				{/each}
				{#if more.length}
					<button
						class="more"
						class:on={moreOpen || more.some((l) => current(l.href))}
						aria-expanded={moreOpen}
						aria-controls="more-sheet"
						onclick={() => (moreOpen = !moreOpen)}
					>
						<Icon name="more" />
						<span>More</span>
						{#if more.some((l) => l.count)}<span class="count">{more.reduce((n, l) => n + (l.count ?? 0), 0)}</span>{/if}
					</button>
				{/if}
			</nav>

			{#if data.me?.id}
				<div class="me">
					<Crest src={data.me.image_url} name={data.me.name} size={34} />
					<div class="who">
						<strong>{data.me.name}</strong>
						<span class="muted small-text">{data.me.manager_name}</span>
					</div>
					<button class="quiet small" aria-label="Sign out" title="Sign out" onclick={leave}><Icon name="out" size={16} /></button>
				</div>
			{:else if data.me}
				<div class="me">
					<div class="who"><strong>{data.me.email}</strong><span class="muted small-text">My account</span></div>
					<button class="quiet small" aria-label="Sign out" title="Sign out" onclick={leave}><Icon name="out" size={16} /></button>
				</div>
			{/if}
		</aside>

		{#if moreOpen}
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions: a backdrop; Escape and the More button close it too -->
			<div class="backdrop" onclick={() => (moreOpen = false)}></div>
			<div class="sheet" id="more-sheet" role="dialog" aria-label="More sections">
				<nav aria-label="More">
					{#each more as link (link.href)}
						<a href={link.href} aria-current={current(link.href) ? 'page' : undefined}>
							<Icon name={link.icon} />
							<span>{link.label}</span>
							{#if link.count}<span class="count">{link.count}</span>{/if}
						</a>
					{/each}
				</nav>
				{#if data.me?.id}
					<div class="sheet-me">
						<Crest src={data.me.image_url} name={data.me.name} size={34} />
						<div class="who">
							<strong>{data.me.name}</strong>
							<span class="muted small-text">{data.me.manager_name}</span>
						</div>
						<button class="small" onclick={leave}><Icon name="out" size={16} /> Sign out</button>
					</div>
				{:else if data.me}
					<div class="sheet-me">
						<div class="who"><strong>{data.me.email}</strong><span class="muted small-text">My account</span></div>
						<button class="small" onclick={leave}><Icon name="out" size={16} /> Sign out</button>
					</div>
				{:else}
					<a class="button primary" href="/">Sign in</a>
				{/if}
			</div>
		{/if}

		<div class="page">
			{#if data.dynasty && !viewingNewLeague && !path.startsWith('/draft/')}
				<LiveScores sports={data.dynasty.leagues.map((l) => l.competition)} rosters={data.myTeam?.rosters ?? []} signedIn={!!data.me}>
					{#snippet actions()}{#if data.me?.id}<NotificationCenter accountId={data.me.id} />{/if}{/snippet}
				</LiveScores>
			{/if}
			{#if liveDraft && !viewingNewLeague && !path.startsWith('/draft/')}
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

<svelte:window onkeydown={(e) => e.key === 'Escape' && (moreOpen = false)} />

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
		transition: grid-template-columns 180ms ease;
	}

	.sidebar-toggle { display: none; }
	@media (min-width: 861px) {
		.shell.collapsed { grid-template-columns: 0 minmax(0, 1fr); }
		.rail { width: 12.5rem; transition: transform 180ms ease, visibility 180ms; }
		.collapsed .rail { transform: translateX(-100%); visibility: hidden; }
		.sidebar-toggle {
			display: grid;
			place-items: center;
			position: fixed;
			left: 12.5rem;
			top: 50dvh;
			z-index: 15;
			width: 1.35rem;
			height: 3.25rem;
			padding: 0;
			border-radius: 0 var(--radius-small) var(--radius-small) 0;
			border-left: 0;
			background: var(--surface);
			color: var(--ink-soft);
			transition: left 180ms ease;
		}
		.collapsed .sidebar-toggle { left: 0; }
		.collapsed main { padding-left: 1.75rem; }
	}
	@media (prefers-reduced-motion: reduce) {
		.shell, .rail, .sidebar-toggle { transition: none; }
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

	/* The More button and its sheet exist only for the tab bar. */
	.more,
	.backdrop,
	.sheet {
		display: none;
	}

	/* ---- narrow screens: the rail becomes a tab bar ---- */
	@media (max-width: 860px) {
		nav a.extra {
			display: none;
		}
		.more {
			display: flex;
			flex-direction: column;
			align-items: center;
			gap: 0.15rem;
			position: relative;
			padding: 0.4rem 0.2rem;
			border: 0;
			border-radius: var(--radius-small);
			background: none;
			color: var(--ink-soft);
			font: 600 0.68rem var(--body);
		}
		.more.on {
			background: var(--brand-soft);
			color: var(--brand);
		}
		.backdrop {
			display: block;
			position: fixed;
			inset: 0;
			z-index: 18;
			background: rgb(0 0 0 / 0.45);
		}
		.sheet {
			display: grid;
			gap: 0.8rem;
			position: fixed;
			z-index: 19;
			inset: auto 0 calc(3.9rem + env(safe-area-inset-bottom)) 0;
			max-height: 70dvh;
			overflow-y: auto;
			padding: 0.8rem 0.8rem 1rem;
			background: var(--surface);
			border-top: 1px solid var(--rule-strong);
			border-radius: var(--radius) var(--radius) 0 0;
			box-shadow: 0 -8px 24px rgb(0 0 0 / 0.25);
		}
		.sheet nav {
			grid-auto-flow: row;
			grid-template-columns: repeat(2, minmax(0, 1fr));
			overflow: visible;
		}
		.sheet nav a {
			flex-direction: row;
			gap: 0.6rem;
			padding: 0.7rem 0.75rem;
			font-size: 0.9rem;
		}
		.sheet .count {
			position: static;
			margin-left: auto;
		}
		.sheet-me {
			display: flex;
			align-items: center;
			gap: 0.6rem;
			padding-top: 0.8rem;
			border-top: 1px solid var(--rule);
		}
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
			grid-auto-columns: minmax(0, 1fr);
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
