<script lang="ts">
	// A head-to-head league's whole season: every period's matchups, or one
	// team's opponent and result in each.
	import { getSchedule, type Franchise, type League, type Matchup, type SchedulePeriod, type TeamIdentity } from '#lib/api.ts';
	import { teamIdentity } from '#lib/identity.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	// team is the franchise whose schedule to show; empty shows everyone's.
	let { league, franchises, identities = [], team = '' }: {
		league: League; franchises: Franchise[]; identities?: TeamIdentity[]; team?: string;
	} = $props();

	const identity = (id: string | null) => teamIdentity(franchises.find((f) => f.id === id), identities, league.id);

	let periods = $state<SchedulePeriod[]>();
	const load = () => getSchedule(league.id).then((p) => (periods = p), toast.error);
	$effect(() => {
		void league.id;
		load();
	});
	onScoresChange(() => [league.competition], load);

	const today = new Date().toLocaleDateString('en-CA');
	const started = (p: SchedulePeriod) => p.starts_on <= today;
	const regular = $derived(periods?.filter((p) => !p.is_playoff).length ?? 0);
	const unit = $derived(league.settings.format.matchup_days === 7 ? 'Week' : 'Matchup');
	const label = (p: SchedulePeriod) =>
		!p.is_playoff ? `${unit} ${p.seq}` : p.seq === periods?.at(-1)?.seq ? 'Final' : `Playoffs, round ${p.seq - regular}`;
	const days = (p: SchedulePeriod) => (p.starts_on === p.ends_on ? dayLabel(p.starts_on) : `${dayLabel(p.starts_on)} – ${dayLabel(p.ends_on)}`);

	// The team's side of each period: who it plays and how it is going.
	const mine = $derived(
		(periods ?? []).map((period) => {
			const matchup = period.matchups.find((m) => m.home_franchise_id === team || m.away_franchise_id === team);
			const home = matchup?.home_franchise_id === team;
			return {
				period,
				matchup,
				opponent: matchup ? (home ? matchup.away_franchise_id : matchup.home_franchise_id) : null,
				ours: (home ? matchup?.home_points : matchup?.away_points) ?? 0,
				theirs: (home ? matchup?.away_points : matchup?.home_points) ?? 0
			};
		})
	);
	const result = (ours: number, theirs: number) => (ours > theirs ? 'W' : ours < theirs ? 'L' : 'T');
</script>

{#snippet score(period: SchedulePeriod, m: Matchup, ours: number, theirs: number)}
	<a href="/matchups/{m.id}">
		{#if !started(period)}
			View
		{:else}
			{#if m.final && team}<strong class="result {result(ours, theirs)}">{result(ours, theirs)}</strong>{/if}
			{points(ours)} – {points(theirs)}
		{/if}
	</a>
	{#if started(period) && !m.final}<span class="pill good">Live</span>{/if}
{/snippet}

{#if !periods}
	<p class="muted small-text">Loading…</p>
{:else if periods.length === 0}
	<p class="muted small-text">No matchups are scheduled yet.</p>
{:else}
	<div class="panel scroll">
		<table>
			<thead>
				<tr>
					<th>{unit}</th>
					<th class="wide">Days</th>
					<th>{team ? 'Opponent' : 'Matchups'}</th>
					{#if team}<th class="num">Score</th>{/if}
				</tr>
			</thead>
			<tbody>
				{#each mine as { period, matchup, opponent, ours, theirs } (period.id)}
					<tr class:now={started(period) && period.ends_on >= today}>
						<td><strong>{label(period)}</strong></td>
						<td class="wide muted small-text">{days(period)}</td>
						{#if team}
							<td>
								{#if opponent}
									<span class="side"><Crest src={identity(opponent).image_url} name={identity(opponent).name} size={22} /> {identity(opponent).name}</span>
								{:else}
									<span class="muted">{matchup ? 'Bye' : period.matchups.length ? 'Not playing' : 'To be decided'}</span>
								{/if}
							</td>
							<td class="num">{#if matchup && opponent}{@render score(period, matchup, ours, theirs)}{/if}</td>
						{:else}
							<td>
								<div class="games">
									{#each period.matchups as m (m.id)}
										<span class="game">
											<span class="side">{identity(m.home_franchise_id).name}</span>
											{#if m.away_franchise_id}
												{#if started(period)}{@render score(period, m, m.home_points, m.away_points)}{:else}<a href="/matchups/{m.id}">vs</a>{/if}
												<span class="side">{identity(m.away_franchise_id).name}</span>
											{:else}
												<span class="muted">bye</span>
											{/if}
										</span>
									{:else}
										<span class="muted">To be decided</span>
									{/each}
								</div>
							</td>
						{/if}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<style>
	tr.now {
		background: var(--brand-soft);
	}
	.side {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		font-weight: 600;
	}
	.games {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 0.25rem 1.25rem;
	}
	.game {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.result.W {
		color: var(--good);
	}
	.result.L {
		color: var(--bad);
	}
	@media (max-width: 640px) {
		.wide {
			display: none;
		}
	}
</style>
