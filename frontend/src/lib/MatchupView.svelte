<script lang="ts">
	import { getLineup, getMatchup, type Franchise, type League, type Lineup, type MatchupDetail, type TeamIdentity } from '#lib/api.ts';
	import { matchupRows } from '#lib/matchupView.ts';
	import { statColumns, columnValue, statValue } from '#lib/statLine.ts';
	import { teamIdentity } from '#lib/identity.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel, points, shiftDay, sportsToday, timeOfDay } from '#lib/ui/time.ts';

	let { id, league, franchises, identities = [] }: {
		id: string; league: League; franchises: Franchise[]; identities?: TeamIdentity[];
	} = $props();
	let matchup = $state<MatchupDetail>();
	let lineups = $state<(Lineup | undefined)[]>([]);
	let day = $state('');
	let wholePeriod = $state(false);
	let showOther = $state(false);
	let loading = $state(true);
	let loadingLineups = $state(false);
	let error = $state('');
	let lineupError = $state('');
	let revision = $state(0);
	let attempt = $state(0);
	const weekly = $derived(league.settings.lineup.period === 'week');
	const pickedGames = $derived(league.settings.lineup.slots.some(s => (s.games_per_week ?? 0) > 0));

	$effect(() => {
		const key = id;
		void attempt;
		let active = true;
		loading = true;
		getMatchup(key).then(m => {
			if (!active) return;
			matchup = m;
			lineups = [];
			const today = sportsToday();
			day = today < m.period.starts_on ? m.period.starts_on : today > m.period.ends_on ? m.period.ends_on : today;
			wholePeriod = false;
			error = '';
			loading = false;
		}, e => { if (active) { error = String(e); loading = false; } });
		return () => { active = false; };
	});
	$effect(() => {
		const current = matchup;
		const selected = day;
		const leagueId = league.id;
		void revision;
		if (!current || current.id !== id || !selected) return;
		let active = true;
		loadingLineups = true;
		Promise.all([current.home_franchise_id, current.away_franchise_id].map(f =>
			f ? getLineup(leagueId, f, selected) : undefined)).then(found => {
			if (active) { lineups = found; loadingLineups = false; lineupError = ''; }
		}, e => { if (active) { loadingLineups = false; lineupError = String(e); } });
		return () => { active = false; };
	});
	onScoresChange(() => [league.competition], async () => {
		const key = id;
		try { const found = await getMatchup(key); if (key === id) { matchup = found; revision++; } }
		catch { /* Keep the last score until the connection recovers. */ }
	});
	const sides = $derived(matchup ? [
		{ id: matchup.home_franchise_id, total: matchup.home_points, scored: matchup.home_players, lineup: lineups[0] },
		{ id: matchup.away_franchise_id, total: matchup.away_points, scored: matchup.away_players, lineup: lineups[1] }
	].map(side => ({ ...side,
		identity: side.id ? teamIdentity(franchises.find(f => f.id === side.id), identities, league.id) : { name: 'Bye', image_url: '' },
		rows: matchupRows(side.scored, side.lineup, matchup!.period, wholePeriod, day, weekly)
	})) : []);
	const windowStart = $derived(weekly ? lineups.find(Boolean)?.day ?? day : day);
	const windowEnd = $derived(weekly ? lineups.find(Boolean)?.last_day ?? day : day);
	const columns = (positions: string[]) => league.competition === 'nfl' ? [
		['pass_yds','PASS YD','Passing yards'], ['pass_td','PASS TD','Passing touchdowns'], ['interceptions','INT','Interceptions'],
		['rush_yds','RUSH YD','Rushing yards'], ['rush_td','RUSH TD','Rushing touchdowns'],
		['receptions','REC','Receptions'], ['rec_yds','REC YD','Receiving yards'], ['rec_td','REC TD','Receiving touchdowns'],
		['fumbles_lost','FUM','Fumbles lost']
	].map(([key,label,title]) => ({key,keys:[key],label,title})) : statColumns(league.competition,positions);
	const dates = $derived.by(() => {
		if (!matchup || weekly) return [];
		const days: string[] = [];
		for (let d = matchup.period.starts_on; d <= matchup.period.ends_on; d = shiftDay(d, 1)) days.push(d);
		return days;
	});
	function step(direction: number) {
		if (!matchup) return;
		const next = shiftDay(direction < 0 ? windowStart : windowEnd, direction);
		day = next < matchup.period.starts_on ? matchup.period.starts_on : next > matchup.period.ends_on ? matchup.period.ends_on : next;
		wholePeriod = false;
	}
</script>

