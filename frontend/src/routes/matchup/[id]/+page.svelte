<script lang="ts">
	// One head-to-head matchup: each side's total and the starters behind it.
	import { page } from '$app/state';
	import { getMatchup, type MatchupDetail } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let matchup = $state<MatchupDetail>();
	const load = () => getMatchup(page.params.id!).then((m) => (matchup = m), toast.error);
	$effect(() => {
		void page.params.id;
		load();
	});
	onScoresChange(() => (matchup ? [matchup.competition] : []), load);

	const franchise = (id: string | null) => data.dynasty?.franchises.find((f) => f.id === id);
	const sides = $derived(
		matchup
			? [
					{ franchise: franchise(matchup.home_franchise_id), total: matchup.home_points, other: matchup.away_points, players: matchup.home_players },
					{ franchise: franchise(matchup.away_franchise_id), total: matchup.away_points, other: matchup.home_points, players: matchup.away_players }
				]
			: []
	);
</script>

<svelte:head><title>Matchup</title></svelte:head>

{#if !matchup}
	<p class="muted">Loading…</p>
{:else}
	<div class="stack" data-sport={matchup.competition}>
		<p class="row small-text">
			<SportBadge sport={matchup.competition} solid />
			<span class="muted">
				{matchup.period.is_playoff ? 'Playoffs' : `Matchup ${matchup.period.seq}`} ·
				{dayLabel(matchup.period.starts_on)}{matchup.period.ends_on !== matchup.period.starts_on ? ` – ${dayLabel(matchup.period.ends_on)}` : ''}
			</span>
			<span class="pill {matchup.final ? '' : 'good'}">{matchup.final ? 'Final' : 'In progress'}</span>
		</p>

		<div class="sides">
			{#each sides as side (side.franchise?.id ?? 'bye')}
				<section class="card flush">
					{#if side.franchise}
						<header class:behind={matchup.final && side.total < side.other}>
							<Crest name={side.franchise.name} size={40} />
							<a class="name" href="/franchise/{side.franchise.slug}">{side.franchise.name}</a>
							<span class="total">{points(side.total)}</span>
						</header>
						<ul>
							{#each side.players as player (player.player_id)}
								<li>
									<Headshot name={player.full_name} src={player.headshot_url} size={30} />
									<span class="who">{player.full_name}</span>
									<span class="muted small-text">{player.games} {player.games === 1 ? 'game' : 'games'}</span>
									<strong class="pts">{points(player.points)}</strong>
								</li>
							{:else}
								<li class="muted small-text">No starter has scored yet.</li>
							{/each}
						</ul>
					{:else}
						<header><span class="name muted">Bye</span></header>
					{/if}
				</section>
			{/each}
		</div>
		<p class="muted small-text">Only starters score. Totals follow the games as they are played.</p>
	</div>
{/if}

<style>
	.sides {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 19rem), 1fr));
		gap: 1rem;
		align-items: start;
	}
	header {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		padding: 1rem 1.1rem;
		border-bottom: 1px solid var(--rule);
	}
	.name {
		flex: 1;
		font: 700 1.2rem var(--display);
		color: var(--ink);
	}
	.total {
		font: 800 2rem/1 var(--display);
		font-variant-numeric: tabular-nums;
	}
	.behind .total,
	.behind .name {
		color: var(--ink-soft);
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		padding: 0.55rem 1.1rem;
		border-bottom: 1px solid var(--rule);
	}
	li:last-child {
		border-bottom: none;
	}
	.who {
		flex: 1;
		font-weight: 600;
	}
	.pts {
		min-width: 3rem;
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
</style>
