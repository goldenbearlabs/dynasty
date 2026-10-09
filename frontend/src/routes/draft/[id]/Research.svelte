<script lang="ts">
	import {
		getPlayerResearch,
		getPlayerSeasons,
		type Competition,
		type Player,
		type PlayerResearch,
		type PlayerSeason
	} from '#lib/api.ts';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { age, dayLabel, points } from '#lib/ui/time.ts';
	import type { Snippet } from 'svelte';
	let {
		selectedId,
		competitions,
		action
	}: { selectedId?: string; competitions: Competition[]; action: Snippet<[Player]> } = $props();
	let result = $state<PlayerResearch>();
	let error = $state('');
	let retry = $state(0);
	$effect(() => {
		const id = selectedId;
		void retry;
		result = undefined;
		error = '';
		if (!id) return;
		let active = true;
		getPlayerResearch(id).then(
			(r) => {
				if (active) result = r;
			},
			(e) => {
				if (active) error = e.message;
			}
		);
		return () => {
			active = false;
		};
	});
	const stats = $derived(
		(competitions.find((c) => c.key === result?.player.competition)?.stats ?? []).filter((s) =>
			result?.games.some((g) => g.stats[s.key] !== undefined)
		)
	);
	const average = $derived(
		result?.games.length
			? result.games.reduce((total, g) => total + g.points, 0) / result.games.length
			: undefined
	);
	const short = (key: string) => key.replace(/^(bat|pit)_/, '').replaceAll('_', ' ');

	// Season-by-season history, kept by the nightly stat-history sync. It
	// includes seasons in other leagues: an NBA player's college years, the
	// junior hockey behind an NHL prospect.
	const seasonsShown = 6;
	let seasons = $state<PlayerSeason[]>();
	let seasonsError = $state('');
	let allSeasons = $state(false);
	$effect(() => {
		const id = selectedId;
		seasons = undefined;
		seasonsError = '';
		allSeasons = false;
		if (!id) return;
		let active = true;
		getPlayerSeasons(id).then(
			(s) => {
				if (active) seasons = s;
			},
			(e) => {
				if (active) seasonsError = e.message;
			}
		);
		return () => {
			active = false;
		};
	});
	const competition = $derived(competitions.find((c) => c.key === result?.player.competition));
	const seasonStats = $derived(
		(competition?.stats ?? []).filter((s) => seasons?.some((season) => season.stats[s.key]))
	);
	const number = (n: number | undefined) =>
		n === undefined ? '—' : Number.isInteger(n) ? n.toLocaleString() : n.toFixed(1);
</script>

