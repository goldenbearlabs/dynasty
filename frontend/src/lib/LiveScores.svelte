<script lang="ts">
	import { onMount } from 'svelte';
	import type { GameDetail, LeagueRoster, RosterPlayer, Scoreboard } from '#lib/api.ts';
	import { LiveSocket } from '#lib/socket.svelte.ts';
	import ScoreCard from '#lib/ScoreCard.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { points } from '#lib/ui/time.ts';

	let { sports, rosters = [], signedIn = false }: {
		sports: string[]; rosters?: LeagueRoster[]; signedIn?: boolean;
	} = $props();
	let boards = $state<Record<string, Scoreboard>>({});
	let details = $state<Record<string, GameDetail>>({});
	let mineOnly = $state(false);
	let sport = $state('all');
	let status = $state('all');
	let compact = $state(true);
	let preferencesLoaded = $state(false);
	onMount(() => {
		try {
			const saved = JSON.parse(localStorage.getItem('crossover.scorebar.view') ?? '{}');
			mineOnly = saved.mineOnly === true;
			sport = sports.includes(saved.sport) ? saved.sport : 'all';
			status = ['live', 'scheduled', 'final'].includes(saved.status) ? saved.status : 'all';
			compact = saved.compact !== false;
		} catch { /* Use defaults when storage is unavailable. */ }
		preferencesLoaded = true;
	});
	$effect(() => {
		if (!preferencesLoaded) return;
		try { localStorage.setItem('crossover.scorebar.view', JSON.stringify({ mineOnly, sport, status, compact })); } catch { /* Keep session choices. */ }
	});
	$effect(() => { if (sport !== 'all' && !sports.includes(sport)) sport = 'all'; });
	function resetFilters() { mineOnly = false; sport = 'all'; status = 'all'; }

	let strip: HTMLDivElement;
	// Use stable keys: a refreshed roster must not reconnect every sport.
	const sportKey = $derived(sports.join(','));
	$effect(() => {
		const keys = sportKey.split(',').filter(Boolean);
		boards = {};
		const opened = keys.map((sport) => new LiveSocket<Scoreboard>(`/scores/${sport}/ws`, (board) => {
			boards = { ...boards, [sport]: board };
		}));
		return () => opened.forEach((socket) => socket.close());
	});

	const playersFor = (sport: string, away: string, home: string): RosterPlayer[] =>
		rosters.filter((r) => r.competition === sport).flatMap((r) => r.players)
			.filter((p) => p.team_abbrev && (p.team_abbrev === away || p.team_abbrev === home));
	const games = $derived(Object.values(boards).flatMap((b) => b.games).map((game) => ({
		game, players: playersFor(game.competition, game.away_abbrev, game.home_abbrev)
	})).sort((a, b) => {
		const rank = { live: 0, scheduled: 1, final: 2 };
		return rank[a.game.status] - rank[b.game.status]
			|| Number(b.players.length > 0) - Number(a.players.length > 0)
			|| a.game.starts_at.localeCompare(b.game.starts_at);
	}));
	const ownedKey = $derived(games.filter((g) => g.players.length).map((g) => g.game.id).sort().join(','));
	const detailKey = $derived(compact ? '' : ownedKey);
	// Open box scores only while player details are visible.
	$effect(() => {
		details = {};
		const opened = detailKey.split(',').filter(Boolean).map((id) =>
			new LiveSocket<GameDetail>(`/games/${id}/ws`, (game) => {
				details = { ...details, [id]: game };
			}));
		return () => opened.forEach((socket) => socket.close());
	});
	const showing = $derived(games.filter(({ game, players }) =>
		(!mineOnly || !signedIn || players.length > 0) &&
		(sport === 'all' || game.competition === sport) &&
		(status === 'all' || game.status === status)));
	const filtered = $derived((mineOnly && signedIn) || sport !== 'all' || status !== 'all');
	const loaded = $derived(sports.every((sport) => boards[sport]));
</script>

