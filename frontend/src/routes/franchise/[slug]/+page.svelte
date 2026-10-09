<script lang="ts">
	// A franchise across every sport. The organization view says where each
	// team stands at a glance; picking a league opens that team: its lineup,
	// its matchup, the standings, and its roster and reserve list.
	import { untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import {
		changeRoster,
		getLineup,
		getMatchups,
		getStandings,
		type LeagueRoster,
		type List,
		type Matchups as MatchupPeriod,
		type RosterPlayer,
		type Standings
	} from '#lib/api.ts';
	import TeamIdentityEditor from '#lib/TeamIdentityEditor.svelte';
 import PlayerNickname from '#lib/PlayerNickname.svelte';
 import { teamIdentity } from '#lib/identity.ts';
 import LineupEditor from '#lib/LineupEditor.svelte';
	import Matchups from '#lib/Matchups.svelte';
	import TradeAsset from '#lib/TradeAsset.svelte';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { clockTime, dayLabel, points } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const team = $derived(data.franchise);
	const mine = $derived(data.me?.id === team.id);
	const canEdit = $derived(mine || data.me?.is_commissioner === true);
	const leagues = $derived(data.dynasty?.leagues ?? []);
	const franchises = $derived(data.dynasty?.franchises ?? []);
	const nameOf = (id: string | null, leagueID?: string) => teamIdentity(franchises.find((f) => f.id === id),data.dynasty?.team_identities,leagueID).name;

	// ---- which view: the organization, or one league ----
	let view = $state(untrack(() => page.url.searchParams.get('view') ?? ''));
	const league = $derived(leagues.find((l) => l.competition === view));
	const tabs = $derived([
		{ value: '', label: 'Organization' },
		...leagues.map((l) => ({ value: l.competition, label: l.name, sport: l.competition }))
	]);
	function show(to: string) {
		view = to;
		const query = new URLSearchParams(page.url.search);
		if (to) query.set('view', to);
		else query.delete('view');
		goto(`?${query}`, { replaceState: true });
	}

	// ---- where the team stands in each league ----
	let tables = $state<Standings[]>([]);
	let periods = $state<Record<string, MatchupPeriod>>({}); // by league id: the matchups in progress
	let openSlots = $state<Record<string, number>>({}); // by league id, for the manager's own teams

	const today = () => new Date().toLocaleDateString('en-CA');
	type Phase = { key: 'in' | 'upcoming' | 'over' | 'none'; label: string };
	function phaseOf(table: Standings | undefined): Phase {
		const season = table?.season;
		if (!season) return { key: 'none', label: 'No season yet' };
		if (season.status === 'complete') return { key: 'over', label: `${season.year} season over` };
		if (today() < season.starts_on) return { key: 'upcoming', label: `Starts ${dayLabel(season.starts_on)}` };
		return { key: 'in', label: 'In season' };
	}

	async function refresh() {
		try {
			tables = await getStandings();
		} catch (e) {
			toast.error(e);
			return;
		}
		for (const table of tables) {
			if (phaseOf(table).key !== 'in') continue;
			if (table.format === 'head_to_head') {
				getMatchups(table.league_id).then((m) => (periods[table.league_id] = m), () => {});
			}
			if (mine) {
				getLineup(table.league_id, team.id).then(
					(l) => (openSlots[table.league_id] = l.slots.reduce((n, s) => n + s.count, 0) - l.players.filter((p) => p.slot).length),
					() => {}
				);
			}
		}
	}
	$effect(() => {
		void team.id;
		untrack(refresh);
	});
	onScoresChange(() => leagues.map((l) => l.competition), refresh);

	const ordinal = (n: number) => n + (['th', 'st', 'nd', 'rd'][n % 100 > 10 && n % 100 < 14 ? 0 : n % 10] ?? 'th');
	// Everything the cards and the league header say about one league.
	const standing = (leagueId: string) => {
		const table = tables.find((t) => t.league_id === leagueId);
		const place = table ? table.rows.findIndex((r) => r.franchise_id === team.id) : -1;
		const row = place >= 0 ? table!.rows[place] : undefined;
		const period = periods[leagueId];
		const matchup = period?.matchups.find((m) => m.home_franchise_id === team.id || m.away_franchise_id === team.id);
		const home = matchup?.home_franchise_id === team.id;
		return {
			table,
			phase: phaseOf(table),
			headToHead: table?.format === 'head_to_head',
			row,
			rank: place + 1,
			of: table?.rows.length ?? 0,
			record: row ? `${row.wins}-${row.losses}${row.ties ? `-${row.ties}` : ''}` : '',
			period: period?.period,
			matchup: matchup && {
				id: matchup.id,
				bye: matchup.away_franchise_id === null,
				opponent: nameOf(home ? matchup.away_franchise_id : matchup.home_franchise_id,leagueId),
				ours: home ? matchup.home_points : matchup.away_points,
				theirs: home ? matchup.away_points : matchup.home_points,
				final: matchup.final
			}
		};
	};
	// In-season leagues first, then those about to start, then the rest.
	const cards = $derived(
		data.rosters
			.map((roster) => ({ roster, ...standing(roster.league_id) }))
			.toSorted((a, b) => ['in', 'upcoming', 'over', 'none'].indexOf(a.phase.key) - ['in', 'upcoming', 'over', 'none'].indexOf(b.phase.key))
	);
	const active = $derived(cards.filter((c) => c.phase.key === 'in').length);

	// ---- rookie-draft picks waiting to be signed ----
	let clock = $state(Date.now());
	$effect(() => {
		const timer = setInterval(() => (clock = Date.now()), 30_000);
		return () => clearInterval(timer);
	});
	// "6 days 4 hours", "3 hours 12 minutes": how long is left to sign.
	function timeLeft(until: string): string {
		const minutes = Math.max(0, Math.floor((new Date(until).getTime() - clock) / 60_000));
		const count = (n: number, unit: string) => `${n} ${unit}${n === 1 ? '' : 's'}`;
		const days = Math.floor(minutes / 1440);
		const hours = Math.floor((minutes % 1440) / 60);
		if (days > 0) return `${count(days, 'day')} ${count(hours, 'hour')}`;
		return hours > 0 ? `${count(hours, 'hour')} ${count(minutes % 60, 'minute')}` : count(minutes, 'minute');
	}
	// The soonest deadline among a roster's unsigned picks, if the draft has ended.
	const deadline = (r: LeagueRoster) =>
		on(r, 'rights')
			.map((p) => p.rights_until)
			.filter((d): d is string => d !== null)
			.sort()[0];

	// Picks in the drafts to come, by year.
	const pickYears = $derived(
		[...new Set(data.picks.map((p) => p.year))].sort().map((year) => ({ year, picks: data.picks.filter((p) => p.year === year) }))
	);

	// ---- one league ----
	const roster = $derived(data.rosters.find((r) => r.league_id === league?.id));
	const now = $derived(league ? standing(league.id) : undefined);
	const on = (r: LeagueRoster, list: List) => r.players.filter((p) => p.list === list);
	const lists: { key: List; title: string; other: List; move: string }[] = [
		{ key: 'main', title: 'Main roster', other: 'reserve', move: 'To reserve' },
		{ key: 'reserve', title: 'Reserve list', other: 'main', move: 'To main' }
	];

	// A commissioner editing someone else's roster is overriding the rules.
	async function change(r: LeagueRoster, action: 'drop' | 'move', player: RosterPlayer, list?: List) {
		const days = r.limits.reserve_lock_days;
		if (mine && list === 'reserve' && days > 0 && player.status !== 'prospect' && player.list !== 'rights') {
			if (!confirm(`${player.full_name} will be locked on the reserve list for ${days} ${days === 1 ? 'day' : 'days'}. Move him?`)) return;
		}
		if (action === 'drop' && !confirm(`${player.list === 'rights' ? 'Release' : 'Drop'} ${player.full_name}?`)) return;
		try {
			await changeRoster(r.league_id, action, { player_id: player.player_id, list, franchise_id: team.id, force: !mine });
			await invalidateAll();
			toast.good(action === 'drop' ? `Released ${player.full_name}.` : player.list === 'rights' ? `Signed ${player.full_name}.` : `Moved ${player.full_name}.`);
		} catch (e) {
			toast.error(e);
		}
	}
</script>

<svelte:head><title>{team.name}</title></svelte:head>

<div class="stack">
	<header class="head">
		<Crest name={league ? roster?.team_name || team.name : team.name} src={league ? roster?.image_url || team.image_url : team.image_url} size={56} />
		<div class="grow">
			<h1>{league ? roster?.team_name || team.name : team.name}</h1>
 {#if league}<p class="muted small-text">{team.name} · {league.name}</p>{/if}
			<div class="row muted">
				Managed by {team.manager_name}
				{#if mine}<span class="pill brand">You</span>{/if}
				{#if team.is_commissioner}<span class="pill gold">Commissioner</span>{/if}
			</div>
		</div>
		{#if data.me && !mine}
			<a class="button primary" href="/trades/new?with={team.slug}"><Icon name="trade" size={16} /> Propose a trade</a>
		{:else if mine}
			<a class="button" href="/trades/new"><Icon name="trade" size={16} /> New trade</a>
		{/if}
	</header>

	<Tabs {tabs} bind:value={() => view, show} label="View" />
 {#if canEdit}{#key team.id}<TeamIdentityEditor franchise={team} {leagues} identities={data.dynasty?.team_identities ?? []} />{/key}{/if}

	{#if canEdit && !mine}
		<p class="card small-text"><span class="pill gold">Commissioner</span> Changes you make here skip the roster rules.</p>
	{/if}

	{#if !league}
		<!-- ---- the organization ---- -->
		<p class="muted">
			{#if cards.length === 0}
				This franchise is not in any league yet.
			{:else}
				{active} of {cards.length} {cards.length === 1 ? 'league' : 'leagues'} in season.
				{#if mine}Open a league to set its lineup.{/if}
			{/if}
		</p>

		<div class="cards">
			{#each cards as c (c.roster.league_id)}
				<button class="card league" data-sport={c.roster.competition} class:quiet={c.phase.key !== 'in'} onclick={() => show(c.roster.competition)}>
					<span class="spread">
						<span class="row"><Crest name={c.roster.team_name} src={c.roster.image_url} size={34} /><SportBadge sport={c.roster.competition} solid /> <strong class="name">{c.roster.team_name}</strong></span>
						<span class="pill" class:good={c.phase.key === 'in'}>{c.phase.label}</span>
					</span>

					{#if c.phase.key === 'in' || c.phase.key === 'over'}
						<span class="facts">
							{#if c.headToHead}
								<span><small>Record</small><strong>{c.record || '0-0'}</strong></span>
							{/if}
							<span><small>Place</small><strong>{c.rank > 0 ? `${ordinal(c.rank)} of ${c.of}` : '—'}</strong></span>
							<span><small>Season points</small><strong>{c.row ? points(c.row.points) : '—'}</strong></span>
						</span>
					{/if}

					{#if c.phase.key === 'in' && c.matchup}
						<span class="versus">
							{#if c.matchup.bye}
								<span class="muted">No matchup this round</span>
							{:else}
								<span class="muted small-text">{c.matchup.final ? 'Final' : 'This matchup'}</span>
								<span class="score" class:ahead={c.matchup.ours > c.matchup.theirs}>
									<strong>{points(c.matchup.ours)}</strong> – {points(c.matchup.theirs)}
									<span class="muted">vs {c.matchup.opponent}</span>
								</span>
							{/if}
						</span>
					{/if}

					<span class="row foot">
						<span class="muted small-text">
							{on(c.roster, 'main').length}/{c.roster.limits.main} main · {on(c.roster, 'reserve').length}/{c.roster.limits.reserve} reserve
						</span>
						{#if c.roster.overage > 0}<span class="pill bad">Over the limit by {c.roster.overage}</span>{/if}
						{#if on(c.roster, 'rights').length > 0}
							{@const due = deadline(c.roster)}
							<span class="pill gold">
								{on(c.roster, 'rights').length} {on(c.roster, 'rights').length === 1 ? 'pick' : 'picks'} to sign{due ? ` · ${timeLeft(due)} left` : ''}
							</span>
						{/if}
						{#if mine && c.phase.key === 'in' && openSlots[c.roster.league_id] > 0}
							<span class="pill gold">{openSlots[c.roster.league_id]} open lineup {openSlots[c.roster.league_id] === 1 ? 'slot' : 'slots'}</span>
						{/if}
					</span>
				</button>
			{/each}
		</div>

		<section class="panel" id="picks">
			<h2 class="bar"><span class="pill gold">Picks</span> Draft picks</h2>
			{#if data.picks.length === 0}
				<p class="muted small-text pad">No picks in upcoming drafts.</p>
			{:else}
				<p class="muted small-text pad">
					Every pick in the drafts to come, in every league. Any of them can be traded, for players or picks in any league,
					until its draft starts.{#if mine} <a href="/trades/new">Start a trade</a>.{/if}
				</p>
				{#each pickYears as group (group.year)}
					<h3 class="eyebrow listhead">{group.year} <span class="muted">· {group.picks.length} {group.picks.length === 1 ? 'pick' : 'picks'}</span></h3>
					<ul class="picks">
						{#each group.picks as pick (pick.id)}
							<li>
								<TradeAsset
									draft={pick.draft_name}
									round={pick.round}
									sport={pick.competitions.length === 1 ? pick.competitions[0] : ''}
									via={pick.original_franchise_id !== team.id ? nameOf(pick.original_franchise_id) : ''}
								/>
							</li>
						{/each}
					</ul>
				{/each}
			{/if}
		</section>
	{:else if roster && now}
		<!-- ---- one league ---- -->
		<div class="stack" data-sport={league.competition}>
			<div class="summary">
				<div class="fact"><small>Status</small><span class="pill" class:good={now.phase.key === 'in'}>{now.phase.label}</span></div>
				{#if now.phase.key === 'in' || now.phase.key === 'over'}
					{#if now.headToHead}<div class="fact"><small>Record</small><strong>{now.record || '0-0'}</strong></div>{/if}
					<div class="fact"><small>Place</small><strong>{now.rank > 0 ? `${ordinal(now.rank)} of ${now.of}` : '—'}</strong></div>
					<div class="fact"><small>Season points</small><strong>{now.row ? points(now.row.points) : '—'}</strong></div>
				{/if}
				{#if now.matchup && !now.matchup.bye}
					<a class="fact matchup" href="/matchup/{now.matchup.id}">
						<small>{now.matchup.final ? 'Final' : 'This matchup'} vs {now.matchup.opponent}</small>
						<strong class:ahead={now.matchup.ours > now.matchup.theirs}>{points(now.matchup.ours)} – {points(now.matchup.theirs)}</strong>
					</a>
				{/if}
			</div>

			{#if on(roster, 'rights').length > 0}
				{@const due = deadline(roster)}
				{@const room = roster.limits.reserve - on(roster, 'reserve').length}
				{@const mainRoom = roster.limits.main - on(roster, 'main').length}
				<section class="panel signing">
					<h2 class="bar">
						<span class="pill gold">Sign</span> Draft picks to sign
						{#if due}<span class="left">{timeLeft(due)} left</span>{/if}
					</h2>
					<p class="pad small-text muted">
						{#if due}
							Sign each pick to the reserve list or the main roster by {clockTime(due)}. Any left unsigned go on waivers, then
							become free agents.
						{:else}
							These picks can be signed now; the deadline is set when the draft ends.
						{/if}
						Room: {Math.max(room, 0)} on the reserve list, {Math.max(mainRoom, 0)} on the main roster.
						{#if room <= 0 && mainRoom <= 0}Drop or trade someone below to make room.{/if}
					</p>
					<table>
						<tbody>
							{#each on(roster, 'rights') as player (player.player_id)}
								<tr>
									<td>
										<div class="player">
											<Headshot name={player.full_name} src={player.headshot_url} />
											<div>
												<strong>{player.full_name}</strong>
												<div class="muted small-text">{[player.positions.join('/'), player.team_abbrev, player.note].filter(Boolean).join(' · ')}</div>
											</div>
										</div>
									</td>
									{#if canEdit}
										<td class="actions">
											<button class="small primary" disabled={mine && room <= 0} onclick={() => change(roster, 'move', player, 'reserve')}>Sign to reserve</button>
											<button class="small" disabled={mine && mainRoom <= 0} onclick={() => change(roster, 'move', player, 'main')}>Sign to main</button>
											<button class="small quiet danger" onclick={() => change(roster, 'drop', player)}>Release</button>
										</td>
									{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{/if}

			<section class="stack tight">
				<h2>Lineup</h2>
				{#key league.id + team.id}
					<LineupEditor {league} franchise={team} {canEdit} commissioner={data.me?.is_commissioner === true} />
				{/key}
			</section>

			<details class="panel fold">
				<summary>
					<strong>Roster and reserve list</strong>
					<span class="muted small-text">
						{on(roster, 'main').length} of {roster.limits.main} on the main roster · {on(roster, 'reserve').length} of {roster.limits.reserve} on reserve
					</span>
					{#if roster.overage > 0}<span class="pill bad">Over the limit by {roster.overage}</span>{/if}
				</summary>
				<div class="meters">
					<Meter label="Main" value={on(roster, 'main').length} max={roster.limits.main} />
					<Meter label="Reserve" value={on(roster, 'reserve').length} max={roster.limits.reserve} />
				</div>
				{#each lists as list (list.key)}
					{@const players = on(roster, list.key)}
					<h3 class="eyebrow listhead">{list.title}</h3>
					{#if players.length === 0}
						<p class="muted small-text pad">Nobody yet.</p>
					{:else}
						<div class="scroll">
							<table>
								<tbody>
									{#each players as player (player.player_id)}
										<tr>
											<td>
												<div class="player">
													<Headshot name={player.full_name} src={player.headshot_url} />
													<div>
														<div class="row who">
															<a href="/player/{player.player_id}"><strong>{player.full_name}</strong></a>
															{#if player.class}<span class="pill">{player.class}</span>{/if}
															{#if player.status === 'prospect'}<span class="pill gold">Prospect</span>{/if}
															{#if player.status === 'inactive'}<span class="pill">Inactive</span>{/if}
															{#if player.locked_until}<span class="pill">Locked until {clockTime(player.locked_until)}</span>{/if}
														</div>
														<PlayerNickname franchiseID={team.id} playerID={player.player_id} playerName={player.full_name} initialNickname={player.nickname} editable={canEdit} />
 <div class="muted small-text">
															{[player.positions.join('/'), player.team_abbrev, player.note].filter(Boolean).join(' · ')}
														</div>
													</div>
												</div>
											</td>
											{#if canEdit}
												<td class="actions">
													<button class="small" disabled={mine && !!player.locked_until} onclick={() => change(roster, 'move', player, list.other)}>{list.move}</button>
													<button class="small quiet danger" onclick={() => change(roster, 'drop', player)}>Drop</button>
												</td>
											{/if}
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				{/each}
				{#if mine}
					<p class="pad small-text"><a href="/players?competition={roster.competition}">Find {roster.name} players to add</a></p>
				{/if}
			</details>

			<div class="columns">
				<section class="panel">
					<h2 class="bar">Standings</h2>
					{#if !now.table || now.table.rows.length === 0}
						<p class="muted small-text pad">No season yet.</p>
					{:else}
						<table class="standings">
							<tbody>
								{#each now.table.rows as row, i (row.franchise_id)}
									<tr class:me={row.franchise_id === team.id}>
										<td class="rank">{i + 1}</td>
										<td>{nameOf(row.franchise_id,league.id)}</td>
										{#if now.headToHead}<td class="num">{row.wins}-{row.losses}{row.ties ? `-${row.ties}` : ''}</td>{/if}
										<td class="num">{points(row.points)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</section>
				{#if now.headToHead && now.phase.key === 'in'}
					<section class="panel">
						<h2 class="bar">Matchups</h2>
						<div class="pad"><Matchups {league} {franchises} identities={data.dynasty?.team_identities} /></div>
					</section>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 1rem;
	}
	.head .row {
		gap: 0.4rem 0.6rem;
		margin-top: 0.3rem;
	}
	.grow {
		flex: 1;
	}

	/* ---- the organization ---- */
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(19rem, 1fr));
		gap: 0.8rem;
	}
	.league {
		display: grid;
		gap: 0.7rem;
		align-content: start;
		justify-content: stretch;
		justify-items: stretch;
		text-align: left;
		white-space: normal;
		font-weight: 400;
		border-top: 3px solid var(--sport);
	}
	.league.quiet {
		border-top-color: var(--rule-strong);
	}
	.league .row {
		gap: 0.5rem;
	}
	.name {
		font: 700 1.05rem/1.2 var(--display);
	}
	.facts {
		display: flex;
		gap: 1.4rem;
	}
	.facts span,
	.fact {
		display: grid;
		gap: 0.1rem;
	}
	small {
		font: 600 0.66rem/1.2 var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.facts strong,
	.fact strong {
		font: 700 1.05rem/1.2 var(--display);
	}
	.versus {
		display: grid;
		gap: 0.1rem;
		padding-top: 0.6rem;
		border-top: 1px solid var(--rule);
	}
	.score strong {
		font-size: 1.05rem;
	}
	.ahead,
	.score.ahead strong {
		color: var(--good);
	}
	.foot {
		flex-wrap: wrap;
	}

	/* ---- one league ---- */
	.summary {
		display: flex;
		flex-wrap: wrap;
		gap: 0.8rem 2rem;
		align-items: end;
		padding: 0.8rem 1rem;
		background: var(--surface);
		border-block: 1px solid var(--rule);
		border-top: 3px solid var(--sport);
	}
	.matchup {
		margin-left: auto;
		color: var(--ink);
		text-align: right;
	}
	.matchup:hover {
		text-decoration: none;
	}
	.fold > summary {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4rem 0.9rem;
		padding: 0.75rem 1rem;
		cursor: pointer;
	}
	.meters {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
		gap: 1.2rem;
		padding: 0.4rem 1rem 0.9rem;
		max-width: 34rem;
	}
	.listhead {
		padding: 0.4rem 1rem;
		background: var(--surface-2);
		border-block: 1px solid var(--rule);
	}
	.pad {
		padding: 0.65rem 1rem;
	}
	td:first-child {
		padding-left: 1rem;
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.who {
		gap: 0.25rem 0.45rem;
	}
	.columns {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(19rem, 1fr));
		gap: 1rem;
		align-items: start;
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.6rem 1rem;
		font-size: 1rem;
		border-bottom: 1px solid var(--rule);
	}
	.standings .rank {
		width: 2.2rem;
		font: 650 0.8rem var(--mono);
		color: var(--ink-faint);
	}
	.standings tr.me td {
		background: var(--brand-soft);
		font-weight: 650;
	}
	.signing {
		border-top: 3px solid var(--gold);
	}
	.left {
		margin-left: auto;
		font: 650 0.85rem var(--mono);
		color: var(--gold);
	}
	.picks {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
	}
	.picks li {
		padding: 0.7rem 1rem;
		border-bottom: 1px solid var(--rule);
	}
</style>
