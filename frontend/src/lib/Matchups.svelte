<script lang="ts">
	// One period of a head-to-head league: who plays whom, and the score.
	import { getMatchups, type Franchise, type League, type Matchups } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import { teamIdentity } from '#lib/identity.ts';
	import MatchupEditor from '#lib/MatchupEditor.svelte';
 import type { TeamIdentity } from '#lib/api.ts';
 import Crest from '#lib/ui/Crest.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	// mine is the signed-in franchise: its matchup comes first and stands out.
	// editable lets the commissioner set a period's matchups by hand.
	let { league, franchises, identities = [], mine, editable = false }: {
		league: League; franchises: Franchise[]; identities?: TeamIdentity[]; mine?: string; editable?: boolean;
	} = $props();

	const identity = (id: string | null) => teamIdentity(franchises.find(f => f.id===id),identities,league.id);
 const name = (id: string | null) => identity(id).name;

	let view = $state<Matchups>();
	let seq = $state<number>(); // undefined shows the period in progress
	let editing = $state(false);
	const load = () => getMatchups(league.id, seq).then((m) => (view = m), toast.error);
	$effect(() => {
		void [league.id, seq];
		load();
	});
	onScoresChange(() => [league.competition], load);

	const period = $derived(view?.period);
	const isMine = (m: Matchups['matchups'][number]) => !!mine && (m.home_franchise_id === mine || m.away_franchise_id === mine);
	const matchups = $derived(view?.matchups.toSorted((a, b) => Number(isMine(b)) - Number(isMine(a))) ?? []);
	const last = $derived(view?.periods.at(-1)?.seq ?? 0);
	const label = $derived.by(() => {
		if (!view || !period) return '';
		const regular = view.periods.filter((p) => !p.is_playoff).length;
		if (!period.is_playoff) return `${league.settings.format.matchup_days === 7 ? 'Week' : 'Matchup'} ${period.seq} of ${regular}`;
		return period.seq === last ? 'Final' : `Playoffs, round ${period.seq - regular}`;
	});
</script>

{#if view && period}
	<div class="stack tight">
		<div class="spread">
			<div>
				<strong>{label}</strong>
				{#if period.by_hand}<span class="pill gold">Set by the commissioner</span>{/if}
				<span class="muted small-text">
					{period.starts_on === period.ends_on ? dayLabel(period.starts_on) : `${dayLabel(period.starts_on)} – ${dayLabel(period.ends_on)}`}
				</span>
			</div>
			<nav class="row" aria-label="Matchup period">
				{#if editable && !editing && !matchups.some((m) => m.final)}<button class="small" onclick={() => (editing = true)}>Edit</button>{/if}
				<button aria-label="Earlier" disabled={period.seq <= 1} onclick={() => ((editing = false), (seq = period.seq - 1))}><Icon name="left" size={16} /></button>
				<button aria-label="Later" disabled={period.seq >= last} onclick={() => ((editing = false), (seq = period.seq + 1))}><Icon name="right" size={16} /></button>
			</nav>
		</div>

		{#if editing}
			{#key period.id}
				<MatchupEditor
					{period}
					{matchups}
					teams={franchises.map((f) => ({ id: f.id, name: name(f.id) }))}
					ondone={(changed) => ((editing = false), changed && load())}
				/>
			{/key}
		{:else if matchups.length === 0}
			<p class="muted small-text">This round's matchups are set once the round before it has finished.</p>
		{:else}
			<div class="matchups">
				{#each matchups as m (m.id)}
					<a class="card" class:mine={isMine(m)} href="/matchups/{m.id}">
						{#each [{ id: m.home_franchise_id, points: m.home_points, other: m.away_points }, { id: m.away_franchise_id, points: m.away_points, other: m.home_points }] as side (side.id)}
							<div class="side" class:behind={m.final && side.points < side.other}>
								{#if side.id}
									<Crest src={identity(side.id).image_url} name={name(side.id)} size={26} />
									<span class="team">{name(side.id)}</span>
									<span class="score">{points(side.points)}</span>
								{:else}
									<span class="team muted">Bye</span>
								{/if}
							</div>
						{/each}
						{#if m.final}<span class="eyebrow">Final</span>{/if}
					</a>
				{/each}
			</div>
		{/if}
	</div>
{:else if view}
	<p class="muted small-text">No matchups are scheduled yet.</p>
{/if}

<style>
	nav {
		gap: 0.4rem;
	}
	nav button {
		padding: 0.4rem 0.55rem;
	}
	.matchups {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
		gap: 0.8rem;
	}
	a.card {
		display: grid;
		gap: 0.5rem;
		color: var(--ink);
	}
	a.card:hover {
		text-decoration: none;
		border-color: var(--rule-strong);
	}
	a.card.mine {
		border-color: var(--brand);
	}
	.side {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		font-weight: 650;
	}
	.side.behind {
		color: var(--ink-soft);
		font-weight: 500;
	}
	.team {
		flex: 1;
	}
	.score {
		font: 750 1.1rem var(--display);
		font-variant-numeric: tabular-nums;
	}
</style>
