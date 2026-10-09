<script lang="ts">
	// One head-to-head matchup: the two lineups side by side, slot against
	// slot, with what each starter has scored over the matchup.
	import { page } from '$app/state';
	import { getLineup, getMatchup, type Lineup, type MatchupDetail } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import { teamIdentity } from '#lib/identity.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let matchup = $state<MatchupDetail>();
	let lineups = $state<(Lineup | undefined)[]>([]); // home, then away
	const leagueOf = (competition?: string) => data.dynasty?.leagues.find((l) => l.competition === competition);

	async function load() {
		const id = page.params.id!;
		try {
			const m = await getMatchup(id);
			const league = leagueOf(m.competition);
			// The lineup that is playing: the last one of a finished matchup,
			// today's while it is on, the first of one still to come.
			const today = new Date().toLocaleDateString('en-CA');
			const day = m.final ? m.period.ends_on : m.period.starts_on > today ? m.period.starts_on : undefined;
			const found = await Promise.all(
				[m.home_franchise_id, m.away_franchise_id].map((f) => (league && f ? getLineup(league.id, f, day) : undefined))
			);
			if (id !== page.params.id) return;
			[matchup, lineups] = [m, found];
		} catch (e) {
			toast.error(e);
		}
	}
	$effect(() => {
		void page.params.id;
		load();
	});
	onScoresChange(() => (matchup ? [matchup.competition] : []), load);

	const league = $derived(leagueOf(matchup?.competition));
	const sides = $derived(
		matchup
			? [
					{ id: matchup.home_franchise_id, total: matchup.home_points, other: matchup.away_points, scored: matchup.home_players, lineup: lineups[0] },
					{ id: matchup.away_franchise_id, total: matchup.away_points, other: matchup.home_points, scored: matchup.away_players, lineup: lineups[1] }
				].map((side) => {
					const franchise = data.dynasty?.franchises.find((f) => f.id === side.id);
					return { ...side, franchise, identity: teamIdentity(franchise, data.dynasty?.team_identities, league?.id) };
				})
			: []
	);

	type Side = (typeof sides)[number];
	type Cell = { id: string; name: string; nickname?: string; headshot: string; detail: string; points: number };

	// A player's points are his total over the matchup, not just the lineup's days.
	function cell(side: Side, id: string): Cell | undefined {
		const scored = side.scored.find((p) => p.player_id === id);
		const listed = side.lineup?.players.find((p) => p.player_id === id);
		if (!scored && !listed) return undefined;
		return {
			id,
			name: listed?.full_name ?? scored!.full_name,
			nickname: listed?.nickname || scored?.nickname,
			headshot: listed?.headshot_url ?? scored!.headshot_url,
			detail: listed ? [listed.positions.join('/'), listed.team_abbrev].filter(Boolean).join(' · ') : '',
			points: scored?.points ?? 0
		};
	}

	// One row for every starting place, the two teams' starters facing each other.
	const slots = $derived(lineups.find((l) => l)?.slots ?? league?.settings.lineup.slots ?? []);
	const starters = $derived(
		slots.flatMap((slot) =>
			Array.from({ length: slot.count }, (_, i) => ({
				slot: slot.name,
				cells: sides.map((side) => {
					const player = side.lineup?.players.filter((p) => p.slot === slot.name)[i];
					return player && cell(side, player.player_id);
				})
			}))
		)
	);
	// Whoever scored earlier in the matchup and has since left the lineup.
	const earlier = $derived.by(() => {
		const gone = sides.map((side) =>
			side.scored.filter((p) => !side.lineup?.players.some((l) => l.player_id === p.player_id && l.slot))
		);
		return Array.from({ length: Math.max(0, ...gone.map((g) => g.length)) }, (_, i) =>
			sides.map((side, s) => gone[s][i] && cell(side, gone[s][i].player_id))
		);
	});
</script>

<svelte:head><title>{sides.length ? sides.map((s) => s.identity.name || 'Bye').join(' vs ') : 'Matchup'}</title></svelte:head>

