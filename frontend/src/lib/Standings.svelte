<script lang="ts">
	// Each league's table for its current season, and the cross-sport table
	// when the dynasty crowns an overall champion.
	import { teamIdentity } from '#lib/identity.ts';
 import type { TeamIdentity, Franchise, League, OverallYear, Standings } from '#lib/api.ts';
	import Matchups from '#lib/Matchups.svelte';
	import Crest from '#lib/ui/Crest.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { points } from '#lib/ui/time.ts';

	type Props = { tables: Standings[]; overall: OverallYear[]; franchises: Franchise[]; leagues: League[]; identities?: TeamIdentity[] };
	let { tables, overall, franchises, leagues, identities = [] }: Props = $props();

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
			<div class="panel scroll" data-sport={table.competition}>
				<table>
					<thead>
						<tr>
							<th class="rank">#</th>
							<th>Franchise</th>
							{#if headToHead}<th class="num">Record</th>{/if}
							<th class="wide">Top scorer</th>
							<th class="num">Points</th>
						</tr>
					</thead>
					<tbody>
						{#each table.rows as row, i (row.franchise_id)}
							{@const f = franchise(row.franchise_id)}
							<tr>
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
								<td class="wide muted small-text">
									{#if row.players[0]}{row.players[0].nickname ? `${row.players[0].full_name} “${row.players[0].nickname}”` : row.players[0].full_name} · {points(row.players[0].points)}{/if}
								</td>
								<td class="num" class:total={!headToHead}>{points(row.points)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			<p class="muted small-text">
				{table.season.year} season, {table.season.status === 'complete' ? 'final' : 'in progress'}. Only starters score.
				{#if headToHead}Ranked by record, then points.{/if}
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
						{#each tables as t (t.league_id)}<th class="num">{t.competition}</th>{/each}
						<th class="num">Points</th>
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
								<td class="num muted">{row.finishes[t.competition] ?? '–'}</td>
							{/each}
							<td class="num total">{row.points}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<p class="muted small-text">
			{latest.year} overall title, {latest.final ? 'final' : 'in progress'}: points for each franchise's place in each sport.
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
	@media (max-width: 640px) {
		.wide {
			display: none;
		}
	}
</style>
