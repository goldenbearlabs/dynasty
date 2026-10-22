<script lang="ts">
	// A franchise across every sport. The organization view says where each
	// team stands at a glance; picking a league opens that team: its lineup,
	// its matchup, the standings, and its roster and reserve list.
	import { untrack } from 'svelte';
	import { goto, snapshot, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { capturePageScroll, restorePageScroll } from '#lib/playerNavigation.svelte.ts';
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
	import SeasonRoster from '#lib/SeasonRoster.svelte';
	import { dropQuestion } from '#lib/dropPlayer.ts';
	import { draftTag, draftTitle } from '#lib/draftSlot.ts';
	import PlayerActions from '#lib/PlayerActions.svelte';
	import PlayerNickname from '#lib/PlayerNickname.svelte';
	import TeamIdentityEditor from '#lib/TeamIdentityEditor.svelte';
	import WatchStar from '#lib/WatchStar.svelte';
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { clockTime, dayLabel, points, timeLeft as left } from '#lib/ui/time.ts';
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
	// A league whose season is open and still waiting for this franchise's roster.
	const waiting = (r: LeagueRoster) => !!r.season && !r.season.set_at && !r.season.expired;
	let settingSeason = $state(untrack(() => page.url.searchParams.get('season') === 'roster'));
	function setSeasonRoster(competition: string) {
		show(competition);
		settingSeason = true;
	}

	// Another team's page, on the same view.
	function switchTeam(slug: string) {
		goto(`/franchise/${slug}${page.url.search}`);
	}

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
	const timeLeft = (until: string) => left(until, clock);
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
	let managing = $state(false); // the full main and reserve lists, for moving players between them
	$effect(() => {
		void league?.id;
		void team.id;
		plan = undefined;
	});
	const on = (r: LeagueRoster, list: List) => r.players.filter((p) => (plan?.[p.player_id] ?? p.list) === list);
	const reserve = $derived(roster ? on(roster, 'reserve') : []);
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
		const inLeague = leagues.find((l) => l.id === r.league_id);
		if (action === 'drop' && inLeague && !confirm(dropQuestion(inLeague, player.full_name, player.list === 'rights'))) return;
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
	snapshot({
		id: 'team-view',
		capture: () =>
			$state.snapshot({ view, managing, tables, periods, openSlots, scroll: capturePageScroll() }),
		restore: (s) => {
			view = s.view;
			managing = s.managing;
			tables = s.tables;
			periods = s.periods;
			openSlots = s.openSlots;
			restorePageScroll(s.scroll);
		},
	});
</script>

<svelte:head><title>{team.name}</title></svelte:head>

<div class="stack page">
	<header class="head">
		<Crest name={league ? roster?.team_name || team.name : team.name} src={league ? roster?.image_url || team.image_url : team.image_url} size={44} />
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
		{#if franchises.length > 1}
			<select class="switch" aria-label="Team to view" value={team.slug} onchange={(e) => switchTeam(e.currentTarget.value)}>
				{#each franchises as f (f.id)}
					<option value={f.slug}>{nameOf(f.id, league?.id)}{f.id === data.me?.id ? ' (you)' : ''}</option>
				{/each}
			</select>
		{/if}
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
		{@const due = canEdit ? cards.filter((c) => waiting(c.roster)) : []}
		{#if due.length}
			<section class="card season-due" aria-label="Season rosters to set">
				<div>
					<p class="eyebrow">Season {due.length === 1 ? 'roster' : 'rosters'} to set</p>
					<p class="small-text">
						The commissioner has opened {due.length === 1 ? 'a season' : 'seasons'}. Choose who is on {mine ? 'your' : 'the'} main roster and who goes on reserve
						for {due.length === 1 ? 'it' : 'each one'}.
					</p>
				</div>
				<div class="row">
					{#each due as c (c.roster.league_id)}
						<button class="primary" onclick={() => setSeasonRoster(c.roster.competition)}>
							<SportBadge sport={c.roster.competition} solid /> Set {c.roster.name} roster
							{#if c.roster.season?.deadline}<small class="locks">· locks in {timeLeft(c.roster.season.deadline)}</small>{/if}
						</button>
					{/each}
				</div>
			</section>
		{/if}
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
									{#if canEdit && waiting(c.roster)}<span class="pill gold">Set season roster{c.roster.season?.deadline ? ` · ${timeLeft(c.roster.season.deadline)} left` : ''}</span>{/if}
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
		<div class="stack league-view" data-sport={league.competition}>
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

			{#if canEdit && waiting(roster)}
				{#if settingSeason}
					<SeasonRoster {roster} franchiseId={team.id} force={!mine} oncancel={() => (settingSeason = false)} />
				{:else}
					<section class="card season-due" aria-label="Set your season roster">
						<div>
							<p class="eyebrow">{roster.season!.year} season · {roster.season!.started ? 'under way since' : 'starts'} {dayLabel(roster.season!.starts_on)}</p>
							<p>
								<strong>Set {mine ? 'your' : 'this'} season roster{roster.season!.started ? ' now' : ''}.</strong> Choose who is on the main roster and who goes on
								reserve. It is set once, and then holds for the season.
							</p>
							{#if roster.season!.deadline}
								<p class="locks-at">
									<Icon name="clock" size={14} /> Lists lock automatically in <strong>{timeLeft(roster.season!.deadline)}</strong> ({clockTime(roster.season!.deadline)})
									with the roster as it stands if {mine ? 'you have' : 'it has'} not set it.
								</p>
							{/if}
						</div>
						<button class="primary" onclick={() => (settingSeason = true)}>Set season roster</button>
					</section>
				{/if}
			{/if}

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
												<a href="/player/{player.player_id}"><strong>{player.full_name}</strong></a><WatchStar playerID={player.player_id} playerName={player.full_name} /><ProspectRank id={player.player_id} />
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

			<section class="stack tight player-views">
				<div class="spread">
					<h2>Lineup</h2>
					{#if mine}<a class="small-text" href="/watchlist?competition={league.competition}">{league.name} watchlist →</a>{/if}
				</div>
				{#key league.id + team.id}
					<LineupEditor {league} franchise={team} {roster} competition={data.competitions.find(c => c.key === league.competition)!} {canEdit} commissioner={data.me?.is_commissioner === true} />
				{/key}
			</section>



			<!-- The reserve list at a glance; the main roster is the lineup above. -->
			<section class="panel">
				<h2 class="bar">
					Reserve
					<span class="muted small-text">{reserve.length}/{roster.limits.reserve} · main {on(roster, 'main').length}/{roster.limits.main}</span>
					{#if roster.overage > 0}<span class="pill bad">{roster.overage} over</span>{/if}
					{#if roster.season?.locked}<span class="pill" title="Lists held until {dayLabel(roster.season.ends_on)}"><Icon name="lock" size={11} /> Lists set</span>{/if}
					{#if canEdit}<button class="small quiet count" aria-expanded={managing} onclick={() => (managing = !managing)}>{managing ? 'Done' : 'Manage lists'}</button>{/if}
				</h2>
				{#if reserve.length === 0}
					<p class="muted small-text pad">Nobody on reserve.</p>
				{:else}
					<ul class="reserve">
						{#each reserve as player (player.player_id)}
							<li>
								<Headshot name={player.full_name} src={player.headshot_url} size={22} />
								<a href="/player/{player.player_id}" title={player.full_name}>{player.full_name}</a>
								<span class="muted pos">{player.positions.join('/')}</span>
								{#if player.team_abbrev}<TeamLogo competition={roster.competition} abbrev={player.team_abbrev} size={16} />{/if}
								<InjuryBadge designation={player.injury_designation} />
								<ProspectRank id={player.player_id} />
								{#if player.status === 'prospect' && draftTag(player)}<span class="pill draft" title={draftTitle(player)}>{draftTag(player)}</span>{/if}
								{#if player.injury_reserve_locked || player.locked_until}<span title={player.locked_until ? `Locked until ${clockTime(player.locked_until)}` : 'Injury swap: on reserve for the season'}><Icon name="lock" size={11} /></span>{/if}
								<span class="end"><PlayerActions leagueId={roster.league_id} playerId={player.player_id} ownerId={team.id} name={player.full_name} detail={`${player.positions.join('/')} · Reserve`} onDrop={canEdit ? () => change(roster, 'drop', player) : undefined} /></span>
							</li>
						{/each}
					</ul>
				{/if}
			</section>

			{#if managing && canEdit}
			<section class="panel">
				<h2 class="bar">Main roster and reserve list</h2>
				<div class="meters">
					<Meter label="Main" value={on(roster, 'main').length} max={roster.limits.main} />
					<Meter label="Reserve" value={on(roster, 'reserve').length} max={roster.limits.reserve} />
				</div>
				{#if canEdit && mine && roster.season?.locked}
					<p class="pad small-text muted">
						Everyone stays on the list he is on until the season ends. You can still drop a player, add a free agent, make a trade, or send an
						injured player down with an injury swap.
					</p>
				{:else if canEdit}
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
															<a href="/player/{player.player_id}"><strong>{player.full_name}</strong></a><WatchStar playerID={player.player_id} playerName={player.full_name} /><ProspectRank id={player.player_id} /><PlayerActions leagueId={roster.league_id} playerId={player.player_id} ownerId={team.id} name={player.full_name} detail={`${player.positions.join('/')} · ${list.title}`} onDrop={canEdit && !plan ? () => change(roster, 'drop', player) : undefined} />
															<InjuryBadge designation={player.injury_designation} />
																{#if player.class}<span class="pill">{player.class}</span>{/if}
															{#if player.status === 'prospect'}<span class="pill gold">Prospect</span>{#if draftTag(player)}<span class="pill draft" title={draftTitle(player)}>{draftTag(player)}</span>{/if}{/if}
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
														{#if !(mine && roster.season?.locked)}<button class="small" disabled={player.injury_reserve_locked || (mine && !!player.locked_until)} onclick={() => change(roster, 'move', player, list.other)}>{list.move}</button>{/if}
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
			</section>
			{/if}

			<div class="columns">
				<section class="panel">
					<h2 class="bar">Standings</h2>
					{#if !now.table || now.table.rows.length === 0}
						<p class="muted small-text pad">No season yet.</p>
					{:else}
						<table class="standings">
							<tbody>
								{#each now.table.rows as row, i (row.franchise_id)}
									<tr class:me={row.franchise_id === team.id} class:cut={now.headToHead && i === league.settings.format.playoff_teams - 1 && i < now.table.rows.length - 1}>
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
	.player-views { min-width: 0; grid-template-columns: minmax(0,1fr); }
	/* Grid children may shrink below their content, so a wide table
	   scrolls inside itself instead of widening the page on a phone. */
	.page,
	.league-view {
		grid-template-columns: minmax(0, 1fr);
	}
	.page {
		gap: 0.75rem;
	}
	.league-view {
		gap: 0.6rem;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 0.8rem;
	}
	.switch {
		max-width: 14rem;
		font-size: 0.85rem;
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
		margin-top: 0.15rem;
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.45rem 0.9rem;
		font-size: 0.95rem;
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
		.stats {
			flex-wrap: wrap;
			gap: 0.4rem 1rem;
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

	/* A season waiting for a roster: hard to miss. */
	.season-due {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.8rem 1.5rem;
		padding: 0.9rem 1rem;
		border: 1px solid var(--gold);
		border-left-width: 4px;
		background: color-mix(in srgb, var(--gold) 8%, var(--surface));
	}
	.season-due .eyebrow {
		color: var(--gold);
	}
	.season-due p:not(.eyebrow) {
		margin-top: 0.2rem;
		max-width: 44rem;
	}
	.locks-at {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--gold);
		font-size: 0.85rem;
	}
	.locks {
		font: 500 0.8rem var(--body);
		letter-spacing: 0;
		text-transform: none;
		color: inherit;
		opacity: 0.85;
	}

	/* ---- one league ---- */
	.summary {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1.6rem;
		align-items: end;
		padding: 0.5rem 0.9rem;
		background: var(--surface);
		border-block: 1px solid var(--rule);
		border-top: 3px solid var(--sport);
	}
	.fact {
		display: grid;
		gap: 0.1rem;
	}
	.fact strong {
		font: 700 1rem/1.2 var(--display);
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
	/* The reserve list: one short line a player, in columns. */
	.reserve {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		margin: 0;
		padding: 0.3rem 0;
		list-style: none;
	}
	.reserve li {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		padding: 0.2rem 0.9rem;
		font-size: 0.82rem;
	}
	.reserve a {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--ink);
		font-weight: 600;
	}
	.reserve .pos {
		flex: none;
		font: 600 0.68rem var(--mono);
	}
	.reserve .end {
		margin-left: auto;
	}
	.draft {
		flex: none;
		font: 650 0.6rem var(--mono);
		white-space: nowrap;
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
	/* The playoff cutoff: a line under the last team in. */
	.standings tr.cut td {
		border-bottom: 2px solid var(--sport, var(--brand));
	}
	@media (max-width: 640px) {
		.page {
			gap: 0.55rem;
		}
		/* Two rows: the crest beside the name and who runs it; then the
		   team switcher and the trade button, side by side. */
		.head {
			display: grid;
			grid-template-columns: auto minmax(0, 1fr) auto;
			grid-template-areas: 'crest info info' 'switch switch action';
			gap: 0.55rem 0.6rem;
		}
		.head > :global(:first-child) {
			grid-area: crest;
		}
		.head > .grow {
			grid-area: info;
		}
		.head > .switch {
			grid-area: switch;
			max-width: none;
			min-width: 0;
			padding: 0.35rem 0.5rem;
			font-size: 0.8rem;
		}
		.head > .button {
			grid-area: action;
			padding: 0.35rem 0.65rem;
			font-size: 0.8rem;
			white-space: nowrap;
		}
		/* With no team switcher, the trade button takes the row. */
		.head > .button:nth-child(3) {
			grid-column: 1 / -1;
			justify-self: start;
		}
		.title {
			gap: 0.3rem;
		}
		.title h1 {
			font-size: 1.3rem;
			overflow-wrap: normal;
			line-height: 1.15;
		}
		.meta {
			gap: 0.25rem 0.4rem;
			margin-top: 0.1rem;
			font-size: 0.75rem;
		}
		.meta .pill {
			padding: 0.05rem 0.35rem;
			font-size: 0.58rem;
		}
		/* Record, place and points side by side; the matchup beneath. */
		.summary {
			display: grid;
			grid-template-columns: repeat(3, auto);
			justify-content: start;
			gap: 0.35rem 1.2rem;
			padding: 0.45rem 0.75rem;
		}
		.fact strong {
			font-size: 0.9rem;
		}
		.matchup {
			grid-column: 1 / -1;
			margin-left: 0;
			text-align: left;
		}
		.reserve {
			grid-template-columns: minmax(0, 1fr);
		}
		.reserve li {
			padding-inline: 0.75rem;
		}
		.bar {
			padding: 0.4rem 0.75rem;
		}
	}
</style>