<section class="scorebar" class:compact aria-label="Today's games and your players">
	<div class="toolbar">
		<div class="controls">
			<select aria-label="Filter games by sport" bind:value={sport}>
				<option value="all">All sports</option>
				{#each sports as key (key)}<option value={key}>{key.toUpperCase()}</option>{/each}
			</select>
			<select aria-label="Filter games by status" bind:value={status}>
				<option value="all">All games</option>
				<option value="live">Live</option>
				<option value="scheduled">Upcoming</option>
				<option value="final">Final</option>
			</select>
			{#if signedIn}
				<button class="view-option" aria-pressed={mineOnly} onclick={() => mineOnly = !mineOnly}>My players</button>
			{/if}
			<button class="view-option" aria-pressed={!compact} title="Show player details beside each game" onclick={() => compact = !compact}>
				<Icon name="players" size={14} /> Details
			</button>
			<a href="/matchups" class="all-scores">Matchups <Icon name="right" size={14} /></a>
			{#if showing.length > 0}
				<div class="arrows">
					<button aria-label="Scroll scores left" onclick={() => strip?.scrollBy({ left: -320, behavior: 'smooth' })}><Icon name="left" size={14} /></button>
					<button aria-label="Scroll scores right" onclick={() => strip?.scrollBy({ left: 320, behavior: 'smooth' })}><Icon name="right" size={14} /></button>
				</div>
			{/if}
		</div>
	</div>
	<div class="strip" bind:this={strip} role="region" aria-label="Game scores, scroll for more">
		{#each showing as { game, players } (game.id)}
			<a class="fixture" class:owned={players.length > 0} href="/game/{game.id}" data-sport={game.competition}>
				<div class="match">
					<span class="sport">{game.competition}</span>
					<ScoreCard {game} {compact} />
					{#if compact && players.length}<span class="owned-label">{players.length} of yours</span>{/if}
				</div>
				{#if players.length && !compact}
					<div class="players">
						<span class="eyebrow">Your players</span>
						{#each players as player (player.player_id)}
							{@const line = details[game.id]?.lines.find((l) => l.player_id === player.player_id)}
							<div class="player">
								<Headshot name={player.full_name} src={player.headshot_url} size={30} />
								<div class="identity"><strong>{player.full_name}</strong><span>{player.positions.join('/')} · {player.list === 'reserve' ? 'Reserve' : 'Roster'}</span></div>
								<span class="fantasy" title="Fantasy points in this game">{line ? points(line.points) : '—'}<small>FP</small></span>
							</div>
						{/each}
					</div>
				{/if}
			</a>
		{:else}
			<div class="empty muted small-text">
				<span>{!loaded ? 'Loading today’s games…' : filtered ? 'No games match your filters.' : 'No games scheduled today.'}</span>
				{#if loaded && filtered}<button class="quiet small" onclick={resetFilters}>Clear filters</button>{/if}
			</div>
		{/each}
	</div>
</section>

<style>
	.scorebar { background: var(--surface); border-bottom: 1px solid var(--rule); min-width: 0; }
	.toolbar { display: flex; align-items: center; justify-content: flex-end; gap: 0.5rem; padding: 0.35rem 1.25rem; flex-wrap: wrap; border-bottom: 1px solid var(--rule); }
	.controls { display: flex; align-items: center; gap: 0.4rem; }
	.controls { flex-wrap: wrap; }
	.controls select, .view-option { min-height: 1.8rem; padding: 0.2rem 0.45rem; font-size: 0.72rem; }
	.view-option { display: inline-flex; align-items: center; gap: 0.25rem; color: var(--ink-soft); }
	.view-option[aria-pressed='true'] { color: var(--brand); background: var(--brand-soft); border-color: var(--brand); }
	.compact .match { width: 8rem; padding: 0.35rem 0.65rem; position: relative; }
	.compact .sport { margin-bottom: 0.1rem; font-size: 0.55rem; }
	.owned-label { position: absolute; top: 0.35rem; right: 0.5rem; color: var(--sport); font-size: 0.55rem; }
	.all-scores { display: inline-flex; align-items: center; gap: 0.15rem; font-size: 0.75rem; white-space: nowrap; }
	.arrows { display: flex; gap: 0.2rem; }
	.arrows button { padding: 0.2rem; border-color: var(--rule); }
	.strip { display: flex; overflow-x: auto; overscroll-behavior-x: contain; scrollbar-width: thin; }
	.fixture { flex: none; display: flex; align-items: stretch; color: var(--ink); border-right: 1px solid var(--rule); border-top: 2px solid transparent; }
	.fixture:hover { background: var(--surface-2); text-decoration: none; }
	.fixture.owned { border-top-color: var(--sport); }
	.match { width: 10rem; padding: 0.5rem 0.85rem 0.65rem; }
	.sport { display: block; margin-bottom: 0.3rem; font: 700 0.6rem var(--mono); text-transform: uppercase; color: var(--sport); }
	.players { display: grid; align-content: center; gap: 0.35rem; padding: 0.6rem 0.85rem 0.6rem 0; min-width: 13rem; }
	.players .eyebrow { color: var(--sport); font-size: 0.58rem; }
	.player { display: flex; align-items: center; gap: 0.45rem; }
	.identity { display: grid; line-height: 1.3; }
	.identity strong { font-size: 0.75rem; }
	.identity span { color: var(--ink-soft); font-size: 0.62rem; }
	.fantasy { display: grid; margin-left: auto; padding-left: 0.7rem; text-align: right; font: 750 0.95rem var(--display); font-variant-numeric: tabular-nums; }
	.fantasy small { font: 500 0.55rem var(--mono); color: var(--ink-soft); }
	.empty { padding: 0.55rem 1.25rem; display: flex; align-items: center; gap: 0.5rem; }
	@media (max-width: 640px) {
		.toolbar { padding: 0.4rem 0.85rem; }
		.arrows { display: none; }
		.controls { gap: 0.35rem; }
		.controls { width: 100%; }
		.all-scores { display: none; }
		.compact .match { width: 7.5rem; }
		/* A phone keeps the strip short: tighter cards, smaller faces, two of your players a game at most. */
		.match { width: 7.6rem; padding: 0.4rem 0.6rem 0.45rem; }
		.sport { margin-bottom: 0.15rem; }
		.players { min-width: 10rem; gap: 0.2rem; padding: 0.4rem 0.6rem 0.4rem 0; }
		.player:nth-of-type(n + 3) { display: none; }
		.player :global(.headshot) { --size: 22px; }
		.match :global(.game) { gap: 0.25rem; }
		.match :global(.side img), .match :global(.side .logo) { width: 1.3rem; height: 1.3rem; }
		.identity strong { font-size: 0.7rem; }
		.fantasy { font-size: 0.85rem; }
	}
</style>
