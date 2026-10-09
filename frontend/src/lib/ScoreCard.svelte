<script lang="ts">
	// One game on a scoreboard: both teams, the score, and where it stands.
	import type { GameSummary } from '#lib/api.ts';
	import { timeOfDay } from '#lib/ui/time.ts';

	let { game, big = false }: { game: GameSummary; big?: boolean } = $props();

	const started = $derived(game.status !== 'scheduled');
	const sides = $derived([
		{ abbrev: game.away_abbrev, name: game.away_name, logo: game.away_logo, score: game.away_score, other: game.home_score },
		{ abbrev: game.home_abbrev, name: game.home_name, logo: game.home_logo, score: game.home_score, other: game.away_score }
	]);
</script>

<div class="game" class:big class:live={game.status === 'live'}>
	<div class="state">
		{#if game.status === 'live'}
			<span class="dot" aria-hidden="true"></span><strong>{game.detail || 'Live'}</strong>
		{:else if game.status === 'final'}
			{game.detail || 'Final'}
		{:else}
			{timeOfDay(game.starts_at)}
		{/if}
	</div>
	{#each sides as side (side.abbrev)}
		<div class="side" class:behind={game.status === 'final' && side.score < side.other}>
			{#if side.logo}<img src={side.logo} alt="" loading="lazy" />{:else}<span class="logo"></span>{/if}
			<span class="team">{big ? side.name : side.abbrev}</span>
			{#if started}<span class="score">{side.score}</span>{/if}
		</div>
	{/each}
</div>

<style>
	.game {
		display: grid;
		gap: 0.4rem;
	}
	.state {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font: 600 0.72rem var(--mono);
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.live .state {
		color: var(--bad);
	}
	.dot {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 50%;
		background: var(--bad);
		animation: pulse 1.6s ease-in-out infinite;
	}
	.side {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		font-weight: 650;
	}
	.side.behind {
		color: var(--ink-soft);
		font-weight: 500;
	}
	img,
	.logo {
		width: 1.6rem;
		height: 1.6rem;
		object-fit: contain;
	}
	@media (prefers-color-scheme: dark) {
		img {
			/* Most logos are drawn for a light page: give them one. */
			background: #eceef5;
			border-radius: 50%;
			padding: 2px;
		}
	}
	.team {
		flex: 1;
	}
	.score {
		font: 750 1.15rem var(--display);
		font-variant-numeric: tabular-nums;
	}
	.big .side {
		font-size: 1.15rem;
	}
	.big .score {
		font-size: 1.9rem;
	}
	.big img,
	.big .logo {
		width: 2.4rem;
		height: 2.4rem;
	}
	@keyframes pulse {
		50% {
			opacity: 0.35;
		}
	}
</style>
