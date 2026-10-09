<script lang="ts">
	// One game: the score, and every player's line with what it is worth in
	// this league and who has him. The server pushes it as the game goes on.
	import { page } from '$app/state';
	import type { GameDetail, GameLine } from '#lib/api.ts';
	import ScoreCard from '#lib/ScoreCard.svelte';
	import { LiveSocket } from '#lib/socket.svelte.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let game = $state<GameDetail>();
	$effect(() => {
		const socket = new LiveSocket<GameDetail>(`/games/${page.params.id}/ws`, (g) => (game = g));
		return () => socket.close();
	});

	// The sport's stats, in its own order, that anyone in this game recorded.
	const columns = $derived(
		(data.competitions.find((c) => c.key === game?.competition)?.stats ?? []).filter((stat) =>
			game!.lines.some((line) => line.stats[stat.key])
		)
	);
	const scored = $derived(data.dynasty?.leagues.some((l) => l.competition === game?.competition) ?? false);
	const sides = $derived(
		game
			? [
					{ name: game.away_name, lines: game.lines.filter((l) => !l.at_home) },
					{ name: game.home_name, lines: game.lines.filter((l) => l.at_home) }
				]
			: []
	);

	const short = (key: string) => key.replace(/^(bat|pit)_/, '').replaceAll('_', ' ');
	const value = (line: GameLine, key: string) => {
		const n = line.stats[key];
		return n === undefined ? '' : Number.isInteger(n) ? n : n.toFixed(1);
	};
</script>

<svelte:head><title>{game ? `${game.away_abbrev} at ${game.home_abbrev}` : 'Game'}</title></svelte:head>

{#if !game}
	<p class="muted">Loading…</p>
{:else}
	<div class="stack" data-sport={game.competition}>
		<p class="row small-text">
			<SportBadge sport={game.competition} solid />
			<a href="/scores?sport={game.competition}&day={game.day}">{dayLabel(game.day)} scores</a>
		</p>
		<header class="card"><ScoreCard {game} big /></header>

		{#if game.lines.length === 0}
			<Empty icon="scores" title={game.status === 'scheduled' ? 'Not started yet' : 'No box score yet'}>
				Each player's line appears here once the game is under way.
			</Empty>
		{:else}
			{#each sides as side (side.name)}
				{#if side.lines.length > 0}
					<section class="stack tight">
						<h2>{side.name}</h2>
						<div class="card flush scroll">
							<table>
								<thead>
									<tr>
										<th>Player</th>
										{#each columns as stat (stat.key)}<th class="num" title={stat.label}>{short(stat.key)}</th>{/each}
										{#if scored}<th class="num">Fantasy</th>{/if}
									</tr>
								</thead>
								<tbody>
									{#each side.lines as line (line.player_id)}
										<tr>
											<td>
												<div class="player">
													<Headshot name={line.full_name} src={line.headshot_url} size={30} />
													<span>
														<strong>{line.full_name}</strong> <span class="muted small-text">{line.positions.join('/')}</span>
														{#if line.owner_slug}
															<a class="pill brand" href="/franchise/{line.owner_slug}">{line.owner_name}</a>
														{/if}
													</span>
												</div>
											</td>
											{#each columns as stat (stat.key)}<td class="num">{value(line, stat.key)}</td>{/each}
											{#if scored}<td class="num total">{points(line.points)}</td>{/if}
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					</section>
				{/if}
			{/each}
		{/if}
	</div>
{/if}

<style>
	header {
		max-width: 26rem;
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		white-space: nowrap;
	}
	th.num {
		text-align: right;
	}
	.total {
		font: 700 1rem var(--display);
	}
</style>
