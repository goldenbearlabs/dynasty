<script lang="ts">
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
	let connections = $state<LiveSocket<Scoreboard>[]>([]);
	let mineOnly = $state(false);
	let strip: HTMLDivElement;
	// Use stable keys: a refreshed roster must not reconnect every sport.
	const sportKey = $derived(sports.join(','));
	$effect(() => {
		const keys = sportKey.split(',').filter(Boolean);
		boards = {};
		const opened = keys.map((sport) => new LiveSocket<Scoreboard>(`/scores/${sport}/ws`, (board) => {
			boards = { ...boards, [sport]: board };
		}));
		connections = opened;
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
	// Only games involving our players need the additional box-score feed.
	$effect(() => {
		details = {};
		const opened = ownedKey.split(',').filter(Boolean).map((id) =>
			new LiveSocket<GameDetail>(`/games/${id}/ws`, (game) => {
				details = { ...details, [id]: game };
			}));
		return () => opened.forEach((socket) => socket.close());
	});
	const showing = $derived(mineOnly && signedIn ? games.filter((g) => g.players.length) : games);
	const connected = $derived(connections.length > 0 && connections.every((s) => s.connected));
	const loaded = $derived(sports.every((sport) => boards[sport]));
	const liveCount = $derived(games.filter((g) => g.game.status === 'live').length);
</script>

<section class="scorebar" aria-label="Today's games and your players">
	<div class="toolbar">
		<div class="label"><span class="signal" class:connected aria-hidden="true"></span>
			<strong>Game day</strong><span class="status">{connected || !sports.length ? (liveCount ? `${liveCount} live` : 'Today') : loaded ? 'Reconnecting…' : 'Connecting…'}</span>
		</div>
		<div class="controls">
			{#if signedIn}
				<div class="filters" aria-label="Games to show">
					<button aria-pressed={!mineOnly} onclick={() => (mineOnly = false)}>All games</button>
					<button aria-pressed={mineOnly} onclick={() => (mineOnly = true)}>My players</button>
				</div>
			{/if}
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
					<ScoreCard {game} />
				</div>
				{#if players.length}
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
			<p class="empty muted small-text">{!loaded ? 'Loading today’s games…' : mineOnly ? 'None of your players have a game today. See all games or set your lineup.' : 'No games scheduled today.'}</p>
		{/each}
	</div>
</section>

<style>
	.scorebar { background: var(--surface); border-bottom: 1px solid var(--rule); min-width: 0; }
	.toolbar { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; padding: 0.55rem 1.25rem; border-bottom: 1px solid var(--rule); }
	.label, .controls { display: flex; align-items: center; gap: 0.6rem; }
	.label strong { font: 750 0.9rem var(--display); white-space: nowrap; }
	.status { color: var(--ink-soft); font-size: 0.72rem; }
	.signal { width: 6px; height: 6px; border-radius: 50%; background: var(--gold); }
	.signal.connected { background: var(--good); }
	.filters { display: flex; gap: 0.1rem; }
	.filters button { padding: 0.25rem 0.5rem; font-size: 0.72rem; border: 0; border-radius: 4px; color: var(--ink-soft); }
	.filters button[aria-pressed='true'] { color: var(--brand); background: var(--brand-soft); }
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
	.empty { padding: 0.85rem 1.25rem; }
	@media (max-width: 640px) {
		.toolbar { padding-inline: 0.85rem; }
		.status, .arrows { display: none; }
		.controls { gap: 0.35rem; }
		.filters button { padding-inline: 0.35rem; }
	}
</style>
