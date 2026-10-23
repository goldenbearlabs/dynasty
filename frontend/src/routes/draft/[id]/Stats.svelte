<script lang="ts">
	// The draft by the numbers: how long picks take, how old the players
	// are, where they come from, and each team's draft side by side, for the
	// whole draft or one team, sport or round. It describes what happened;
	// it does not judge any pick.
	import { page } from '$app/state';
	import type { Draft, DraftPick, Franchise, ProTeam } from '#lib/api.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';

	let { draft, picks, franchises, showSport }: { draft: Draft; picks: DraftPick[]; franchises: Franchise[]; showSport: boolean } = $props();

	const name = (id: string) => franchises.find((f) => f.id === id)?.name ?? 'Unknown team';

	// ---- filters ----
	let team = $state('');
	let sport = $state('');
	let round = $state(0);
	const rounds = $derived(Math.max(0, ...picks.map((p) => p.round)));
	const shown = (p: DraftPick) =>
		(!team || p.current_franchise_id === team) && (!sport || p.competition === sport) && (!round || p.round === round);
	const filtered = $derived(team !== '' || sport !== '' || round !== 0);

	// Every pick made, in the order made; then those the filters keep.
	const allMade = $derived(picks.filter((p) => p.player_id && p.picked_at).toSorted((a, b) => a.picked_at!.localeCompare(b.picked_at!)));
	const made = $derived(allMade.filter(shown));
	const slots = $derived(picks.filter(shown));

	// ---- time on the clock ----
	// A pick took from the pick made before it, filtered or not. A gap far
	// longer than the clock is a break (a pause, a night's sleep) and is left out.
	const longest = $derived(Math.max(30 * 60, draft.pick_clock_seconds * 3));
	const timedAll = $derived(
		allMade.slice(1).map((pick, i) => ({ pick, seconds: (Date.parse(pick.picked_at!) - Date.parse(allMade[i].picked_at!)) / 1000 }))
	);
	const timed = $derived(timedAll.filter((t) => shown(t.pick)));
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
	const pickTime = $derived(median(onClock.map((t) => t.seconds)));
	// At the pace so far, how long the picks still to make will take.
	const remaining = $derived(picks.filter((p) => !p.player_id && !p.passed_at).length);

	// ---- ages ----
	const YEAR = 365.2425 * 24 * 3600 * 1000;
	const ageAt = (p: DraftPick) => (p.player_birth_date ? (Date.parse(p.picked_at!) - Date.parse(p.player_birth_date)) / YEAR : undefined);
	const aged = $derived(made.flatMap((p) => (ageAt(p) === undefined ? [] : [{ pick: p, age: ageAt(p)! }])).toSorted((a, b) => a.age - b.age));
	const years = (age: number | undefined) => (age === undefined ? '—' : age.toFixed(1));
	const ageBands = $derived.by(() => {
		const bands = ['20 or under', '21', '22', '23', '24', '25–27', '28 or over'];
		const band = (age: number) => (age < 21 ? 0 : age < 22 ? 1 : age < 23 ? 2 : age < 24 ? 3 : age < 25 ? 4 : age < 28 ? 5 : 6);
		const counts = bands.map(() => 0);
		for (const a of aged) counts[band(a.age)]++;
		return bands.map((label, i) => ({ label, count: counts[i] })).filter((b, i, all) => b.count > 0 || (i > all.findIndex((x) => x.count > 0) && i < all.findLastIndex((x) => x.count > 0)));
	});

	// ---- where they come from ----
	function tally(keys: string[]): { key: string; count: number }[] {
		const counts = new Map<string, number>();
		for (const k of keys) if (k) counts.set(k, (counts.get(k) ?? 0) + 1);
		return [...counts].map(([key, count]) => ({ key, count })).toSorted((a, b) => b.count - a.count || a.key.localeCompare(b.key));
	}
	const sports = $derived(tally(made.map((p) => p.competition)));
	const positions = $derived(tally(made.map((p) => p.player_positions[0] ?? '')));
	const classes = $derived(tally(made.map((p) => p.player_class)));
	const proTeams = $derived(
		tally(made.filter((p) => p.player_team).map((p) => `${p.competition}|${p.player_team}`)).map(({ key, count }) => ({
			competition: key.split('|')[0],
			abbrev: key.split('|')[1],
			count
		}))
	);
	// Teams in the pro leagues here that nobody has drafted from. College
	// is left out: most of its teams are not in the pool at all.
	const undrafted = $derived(
		((page.data.teams as ProTeam[] | undefined) ?? [])
			.filter((t) => t.competition !== 'cbb' && sports.some((s) => s.key === t.competition))
			.filter((t) => !proTeams.some((p) => p.competition === t.competition && p.abbrev === t.abbrev))
	);
	// A franchise taking two or more players from the same real team.
	const stacks = $derived(
		tally(made.filter((p) => p.player_team).map((p) => `${p.current_franchise_id}|${p.competition}|${p.player_team}`))
			.filter((s) => s.count > 1)
			.map(({ key, count }) => {
				const [franchise, competition, abbrev] = key.split('|');
				return { franchise, competition, abbrev, count };
			})
	);
	// The longest run of picks, in draft order, at one position.
	const positionRun = $derived.by(() => {
		const ordered = made.toSorted((a, b) => a.position - b.position);
		let best = { position: '', length: 0, from: 0, to: 0 };
		let run = { position: '', length: 0, from: 0 };
		for (const p of ordered) {
			const position = p.player_positions[0] ?? '';
			run = position && position === run.position ? { ...run, length: run.length + 1 } : { position, length: 1, from: p.position };
			if (run.length > best.length) best = { ...run, to: p.position };
		}
		return best;
	});
	const autos = $derived(made.filter((p) => p.auto_picked).length);
	const traded = $derived(made.filter((p) => p.original_franchise_id !== p.current_franchise_id).length);
	const passed = $derived(slots.filter((p) => p.passed_at).length);

	// ---- each round ----
	const byRound = $derived(
		Array.from({ length: rounds }, (_, i) => i + 1)
			.filter((r) => !round || r === round)
			.map((r) => {
				const theirs = made.filter((p) => p.round === r);
				return {
					round: r,
					picks: theirs.length,
					of: slots.filter((p) => p.round === r).length,
					age: mean(theirs.flatMap((p) => (ageAt(p) === undefined ? [] : [ageAt(p)!]))),
					time: median(onClock.filter((t) => t.pick.round === r).map((t) => t.seconds))
				};
			})
			.filter((r) => r.of > 0)
	);

	// ---- each team, sortable ----
	type Sort = 'name' | 'picks' | 'age' | 'time' | 'auto' | 'traded';
	let sortBy = $state<Sort>('picks');
	let ascending = $state(false);
	function sortOn(column: Sort) {
		ascending = sortBy === column ? !ascending : column === 'name' || column === 'age' || column === 'time';
		sortBy = column;
	}
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
					traded: theirs.filter((p) => p.original_franchise_id !== p.current_franchise_id).length,
					sports: tally(theirs.map((p) => p.competition))
				};
			})
			.filter((t) => t.picks > 0)
			.toSorted((a, b) => {
				const value = (t: typeof a) => (sortBy === 'name' ? t.franchise.name : (t[sortBy] ?? (ascending ? Infinity : -Infinity)));
				const [x, y] = [value(a), value(b)];
				const order = typeof x === 'string' ? x.localeCompare(y as string) : (x as number) - (y as number);
				return (ascending ? order : -order) || a.franchise.name.localeCompare(b.franchise.name);
			})
	);

	// ---- lists: five rows, or every row on request ----
	let open = $state<Record<string, boolean>>({});
	const SHORT = 5;
	const some = <T,>(list: T[], key: string) => (open[key] ? list : list.slice(0, SHORT));