{#if loading}<p class="muted" role="status">Loading matchup…</p>
{:else if error}<p role="alert">{error}</p><button class="small" onclick={() => attempt++}>Try again</button>
{:else if matchup}
	<div class="stack matchup-view" data-sport={league.competition}>
		<section class="scoreboard card" aria-label="Matchup totals">
			{#each sides as side, index (index)}
				<div class="score-team" class:leader={side.total > sides[1-index].total}>
					<Crest src={side.identity.image_url} name={side.identity.name} size={40} />
					<strong>{side.identity.name}</strong>
					<span class="score">{side.id ? points(side.total) : '—'}</span>
					<span class="muted small-text">{matchup.period.double_games ? 'Double matchup total' : 'Matchup total'}</span>
				</div>
				{#if index === 0}<div class="score-state"><span class="pill" class:good={!matchup.final && sportsToday() >= matchup.period.starts_on}>{matchup.final ? 'Final' : sportsToday() < matchup.period.starts_on ? 'Upcoming' : 'In progress'}</span><span class="muted small-text">{dayLabel(matchup.period.starts_on)}<br />– {dayLabel(matchup.period.ends_on)}</span>{#if matchup.period.is_playoff}<span class="small-text">{matchup.bracket === 'consolation' ? 'Consolation' : 'Championship'}</span>{/if}</div>{/if}
			{/each}
		</section>

		<div class="spread view-controls">
			<div class="row modes" aria-label="Stats view">
				<button aria-pressed={!wholePeriod} onclick={() => wholePeriod = false}>{weekly ? pickedGames ? 'Selected games' : 'Weekly lineup' : 'Daily stats'}</button>
				<button aria-pressed={wholePeriod} onclick={() => wholePeriod = true}>Matchup totals</button>
			</div>
			<label class="row small-text"><input type="checkbox" bind:checked={showOther} /> Show uncounted games</label>
		</div>

		{#if !wholePeriod}
			<nav class="row date-controls" aria-label={weekly ? 'Lineup week' : 'Scoring day'}>
				<button class="small" aria-label={weekly ? 'Previous lineup week' : 'Previous scoring day'} disabled={loadingLineups || windowStart <= matchup.period.starts_on} onclick={() => step(-1)}><Icon name="left" size={16} /></button>
				{#if weekly}
					<strong>{dayLabel(windowStart < matchup.period.starts_on ? matchup.period.starts_on : windowStart)} – {dayLabel(windowEnd > matchup.period.ends_on ? matchup.period.ends_on : windowEnd)}</strong>
				{:else}
					<select aria-label="Scoring day" bind:value={day}>{#each dates as d (d)}<option value={d}>{dayLabel(d)}{d === sportsToday() ? ' · Today' : ''}</option>{/each}</select>
				{/if}
				<button class="small" aria-label={weekly ? 'Next lineup week' : 'Next scoring day'} disabled={loadingLineups || windowEnd >= matchup.period.ends_on} onclick={() => step(1)}><Icon name="right" size={16} /></button>
			</nav>
		{/if}
		{#if lineupError}<p role="alert">{lineupError}</p><button class="small" onclick={() => revision++}>Retry lineups</button>{/if}
		{#if loadingLineups}<p class="muted small-text" role="status">Loading lineups…</p>{/if}
		<div class="teams" hidden={loadingLineups} aria-busy={loadingLineups}>
			{#each sides as side, index (index)}
				<section class="card flush team-panel">
					<header class="spread"><strong>{side.identity.name}</strong><span class="scope-score">{side.id ? points(side.rows.reduce((n,p) => n+p.points,0)) : '—'} <small>{wholePeriod ? 'total FP' : weekly ? side.lineup?.doubled ? 'matchup FP' : 'week FP' : 'day FP'}</small></span></header>
					{#if side.lineup?.starts && !wholePeriod}<p class="cap small-text">Pitcher starts: {side.lineup.starts.made.filter(s => s.counts).length} / {side.lineup.starts.limit}{side.lineup.doubled ? ' · shared across double matchup' : ' this week'}</p>{/if}
					{#if !side.id}<p class="empty muted">No opponent this round.</p>
					{:else if side.rows.length === 0}<p class="empty muted">No starters or scored contributions for this view.</p>
					{:else}
						{#each side.rows as player (player.id)}
							<article class="player-row">
								<div class="player-heading">
									<Headshot name={player.name} src={player.headshot} size={30} />
									<div class="identity"><a href="/player/{player.id}">{player.name}</a><span class="muted small-text">{player.slot} · {player.positions.join('/')}{player.earlier ? ' · Started earlier' : ''}</span></div>
									<strong class="player-points">{points(player.points)}<small>FP</small></strong>
								</div>
								{#if player.selection && pickedGames}<p class="selection small-text">{player.selection}</p>{/if}
								<div class="player-games">
									{#each player.games.filter(g => showOther || g.counted || g.label.startsWith('Selected') || g.label === 'Auto candidate') as game, gameIndex (gameIndex)}
										<div class="game-line" class:uncounted={!game.counted && game.status !== 'scheduled'}>
											<div class="game-heading"><span>{#if game.gameId}<a class="game-link" href="/game/{game.gameId}">{dayLabel(game.day)} · {game.opponent}</a>{:else}{dayLabel(game.day)} · {game.opponent}{/if}</span><span class="game-status" class:counted={game.counted}>{game.label}{game.status === 'live' ? ' · Live' : game.status === 'final' ? ' · Final' : ` · ${timeOfDay(game.starts_at)}`}</span></div>
											{#if Object.keys(game.stats ?? {}).length}
												<div class="stat-line">{#each columns(player.positions) as col (col.key)}<span title={col.title}><strong>{statValue(columnValue(game.stats,col))}</strong> {col.label}</span>{/each}<span class="game-points">{points(game.points)} FP</span></div>
											{:else}<span class="muted small-text">{game.status === 'scheduled' ? 'Not played yet' : 'Stats not available yet'}</span>{/if}
										</div>
									{:else}<p class="muted small-text">{player.games.length ? 'No counted or selected games in this view.' : 'No game in this view.'}</p>{/each}
								</div>
							</article>
						{/each}
					{/if}
				</section>
			{/each}
		</div>
		<p class="muted small-text">Only counted starter games contribute to the score.{#if pickedGames} Selected games show your manager’s picks; Auto uses the last games with stats and can change as the week is played.{/if} Matchup totals always cover the full dates above.</p>
	</div>
{/if}

<style>
	.matchup-view { min-width: 0; grid-template-columns: minmax(0,1fr); }
	.scoreboard { display: grid; grid-template-columns: minmax(0,1fr) auto minmax(0,1fr); gap: 1rem; align-items: center; border-top: 3px solid var(--sport); }
	.score-team { display: grid; justify-items: center; text-align: center; gap: .35rem; min-width: 0; }
	.score-team strong { overflow-wrap: anywhere; }
	.score { font: 800 2.5rem/1.1 var(--display); font-variant-numeric: tabular-nums; }
	.leader .score { color: var(--sport); }
	.score-state { display: grid; gap: .5rem; text-align: center; justify-items: center; }
	.modes { gap: .3rem; }
	.modes button { font-size: .8rem; padding: .4rem .65rem; }
	.modes button[aria-pressed='true'] { background: var(--brand-soft); color: var(--brand); border-color: var(--brand); }
	.date-controls { gap: .5rem; }
	.date-controls select { font-size: .85rem; }
	.teams { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 1rem; }
	.teams[hidden] { display: none; }
	.team-panel header { padding: .75rem 1rem; border-bottom: 1px solid var(--rule); background: var(--surface-2); }
	.scope-score { font-weight: 750; font-variant-numeric: tabular-nums; }
	.scope-score small { color: var(--ink-soft); font-weight: 500; font-size: .7rem; }
	.player-row { padding: .8rem 1rem; border-bottom: 1px solid var(--rule); }
	.player-row:last-child { border-bottom: 0; }
	.player-heading { display: flex; align-items: center; gap: .5rem; }
	.identity { display: grid; min-width: 0; flex: 1; }
	.identity a { font-weight: 650; color: var(--ink); }
	.player-points { text-align: right; font-variant-numeric: tabular-nums; }
	.player-points small { display: block; font-size: .6rem; font-weight: 500; color: var(--ink-soft); }
	.selection { margin-top: .35rem; color: var(--brand); }
	.player-games { display: grid; gap: .45rem; margin-top: .5rem; }
	.game-line { padding: .4rem .5rem; background: var(--surface-2); border-radius: var(--radius-small); }
	.game-heading { display: flex; flex-wrap: wrap; gap: .2rem .6rem; justify-content: space-between; font-size: .7rem; }
	.game-link { color: var(--ink); }
	.game-status { color: var(--ink-soft); }
	.game-status.counted { color: var(--good); }
	.stat-line { display: flex; flex-wrap: wrap; gap: .25rem .6rem; margin-top: .25rem; font-size: .67rem; color: var(--ink-soft); font-variant-numeric: tabular-nums; }
	.stat-line strong, .game-points { color: var(--ink); }
	.uncounted { border-left: 2px solid var(--rule-strong); }
	.cap, .empty { padding: .75rem 1rem; }
	@media(max-width: 700px) {
		.teams { grid-template-columns: minmax(0,1fr); }
		.scoreboard { padding: .8rem .5rem; gap: .35rem; }
		.score { font-size: 1.85rem; }
		.score-state { font-size: .7rem; }
		.score-state .small-text { font-size: .65rem; }
	}
</style>
