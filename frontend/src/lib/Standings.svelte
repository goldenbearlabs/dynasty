<script lang="ts">
	// Each league's table for its current season, and the cross-sport table
	// when the dynasty crowns an overall champion.
	import { teamIdentity } from '#lib/identity.ts';
	import type { Dynasty, TeamIdentity, Franchise, League, Odds, OverallYear, Standings } from '#lib/api.ts';
	import Matchups from '#lib/Matchups.svelte';
	import Crest from '#lib/ui/Crest.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { points } from '#lib/ui/time.ts';
	import { page } from '$app/state';

	type Props = { tables: Standings[]; overall: OverallYear[]; franchises: Franchise[]; leagues: League[]; identities?: TeamIdentity[]; odds?: Odds };
	let { tables, overall, franchises, leagues, identities = [], odds }: Props = $props();

	const franchise = (id: string | null) => franchises.find((f) => f.id === id);
	const latest = $derived(overall[0]);

	let shown = $state('');
	const tabs = $derived([
		...tables.map((t) => ({ value: t.competition, label: t.competition.toUpperCase(), sport: t.competition })),
		...(latest ? [{ value: 'overall', label: 'Overall' }] : [])
	]);
	const table = $derived(tables.find((t) => t.competition === shown) ?? (shown === 'overall' ? undefined : tables[0]));
	const headToHead = $derived(table?.format === 'head_to_head');
	const league = $derived(leagues.find((l) => l.id === table?.league_id));
	// The last playoff place, which a line follows.
	const cutoff = $derived(headToHead && league ? league.settings.format.playoff_teams : 0);

	// Simulated chances, shown while a season is still being played.
	const percent = (p: number) => `${(p * 100).toFixed(1)}%`;
	const leagueOdds = $derived(
		table?.season?.status === 'active' && headToHead ? odds?.leagues.find((l) => l.league_id === table.league_id) : undefined
	);
	const playoffOdds = (id: string) => leagueOdds?.rows.find((r) => r.franchise_id === id);
	const titleOdds = (id: string) => (latest && !latest.final ? odds?.overall.find((r) => r.franchise_id === id) : undefined);
	const rules = $derived(
		((page.data.dynasty as Dynasty | null)?.settings.overall_title.points_by_finish ?? []).map((p, i) => `${p} for ${i + 1}${['st', 'nd', 'rd'][i] ?? 'th'}`)
	);
	const record = (r: { wins: number; losses: number; ties: number }) => `${r.wins}-${r.losses}${r.ties ? `-${r.ties}` : ''}`;
</script>