{#snippet team(side: Side, away: boolean)}
	<div class="team" class:away class:behind={matchup?.final && side.total < side.other}>
		{#if side.franchise}
			<Crest src={side.identity.image_url} name={side.identity.name} size={40} />
			<a class="name" href="/franchise/{side.franchise.slug}">{side.identity.name}</a>
			<span class="total">{points(side.total)}</span>
		{:else}
			<span class="name muted">Bye</span>
		{/if}
	</div>
{/snippet}

{#snippet player(side: Side, shown: Cell | undefined, away: boolean)}
	<div class="player" class:away>
		{#if shown}
			<span class="shot"><Headshot name={shown.name} src={shown.headshot} size={30} /></span>
			<span class="who">
				<a href="/player/{shown.id}">{shown.name}</a>{#if shown.nickname} <span class="pill brand">“{shown.nickname}”</span>{/if}
				{#if shown.detail}<span class="muted small-text">{shown.detail}</span>{/if}
			</span>
			<strong class="pts">{points(shown.points)}</strong>
		{:else if side.franchise}
			<span class="who muted">Empty</span>
			<span class="pts muted">–</span>
		{/if}
	</div>
{/snippet}

{#if !matchup}
	<p class="muted">Loading…</p>
{:else}
	<div class="stack" data-sport={matchup.competition}>
		<p class="row small-text">
			<SportBadge sport={matchup.competition} solid />
			<a href="/matchups">Matchups</a>
			<span class="muted">
				{matchup.period.is_playoff ? 'Playoffs' : `Matchup ${matchup.period.seq}`} ·
				{dayLabel(matchup.period.starts_on)}{matchup.period.ends_on !== matchup.period.starts_on ? ` – ${dayLabel(matchup.period.ends_on)}` : ''}
			</span>
			<span class="pill {matchup.final ? '' : 'good'}">{matchup.final ? 'Final' : 'In progress'}</span>
		</p>

		<section class="card flush board">
			<header class="line">
				{@render team(sides[0], false)}
				<span class="slot">vs</span>
				{@render team(sides[1], true)}
			</header>
			{#each starters as row, i (i)}
				<div class="line">
					{@render player(sides[0], row.cells[0], false)}
					<span class="slot">{row.slot}</span>
					{@render player(sides[1], row.cells[1], true)}
				</div>
			{/each}
			{#if earlier.length}
				<p class="eyebrow divider">Started earlier in this matchup</p>
				{#each earlier as cells, i (i)}
					<div class="line">
						{#each cells as shown, s (s)}
							{#if s === 1}<span class="slot"></span>{/if}
							{#if shown}{@render player(sides[s], shown, s === 1)}{:else}<div></div>{/if}
						{/each}
					</div>
				{/each}
			{/if}
		</section>
		<p class="muted small-text">Only starters score. Totals follow the games as they are played.</p>
	</div>
{/if}

<style>
	/* Every line is home, slot, away, so the slots run down the middle. */
	.line {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 3.2rem minmax(0, 1fr);
		align-items: center;
		border-bottom: 1px solid var(--rule);
	}
	.line:last-child {
		border-bottom: none;
	}
	.slot {
		align-self: stretch;
		display: grid;
		place-items: center;
		background: var(--surface-2);
		color: var(--ink-soft);
		font: 700 0.68rem var(--mono);
		text-transform: uppercase;
	}
	.team,
	.player {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		min-width: 0;
		padding: 0.55rem 0.9rem;
	}
	.team {
		padding-block: 1rem;
	}
	.away {
		flex-direction: row-reverse;
		text-align: right;
	}
	.name {
		flex: 1;
		min-width: 0;
		font: 700 1.2rem/1.15 var(--display);
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
	.shot {
		display: flex;
	}
	.who {
		flex: 1;
		min-width: 0;
		display: grid;
		line-height: 1.3;
		font-weight: 600;
	}
	.who a {
		color: var(--ink);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.who .muted {
		font-weight: 400;
	}
	.pts {
		min-width: 2.8rem;
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.away .pts {
		text-align: left;
	}
	.divider {
		margin: 0;
		padding: 0.5rem 0.9rem;
		text-align: center;
		border-bottom: 1px solid var(--rule);
	}

	@media (max-width: 640px) {
		.line {
			grid-template-columns: minmax(0, 1fr) 2.4rem minmax(0, 1fr);
		}
		.team,
		.player {
			gap: 0.4rem;
			padding-inline: 0.5rem;
		}
		.team {
			flex-direction: column;
			text-align: center;
		}
		.name {
			font-size: 0.95rem;
		}
		.total {
			font-size: 1.5rem;
		}
		.shot {
			display: none;
		}
		.who {
			font-size: 0.85rem;
		}
	}
</style>
