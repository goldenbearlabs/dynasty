<script lang="ts">
	// How each league is played: its lineup, roster and scoring, read from
	// the league's own rules so the page is right whatever the commissioner
	// has changed, and why the starting rules are what they are.
	import type { Competition, LeagueSettings } from '#lib/api.ts';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Guide = { key: string; name: string; rules: LeagueSettings; sport: Competition; custom: boolean };
	// The dynasty's leagues with their rules, or every sport's recommended rules before there is one.
	const guides = $derived.by<Guide[]>(() => {
		const sportOf = (key: string) => data.competitions.find((c) => c.key === key)!;
		if (data.dynasty) {
			return data.dynasty.leagues.map((l) => ({ key: l.competition, name: l.name, rules: l.settings, sport: sportOf(l.competition), custom: true }));
		}
		return data.competitions.map((c) => ({ key: c.key, name: c.name, rules: c.defaults, sport: c, custom: false }));
	});

	const starters = (rules: LeagueSettings) => rules.lineup.slots.reduce((n, s) => n + s.count, 0);
	const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
	const signed = (n: number) => (n > 0 ? `+${n}` : `${n}`);
	const positions = (list: string[]) => (list.includes('*') ? 'anyone' : list.join(', '));
	const scoring = (g: Guide) =>
		g.sport.stats.filter((s) => g.rules.scoring[s.key]).map((s) => ({ label: s.label, points: g.rules.scoring[s.key] }));
	// Scoring written out the same way whatever order its stats were saved in.
	const signature = (points: Record<string, number>) =>
		JSON.stringify(Object.entries(points).filter(([, v]) => v !== 0).sort(([a], [b]) => a.localeCompare(b)));
	// The other leagues scored exactly like this one.
	const sameScoring = (g: Guide) => guides.filter((o) => o.key !== g.key && signature(o.rules.scoring) === signature(g.rules.scoring));
	const names = (list: Guide[]) => list.map((o) => o.name).join(list.length === 2 ? ' and ' : ', ').replace(/, ([^,]*)$/, ' and $1');
	// Whether a league is still scored the way its sport was balanced.
	const recommended = (g: Guide) => signature(g.rules.scoring) === signature(g.sport.defaults.scoring);
	const limited = (rules: LeagueSettings) => rules.lineup.slots.filter((s) => s.games_per_week > 0);

	// A first-round player's effect on a weekly matchup under the recommended
	// rules, from last season's games. See "How the rules were balanced".
	const balance = [
		{ key: 'nba', top: 29.2, first: 18.9, third: 11.3 },
		{ key: 'nfl', top: 21.3, first: 15.7, third: 8.3 },
		{ key: 'cbb', top: 18.9, first: 13.8, third: 7.0 },
		{ key: 'nhl', top: 16.5, first: 11.6, third: 7.4 },
		{ key: 'mlb', top: 12.0, first: 8.0, third: 5.3 },
		{ key: 'wnba', top: 41.3, first: 30.0, third: 15.2 }
	];
	const nameOf = (key: string) => data.competitions.find((c) => c.key === key)?.name ?? key.toUpperCase();
</script>

<svelte:head><title>Rules</title></svelte:head>