<div class="stack tight">
	<Tabs {tabs} bind:value={() => shown || tables[0]?.competition || '', (v) => (shown = v)} label="Standings" />

	{#if table}
		{#if !table.season}
			<Empty icon="trophy" title="No season yet">The commissioner starts each league's season. Points count from its first day.</Empty>
		{:else}
			{@const champion = franchise(table.season.champion_franchise_id)}
			{#if champion}
				<p class="card champion" data-sport={table.competition}>
					<Icon name="trophy" size={22} />
					<span><strong>{teamIdentity(champion,identities,league?.id).name}</strong> won the {table.season.year} title.</span>
				</p>
			{/if}
			{#if table.consolation_champion_id}<p class="card">{teamIdentity(franchise(table.consolation_champion_id),identities,league?.id).name} won the consolation bracket.</p>{/if}
			<div class="panel scroll" data-sport={table.competition}>
				<table>
					<thead>
						<tr>
							<th class="rank">#</th>
							<th>Franchise</th>
							{#if headToHead}<th class="num">Record</th>{/if}
							<th class="num">{headToHead ? 'Points for' : 'Points'}</th>
							{#if headToHead}<th class="num wide">Points against</th>{/if}
							{#if leagueOdds}<th class="num" title="Chance of a playoff place, from simulating the rest of the season">Playoffs</th>{/if}
						</tr>
					</thead>
					<tbody>
						{#each table.rows as row, i (row.franchise_id)}
							{@const f = franchise(row.franchise_id)}
							{@const chance = playoffOdds(row.franchise_id)}
							<tr class:cut={i === cutoff - 1 && i < table.rows.length - 1}>
								<td class="rank">{i + 1}</td>
								<td>
									<a class="who" href="/franchise/{f?.slug}">
										<Crest name={teamIdentity(f,identities,league?.id).name} src={teamIdentity(f,identities,league?.id).image_url} size={28} />
 <strong>{teamIdentity(f,identities,league?.id).name}</strong>
									</a>
								</td>
								{#if headToHead}
									<td class="num total">{row.wins}-{row.losses}{row.ties ? `-${row.ties}` : ''}</td>
								{/if}
								<td class="num" class:total={!headToHead}>{points(headToHead ? row.points_for : row.points)}</td>
								{#if headToHead}<td class="num wide">{points(row.points_against)}</td>{/if}
								{#if leagueOdds}
									<td class="num odds" title={chance ? `${chance.clinched ? 'Clinched. ' : chance.eliminated ? 'Eliminated. ' : ''}Projected ${chance.wins.toFixed(1)}-${chance.losses.toFixed(1)}` : undefined}>{chance ? percent(chance.playoffs) : '–'}</td>
								{/if}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			<p class="muted small-text">
				{table.season.year} season, {table.season.status === 'complete' ? 'final' : 'in progress'}. Only starters score.
				{#if cutoff}The line marks the playoff cutoff.{/if}
				{#if leagueOdds}Playoff chances come from playing out the rest of the season thousands of times: results so far are fixed, and the matchups further out are less certain. Only a clinched or eliminated team shows 100% or 0%.{/if}
				{#if headToHead && league}Ranked by {league.settings.format.standings_by === 'win_percentage' ? 'win percentage' : league.settings.format.standings_by === 'points' ? 'points for' : 'wins, then fewer losses'}; tiebreakers: {(league.settings.format.tiebreakers ?? ['points_for','head_to_head','points_against']).map((r) => r === 'points_for' ? 'points for' : r === 'head_to_head' ? 'head-to-head' : 'points against (higher first)').join(', ')}.{/if}
			</p>
			{#if headToHead && league}
				{#key league.id}<Matchups {league} {franchises} {identities} />{/key}
			{/if}
		{/if}
	{:else if latest}
		<div class="panel scroll">
			<table>
				<thead>
					<tr>
						<th class="rank">#</th>
						<th>Franchise</th>
						{#each tables as t (t.league_id)}<th class="num wide">{t.competition}</th>{/each}
						<th class="num">Record</th>
						<th class="num" title="Points for above each league's median, added up across leagues">Adj. PF</th>
						<th class="num">Points</th>
						{#if odds && !latest.final}<th class="num" title="Chance of winning the overall title, from the simulated seasons">Title</th>{/if}
					</tr>
				</thead>
				<tbody>
					{#each latest.rows as row, i (row.franchise_id)}
						{@const f = franchise(row.franchise_id)}
						<tr>
							<td class="rank">{i + 1}</td>
							<td>
								<a class="who" href="/franchise/{f?.slug}">
									<Crest src={f?.image_url} name={f?.name ?? ''} size={28} />
									<strong>{f?.name}</strong>
								</a>
							</td>
							{#each tables as t (t.league_id)}
								<td class="num muted wide">{row.finishes[t.competition] ?? '–'}</td>
							{/each}
							<td class="num">{record(row)}</td>
							<td class="num" class:up={row.adjusted_points > 0} class:down={row.adjusted_points < 0}>{row.adjusted_points > 0 ? '+' : ''}{points(row.adjusted_points)}</td>
							<td class="num total">{row.points}</td>
							{#if odds && !latest.final}<td class="num odds">{percent(titleOdds(row.franchise_id)?.title ?? 0)}</td>{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="muted small-text">
			{latest.year} overall title, {latest.final ? 'final' : 'in progress'}: points for each franchise's place in each sport
			{#if rules.length}({rules.join(', ')}){/if}, ties broken by adjusted points for: points for above each league's median, added up.
			{#if odds && !latest.final}Title chances combine every league's simulated seasons.{/if}
		</p>
	{/if}
</div>

<style>
	.champion {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding: 0.8rem 1rem;
		background: var(--gold-soft);
		color: var(--gold);
		border-color: transparent;
	}
	.champion span {
		color: var(--ink);
	}
	.rank {
		width: 2.5rem;
		font: 700 0.85rem var(--mono);
		color: var(--ink-faint);
	}
	.who {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		color: var(--ink);
	}
	.total {
		font: 750 1.05rem var(--display);
	}
	th.num {
		text-align: right;
	}
	/* The playoff cutoff: a line under the last team in. */
	tr.cut td {
		border-bottom: 2px solid var(--sport, var(--brand));
	}
	.odds {
		font: 650 0.85rem var(--mono);
	}
	.up {
		color: var(--good);
	}
	.down {
		color: var(--bad);
	}
	@media (max-width: 640px) {
		.wide {
			display: none;
		}
	}
</style>
