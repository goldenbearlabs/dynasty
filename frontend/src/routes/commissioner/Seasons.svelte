<script lang="ts">
	// Start each league's season, set when its games count, and close it
	// with a champion.
	import { invalidateAll } from '$app/navigation';
	import {
		advancePlayoffs,
		closeSeason,
		createSeason,
		generateSchedule,
		getSeasons,
		getStandings,
		reopenSeason,
		setSeasonDates,
		type Competition,
		type Dynasty,
		type League,
		type Season,
		type Standings
	} from '#lib/api.ts';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { dynasty, competitions }: { dynasty: Dynasty; competitions: Competition[] } = $props();

	let seasons = $state<Season[]>([]);
	let standings = $state<Standings[]>([]);
	// The form for each league: a new season's year and dates, or the
	// current season's dates and the champion to name.
	let forms = $state<Record<string, { year: number; starts_on: string; ends_on: string; champion: string }>>({});

	const latest = (league: League) => seasons.find((s) => s.league_id === league.id); // newest first
	const table = (league: League) => standings.find((s) => s.league_id === league.id);
	const name = (id: string | null) => dynasty.franchises.find((f) => f.id === id)?.name ?? '';

	// The usual real-season dates for a sport, as a starting point.
	function usualDates(league: League, year: number) {
		const [from, to] = competitions.find((c) => c.key === league.competition)!.season;
		return { starts_on: `${year}-${from}`, ends_on: `${to < from ? year + 1 : year}-${to}` };
	}

	async function load() {
		[seasons, standings] = await Promise.all([getSeasons(), getStandings()]);
		for (const league of dynasty.leagues) {
			const season = latest(league);
			const leader = table(league)?.rows[0]?.franchise_id ?? '';
			forms[league.id] =
				season?.status === 'active'
					? { year: season.year, starts_on: season.starts_on, ends_on: season.ends_on, champion: leader }
					: (() => {
							const year = season ? season.year + 1 : new Date().getFullYear();
							return { year, ...usualDates(league, year), champion: '' };
						})();
		}
	}
	load().catch(toast.error);

	const run = (work: Promise<unknown>, done: string) =>
		work
			.then(load)
			.then(invalidateAll)
			.then(() => toast.good(done), toast.error);

	function close(league: League, season: Season) {
		const champion = forms[league.id].champion;
		if (confirm(`Close the ${season.year} ${league.name} season and name ${name(champion)} champion?`)) {
			run(closeSeason(season.id, champion), `${name(champion)} are the ${season.year} ${league.name} champions.`);
		}
	}
</script>

<section class="stack tight">
	<p class="muted">
		A season says which games count. Points come from each franchise's starters between its first and last day. Closing a
		season names the champion.
	</p>

	{#each dynasty.leagues as league (league.id)}
		{@const season = latest(league)}
		{@const form = forms[league.id]}
		{#if form}
			<div class="card stack" data-sport={league.competition}>
				<div class="spread">
					<div class="row">
						<SportBadge sport={league.competition} solid />
						<h2>{league.name}</h2>
					</div>
					{#if season?.status === 'active'}
						<span class="pill good">{season.year} season in progress</span>
					{:else if season}
						<span class="pill gold">{season.year} champion: {name(season.champion_franchise_id)}</span>
					{:else}
						<span class="pill">No season yet</span>
					{/if}
				</div>

				{#if season?.status === 'active'}
					<div class="row end">
						<label class="field">First day <input type="date" bind:value={form.starts_on} /></label>
						<label class="field">Last day <input type="date" bind:value={form.ends_on} /></label>
						<button onclick={() => run(setSeasonDates(season.id, form.starts_on, form.ends_on), 'Season dates saved.')}>Save dates</button>
					</div>
					{#if league.settings.format.type === 'head_to_head'}
						<div class="row schedule">
							<span class="muted small-text">
								Head-to-head: {league.settings.format.matchup_days}-day matchups, {league.settings.format.playoff_teams} playoff teams.
								The schedule is built when the season starts and kept up to date when the dates, rules or franchises change.
							</span>
							<button class="small" onclick={() => run(generateSchedule(season.id), 'Schedule rebuilt from tomorrow on.')}>
								Rebuild schedule
							</button>
							<button class="small" onclick={() => run(advancePlayoffs(), 'Playoff bracket is up to date.')}>Update playoff bracket</button>
						</div>
					{/if}
					<div class="row end close">
						<label class="field">
							Champion
							<select bind:value={form.champion}>
								{#each table(league)?.rows ?? [] as row (row.franchise_id)}
									<option value={row.franchise_id}>{name(row.franchise_id)} · {points(row.points)} pts</option>
								{/each}
							</select>
						</label>
						<button class="primary" onclick={() => close(league, season)}>Close season</button>
					</div>
				{:else}
					{#if season}
						<p class="muted small-text">
							{season.year} ran {dayLabel(season.starts_on)} to {dayLabel(season.ends_on)}.
							<button class="quiet small" onclick={() => run(reopenSeason(season.id), 'Season reopened.')}>Reopen it</button>
						</p>
					{/if}
					<div class="row end">
						<label class="field">Year <input type="number" bind:value={form.year} /></label>
						<label class="field">First day <input type="date" bind:value={form.starts_on} /></label>
						<label class="field">Last day <input type="date" bind:value={form.ends_on} /></label>
						<button
							class="primary"
							onclick={() =>
								run(
									createSeason({ league_id: league.id, year: form.year, starts_on: form.starts_on, ends_on: form.ends_on }),
									`${form.year} ${league.name} season started.`
								)}
						>
							Start {form.year} season
						</button>
					</div>
				{/if}
			</div>
		{/if}
	{/each}
</section>

<style>
	.schedule {
		padding-top: 0.2rem;
	}
	.close {
		padding-top: 1rem;
		border-top: 1px solid var(--rule);
	}
</style>