</script>

{#snippet more(key: string, total: number)}
	{#if total > SHORT}
		<button class="more" onclick={() => (open[key] = !open[key])}>{open[key] ? 'Show fewer' : `Show all ${total}`}</button>
	{/if}
{/snippet}

{#snippet bars(key: string, rows: { label: string; count: number; sport?: string }[])}
	{@const top = Math.max(1, ...rows.map((r) => r.count))}
	<ul class="bars">
		{#each some(rows, key) as row (row.label)}
			<li title="{row.label}: {row.count} of {made.length} picks ({Math.round((row.count / Math.max(1, made.length)) * 100)}%)">
				<span class="label">{#if row.sport}<SportBadge sport={row.sport} />{:else}{row.label}{/if}</span>
				<span class="track">{#if row.count}<span class="fill" data-sport={row.sport} style:width="{(row.count / top) * 100}%"></span>{/if}</span>
				<span class="value">{row.count}</span>
				<span class="share">{Math.round((row.count / Math.max(1, made.length)) * 100)}%</span>
			</li>
		{/each}
	</ul>
	{@render more(key, rows.length)}
{/snippet}

{#snippet ranked(key: string, rows: { pick: DraftPick; figure: string }[])}
	<table class="ranked">
		<tbody>
			{#each some(rows, key) as row, i (row.pick.id)}
				<tr>
					<td class="rank">{i + 1}</td>
					<td class="who"><strong>{row.pick.player_name}</strong><span>#{row.pick.position} · {name(row.pick.current_franchise_id)}</span></td>
					<td class="figure">{row.figure}</td>
				</tr>
			{/each}
		</tbody>
	</table>
	{@render more(key, rows.length)}
{/snippet}

{#snippet sortHead(column: Sort, label: string, numeric = true)}
	<th class:num={numeric} aria-sort={sortBy === column ? (ascending ? 'ascending' : 'descending') : 'none'}>
		<button class="sort" class:on={sortBy === column} onclick={() => sortOn(column)}>
			{label}{#if sortBy === column}<span aria-hidden="true">{ascending ? ' ↑' : ' ↓'}</span>{/if}
		</button>
	</th>
{/snippet}

<div class="stats">
	<div class="filters" role="group" aria-label="Filter the stats">
		<label>
			<small>Team</small>
			<select bind:value={team}>
				<option value="">All teams</option>
				{#each franchises.toSorted((a, b) => a.name.localeCompare(b.name)) as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
			</select>
		</label>
		{#if showSport}
			<label>
				<small>Sport</small>
				<select bind:value={sport}>
					<option value="">All sports</option>
					{#each tally(allMade.map((p) => p.competition)) as s (s.key)}<option value={s.key}>{s.key.toUpperCase()}</option>{/each}
				</select>
			</label>
		{/if}
		{#if rounds > 1}
			<label>
				<small>Round</small>
				<select bind:value={round}>
					<option value={0}>All rounds</option>
					{#each Array.from({ length: rounds }, (_, i) => i + 1) as r (r)}<option value={r}>Round {r}</option>{/each}
				</select>
			</label>
		{/if}
		{#if filtered}<button class="quiet small" onclick={() => ((team = ''), (sport = ''), (round = 0))}>Clear filters</button>{/if}
	</div>

	{#if made.length === 0}
		<p class="muted empty">{filtered ? 'No picks made that match these filters yet.' : 'Stats fill in as picks are made.'}</p>
	{:else}
		<div class="tiles">
			<div class="tile">
				<small>Picks made</small>
				<strong>{made.length}<span class="of">/{slots.length}</span></strong>
				<span class="meter"><span style:width="{(made.length / Math.max(1, slots.length)) * 100}%"></span></span>
			</div>
			<div class="tile">
				<small>Median time per pick</small>
				<strong>{duration(pickTime)}</strong>
				<span class="muted">average {duration(mean(onClock.map((t) => t.seconds)))}</span>
			</div>
			<div class="tile">
				<small>Average age</small>
				<strong>{years(mean(aged.map((a) => a.age)))}</strong>
				{#if aged.length}<span class="muted">median {years(median(aged.map((a) => a.age)))} · {years(aged[0].age)}–{years(aged.at(-1)!.age)}</span>{/if}
			</div>
			<div class="tile">
				<small>Time drafting</small>
				<strong>{duration(onClock.reduce((n, t) => n + t.seconds, 0))}</strong>
				<span class="muted">{breaks ? `${breaks} ${breaks === 1 ? 'break' : 'breaks'} left out` : 'no breaks'}</span>
			</div>
			{#if !filtered && remaining > 0 && pickTime && draft.status !== 'complete'}
				<div class="tile">
					<small>At this pace</small>
					<strong>{duration(remaining * pickTime)}</strong>
					<span class="muted">to make the {remaining} picks left</span>
				</div>
			{/if}
			<div class="tile">
				<small>Auto picks</small>
				<strong>{autos}</strong>
				<span class="muted">{Math.round((autos / made.length) * 100)}% of picks</span>
			</div>
			<div class="tile">
				<small>Made with traded picks</small>
				<strong>{traded}</strong>
				<span class="muted">{passed ? `${passed} passed` : `${Math.round((traded / made.length) * 100)}% of picks`}</span>
			</div>
		</div>

		<div class="cards">
			{#if onClock.length}
				<section class="card">
					<h3>Quickest picks</h3>
					{@render ranked('quick', onClock.toSorted((a, b) => a.seconds - b.seconds).map((t) => ({ pick: t.pick, figure: duration(t.seconds) })))}
				</section>
				<section class="card">
					<h3>Longest on the clock</h3>
					{@render ranked('slow', onClock.toSorted((a, b) => b.seconds - a.seconds).map((t) => ({ pick: t.pick, figure: duration(t.seconds) })))}
				</section>
			{/if}
			{#if aged.length}
				<section class="card">
					<h3>Youngest</h3>
					{@render ranked('young', aged.map((a) => ({ pick: a.pick, figure: years(a.age) })))}
				</section>
				<section class="card">
					<h3>Oldest</h3>
					{@render ranked('old', aged.toReversed().map((a) => ({ pick: a.pick, figure: years(a.age) })))}
				</section>
				<section class="card">
					<h3>Ages</h3>
					{@render bars('ages', ageBands.map((b) => ({ label: b.label, count: b.count })))}
				</section>
			{/if}
			<section class="card">
				{#if sports.length > 1}
					<h3>By sport</h3>
					{@render bars('sports', sports.map((s) => ({ label: s.key.toUpperCase(), count: s.count, sport: s.key })))}
				{/if}
				<h3>By position</h3>
				{@render bars('positions', positions.map((p) => ({ label: p.key, count: p.count })))}
				{#if positionRun.length > 1}
					<p class="note">Longest run: <strong>{positionRun.length} {positionRun.position}</strong> in a row, picks #{positionRun.from}–#{positionRun.to}</p>
				{/if}
			</section>
			{#if classes.length}
				<section class="card">
					<h3>By year in college</h3>
					{@render bars('classes', classes.map((c) => ({ label: c.key, count: c.count })))}
				</section>
			{/if}
			{#if proTeams.length}
				<section class="card">
					<h3>Most drafted from</h3>
					<ol class="logos">
						{#each some(proTeams, 'most') as t (t.competition + t.abbrev)}
							<li><TeamLogo competition={t.competition} abbrev={t.abbrev} size={28} /><span class="value">{t.count}</span></li>
						{/each}
					</ol>
					{@render more('most', proTeams.length)}
				</section>
				<section class="card">
					<h3>Least drafted from</h3>
					<ol class="logos">
						{#each some(proTeams.toReversed(), 'least') as t (t.competition + t.abbrev)}
							<li><TeamLogo competition={t.competition} abbrev={t.abbrev} size={28} /><span class="value">{t.count}</span></li>
						{/each}
					</ol>
					{@render more('least', proTeams.length)}
					{#if undrafted.length}
						<h3>Not drafted from yet · {undrafted.length}</h3>
						<ol class="logos tight">
							{#each some(undrafted, 'none') as t (t.competition + t.abbrev)}
								<li><TeamLogo competition={t.competition} abbrev={t.abbrev} size={24} /></li>
							{/each}
						</ol>
						{@render more('none', undrafted.length)}
					{/if}
				</section>
			{/if}
			{#if stacks.length}
				<section class="card">
					<h3>Stacks · teammates on one roster</h3>
					<table class="ranked">
						<tbody>
							{#each some(stacks, 'stacks') as s (s.franchise + s.competition + s.abbrev)}
								<tr>
									<td class="who"><strong>{name(s.franchise)}</strong></td>
									<td><TeamLogo competition={s.competition} abbrev={s.abbrev} size={22} /></td>
									<td class="figure">{s.count}</td>
								</tr>
							{/each}
						</tbody>
					</table>
					{@render more('stacks', stacks.length)}
				</section>
			{/if}
		</div>

		{#if byRound.length > 1}
			<section class="card flush">
				<h3 class="pad">By round</h3>
				<div class="scroll">
					<table>
						<thead><tr><th>Round</th><th class="num">Picks</th><th class="num">Avg age</th><th class="num">Median pick</th></tr></thead>
						<tbody>
							{#each byRound as r (r.round)}
								<tr>
									<td>Round {r.round}</td>
									<td class="num">{r.picks}<span class="muted">/{r.of}</span></td>
									<td class="num">{years(r.age)}</td>
									<td class="num">{duration(r.time)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</section>
		{/if}

		{#if !team}
			<section class="card flush">
				<h3 class="pad">By team <span class="muted">· click a heading to sort</span></h3>
				<div class="scroll">
					<table>
						<thead>
							<tr>
								{@render sortHead('name', 'Team', false)}
								{@render sortHead('picks', 'Picks')}
								{@render sortHead('age', 'Avg age')}
								{@render sortHead('time', 'Median pick')}
								{@render sortHead('auto', 'Auto')}
								{@render sortHead('traded', 'Via trade')}
								{#if showSport}<th>Sports</th>{/if}
							</tr>
						</thead>
						<tbody>
							{#each byTeam as t (t.franchise.id)}
								<tr>
									<td><button class="team" title="Show only {t.franchise.name}" onclick={() => (team = t.franchise.id)}><Crest src={t.franchise.image_url} name={t.franchise.name} size={24} />{t.franchise.name}</button></td>
									<td class="num">{t.picks}</td>
									<td class="num">{years(t.age)}</td>
									<td class="num">{duration(t.time)}</td>
									<td class="num">{t.auto || ''}</td>
									<td class="num">{t.traded || ''}</td>
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
	{/if}
</div>

<style>
	.stats {
		display: grid;
		gap: 0.75rem;
		padding: 0 0.75rem 0.75rem;
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
		font-weight: 400;
	}

	/* ---- filters, kept in view as the pane scrolls ---- */
	.filters {
		position: sticky;
		top: 0;
		z-index: 2;
		display: flex;
		flex-wrap: wrap;
		align-items: end;
		gap: 0.5rem 0.75rem;
		margin-inline: -0.75rem;
		padding: 0.6rem 0.75rem;
		background: var(--surface);
		border-bottom: 1px solid var(--rule);
	}
	.filters label {
		display: grid;
		gap: 0.2rem;
	}
	.filters select {
		min-width: 9rem;
		padding: 0.3rem 0.45rem;
		font-size: 0.82rem;
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
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
		gap: 0.5rem;
		align-items: start;
	}
	.card {
		display: grid;
		gap: 0.45rem;
		padding: 0.75rem 0.85rem;
	}
	.card.flush {
		padding: 0;
	}
	h3 {
		font: 700 0.68rem/1.2 var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-soft);
	}
	h3:not(:first-child) {
		margin-top: 0.5rem;
	}
	h3 .muted {
		text-transform: none;
		letter-spacing: 0;
		font-family: var(--body);
	}
	.pad {
		padding: 0.75rem 0.85rem 0.35rem;
	}
	.note {
		font-size: 0.78rem;
		color: var(--ink-soft);
	}
	.more {
		justify-self: start;
		padding: 0;
		border: 0;
		background: none;
		color: var(--brand);
		font-size: 0.75rem;
		font-weight: 600;
	}

	/* A numbered list as a table: rank, who, the figure. */
	.ranked {
		width: 100%;
		font-size: 0.8rem;
	}
	.ranked td {
		padding: 0.3rem 0.4rem;
		border-bottom: 1px solid var(--rule);
		vertical-align: middle;
	}
	.ranked tr:last-child td {
		border-bottom: 0;
	}
	.rank {
		width: 1.6rem;
		font: 600 0.72rem var(--mono);
		color: var(--ink-faint);
	}
	.ranked .who {
		max-width: 0;
		width: 100%;
	}
	.who strong,
	.who span {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.who span {
		font-size: 0.7rem;
		color: var(--ink-soft);
	}
	.figure,
	.value {
		font: 700 0.82rem var(--mono);
		font-variant-numeric: tabular-nums;
		text-align: right;
		white-space: nowrap;
	}

	.bars {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.35rem;
		font-size: 0.78rem;
	}
	.bars li {
		display: grid;
		grid-template-columns: 5.2rem minmax(0, 1fr) 1.8rem 2.4rem;
		gap: 0.5rem;
		align-items: center;
	}
	.label {
		font: 600 0.72rem var(--mono);
		color: var(--ink-soft);
		white-space: nowrap;
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
	.share {
		font: 400 0.72rem var(--mono);
		color: var(--ink-faint);
		text-align: right;
	}
	.logos {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr));
		gap: 0.35rem;
	}
	.logos li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.4rem;
		padding: 0.3rem 0.5rem;
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
	}
	.logos.tight {
		grid-template-columns: repeat(auto-fill, minmax(5rem, 1fr));
	}

	/* ---- each round, each team ---- */
	.scroll {
		overflow-x: auto;
	}
	.sort {
		padding: 0;
		border: 0;
		background: none;
		font: inherit;
		color: inherit;
		text-transform: inherit;
		letter-spacing: inherit;
	}
	.sort.on {
		color: var(--ink);
	}
	.team {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0;
		border: 0;
		background: none;
		font-weight: 600;
		white-space: nowrap;
	}
	.team:hover {
		color: var(--brand);
	}
	.mix {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem 0.6rem;
		font: 600 0.75rem var(--mono);
	}
</style>