<section class="research" id="draft-player-research" aria-label="Player research">
	<div class="section-title">
		<h2>Player research</h2>
		<span class="muted">{selectedId ? 'Profile & game log' : 'Select a player'}</span>
		<a class="back" href="#draft-player-pool">Back to players ↑</a>
	</div>
	<div class="body">
		{#if !selectedId}
			<div class="prompt">
				<Icon name="search" size={24} />
				<h3>Find your next pick</h3>
				<p>
					Choose a name from the player pool, board, or queue to see their profile and recent games
					here.
				</p>
			</div>
		{:else if error}
			<p role="alert">{error}</p>
			<button class="small" onclick={() => retry++}>Try again</button>
		{:else if !result}<p class="muted small-text">Loading player research…</p>
		{:else}
			{@const player = result.player}
			<header class="identity">
				<Headshot name={player.full_name} src={player.headshot_url} size={48} />
				<div>
					<SportBadge sport={player.competition} />
					<h3>{player.full_name}</h3>
					<p>{player.positions.join('/')} · {player.team_name || 'No current team'}</p>
				</div>
			</header>
			<dl>
				<div>
					<dt>Status</dt>
					<dd>{player.status}</dd>
				</div>
				{#if player.birth_date}<div>
						<dt>Age</dt>
						<dd>{age(player.birth_date)}</dd>
					</div>{/if}{#if player.class}<div>
						<dt>Class</dt>
						<dd>{player.class}</dd>
					</div>{/if}
			</dl>
			{#if player.note}<p class="note">{player.note}</p>{/if}
			<div class="actions">{@render action(player)}</div>
			<div class="spread history-title">
				<h3>Seasons</h3>
				{#if seasons?.length}<span>{result.scoring_source === 'defaults' ? 'sport default scoring' : 'current league scoring'}</span>{/if}
			</div>
			{#if seasonsError}
				<p class="muted small-text">{seasonsError}</p>
			{:else if !seasons}
				<p class="muted small-text">Loading seasons…</p>
			{:else if seasons.length}
				<div class="scroll">
					<table>
						<thead>
							<tr>
								<th>Season</th>
								<th class="num" title="Games played">GP</th>
								<th class="num" title="Fantasy points">FP</th>
								<th class="num" title="Fantasy points per game">FP/G</th>
								{#each seasonStats as stat (stat.key)}
									<th class="num" title={stat.label}>{short(stat.key)}</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							{#each allSeasons ? seasons : seasons.slice(0, seasonsShown) as season (season.competition + season.season + season.team + season.league)}
								<tr>
									<td>
										{season.season}
										<small>{[season.team, season.league].filter(Boolean).join(' · ')}</small>
									</td>
									<td class="num">{season.games || '—'}</td>
									<td class="num total">{points(season.points)}</td>
									<td class="num total">{season.games ? points(season.points_per_game) : '—'}</td>
									{#each seasonStats as stat (stat.key)}
										<td class="num">{number(season.stats[stat.key])}</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
				{#if seasons.length > seasonsShown && !allSeasons}
					<button class="small quiet" onclick={() => (allSeasons = true)}>
						Show all {seasons.length} seasons
					</button>
				{/if}
			{:else}
				<p class="muted small-text">No seasons on record for this player.</p>
			{/if}
			<div class="spread history-title">
				<h3>Recent games</h3>
				{#if average !== undefined}<span><strong>{points(average)}</strong> avg FP</span>{/if}
			</div>
			{#if result.games.length}
				<p class="muted small-text">
					Last {result.games.length} recorded finals · {result.scoring_source === 'defaults' ? 'sport default scoring' : 'current league scoring'}
				</p>
				<div class="scroll">
					<table>
						<thead
							><tr
								><th>Date / game</th>{#each stats as stat (stat.key)}<th
										class="num"
										title={stat.label}>{short(stat.key)}</th
									>{/each}<th class="num">FP</th></tr
							></thead
						><tbody>
							{#each result.games as game (game.id)}<tr
									><td
										><a
											href="/game/{game.id}"
											target="_blank"
											rel="noopener noreferrer"
											title="Open box score in a new tab">{dayLabel(game.day)}</a
										><small>{game.away_abbrev} @ {game.home_abbrev}</small></td
									>{#each stats as stat (stat.key)}<td class="num">{game.stats[stat.key] ?? '—'}</td
										>{/each}<td class="num total">{points(game.points)}</td></tr
								>{/each}
						</tbody>
					</table>
				</div>
			{:else}<p class="muted small-text">No recorded final box scores for this player yet.</p>{/if}
		{/if}
	</div>
</section>

<style>
	.research {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		background: var(--surface);
		border-left: 1px solid var(--rule);
	}
	.section-title {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.6rem 0.75rem;
		border-bottom: 1px solid var(--rule);
	}
	h2 {
		font-size: 0.9rem;
	}
	.section-title span {
		font-size: 0.65rem;
	}
	.back {
		display: none;
		font-size: 0.7rem;
	}
	@media (max-width: 640px) {
		.section-title span {
			display: none;
		}
		.back {
			display: inline;
		}
	}
	.body {
		overflow: auto;
		padding: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		min-height: 0;
	}
	.body > :global(*) {
		flex-shrink: 0;
	}
	.prompt {
		display: grid;
		gap: 0.5rem;
		color: var(--ink-soft);
		padding-block: 1.5rem;
		font-size: 0.8rem;
	}
	.identity {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.identity h3 {
		margin-top: 0.3rem;
		font-size: 1.05rem;
	}
	.identity p {
		font-size: 0.72rem;
		color: var(--ink-soft);
		margin-top: 0.2rem;
	}
	dl {
		display: flex;
		gap: 1.25rem;
		margin: 0;
		padding-block: 0.6rem;
		border-block: 1px solid var(--rule);
	}
	dt {
		font-size: 0.62rem;
		color: var(--ink-soft);
	}
	dd {
		margin: 0;
		font-size: 0.8rem;
		text-transform: capitalize;
		font-weight: 600;
	}
	.note {
		font-size: 0.78rem;
		white-space: pre-wrap;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
	.history-title h3 {
		font-size: 0.85rem;
	}
	.history-title span {
		font-size: 0.7rem;
		color: var(--ink-soft);
	}
	.history-title strong {
		color: var(--brand);
		font-size: 1rem;
	}
	th,
	td {
		padding: 0.4rem;
		font-size: 0.68rem;
	}
	th {
		font-size: 0.58rem;
		letter-spacing: 0;
	}
	td:first-child {
		white-space: nowrap;
	}
	td small {
		display: block;
		color: var(--ink-soft);
		font-size: 0.6rem;
	}
	.total {
		font-weight: 700;
	}
</style>
