<script lang="ts">
	// A franchise across every sport. The organization view says where each
	// team stands at a glance; picking a league opens that team: its lineup,
	// its matchup, the standings, and its roster and reserve list.
	import { untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import {
		changeRoster,
		setRoster,
		getLineup,
		getMatchups,
		getStandings,
		rookieRounds,
		type HeldPick,
		type LeagueRoster,
		type List,
		type Matchups as MatchupPeriod,
		type RosterPlayer,
		type Standings
	} from '#lib/api.ts';
	import { teamIdentity } from '#lib/identity.ts';
	import LineupEditor from '#lib/LineupEditor.svelte';
	import Matchups from '#lib/Matchups.svelte';
	import PlayerNickname from '#lib/PlayerNickname.svelte';
	import TeamIdentityEditor from '#lib/TeamIdentityEditor.svelte';
	import WatchStar from '#lib/WatchStar.svelte';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
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
	const nameOf = (id: string | null, leagueId?: string) =>
		teamIdentity(franchises.find((f) => f.id === id), data.dynasty?.team_identities, leagueId).name;

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
				opponent: nameOf(home ? matchup.away_franchise_id : matchup.home_franchise_id, leagueId),
				ours: home ? matchup.home_points : matchup.away_points,
				theirs: home ? matchup.away_points : matchup.home_points,
				final: matchup.final
			}
		};
	};
	// In-season leagues first, then those about to start, then the rest.
	const phases: Phase['key'][] = ['in', 'upcoming', 'over', 'none'];
	const cards = $derived(
		data.rosters
			.map((roster) => ({ roster, ...standing(roster.league_id) }))
			.toSorted((a, b) => phases.indexOf(a.phase.key) - phases.indexOf(b.phase.key))
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

	// ---- draft picks: a grid for each rookie draft, years down and rounds across ----
	const draftKey = (competitions: string[]) => competitions.toSorted().join(',');
	const pickGrids = $derived.by(() => {
		const byDraft = new Map<string, HeldPick[]>();
		for (const p of data.picks) byDraft.set(draftKey(p.competitions), [...(byDraft.get(draftKey(p.competitions)) ?? []), p]);
		return [...byDraft]
			.map(([key, picks]) => {
				const sports = key ? key.split(',') : [];
				const covered = leagues.filter((l) => sports.includes(l.competition));
				// Every round and year shows, even one whose picks were all traded away.
				const rounds = Math.max(...picks.map((p) => p.round), ...covered.map((l) => rookieRounds(l.settings)));
				const years = new Set(picks.map((p) => p.year));
				for (const d of data.drafts) {
					if (d.kind === 'seasonal' && d.status !== 'complete' && draftKey(d.competitions) === key) years.add(d.year);
				}
				return {
					key,
					sports,
					name: covered.length === 1 ? covered[0].name : 'Shared draft',
					order: Math.min(...covered.map((l) => leagues.indexOf(l))),
					rounds: Array.from({ length: rounds }, (_, i) => i + 1),
					years: [...years]
						.sort((a, b) => a - b)
						.map((year) => ({
							year,
							cells: Array.from({ length: rounds }, (_, i) => picks.filter((p) => p.year === year && p.round === i + 1))
						}))
				};
			})
			.toSorted((a, b) => a.order - b.order);
	});
	// Where the picks in one cell came from, for its popup.
	const origins = (held: HeldPick[]) =>
		held.length === 0
			? 'Traded away'
			: held.map((p) => (p.original_franchise_id === team.id ? 'Own pick' : `From ${nameOf(p.original_franchise_id)}`)).join(' · ');

	// ---- one league ----
	const roster = $derived(data.rosters.find((r) => r.league_id === league?.id));
	const now = $derived(league ? standing(league.id) : undefined);
	// While the manager is setting the roster, plan holds where each moved
	// player is headed; nothing is saved until the whole split is.
	let plan = $state<Record<string, List>>();
	let rosterOpen = $state(false);
	$effect(() => {
		void league?.id;
		void team.id;
		plan = undefined;
	});
	const on = (r: LeagueRoster, list: List) => r.players.filter((p) => (plan?.[p.player_id] ?? p.list) === list);
	const over = (r: LeagueRoster) => on(r, 'main').length > r.limits.main || on(r, 'reserve').length > r.limits.reserve;
	// Whether a player sent to reserve now is there for the rest of the season.
	const seasonLocked = (r: LeagueRoster) => r.limits.reserve_lock_season && standing(r.league_id).phase.key === 'in';
	const sentDown = (r: LeagueRoster) => r.players.filter((p) => p.list === 'main' && plan?.[p.player_id] === 'reserve').length;
	const lists: { key: List; title: string; other: List; move: string }[] = [
		{ key: 'main', title: 'Main roster', other: 'reserve', move: 'To reserve' },
		{ key: 'reserve', title: 'Reserve list', other: 'main', move: 'To main' }
	];

	// A main-roster player out injured can swap with a reserve for the rest of the season.
	let injuryReplacement = $state<Record<string, string>>({});
	let swapping = $state(false);
	const injuryEligible = (player: RosterPlayer) => ['IR', 'OUT', 'IL7', 'IL10', 'IL15', 'IL60'].includes(player.injury_designation);
	async function injurySwap(r: LeagueRoster, player: RosterPlayer) {
		const replacement = r.players.find((p) => p.player_id === injuryReplacement[player.player_id]);
		if (!replacement || swapping) return;
		if (!confirm(`Move ${player.full_name} to reserve for the rest of this season and call up ${replacement.full_name}? ${player.full_name} cannot return to the main roster this season, even after recovering.`)) return;
		swapping = true;
		try {
			await changeRoster(r.league_id, 'injury-swap', { player_id: player.player_id, replacement_id: replacement.player_id, franchise_id: team.id });
			await invalidateAll();
			delete injuryReplacement[player.player_id];
			toast.good(`Called up ${replacement.full_name}. ${player.full_name} is on reserve for the rest of the season.`);
		} catch (e) {
			toast.error(e);
		} finally {
			swapping = false;
		}
	}

	// A commissioner editing someone else's roster is overriding the rules.
	async function change(r: LeagueRoster, action: 'drop' | 'move', player: RosterPlayer, list?: List) {
		const days = r.limits.reserve_lock_days;
		if (mine && list === 'reserve' && player.list === 'main' && seasonLocked(r)) {
			if (!confirm(`${player.full_name} will stay on the reserve list until the season ends. Move him?`)) return;
		} else if (mine && list === 'reserve' && days > 0 && player.status !== 'prospect' && player.list !== 'rights') {
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

	async function saveRoster(r: LeagueRoster) {
		const down = sentDown(r);
		if (mine && down > 0 && seasonLocked(r)) {
			if (!confirm(`${down === 1 ? 'The player' : `The ${down} players`} you are sending to reserve will stay there until the season ends. Save?`)) return;
		}
		try {
			await setRoster(r.league_id, { reserve: on(r, 'reserve').map((p) => p.player_id), franchise_id: team.id, force: !mine });
			await invalidateAll();
			plan = undefined;
			toast.good('Roster set.');
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
			<div class="title">
				<h1>{league ? roster?.team_name || team.name : team.name}</h1>
				{#if canEdit}{#key team.id}<TeamIdentityEditor franchise={team} {leagues} identities={data.dynasty?.team_identities ?? []} />{/key}{/if}
			</div>
			<p class="row muted small-text meta">
				<span>{#if league && roster?.team_name && roster.team_name !== team.name}{team.name} · {/if}Managed by {team.manager_name}</span>
				{#if mine}<span class="pill brand">You</span>{/if}
				{#if team.is_commissioner}<span class="pill gold">Commissioner</span>{/if}
			</p>
		</div>
		{#if data.me && !mine}
			<a class="button primary" href="/trades/new?with={team.slug}"><Icon name="trade" size={16} /> Propose a trade</a>
		{:else if mine}
			<a class="button" href="/trades/new"><Icon name="trade" size={16} /> New trade</a>
		{/if}
	</header>

	<Tabs {tabs} bind:value={() => view, show} label="View" />

	{#if canEdit && !mine}
		<p class="card small-text"><span class="pill gold">Commissioner</span> Changes you make here skip the roster rules.</p>
	{/if}

	{#if !league}
		<!-- ---- the organization ---- -->
		<section class="panel">
			<h2 class="bar">
				Leagues
				{#if cards.length}<span class="muted small-text count">{active} of {cards.length} in season</span>{/if}
			</h2>
			{#if cards.length === 0}
				<p class="muted small-text pad">This franchise is not in any league yet.</p>
			{:else}
				<ul class="leagues">
					{#each cards as c (c.roster.league_id)}
						{@const signing = on(c.roster, 'rights').length}
						{@const due = deadline(c.roster)}
						<li data-sport={c.roster.competition} class:quiet={c.phase.key !== 'in'}>
							<button onclick={() => show(c.roster.competition)}>
								<SportBadge sport={c.roster.competition} solid />
								<span class="team">
									<strong>{c.roster.team_name}</strong>
									<span class="muted small-text">
										{#if c.phase.key === 'in' && c.matchup && !c.matchup.bye}
											{c.matchup.final ? 'Final vs' : 'vs'}
											{c.matchup.opponent}
											<span class="score" class:ahead={c.matchup.ours > c.matchup.theirs}>{points(c.matchup.ours)}–{points(c.matchup.theirs)}</span>
										{:else}
											{c.phase.label}
										{/if}
									</span>
								</span>
								<span class="flags">
									{#if c.roster.overage > 0}<span class="pill bad">{c.roster.overage} over</span>{/if}
									{#if signing > 0}<span class="pill gold">{signing} to sign{due ? ` · ${timeLeft(due)}` : ''}</span>{/if}
									{#if mine && c.phase.key === 'in' && openSlots[c.roster.league_id] > 0}
										<span class="pill gold">{openSlots[c.roster.league_id]} open {openSlots[c.roster.league_id] === 1 ? 'slot' : 'slots'}</span>
									{/if}
								</span>
								<span class="stats">
									{#if c.phase.key === 'in' || c.phase.key === 'over'}
										{#if c.headToHead}<span><small>Record</small>{c.record || '0-0'}</span>{/if}
										<span><small>Place</small>{c.rank > 0 ? `${ordinal(c.rank)}/${c.of}` : '—'}</span>
										<span><small>Points</small>{c.row ? points(c.row.points) : '—'}</span>
									{/if}
									<span><small>Roster</small>{on(c.roster, 'main').length}/{c.roster.limits.main} · {on(c.roster, 'reserve').length}/{c.roster.limits.reserve}</span>
								</span>
								<Icon name="right" size={16} />
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="panel" id="picks">
			<h2 class="bar">
				Draft picks
				{#if mine && data.picks.length}<a class="small-text count" href="/trades/new">Trade picks</a>{/if}
			</h2>
			{#if pickGrids.length === 0}
				<p class="muted small-text pad">No picks in upcoming rookie drafts.</p>
			{:else}
				<div class="pick-grids">
					{#each pickGrids as grid (grid.key)}
						<table class="pick-grid" aria-label="{grid.name} picks held">
							<thead>
								<tr>
									<th scope="col" class="draft">{#each grid.sports as sport (sport)}<SportBadge {sport} />{/each}</th>
									{#each grid.rounds as round (round)}<th scope="col">R{round}</th>{/each}
								</tr>
							</thead>
							<tbody>
								{#each grid.years as { year, cells } (year)}
									<tr>
										<th scope="row">{year}</th>
										{#each cells as held, i (i)}
											<td>
												<button
													class="held"
													class:none={held.length === 0}
													class:acquired={held.some((p) => p.original_franchise_id !== team.id)}
													aria-label="{year} round {i + 1}: {held.length} {held.length === 1 ? 'pick' : 'picks'}. {origins(held)}"
													>{held.length}<span class="tip" role="tooltip">{origins(held)}</span></button
												>
											</td>
										{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					{/each}
				</div>
				<p class="muted small-text pad legend">Picks held in each round. <span class="acquired-key">Blue</span> includes a pick from another team; hover for where each came from.</p>
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
					<a class="fact matchup" href="/matchups/{now.matchup.id}">
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
												<a href="/player/{player.player_id}"><strong>{player.full_name}</strong></a><WatchStar playerID={player.player_id} playerName={player.full_name} />
												<div class="muted small-text">{player.positions.join('/')}{#if player.team_abbrev} · <TeamLogo competition={roster.competition} abbrev={player.team_abbrev} size={22} />{/if}{#if player.note} · {player.note}{/if}</div>
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
				{#if mine}<a class="small-text" href="/watchlist?competition={league.competition}">View your {league.name} watchlist →</a>{/if}
				{#key league.id + team.id}
					<LineupEditor {league} franchise={team} {canEdit} commissioner={data.me?.is_commissioner === true} />
				{/key}
			</section>

			{#if canEdit && !plan && tables.length > 0 && roster.limits.reserve_lock_season && roster.limits.reserve > 0 && on(roster, 'main').length + on(roster, 'reserve').length > 0 && (now.phase.key === 'upcoming' || now.phase.key === 'none')}
				<p class="card small-text">
					<span class="pill gold">Set your roster</span> Choose who is on the reserve list before the season starts. Whoever is on it
					then stays there until the season ends.
					<button class="small" onclick={() => ((plan = {}), (rosterOpen = true))}>Set roster</button>
				</p>
			{/if}

			<details class="panel fold" bind:open={rosterOpen}>
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
				{#if canEdit}
					<p class="pad small-text row">
						{#if plan}
							<span class="muted grow">Move players between the lists, then save. Nothing changes until you do.</span>
							<button class="small primary" disabled={mine && over(roster)} onclick={() => saveRoster(roster)}>Save roster</button>
							<button class="small quiet" onclick={() => (plan = undefined)}>Cancel</button>
						{:else}
							<span class="muted grow">Swapping players between two full lists? Set the roster in one save.</span>
							<button class="small" onclick={() => (plan = {})}>Set roster</button>
						{/if}
					</p>
				{/if}
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
															<a href="/player/{player.player_id}"><strong>{player.full_name}</strong></a><WatchStar playerID={player.player_id} playerName={player.full_name} />
															<InjuryBadge designation={player.injury_designation} />
																{#if player.class}<span class="pill">{player.class}</span>{/if}
															{#if player.status === 'prospect'}<span class="pill gold">Prospect</span>{/if}
															{#if player.status === 'inactive'}<span class="pill">Inactive</span>{/if}
															{#if player.injury_reserve_locked}<span class="pill gold">Injury swap · Reserve for season</span>{/if}
																{#if player.locked_until}<span class="pill">Locked until {clockTime(player.locked_until)}</span>{/if}
															{#if player.list !== list.key}<span class="pill brand">Moved</span>{/if}
														</div>
														<PlayerNickname franchiseID={team.id} playerID={player.player_id} playerName={player.full_name} initialNickname={player.nickname} editable={canEdit} />
															<div class="muted small-text">
															{player.positions.join('/')}{#if player.team_abbrev} · <TeamLogo competition={roster.competition} abbrev={player.team_abbrev} size={22} />{/if}{#if player.note} · {player.note}{/if}
														</div>
													</div>
												</div>
											</td>
											{#if canEdit}
												<td class="actions">
													{#if plan}
														<button class="small" disabled={player.injury_reserve_locked || (mine && !!player.locked_until)} onclick={() => (plan![player.player_id] = list.other)}>{list.move}</button>
													{:else}
														<button class="small" disabled={player.injury_reserve_locked || (mine && !!player.locked_until)} onclick={() => change(roster, 'move', player, list.other)}>{list.move}</button>
														<button class="small quiet danger" onclick={() => change(roster, 'drop', player)}>Drop</button>
														{#if list.key === 'main' && injuryEligible(player)}
															<div class="injury-swap">
																<select aria-label="Reserve replacement for {player.full_name}" bind:value={injuryReplacement[player.player_id]} disabled={swapping}>
																	<option value="">Call up from reserve…</option>
																	{#each on(roster, 'reserve').filter((p) => !p.injury_reserve_locked) as replacement (replacement.player_id)}
																		<option value={replacement.player_id}>{replacement.full_name}</option>
																	{/each}
																</select>
																<button class="small" disabled={swapping || !injuryReplacement[player.player_id]} onclick={() => injurySwap(roster, player)}>Season injury swap</button>
															</div>
														{/if}
													{/if}
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
										<td>{nameOf(row.franchise_id, league.id)}</td>
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
						<div class="pad stack tight">
							<Matchups {league} {franchises} identities={data.dynasty?.team_identities} />
							<a class="small-text" href="/matchups?view=schedule&team={team.id}">{team.name}'s full schedule</a>
						</div>
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
	.grow {
		flex: 1;
		min-width: 0;
	}
	.title {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.title h1 {
		overflow-wrap: anywhere;
	}
	.meta {
		gap: 0.4rem 0.6rem;
		margin-top: 0.3rem;
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.6rem 1rem;
		font-size: 1rem;
		border-bottom: 1px solid var(--rule);
	}
	.count {
		margin-left: auto;
		font-weight: 400;
	}
	.pad {
		padding: 0.65rem 1rem;
	}
	small {
		font: 600 0.62rem/1.2 var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.ahead {
		color: var(--good);
	}

	/* ---- the organization: one row per league ---- */
	.leagues {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.leagues li + li {
		border-top: 1px solid var(--rule);
	}
	.leagues button {
		display: grid;
		grid-template-columns: 3.4rem minmax(9rem, 1fr) auto minmax(17rem, max-content) 1rem;
		align-items: center;
		gap: 0.4rem 1.25rem;
		width: 100%;
		padding: 0.7rem 1rem;
		border: 0;
		border-left: 3px solid var(--sport);
		border-radius: 0;
		background: transparent;
		text-align: left;
		font-weight: 400;
		white-space: normal;
	}
	.leagues .quiet button {
		border-left-color: transparent;
	}
	.leagues button:hover {
		background: var(--surface-2);
	}
	.leagues button > :global(.badge) {
		justify-self: start;
	}
	.leagues button > :global(svg) {
		color: var(--ink-faint);
	}
	.team {
		display: grid;
		gap: 0.1rem;
		min-width: 0;
	}
	.team strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.score {
		font-family: var(--mono);
	}
	.stats {
		display: flex;
		justify-content: end;
		gap: 1.25rem;
		font: 650 0.85rem var(--mono);
	}
	.stats span {
		display: grid;
		justify-items: end;
		white-space: nowrap;
		gap: 0.15rem;
	}
	.flags {
		display: flex;
		flex-wrap: wrap;
		justify-content: end;
		gap: 0.3rem;
	}
	@media (max-width: 720px) {
		.leagues button {
			grid-template-columns: 3.4rem minmax(0, 1fr) 1rem;
		}
		.stats,
		.flags {
			grid-column: 2;
			justify-content: start;
		}
		.stats span {
			justify-items: start;
		}
		.flags:empty {
			display: none;
		}
		.leagues button > :global(svg) {
			grid-row: 1;
			grid-column: 3;
		}
	}

	/* ---- draft picks: years down, rounds across, a count in each ---- */
	.pick-grids {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem 2rem;
		padding: 0.9rem 1rem 0.2rem;
	}
	.pick-grid {
		width: auto;
		border-collapse: collapse;
	}
	.pick-grid th,
	.pick-grid td {
		padding: 0;
		border: 0;
		background: none;
		text-align: center;
	}
	.pick-grid thead th {
		padding-bottom: 0.3rem;
		font: 600 0.62rem var(--mono);
		color: var(--ink-faint);
	}
	.pick-grid .draft {
		text-align: left;
	}
	.pick-grid tbody th {
		padding-right: 0.6rem;
		font: 600 0.72rem var(--mono);
		color: var(--ink-soft);
		text-align: left;
	}
	.held {
		position: relative;
		width: 2rem;
		height: 1.7rem;
		margin: 1px;
		padding: 0;
		border: 1px solid var(--rule);
		border-radius: 4px;
		background: var(--surface-2);
		font: 650 0.8rem var(--mono);
	}
	.held.none {
		color: var(--ink-faint);
		background: transparent;
		border-style: dashed;
	}
	.held.acquired {
		color: var(--brand);
		background: var(--brand-soft);
		border-color: var(--brand);
	}
	.tip {
		position: absolute;
		bottom: calc(100% + 6px);
		left: 50%;
		z-index: 5;
		width: max-content;
		max-width: 14rem;
		padding: 0.3rem 0.5rem;
		border-radius: 5px;
		background: var(--ink);
		color: var(--surface);
		font: 500 0.72rem/1.3 var(--body);
		white-space: normal;
		pointer-events: none;
		opacity: 0;
		transform: translate(-50%, 3px);
		transition:
			opacity 120ms,
			transform 120ms;
	}
	.held:hover .tip,
	.held:focus-visible .tip {
		opacity: 1;
		transform: translate(-50%, 0);
	}
	.legend {
		padding-top: 0.4rem;
		font-size: 0.75rem;
	}
	.acquired-key {
		color: var(--brand);
		font-weight: 650;
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
	.fact {
		display: grid;
		gap: 0.1rem;
	}
	.fact strong {
		font: 700 1.05rem/1.2 var(--display);
	}
	.matchup {
		margin-left: auto;
		color: var(--ink);
		text-align: right;
	}
	.matchup:hover {
		text-decoration: none;
	}
	.signing {
		border-top: 3px solid var(--gold);
	}
	.left {
		margin-left: auto;
		font: 650 0.85rem var(--mono);
		color: var(--gold);
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
		max-width: 34rem;
		padding: 0.4rem 1rem 0.9rem;
	}
	.listhead {
		padding: 0.4rem 1rem;
		background: var(--surface-2);
		border-block: 1px solid var(--rule);
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
	.injury-swap {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin-top: 0.4rem;
	}
	.injury-swap select {
		max-width: 15rem;
		font-size: 0.8rem;
	}
	.columns {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(19rem, 1fr));
		gap: 1rem;
		align-items: start;
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
</style>