<div class="stack">
	<header class="stack tight">
		<h1>Rules</h1>
		<p class="muted">
			One franchise, a team in every sport.
			{#if data.dynasty}These are the rules each league is playing under right now.{:else}These are the recommended rules for each sport.{/if}
		</p>
		<nav class="row jump" aria-label="Leagues">
			{#each guides as g (g.key)}<a href="#{g.key}"><SportBadge sport={g.key} /></a>{/each}
			<a class="small-text" href="#balance">How the rules were balanced</a>
		</nav>
	</header>

	{#each guides as g (g.key)}
		{@const count = starters(g.rules)}
		{@const some = limited(g.rules)}
		<section class="panel" id={g.key} data-sport={g.key}>
			<div class="row title">
				<SportBadge sport={g.key} solid />
				<h2>{g.name}</h2>
			</div>

			<div class="grid">
				<div>
					<h3 class="eyebrow">Roster</h3>
					<ul>
						<li><strong>{plural(count, 'starter')}</strong> and {plural(Math.max(g.rules.roster.main - count, 0), 'bench spot')} ({g.rules.roster.main} on the main roster)</li>
						<li>
							{plural(g.rules.roster.reserve, 'reserve spot')}, for
							{g.rules.roster.reserve_eligibility === 'anyone' ? 'anyone' : 'prospects only'}
						</li>
						{#if g.rules.roster.reserve_lock_days > 0}
							<li>A player sent to reserve stays there {plural(g.rules.roster.reserve_lock_days, 'day')}</li>
						{/if}
					</ul>

					<h3 class="eyebrow">Lineup</h3>
					<ul>
						{#each g.rules.lineup.slots as slot (slot.name)}
							<li><strong>{slot.count} × {slot.name}</strong> <span class="muted">({positions(slot.positions)})</span></li>
						{/each}
					</ul>
					<ul>
						<li>
							Set {g.rules.lineup.period === 'week' ? `by the week, starting ${g.rules.lineup.week_start[0].toUpperCase()}${g.rules.lineup.week_start.slice(1)}` : 'by the day'};
							a starter locks when {g.rules.lineup.lock === 'game_start' ? 'his game starts' : `the ${g.rules.lineup.period} starts`}
						</li>
						{#if some.length === 0}
							<li>Every game a starter plays counts</li>
						{:else}
							<li>
								<strong>
									{some.length === g.rules.lineup.slots.length ? 'Each starter' : `Each ${some.map((s) => s.name).join(' and ')}`}
									scores in {some[0].games_per_week === 1 ? 'one game' : `${some[0].games_per_week} games`} a week.
								</strong>
								You pick the game on the lineup page; left alone it is his next one. He locks when that game starts.
							</li>
							{#if some.length < g.rules.lineup.slots.length}<li>Every game counts in the other slots</li>{/if}
						{/if}
						{#if g.rules.lineup.pitcher_starts_per_week > 0}
							<li>
								<strong>A team's pitchers score in {g.rules.lineup.pitcher_starts_per_week} starts a week.</strong>
								A start beyond that, in the order played, scores nothing. A reliever who opens a game uses one only if he
								pitches more than an inning.
							</li>
						{/if}
					</ul>

					<h3 class="eyebrow">Season</h3>
					<ul>
						<li>
							{#if g.rules.format.type === 'head_to_head'}
								Head to head, {g.rules.format.matchup_days === 7 ? 'one-week' : `${g.rules.format.matchup_days}-day`} matchups;
								{g.rules.format.playoff_teams} teams make the playoffs
							{:else}
								Most points over the season wins
							{/if}
						</li>
						<li>
							{#if g.rules.free_agency.mode === 'closed'}
								No free agent adds: drafts and trades only
							{:else}
								Free agents can be added{g.rules.free_agency.weekly_limit > 0 ? `, up to ${g.rules.free_agency.weekly_limit} a week` : ''}{g.rules.free_agency.new_entrants_draft_only ? '; players new to the pool wait for the next draft' : ''}
							{/if}
						</li>
						<li>
							{#if g.rules.waivers.mode === 'none'}
								A dropped player is a free agent straight away
							{:else}
								A dropped player is on waivers for {plural(g.rules.waivers.days, 'day')}, then goes to
								{g.rules.waivers.mode === 'faab' ? `the highest blind bid (budget ${g.rules.waivers.budget} a season)` : 'the claim highest in the waiver order'}
							{/if}
						</li>
						<li>Seasonal draft: {plural(g.rules.draft.rounds, 'round')}</li>
					</ul>
				</div>

				<div>
					<h3 class="eyebrow">Scoring</h3>
					{#if sameScoring(g).length > 0}
						<p class="small-text same">Scored the same as {names(sameScoring(g))}.</p>
					{/if}
					{#if g.custom && !recommended(g)}
						<p class="small-text muted same">
							This league's scoring differs from the <a href="#balance">recommended scoring</a> for its sport.
							{#if data.me?.is_commissioner}<a href="/commissioner?section=rules">Change it in Rules</a>.{/if}
						</p>
					{/if}
					<table>
						<tbody>
							{#each scoring(g) as row (row.label)}
								<tr><td>{row.label}</td><td class="num" class:minus={row.points < 0}>{signed(row.points)}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		</section>
	{/each}

	<section class="panel prose" id="balance">
		<div class="row title"><h2>How the rules were balanced</h2></div>
		<div class="text stack tight">
			<p>
				The aim is that a star is worth about the same in every sport, so a trade of a top hockey player for a top
				football player is a fair one. The recommended rules were tested against every game of last season: for each
				sport a 16-team league was filled, and each player was measured by how much he raises an average team's chance
				of winning a one-week matchup compared with the best player left on waivers.
			</p>
			<div class="scroll">
				<table class="balance">
					<thead>
						<tr><th>League</th><th class="num">Best player</th><th class="num">First round</th><th class="num">Third round</th></tr>
					</thead>
					<tbody>
						{#each balance as b (b.key)}
							<tr data-sport={b.key}>
								<td><SportBadge sport={b.key} /> {nameOf(b.key)}</td>
								<td class="num">+{b.top.toFixed(1)}</td><td class="num">+{b.first.toFixed(1)}</td><td class="num">+{b.third.toFixed(1)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			<p class="muted small-text">
				Points of win probability added per matchup. +16 turns a .500 team into a .660 team. These describe the
				recommended rules; a league whose rules have been changed will differ.
			</p>
			<h3>What that led to</h3>
			<ul>
				<li><strong>Basketball counts one game a week per player.</strong> Stars separate from the field so much that counting every game changes little, and one game keeps lineups a decision. Three-pointers score so that guards are worth as much as big men.</li>
				<li><strong>Hockey scores shots at half a point.</strong> Shot volume is the steadiest sign of who the good players are, and it was the one change that lifted every tier of player. Lineup size made almost no difference, so it stays at eight forwards and four defensemen.</li>
				<li><strong>One goalie, scored generously.</strong> A second goalie slot mostly adds luck. With wins, shutouts and saves weighted up, the best goalies are worth an early pick without swamping the skaters.</li>
				<li><strong>Baseball has a utility slot and four starts a week.</strong> Without the utility slot a designated hitter could not play anywhere. A team's pitchers score in four starts a week between them, however the manager spreads them.</li>
				<li><strong>Football is full-point PPR with a superflex.</strong> Sixteen teams starting up to two quarterbacks makes the position scarce enough to matter.</li>
				<li><strong>The WNBA keeps rosters short.</strong> The whole league is 180 players, so 16 franchises can hold little more than their starters before nobody is left on waivers.</li>
			</ul>
			<h3>What is still uneven</h3>
			<p>
				Baseball is the noisiest sport week to week, so its stars move a matchup about half as much as football's. The
				WNBA runs the other way: with so few players to go round, its stars are worth close to double. No setting
				tested at one-week matchups closed either gap.
			</p>
		</div>
	</section>
</div>

<style>
	.jump {
		gap: 0.4rem 0.6rem;
	}
	section {
		scroll-margin-top: 1rem;
	}
	.title {
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--rule);
	}
	.grid {
		display: grid;
		grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
		gap: 0 2rem;
		padding: 0.5rem 1rem 1rem;
	}
	@media (max-width: 720px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}
	h3.eyebrow {
		margin: 0.9rem 0 0.35rem;
	}
	ul {
		margin: 0;
		padding-left: 1.1rem;
	}
	ul + ul {
		margin-top: 0.4rem;
	}
	li {
		margin: 0.15rem 0;
	}
	.same {
		margin-bottom: 0.4rem;
	}
	.grid td {
		padding: 0.3rem 0;
	}
	.grid td.num {
		font-weight: 650;
		color: var(--good);
	}
	.grid td.minus {
		color: var(--bad);
	}
	.text {
		padding: 0.9rem 1rem 1.1rem;
		max-width: 46rem;
	}
	.text h3 {
		margin-top: 0.5rem;
	}
	.balance td:first-child {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
</style>
