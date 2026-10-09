<script lang="ts">
	// A sport's games for a day. Today's board is pushed by the server as
	// the games are played; other days are simply fetched.
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getScores, type Scoreboard } from '#lib/api.ts';
	import ScoreCard from '#lib/ScoreCard.svelte';
	import { LiveSocket } from '#lib/socket.svelte.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { dayLabel, shiftDay } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// The sports this dynasty plays, or every sport before it has leagues.
	const sports = $derived(
		data.dynasty?.leagues.length
			? data.competitions.filter((c) => data.dynasty!.leagues.some((l) => l.competition === c.key))
			: data.competitions
	);
	const tabs = $derived(sports.map((c) => ({ value: c.key, label: c.name, sport: c.key })));
	let sport = $state(untrack(() => page.url.searchParams.get('sport') ?? sports[0]?.key ?? ''));
	const day = $derived(page.url.searchParams.get('day')); // null means today, live

	let board = $state<Scoreboard>();
	let socket = $state<LiveSocket<Scoreboard>>();

	$effect(() => {
		const [showing, on] = [sport, day];
		if (!showing) return;
		board = undefined;
		if (on) {
			getScores(showing, on).then((b) => (board = b), toast.error);
			return;
		}
		const opened = new LiveSocket<Scoreboard>(`/scores/${showing}/ws`, (b) => (board = b));
		socket = opened;
		return () => opened.close();
	});

	function go(to: string | null) {
		const query = new URLSearchParams(page.url.search);
		if (to) query.set('day', to);
		else query.delete('day');
		goto(`?${query}`);
	}
</script>

<svelte:head><title>Scores</title></svelte:head>

<div class="stack" data-sport={sport}>
	<header class="spread">
		<h1>Scores</h1>
		{#if board}
			<nav class="row days" aria-label="Day">
				<button aria-label="Earlier" onclick={() => go(shiftDay(board!.day, -1))}><Icon name="left" size={16} /></button>
				<strong class="when">{dayLabel(board.day)}</strong>
				<button aria-label="Later" onclick={() => go(shiftDay(board!.day, 1))}><Icon name="right" size={16} /></button>
				<button class="quiet" disabled={!day} onclick={() => go(null)}>Today</button>
			</nav>
		{/if}
	</header>
	<Tabs {tabs} bind:value={sport} label="Sport" />

	{#if !day}
		<p class="muted small-text row">
			<span class="pill {socket?.connected ? 'good' : 'gold'}">{socket?.connected ? 'Live' : 'Connecting…'}</span>
			Scores update on their own while games are being played.
		</p>
	{/if}

	{#if !board}
		<p class="muted">Loading…</p>
	{:else if board.games.length === 0}
		<Empty icon="scores" title="No games">Nothing is scheduled for {dayLabel(board.day)}.</Empty>
	{:else}
		<div class="games">
			{#each board.games as game (game.id)}
				<a class="fixture" href="/game/{game.id}"><ScoreCard {game} /></a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.days {
		gap: 0.4rem;
	}
	.days button {
		padding: 0.45rem 0.6rem;
	}
	.when {
		min-width: 7rem;
		text-align: center;
	}
	.games {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(13rem, 1fr));
		gap: 0;
		background: var(--surface);
		border-top: 1px solid var(--rule);
	}
	a.fixture {
		color: var(--ink);
		padding: 0.85rem 1rem;
		border-bottom: 1px solid var(--rule);
	}
	a.fixture:hover {
		text-decoration: none;
		background: var(--surface-2);
	}
</style>
