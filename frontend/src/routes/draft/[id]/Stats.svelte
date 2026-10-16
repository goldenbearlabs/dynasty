<script lang="ts">
	// The draft by the numbers: how long picks take, how old the players
	// are, where they come from, and each team's draft side by side. It
	// describes what happened; it does not judge any pick.
	import type { Draft, DraftPick, Franchise } from '#lib/api.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';

	let { draft, picks, franchises, showSport }: { draft: Draft; picks: DraftPick[]; franchises: Franchise[]; showSport: boolean } = $props();

	const name = (id: string) => franchises.find((f) => f.id === id)?.name ?? 'Unknown team';
	const made = $derived(picks.filter((p) => p.player_id && p.picked_at).toSorted((a, b) => a.picked_at!.localeCompare(b.picked_at!)));

	// ---- time on the clock ----
	// A pick took from the pick made before it. A gap far longer than the
	// clock is a break (a pause, a night's sleep) and is left out.
	const longest = $derived(Math.max(30 * 60, draft.pick_clock_seconds * 3));
	const timed = $derived(
		made.slice(1).map((pick, i) => ({ pick, seconds: (Date.parse(pick.picked_at!) - Date.parse(made[i].picked_at!)) / 1000 }))
	);
	const onClock = $derived(timed.filter((t) => t.seconds <= longest));
	const breaks = $derived(timed.length - onClock.length);
	const median = (ns: number[]) => {
		if (ns.length === 0) return undefined;
		const s = ns.toSorted((a, b) => a - b);
		return s.length % 2 ? s[(s.length - 1) / 2] : (s[s.length / 2 - 1] + s[s.length / 2]) / 2;
	};
	const mean = (ns: number[]) => (ns.length ? ns.reduce((a, b) => a + b, 0) / ns.length : undefined);
	const duration = (seconds: number | undefined) => {
		if (seconds === undefined) return '—';
		const s = Math.round(seconds);
		if (s < 60) return `${s}s`;
		if (s < 3600) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`;
		return `${Math.floor(s / 3600)}h ${Math.round((s % 3600) / 60)}m`;
	};
	const quickest = $derived(onClock.toSorted((a, b) => a.seconds - b.seconds).slice(0, 3));
	const slowest = $derived(onClock.toSorted((a, b) => b.seconds - a.seconds).slice(0, 3));

	// ---- ages ----
	const YEAR = 365.2425 * 24 * 3600 * 1000;
	const ageAt = (p: DraftPick) => (p.player_birth_date ? (Date.parse(p.picked_at!) - Date.parse(p.player_birth_date)) / YEAR : undefined);
	const aged = $derived(made.flatMap((p) => (ageAt(p) === undefined ? [] : [{ pick: p, age: ageAt(p)! }])).toSorted((a, b) => a.age - b.age));
	const years = (age: number | undefined) => (age === undefined ? '—' : age.toFixed(1));

	// ---- where they come from ----
	function tally<T extends string>(keys: T[]): { key: T; count: number }[] {
		const counts = new Map<T, number>();
		for (const k of keys) if (k) counts.set(k, (counts.get(k) ?? 0) + 1);
		return [...counts].map(([key, count]) => ({ key, count })).toSorted((a, b) => b.count - a.count || a.key.localeCompare(b.key));
	}
	const sports = $derived(tally(made.map((p) => p.competition)));
	const positions = $derived(tally(made.map((p) => p.player_positions[0] ?? '')).slice(0, 8));
	const classes = $derived(tally(made.map((p) => p.player_class)));
	const teams = $derived(
		tally(made.filter((p) => p.player_team).map((p) => `${p.competition}|${p.player_team}`))
			.slice(0, 6)
			.map(({ key, count }) => ({ competition: key.split('|')[0], abbrev: key.split('|')[1], count }))
	);
	const autos = $derived(made.filter((p) => p.auto_picked).length);

	// ---- each team ----
	const byTeam = $derived(
		franchises
			.map((f) => {
				const theirs = made.filter((p) => p.current_franchise_id === f.id);
				return {
					franchise: f,
					picks: theirs.length,
					age: mean(theirs.flatMap((p) => (ageAt(p) === undefined ? [] : [ageAt(p)!]))),
					time: median(onClock.filter((t) => t.pick.current_franchise_id === f.id).map((t) => t.seconds)),
					auto: theirs.filter((p) => p.auto_picked).length,
					sports: tally(theirs.map((p) => p.competition))
				};
			})
			.filter((t) => t.picks > 0)
			.toSorted((a, b) => b.picks - a.picks || a.franchise.name.localeCompare(b.franchise.name))
	);
	const sportCount = $derived(sports.length);
</script>

{#snippet bars(rows: { label: string; count: number; sport?: string }[], total: number)}
	<ul class="bars">
		{#each rows as row (row.label)}
			<li title="{row.label}: {row.count} of {total} picks">
				<span class="label">{#if row.sport}<SportBadge sport={row.sport} />{:else}{row.label}{/if}</span>
				<span class="track"><span class="fill" data-sport={row.sport} style:width="{(row.count / Math.max(1, rows[0].count)) * 100}%"></span></span>
				<span class="value">{row.count}</span>
			</li>
		{/each}
	</ul>
{/snippet}

{#snippet pickRow(pick: DraftPick, figure: string)}
	<li>
		<span class="n">#{pick.position}</span>
		<span class="who"><strong>{pick.player_name}</strong><span class="muted">{name(pick.current_franchise_id)}</span></span>
		<span class="figure">{figure}</span>
	</li>
{/snippet}

<div class="stats">
	{#if made.length === 0}
		<p class="muted empty">Stats fill in as picks are made.</p>
	{:else}
		<div class="tiles">
			<div class="tile">
				<small>Picks made</small>
				<strong>{made.length}<span class="of">/{picks.length}</span></strong>
				<span class="meter"><span style:width="{(made.length / Math.max(1, picks.length)) * 100}%"></span></span>
			</div>
			<div class="tile">
				<small>Median time per pick</small>
				<strong>{duration(median(onClock.map((t) => t.seconds)))}</strong>
				<span class="muted">average {duration(mean(onClock.map((t) => t.seconds)))}</span>
			</div>
			<div class="tile">
				<small>Average age</small>
				<strong>{years(mean(aged.map((a) => a.age)))}</strong>
				{#if aged.length}<span class="muted">{years(aged[0].age)} – {years(aged.at(-1)!.age)}</span>{/if}
			</div>
			<div class="tile">
				<small>Time drafting</small>
				<strong>{duration(onClock.reduce((n, t) => n + t.seconds, 0))}</strong>
				<span class="muted">{breaks ? `${breaks} ${breaks === 1 ? 'break' : 'breaks'} left out` : 'no breaks'}</span>
			</div>
			<div class="tile">
				<small>Auto picks</small>
				<strong>{autos}</strong>
				<span class="muted">{Math.round((autos / made.length) * 100)}% of picks</span>
			</div>
		</div>

		<div class="cards">
			{#if onClock.length}
				<section class="card">
					<h3>Quickest picks</h3>
					<ol class="picks">{#each quickest as t (t.pick.id)}{@render pickRow(t.pick, duration(t.seconds))}{/each}</ol>
					<h3>Longest on the clock</h3>
					<ol class="picks">{#each slowest as t (t.pick.id)}{@render pickRow(t.pick, duration(t.seconds))}{/each}</ol>
				</section>
			{/if}
			{#if aged.length}
				<section class="card">
					<h3>Youngest</h3>
					<ol class="picks">{#each aged.slice(0, 3) as a (a.pick.id)}{@render pickRow(a.pick, years(a.age))}{/each}</ol>
					<h3>Oldest</h3>
					<ol class="picks">{#each aged.slice(-3).toReversed() as a (a.pick.id)}{@render pickRow(a.pick, years(a.age))}{/each}</ol>
				</section>
			{/if}
			<section class="card">
				{#if sportCount > 1}
					<h3>By sport</h3>
					{@render bars(sports.map((s) => ({ label: s.key.toUpperCase(), count: s.count, sport: s.key })), made.length)}
				{/if}
				<h3>By position</h3>
				{@render bars(positions.map((p) => ({ label: p.key, count: p.count })), made.length)}
				{#if classes.length}
					<h3>By year in college</h3>
					{@render bars(classes.map((c) => ({ label: c.key, count: c.count })), made.length)}
				{/if}
			</section>
			{#if teams.length}
				<section class="card">
					<h3>Most drafted from</h3>
					<ol class="from">
						{#each teams as t (t.competition + t.abbrev)}
							<li><TeamLogo competition={t.competition} abbrev={t.abbrev} size={28} /><span class="value">{t.count}</span></li>
						{/each}
					</ol>
				</section>
			{/if}
		</div>

		<section class="card flush">
			<h3 class="pad">By team</h3>
			<div class="scroll">
				<table>
					<thead>
						<tr>
							<th>Team</th>
							<th class="num">Picks</th>
							<th class="num">Avg age</th>
							<th class="num">Median pick</th>
							<th class="num">Auto</th>
							{#if showSport}<th>Sports</th>{/if}
						</tr>
					</thead>
					<tbody>
						{#each byTeam as t (t.franchise.id)}
							<tr>
								<td><span class="team"><Crest src={t.franchise.image_url} name={t.franchise.name} size={24} />{t.franchise.name}</span></td>
								<td class="num">{t.picks}</td>
								<td class="num">{years(t.age)}</td>
								<td class="num">{duration(t.time)}</td>
								<td class="num">{t.auto || ''}</td>
								{#if showSport}
									<td><span class="mix">{#each t.sports as s (s.key)}<span><SportBadge sport={s.key} /> {s.count}</span>{/each}</span></td>
								{/if}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}
</div>

<style>
	.stats {
		display: grid;
		gap: 0.75rem;
		padding: 0.75rem;
		overflow: auto;
		min-height: 0;
		flex: 1;
		align-content: start;
		/* Each section keeps its full height and the pane scrolls, rather than squeezing the table away. */
		grid-auto-rows: max-content;
	}
	.empty {
		padding: 1rem;
	}
	small {
		font: 600 0.62rem/1.2 var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.muted {
		color: var(--ink-soft);
	}

	/* ---- the headline numbers ---- */
	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(9.5rem, 1fr));
		gap: 0.5rem;
	}
	.tile {
		display: grid;
		gap: 0.2rem;
		align-content: start;
		padding: 0.7rem 0.8rem;
		border: 1px solid var(--rule);
		border-radius: var(--radius);
		background: var(--surface);
		font-size: 0.75rem;
	}
	.tile strong {
		font: 700 1.5rem/1.1 var(--display);
		font-variant-numeric: tabular-nums;
	}
	.of {
		font-size: 0.95rem;
		color: var(--ink-faint);
	}
	.meter {
		height: 4px;
		margin-top: 0.2rem;
		border-radius: 4px;
		background: var(--rule);
		overflow: hidden;
	}
	.meter span {
		display: block;
		height: 100%;
		border-radius: 4px;
		background: var(--brand);
	}

	/* ---- the breakdowns ---- */
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
		gap: 0.5rem;
		align-items: start;
	}
	.card {
		display: grid;
		gap: 0.4rem;
		padding: 0.7rem 0.8rem;
	}
	.card.flush {
		padding: 0;
	}
	h3 {
		font: 700 0.66rem/1.2 var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-soft);
	}
	h3:not(:first-child) {
		margin-top: 0.4rem;
	}
	.pad {
		padding: 0.7rem 0.8rem 0.3rem;
	}
	.picks,
	.bars,
	.from {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.3rem;
		font-size: 0.78rem;
	}
	.picks li {
		display: grid;
		grid-template-columns: 2.2rem minmax(0, 1fr) auto;
		gap: 0.5rem;
		align-items: baseline;
	}
	.n {
		font: 600 0.7rem var(--mono);
		color: var(--ink-faint);
	}
	.who {
		display: grid;
		min-width: 0;
	}
	.who > * {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.who .muted {
		font-size: 0.7rem;
	}
	.figure,
	.value {
		font: 700 0.8rem var(--mono);
		font-variant-numeric: tabular-nums;
	}
	.bars li {
		display: grid;
		grid-template-columns: 3.4rem minmax(0, 1fr) 1.8rem;
		gap: 0.5rem;
		align-items: center;
	}
	.label {
		font: 600 0.72rem var(--mono);
		color: var(--ink-soft);
	}
	.track {
		height: 8px;
	}
	.fill {
		display: block;
		height: 100%;
		min-width: 4px;
		border-radius: 0 4px 4px 0;
		background: var(--sport, var(--brand));
	}
	.value {
		text-align: right;
	}
	.from {
		grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr));
	}
	.from li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.4rem;
		padding: 0.3rem 0.5rem;
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
	}

	/* ---- each team ---- */
	.scroll {
		overflow-x: auto;
	}
	.team {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		white-space: nowrap;
	}
	.mix {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem 0.6rem;
		font: 600 0.75rem var(--mono);
	}
</style>
