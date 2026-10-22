<script lang="ts">
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import TradeFlag from '#lib/TradeFlag.svelte';
	import DropToAdd from '#lib/DropToAdd.svelte';
	import { dropPlayer } from '#lib/dropPlayer.ts';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import WatchStar from '#lib/WatchStar.svelte';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	import { page } from '$app/state';
	import { snapshot, invalidateAll } from '$app/navigation';
	import {
		getPlayerProfile,
		getPlayerTimeline,
		getPlayerGameLog,
		changeRoster,
		type PlayerProfile,
		type PlayerEvent,
		type ProfileGame,
		type List,
	} from '#lib/api.ts';
	import MetricHelp from '#lib/research/MetricHelp.svelte';
	import ColumnHeader from '#lib/research/ColumnHeader.svelte';
	import PlayerNickname from '#lib/PlayerNickname.svelte';
	import PlayerSearch from '#lib/PlayerSearch.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { playerNavigation } from '#lib/playerNavigation.svelte.ts';
	import ResearchChart from '#lib/research/ResearchChart.svelte';
	import { advanced, basics, formatValue as fmt } from '#lib/research/columns.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let profile = $state<PlayerProfile>(),
		error = $state(''),
		loading = $state(true),
		retry = $state(0);
	let seasonKey = $state(''),
		rawMode = $state('totals');
	let chartMetric = $state('league_index'),
		chartKind = $state('scatter'),
		chartSport = $state('');
	let games = $state<ProfileGame[]>([]),
		gameTotal = $state(0),
		gamePage = $state(1),
		gameSport = $state('');
	let events = $state<PlayerEvent[]>([]),
		eventTotal = $state(0),
		eventPage = $state(1);
	let gamesBusy = $state(false),
		eventsBusy = $state(false),
		adding = $state(false);
	let historyFilter = $state(''),
		gamesError = $state(''),
		eventsError = $state('');
	let gameRequest = 0,
		eventRequest = 0;
	let expandedGames = $state(false),
		moreStats = $state(false),
		perGame = $state(false),
		seasonSport = $state('');
	let advancedOpen = $state(false),
		historyOpen = $state(false);
	snapshot({
		id: 'player-origin',
		capture: () => ({ ...playerNavigation }),
		restore: (saved) => Object.assign(playerNavigation, saved),
	});
	const origin = $derived(
		playerNavigation.destination === page.url.pathname ? playerNavigation : null,
	);
	function goBack(event: MouseEvent) {
		if (
			origin?.from &&
			!event.metaKey &&
			!event.ctrlKey &&
			!event.shiftKey &&
			!event.altKey &&
			event.button === 0
		) {
			event.preventDefault();
			history.back();
		}
	}
	const displayedGames = $derived(expandedGames ? games : games.slice(0, 5));
	const displayedSeasons = $derived(
		(profile?.seasons ?? []).filter((s) => !seasonSport || s.competition === seasonSport),
	);
	function orderStats(keys: string[], sport: string) {
		const configured =
			data.competitions.find((c) => c.key === sport)?.stats.map((s) => s.key) ?? [];
		return [
			...configured.filter((key) => keys.includes(key)),
			...keys.filter((key) => !configured.includes(key)).sort(),
		];
	}
	const seasonStats = $derived(
		orderStats(
			[...new Set(displayedSeasons.flatMap((s) => Object.keys(s.stats)))],
			seasonSport || profile?.player.competition || '',
		),
	);
	const visibleSeasonStats = $derived(moreStats ? seasonStats : seasonStats.slice(0, 6));
	function explore(key: string) {
		seasonKey = key;
		advancedOpen = true;
	}

	const id = $derived(page.params.id!);
	const season = $derived(profile?.seasons.find((s) => s.key === seasonKey));
	const research = $derived(season?.research);
	const own = $derived(
		profile?.ownership.find((o) => o.competition === profile?.player.competition),
	);
	const waiver = $derived(
		profile?.waivers.find((w) => w.competition === profile?.player.competition),
	);
	const league = $derived(
		data.dynasty?.leagues.find((l) => l.competition === profile?.player.competition),
	);
	const nameSport = (key: string) =>
		data.competitions.find((c) => c.key === key)?.name ?? key.toUpperCase();
	const labelStat = (key: string, sport: string) =>
		data.competitions.find((c) => c.key === sport)?.stats.find((s) => s.key === key)?.label ??
		key.replaceAll('_', ' ').toUpperCase();
	const date = (value: string | null | undefined) =>
		value
			? new Date(
					/^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T12:00:00` : value,
				).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
			: '—';
	const age = $derived.by(() => {
		if (!profile?.player.birth_date) return null;
		const b = new Date(`${profile.player.birth_date.slice(0, 10)}T12:00:00`),
			now = new Date();
		return (
			now.getFullYear() -
			b.getFullYear() -
			(now.getMonth() < b.getMonth() ||
			(now.getMonth() === b.getMonth() && now.getDate() < b.getDate())
				? 1
				: 0)
		);
	});
	const seasonSports = $derived([...new Set(profile?.seasons.map((s) => s.competition) ?? [])]);
	const chartOptions = $derived([
		...basics,
		...advanced,
		...Object.keys(
			profile?.seasons
				.filter((s) => s.competition === chartSport)
				.reduce((a, s) => ({ ...a, ...s.stats }), {} as Record<string, number>) ?? {},
		)
			.sort()
			.map((key) => ({
				key: `raw:${key}`,
				label: labelStat(key, chartSport),
				help: 'Imported season total.',
			})),
	]);
	const chartPoints = $derived(
		(profile?.seasons ?? [])
			.filter((s) => s.competition === chartSport)
			.map((s) => ({
				key: s.key,
				label: `${s.season} · ${s.team || s.league || nameSport(s.competition)}`,
				group: `${s.season}${s.league ? ` · ${s.league}` : ''}`,
				x: s.year,
				y: chartMetric.startsWith('raw:')
					? (s.stats[chartMetric.slice(4)] ?? null)
					: chartMetric === 'games'
						? s.games
						: chartMetric === 'points'
							? s.points
							: chartMetric === 'points_per_game'
								? s.points_per_game
								: ((s.research as unknown as Record<string, number | null> | null)?.[chartMetric] ??
									null),
			})),
	);

	const rawKeys = $derived(Object.keys(season?.stats ?? {}).sort());
	const gameKeys = $derived(
		orderStats(
			[...new Set(displayedGames.flatMap((g) => Object.keys(g.stats)))],
			gameSport || profile?.player.competition || '',
		),
	);
	const visibleEvents = $derived(events.filter((e) => !historyFilter || e.kind === historyFilter));
	const eventKinds = $derived([...new Set(events.map((e) => e.kind))].sort());
	const human = (value: string) => value.replaceAll('_', ' ').replace(/^./, (s) => s.toUpperCase());
	function eventDetail(e: PlayerEvent) {
		const d = e.detail ?? {};
		const parts: string[] = [];
		if (d.from && d.to) parts.push(`${human(String(d.from))} → ${human(String(d.to))}`);
		else if (d.list) parts.push(`${human(String(d.list))} roster`);
		if (e.kind === 'claim' && typeof d.bid === 'number') parts.push(`Winning bid: ${d.bid}`);
		if (d.reason) parts.push(String(d.reason));
		if (d.reversed) parts.push('Reversed');
		if (e.draft_name)
			parts.push(`${e.draft_name} · Round ${e.pick_round}, pick ${e.pick_position}`);
		return parts.join(' · ');
	}
	function apply(p: PlayerProfile) {
		profile = p;
		seasonKey =
			p.seasons.find((s) => s.competition === p.player.competition)?.key ?? p.seasons[0]?.key ?? '';
		chartSport = p.seasons.some((s) => s.competition === p.player.competition)
			? p.player.competition
			: (p.seasons[0]?.competition ?? p.player.competition);
		games = p.game_log.games;
		gameTotal = p.game_log.total;
		gamePage = 1;
		gameSport = '';
		gamesBusy = false;
		gamesError = '';
		events = p.timeline.events;
		eventTotal = p.timeline.total;
		eventPage = 1;
		eventsBusy = false;
		eventsError = '';
		historyFilter = '';
	}
	$effect(() => {
		const playerID = id;
		retry;
		let active = true;
		gameRequest++;
		eventRequest++;
		profile = undefined;
		error = '';
		loading = true;
		expandedGames = false;
		moreStats = false;
		perGame = false;
		seasonSport = '';
		advancedOpen = false;
		historyOpen = false;
		getPlayerProfile(playerID)
			.then((p) => {
				if (active) apply(p);
			})
			.catch((e) => {
				if (active) error = e instanceof Error ? e.message : 'Could not load this player.';
			})
			.finally(() => {
				if (active) loading = false;
			});
		return () => {
			active = false;
		};
	});
	async function loadGames(reset = false) {
		const playerID = id,
			request = ++gameRequest,
			next = reset ? 1 : gamePage + 1,
			sport = gameSport;
		gamesBusy = true;
		gamesError = '';
		if (reset) {
			games = [];
			gameTotal = 0;
		}
		try {
			const result = await getPlayerGameLog(playerID, next, sport);
			if (request === gameRequest && playerID === id) {
				games = reset ? result.games : [...games, ...result.games];
				gamePage = result.page;
				gameTotal = result.total;
			}
		} catch (e) {
			if (request === gameRequest)
				gamesError = e instanceof Error ? e.message : 'Game log could not load.';
		} finally {
			if (request === gameRequest) gamesBusy = false;
		}
	}
	async function loadEvents() {
		const playerID = id,
			request = ++eventRequest;
		eventsBusy = true;
		eventsError = '';
		try {
			const result = await getPlayerTimeline(playerID, eventPage + 1);
			if (request === eventRequest && playerID === id) {
				events = [...events, ...result.events];
				eventPage = result.page;
				eventTotal = result.total;
			}
		} catch (e) {
			if (request === eventRequest)
				eventsError = e instanceof Error ? e.message : 'History could not load.';
		} finally {
			if (request === eventRequest) eventsBusy = false;
		}
	}
	// An add to a full list asks who to drop to make room, and does both at once.
	let makingRoom = $state<List>();
	const myRoster = $derived(
		data.myTeam?.rosters.find((r) => r.competition === profile?.player.competition),
	);
	const isFull = (list: List) =>
		!!myRoster &&
		myRoster.players.filter((p) => p.list === list).length >=
			(list === 'reserve' ? myRoster.limits.reserve : myRoster.limits.main);
	// Dropping your own player, from here: he goes on waivers like any drop.
	async function dropOwn(franchiseId: string) {
		if (!league || !profile) return;
		const playerID = id;
		if (await dropPlayer(league, franchiseId, playerID, profile.player.full_name)) {
			const p = await getPlayerProfile(playerID);
			if (id === playerID) apply(p);
			await invalidateAll();
		}
	}
	async function add(list: List, drop?: string) {
		if (!league || !data.me) return;
		if (!drop && isFull(list)) {
			makingRoom = list;
			return;
		}
		const playerID = id;
		adding = true;
		try {
			await changeRoster(league.id, 'add', {
				player_id: playerID,
				list,
				drop_player_id: drop,
				franchise_id: data.me.id,
			});
			toast.good(drop ? 'Player added. The player you dropped is on waivers.' : 'Player added.');
			const p = await getPlayerProfile(playerID);
			if (id === playerID) apply(p);
			await invalidateAll();
		} catch (e) {
			toast.error(e);
		} finally {
			adding = false;
		}
	}
	function exportStats() {
		if (!profile) return;
		const keys = [...new Set(profile.seasons.flatMap((s) => Object.keys(s.stats)))].sort();
		const cell = (v: unknown) => `"${String(v ?? '').replaceAll('"', '""')}"`;
		const rows = [
			[
				'Player',
				'League',
				'Season',
				'Team',
				'Games',
				'Fantasy points',
				'FP/game',
				'League+',
				'Above replacement',
				...keys,
			],
			...profile.seasons.map((s) => [
				profile!.player.full_name,
				s.league || nameSport(s.competition),
				s.season,
				s.team,
				s.games,
				s.points,
				s.points_per_game,
				s.research?.league_index,
				s.research?.points_above_replacement,
				...keys.map((k) => s.stats[k]),
			]),
		];
		const url = URL.createObjectURL(
			new Blob([rows.map((r) => r.map(cell).join(',')).join('\n')], {
				type: 'text/csv;charset=utf-8',
			}),
		);
		const a = document.createElement('a');
		a.href = url;
		a.download = `${profile.player.full_name}-career.csv`;
		a.click();
		setTimeout(() => URL.revokeObjectURL(url), 1000);
	}
</script>

<svelte:head><title>{profile?.player.full_name ?? 'Player profile'}</title></svelte:head>
<div class="profile">
	<nav class="profile-nav">
		<a href={origin?.from || '/players'} onclick={goBack}>← {origin?.label || 'Players'}</a>
		<details class="find-player">
			<summary>Find another player</summary><PlayerSearch competitions={data.competitions} />
		</details>
	</nav>
	{#if loading}<p class="empty" role="status">Loading player…</p>
	{:else if error}<div class="empty">
			<p role="alert">{error}</p>
			<button onclick={() => retry++}>Try again</button>
		</div>
	{:else if profile}
		{@const p = profile.player}
		<header class="bio">
			<div class="identity">
				<Headshot name={p.full_name} src={p.headshot_url} size={64} />
				<div class="identity-copy">
					<div class="name-row">
						<h1>{p.full_name}</h1>
						<ProspectRank id={p.id} ranking={p} /><WatchStar
							playerID={p.id}
							playerName={p.full_name}
						/>
					</div>
					<div class="bio-line">
						<span>{p.positions?.join(' / ') || 'Position unavailable'}</span
						>{#if p.team_abbrev}<TeamLogo
								competition={p.competition}
								abbrev={p.team_abbrev}
								size={22}
							/>{/if}<span>{nameSport(p.competition)}</span><InjuryBadge
							designation={p.injury_designation}
						/>
					</div>
					<div class="bio-line muted small-text">
						{#if age !== null}<span>Age {age}</span>{/if}{#if p.birth_date}<span
								>Born {date(p.birth_date)}</span
							>{/if}{#if p.class}<span>{p.class}</span>{/if}<span>{human(p.status)}</span
						>{#if p.prospect_rank != null && p.prospect_eta}<span>MLB ETA {p.prospect_eta}</span
							>{/if}
					</div>
				</div>
			</div>
			{#if p.note}<p class="bio-note muted small-text">{p.note}</p>{/if}
		</header>
		<section class="roster-status" aria-label="Fantasy status">
			<div class="ownership">
				{#if own}<div>
						<a href="/franchise/{own.franchise_slug}"><strong>{own.franchise_name}</strong></a><span
							class="muted"
						>
							· {human(own.list)} roster{own.slot ? ` · ${own.slot}` : ''}</span
						>
					</div>{:else}<strong class:available={!waiver}
						>{waiver ? 'On waivers' : 'Available'}</strong
					>{#if waiver}<span class="muted small-text">Clears {date(waiver.clears_at)}</span
						>{:else if !league}<span class="muted small-text">No dynasty league for this sport</span
						>{/if}{/if}
				{#if !profile.starter_eligible}<details class="eligibility">
						<summary>ⓘ Starting restriction</summary>
						<p>
							{profile.eligibility_note ||
								'This player does not currently meet league starting rules.'}
						</p>
					</details>{/if}
			</div>
			{#if data.me && league}<div class="roster-actions">
					{#if !own && waiver}<a class="action-link" href="/waivers?competition={p.competition}"
							>Claim on waivers →</a
						>{:else if !own}<button class="small" disabled={adding} onclick={() => add('main')}
							>+ Add to roster</button
						><button class="small quiet" disabled={adding} onclick={() => add('reserve')}
							>Add to reserve</button
						>{:else if own.franchise_id !== data.me.id}<TradeFlag
							leagueId={own.league_id}
							playerId={p.id}
							ownerId={own.franchise_id}
							name={p.full_name}
						/><a class="action-link" href="/trades/new?with={own.franchise_slug}&player={p.id}"
							>Propose trade →</a
						>{:else}<TradeFlag
							leagueId={own.league_id}
							playerId={p.id}
							ownerId={own.franchise_id}
							name={p.full_name}
						/><a href="/franchise/{own.franchise_slug}">Your roster →</a><button
							class="small quiet danger"
							onclick={() => dropOwn(own.franchise_id)}>− Drop</button
						>{/if}
				</div>{/if}
		</section>
		<section class="primary-section" aria-labelledby="season-heading">
			<div class="section-heading">
				<h2 id="season-heading">Season by season</h2>
				<div class="section-controls">
					{#if seasonSports.length > 1}<select
							aria-label="Season history sport"
							bind:value={seasonSport}
							><option value="">All sports</option>{#each seasonSports as sport}<option
									value={sport}>{nameSport(sport)}</option
								>{/each}</select
						>{/if}<label class="check"
						><input type="checkbox" bind:checked={perGame} /> Per game</label
					><button class="small quiet" onclick={exportStats} disabled={!profile.seasons.length}
						>Export CSV</button
					>
				</div>
			</div>
			{#if displayedSeasons.length}<!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard access to horizontally scrolling statistics.) -->
				<div class="table-wrap" tabindex="0" role="region" aria-label="Season statistics">
					<table>
						<thead
							><tr
								><th>Season</th><th>Team / league</th><ColumnHeader
									label="GP"
									help="Recorded appearances this season."
								/><ColumnHeader
									label={perGame ? 'FP/G' : 'FP'}
									help="Fantasy points under current scoring rules; toggle Per game to see points per appearance."
								/><ColumnHeader
									label="League+"
									help="Season production compared with qualified peers. League average is 100."
								/>{#each visibleSeasonStats as key}<ColumnHeader
										label={labelStat(key, seasonSport || p.competition)}
										help={key.replaceAll('_', ' ')}
									/>{/each}</tr
							></thead
						><tbody
							>{#each displayedSeasons as s (s.key)}<tr
									><th scope="row"
										><button
											class="text-button"
											title="Explore this season"
											onclick={() => explore(s.key)}>{s.season}</button
										></th
									><td
										>{s.team || '—'}<span class="muted team-league"
											>{s.league || nameSport(s.competition)}</span
										></td
									><td>{s.games}</td><td class="points"
										>{fmt(perGame ? s.points_per_game : s.points)}</td
									><td>{fmt(s.research?.league_index)}</td>{#each visibleSeasonStats as key}<td
											>{fmt(
												s.stats[key] == null ? null : s.stats[key] / (perGame ? s.games || 1 : 1),
												perGame ? 2 : 1,
											)}</td
										>{/each}</tr
								>{/each}</tbody
						>
					</table>
				</div>
				<div class="section-foot">
					<span class="muted small-text">Current scoring · League+ average 100</span
					>{#if seasonStats.length > 6}<button
							class="text-button small-text"
							onclick={() => (moreStats = !moreStats)}
							>{moreStats ? 'Fewer columns' : 'All stat columns'}</button
						>{/if}
				</div>{:else}<p class="empty muted">No season stats have been imported yet.</p>{/if}
		</section>
		<section class="primary-section" aria-labelledby="games-heading">
			<div class="section-heading">
				<h2 id="games-heading">{expandedGames ? 'Game log' : 'Last 5 games'}</h2>
				{#if seasonSports.length > 1}<select
						aria-label="Game log sport"
						bind:value={gameSport}
						onchange={() => {
							expandedGames = false;
							loadGames(true);
						}}
						><option value="">All sports</option>{#each seasonSports as sport}<option value={sport}
								>{nameSport(sport)}</option
							>{/each}</select
					>{/if}
			</div>
			{#if games.length}<!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard access to horizontally scrolling statistics.) -->
				<div class="table-wrap" tabindex="0" role="region" aria-label="Game statistics">
					<table>
						<thead
							><tr
								><th>Date</th><th>Game</th>{#if seasonSports.length > 1}<th>Sport</th
									>{/if}<ColumnHeader
									label="FP"
									help="Fantasy value of this game under current scoring. Saved lineups and weekly limits determine which games count."
								/>{#each gameKeys as key}<ColumnHeader
										label={labelStat(key, gameSport || p.competition)}
										help={key.replaceAll('_', ' ')}
									/>{/each}</tr
							></thead
						><tbody
							>{#each displayedGames as g (g.id)}<tr
									><th scope="row">{date(g.day)}</th><td
										><a
											class="game-matchup"
											href="/game/{g.id}"
											aria-label="{g.away_abbrev} at {g.home_abbrev}"
											><TeamLogo
												competition={g.competition}
												abbrev={g.away_abbrev || '?'}
												size={20}
											/><span class="muted">@</span><TeamLogo
												competition={g.competition}
												abbrev={g.home_abbrev || '?'}
												size={20}
											/></a
										></td
									>{#if seasonSports.length > 1}<td>{g.competition.toUpperCase()}</td>{/if}<td
										class="points"
										>{fmt(g.points)}{#if g.scoring_source === 'defaults'}<span
												class="muted"
												title="Sport default scoring">*</span
											>{/if}</td
									>{#each gameKeys as key}<td>{fmt(g.stats[key], 3)}</td>{/each}</tr
								>{/each}</tbody
						>
					</table>
				</div>{:else if !gamesBusy}<p class="empty muted">No recorded final games yet.</p>{/if}
			{#if gamesError}<p role="alert">{gamesError}</p>
				<button class="small" onclick={() => loadGames(!games.length)}>Retry</button>{/if}
			<div class="section-foot">
				<span class="muted small-text" role="status"
					>{gamesBusy
						? 'Loading games…'
						: `${displayedGames.length} of ${gameTotal} recorded games`}</span
				>
				<div class="row">
					{#if !expandedGames && (games.length > 5 || gameTotal > 5)}<button
							class="small quiet"
							onclick={() => {
								expandedGames = true;
								if (games.length <= 5 && games.length < gameTotal) loadGames();
							}}>Show more games</button
						>{:else if expandedGames}<button
							class="small quiet"
							onclick={() => (expandedGames = false)}>Show fewer</button
						>{#if games.length < gameTotal}<button
								class="small"
								disabled={gamesBusy}
								onclick={() => loadGames()}>{gamesBusy ? 'Loading…' : 'Load more games'}</button
							>{/if}{/if}
				</div>
			</div>
		</section>
		<details class="disclosure" bind:open={advancedOpen}>
			<summary>Advanced stats & career charts</summary>
			<div class="details-body">
				{#if advancedOpen}
					<div class="section-heading">
						<label class="season-label"
							>Season <select aria-label="Explore a season" bind:value={seasonKey}
								>{#each profile.seasons as s (s.key)}<option value={s.key}
										>{s.season} · {s.league || nameSport(s.competition)} · {s.team ||
											'Team unavailable'}</option
									>{/each}</select
							></label
						>
					</div>
					{#if season}<p class="muted small-text">
							{season.games} appearances · {season.scoring_source === 'league'
								? 'League scoring'
								: 'Sport default scoring'} · Synced {date(season.synced_at)}
						</p>
						{#if season.research_note}<p class="muted small-text">{season.research_note}</p>{/if}
						<section class="detail-section stack">
							<div class="spread">
								<div>
									<span class="eyebrow">Full stat sheet</span>
									<h2>Raw stats & scoring formula</h2>
								</div>
								<Tabs
									tabs={[
										{ value: 'totals', label: 'Totals' },
										{ value: 'per-game', label: 'Per appearance' },
									]}
									bind:value={rawMode}
									label="Raw stat values"
								/>
							</div>
							<p class="muted small-text">
								Includes stats that score zero fantasy points. Per appearance divides by imported
								games; it is not a per-minute or per-start rate.
							</p>
							<div class="table-wrap">
								<table>
									<thead
										><tr
											><th>Stat</th><th>Raw value</th><ColumnHeader
												label="Weight"
												help="Current points awarded per unit of this stat."
											/><ColumnHeader
												label="Fantasy contribution"
												help="Raw value multiplied by the current scoring weight."
											/></tr
										></thead
									><tbody
										>{#each rawKeys as key}<tr
												><td
													>{labelStat(key, season.competition)}
													<small class="muted">({key})</small></td
												><td
													>{fmt(
														season.stats[key] / (rawMode === 'per-game' ? season.games || 1 : 1),
														3,
													)}</td
												><td>{fmt(profile.scoring_rules[season.competition]?.[key] ?? 0, 3)}</td><td
													>{fmt(
														(season.stats[key] *
															(profile.scoring_rules[season.competition]?.[key] ?? 0)) /
															(rawMode === 'per-game' ? season.games || 1 : 1),
														3,
													)}</td
												></tr
											>{/each}</tbody
									>
								</table>
							</div>
						</section>
						<section class="detail-section stack">
							<h2>Advanced research</h2>
							<div class="research-grid">
								{#each advanced as c}<div>
										<MetricHelp label={c.label} help={c.help} /><strong
											>{fmt(
												(research as unknown as Record<string, unknown> | null)?.[c.key],
											)}</strong
										>
										<p class="muted small-text">{c.help}</p>
									</div>{/each}
							</div>
						</section>
					{:else}<p class="muted">No season stats available.</p>{/if}
					<section class="detail-section stack">
						<h2>Build a career chart</h2>
						<p class="muted small-text">
							Choose a metric and sport to explore changes between seasons. Missing and unqualified
							measurements are omitted. Raw fantasy points are comparable within the same scoring
							rules.
						</p>
						<div class="row">
							<label
								>Sport <select bind:value={chartSport}
									>{#each seasonSports as sport}<option value={sport}>{nameSport(sport)}</option
										>{/each}</select
								></label
							><label
								>Metric <select aria-label="Career chart metric" bind:value={chartMetric}
									>{#each chartOptions as c}<option value={c.key}>{c.label}</option>{/each}</select
								></label
							><label
								>Chart <select aria-label="Career chart type" bind:value={chartKind}
									><option value="scatter">Season trend</option><option value="bar"
										>Season comparison</option
									></select
								></label
							>
						</div>
						<ResearchChart
							points={chartPoints}
							kind={chartKind}
							title="{p.full_name} · {chartOptions.find((c) => c.key === chartMetric)?.label ??
								chartMetric}"
							xLabel="Season ending year"
							yLabel={chartOptions.find((c) => c.key === chartMetric)?.label ?? chartMetric}
						/>
					</section>
					{#if data.me}<div class="detail-section">
							<h2>Your nickname</h2>
							{#key data.me.id + p.id}<PlayerNickname
									franchiseID={data.me.id}
									playerID={p.id}
									playerName={p.full_name}
									load
								/>{/key}
						</div>{/if}
				{/if}
			</div>
		</details>
		<details class="disclosure" bind:open={historyOpen}>
			<summary
				>Fantasy history & transactions <span class="muted small-text">{eventTotal}</span></summary
			>
			<div class="details-body">
				{#if historyOpen}
					<section class="detail-section stack">
						<span class="eyebrow">Points that counted</span>
						<h2>Fantasy scoring history</h2>
						<p class="muted small-text">
							Production from saved starting lineups in fantasy seasons, including weekly game and
							pitcher-start limits. Recalculated under current league rules, matching standings.
							Research totals include every appearance instead.
						</p>
						{#if profile.fantasy_production.length}<div class="table-wrap">
								<table>
									<thead
										><tr
											><th>Fantasy season</th><th>League</th><th>Fantasy team</th><ColumnHeader
												label="Counted games"
												help="Recorded games counted by saved starting lineups and weekly limits."
											/><ColumnHeader
												label="Counted fantasy points"
												help="Points contributed to this franchise in the fantasy season under current league scoring rules."
											/></tr
										></thead
									><tbody
										>{#each profile.fantasy_production as f}<tr
												><td>{f.year}</td><td>{nameSport(f.competition)}</td><td
													><a href="/franchise/{f.franchise_slug}">{f.franchise_name}</a></td
												><td>{f.games}</td><td>{fmt(f.points)}</td></tr
											>{/each}</tbody
									>
								</table>
							</div>{:else}<p class="muted">
								No counted fantasy production is recorded in configured fantasy seasons.
							</p>{/if}
					</section>
					<div class="two-columns">
						<section class="detail-section stack">
							<span class="eyebrow">Roster & lineup</span>
							<h2>Current ownership</h2>
							{#each profile.ownership as o}<div class="history-item">
									<a href="/franchise/{o.franchise_slug}"><strong>{o.franchise_name}</strong></a>
									<p>
										{o.league_name} · {human(o.list)} roster · {o.slot
											? `Starting: ${o.slot}`
											: 'Not currently starting'}
									</p>
									<p class="muted small-text">
										Acquired {date(o.acquired_at)} via {human(o.acquired_via)}
									</p>
									{#if profile.reserve_locked_until[o.league_id] && new Date(profile.reserve_locked_until[o.league_id]) > new Date()}<p
											class="small-text"
										>
											Reserve lock ends {date(profile.reserve_locked_until[o.league_id])}
										</p>{/if}
								</div>{:else}<p class="muted">
									Not on a fantasy roster.
								</p>{/each}{#each profile.waivers as w}<p>
									{nameSport(w.competition)} waivers clear {date(w.clears_at)}.
								</p>{/each}
							<p class="muted small-text">{profile.eligibility_note}</p>
						</section>
						<section class="detail-section stack">
							<span class="eyebrow">Draft pedigree</span>
							<h2>Draft history</h2>
							{#each profile.drafts as d}<div class="history-item">
									<a href="/draft/{d.draft_id}"><strong>{d.draft_name}</strong></a>
									<p>Round {d.round} · Pick {d.position}{d.auto_picked ? ' · Auto-picked' : ''}</p>
									<p class="muted small-text">
										{d.franchise_name} · {date(d.picked_at)} · {human(d.kind)}
									</p>
								</div>{:else}<p class="muted">No recorded fantasy draft selections.</p>{/each}
						</section>
					</div>
					<section class="detail-section stack">
						<div class="spread">
							<div>
								<span class="eyebrow">Player movement</span>
								<h2>Transaction timeline</h2>
							</div>
							<label
								>Event <select bind:value={historyFilter}
									><option value="">All events</option>{#each eventKinds as kind}<option
											value={kind}>{human(kind)}</option
										>{/each}</select
								></label
							>
						</div>
						<p class="muted small-text">
							Recorded acquisitions, drops, roster moves, draft selections, and trades. Event
							filtering applies to the history loaded below.
						</p>
						<ol class="timeline">
							{#each visibleEvents as e (e.id)}<li>
									<div class="spread">
										<strong>{human(e.kind)}</strong><time>{date(e.created_at)}</time>
									</div>
									<p>
										{e.franchise_name || 'League'}{e.competition
											? ` · ${nameSport(e.competition)}`
											: ''}
									</p>
									{#if eventDetail(e)}<p class="muted small-text">
											{eventDetail(e)}
										</p>{/if}{#if e.trade_id}<a class="small-text" href="/trades">View trades →</a
										>{/if}{#if e.draft_id}<a class="small-text" href="/draft/{e.draft_id}"
											>View draft →</a
										>{/if}
								</li>{:else}<li class="muted">No recorded transactions match this view.</li>{/each}
						</ol>
						{#if eventsError}<p role="alert">{eventsError}</p>{/if}
						<div class="spread">
							<span class="muted small-text"
								>{events.length} of {eventTotal} transactions loaded</span
							>{#if events.length < eventTotal}<button disabled={eventsBusy} onclick={loadEvents}
									>{eventsBusy ? 'Loading…' : 'Load more history'}</button
								>{/if}
						</div>
					</section>
					<section class="detail-section stack">
						<h2>Completed trade history</h2>
						{#each profile.trades as t}<div class="history-item">
								<div class="spread">
									<strong>{t.from_name} → {t.to_name}</strong><span class="pill"
										>{human(t.status)}</span
									>
								</div>
								<p class="muted small-text">
									{nameSport(t.competition)} · Proposed {date(t.created_at)}{t.resolved_at
										? ` · Resolved ${date(t.resolved_at)}`
										: ''}
								</p>
								{#if t.note}<p>{t.note}</p>{/if}<a href="/trades" class="small-text"
									>View trades →</a
								>
							</div>{:else}<p class="muted">No recorded completed trades.</p>{/each}
					</section>
				{/if}
			</div>
		</details>
	{/if}
</div>
{#if makingRoom && myRoster && league && profile}
	<DropToAdd
		roster={myRoster}
		{league}
		list={makingRoom}
		adding={profile.player.full_name}
		onconfirm={(drop) => {
			const list = makingRoom!;
			makingRoom = undefined;
			add(list, drop);
		}}
		oncancel={() => (makingRoom = undefined)}
	/>
{/if}

<style>
	.profile {
		min-width: 0;
		max-width: 100%;
		display: flex;
		flex-direction: column;
		gap: 0;
	}
	.profile-nav {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.2rem;
		font-size: 0.85rem;
	}
	.find-player {
		position: relative;
		max-width: 65%;
	}
	.find-player summary {
		cursor: pointer;
		color: var(--ink-soft);
		text-align: right;
	}
	.find-player[open] {
		width: 30rem;
	}
	.find-player :global(.player-search) {
		margin-top: 0.6rem;
	}
	.bio {
		padding-bottom: 1rem;
	}
	.identity {
		display: flex;
		align-items: center;
		gap: 0.85rem;
	}
	.identity-copy {
		min-width: 0;
	}
	.name-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	h1 {
		min-width: 0;
		overflow-wrap: anywhere;
		font-size: clamp(1.35rem, 3vw, 1.9rem);
		line-height: 1.2;
		margin: 0;
	}
	h2 {
		font-size: 1.05rem;
		margin: 0;
	}
	.bio-line {
		display: flex;
		align-items: center;
		gap: 0.4rem 0.75rem;
		flex-wrap: wrap;
		margin-top: 0.35rem;
		font-size: 0.85rem;
	}
	.bio-note {
		margin: 0.65rem 0 0;
	}
	.roster-status {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.7rem 1.5rem;
		padding: 0.85rem 0;
		border-top: 1px solid var(--rule);
		border-bottom: 1px solid var(--rule);
		font-size: 0.85rem;
	}
	.ownership {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.available {
		color: var(--good);
	}
	.roster-actions {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.7rem;
		flex-wrap: wrap;
	}
	.action-link {
		font-weight: 650;
	}
	.eligibility {
		font-size: 0.75rem;
		color: var(--ink-soft);
	}
	.eligibility summary {
		cursor: pointer;
		list-style: none;
	}
	.eligibility p {
		max-width: 36rem;
		margin: 0.4rem 0 0;
	}
	.primary-section {
		padding: 1.15rem 0;
		border-bottom: 1px solid var(--rule);
		min-width: 0;
	}
	.section-heading,
	.section-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.6rem 1rem;
		flex-wrap: wrap;
	}
	.section-heading {
		margin-bottom: 0.65rem;
	}
	.section-foot {
		margin-top: 0.6rem;
	}
	.section-controls {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		flex-wrap: wrap;
	}
	.check {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.8rem;
		white-space: nowrap;
	}
	.check input {
		margin: 0;
		min-height: 0;
		width: auto;
	}
	.table-wrap {
		max-width: 100%;
		overflow: auto;
		overscroll-behavior-x: contain;
	}
	table {
		width: 100%;
		white-space: nowrap;
		font-size: 0.82rem;
		border-collapse: collapse;
	}
	table :global(th),
	td {
		text-align: right;
		padding: 0.48rem 0.65rem;
		font-variant-numeric: tabular-nums;
	}
	thead :global(th) {
		font-size: 0.73rem;
	}
	table :global(th:first-child),
	td:first-child,
	table :global(th:nth-child(2)),
	td:nth-child(2) {
		text-align: left;
	}
	tbody th {
		font-weight: 500;
	}
	thead th:first-child,
	tbody th:first-child {
		position: sticky;
		left: 0;
		z-index: 1;
		background: var(--surface, #fff);
	}
	.points {
		font-weight: 700;
		background: var(--brand-soft);
	}
	.team-league {
		display: block;
		font-size: 0.72rem;
	}
	.text-button {
		background: none;
		border: 0;
		padding: 0;
		color: var(--brand);
		cursor: pointer;
		min-height: 0;
	}
	.game-matchup {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
	}
	.empty {
		padding: 1rem 0;
	}
	.disclosure {
		border-bottom: 1px solid var(--rule);
		min-width: 0;
	}
	.disclosure > summary > .small-text {
		margin-left: 0.4rem;
	}
	.disclosure > summary {
		padding: 1rem 0;
		cursor: pointer;
		font-size: 0.9rem;
		font-weight: 600;
	}
	.details-body {
		padding: 0 0 1rem;
		min-width: 0;
	}
	.detail-section {
		padding: 0.8rem 0;
		min-width: 0;
	}
	.detail-section h2 {
		margin-bottom: 0.5rem;
	}
	.season-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}
	.season-label select {
		max-width: 100%;
	}
	.research-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1rem;
	}
	.research-grid strong {
		display: block;
		font-size: 1.2rem;
		margin: 0.3rem 0;
	}
	.two-columns {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1.5rem;
	}
	.history-item {
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--rule);
	}
	.history-item p {
		margin: 0.25rem 0;
	}
	.timeline {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.timeline li {
		border-bottom: 1px solid var(--rule);
		padding: 0.65rem 0;
	}
	.timeline p {
		margin: 0.25rem 0;
	}
	.timeline time {
		font-size: 0.75rem;
		color: var(--ink-soft);
	}
	.spread {
		flex-wrap: wrap;
		gap: 0.6rem;
	}
	select {
		font-size: 0.8rem;
		min-width: 0;
		max-width: 100%;
	}
	@media (max-width: 700px) {
		.roster-status {
			align-items: flex-start;
			flex-direction: column;
		}
		.roster-actions {
			justify-content: flex-start;
			width: 100%;
		}
		.research-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.two-columns {
			grid-template-columns: 1fr;
			gap: 0;
		}
		.section-heading {
			align-items: flex-start;
		}
		.season-label {
			flex-direction: column;
			align-items: flex-start;
		}
		.primary-section {
			padding: 1rem 0;
		}
		.section-controls {
			gap: 0.45rem;
		}
		.identity {
			gap: 0.6rem;
			align-items: flex-start;
		}
		.identity :global(img) {
			max-width: 52px;
			max-height: 52px;
		}
		table :global(th),
		td {
			padding: 0.5rem 0.5rem;
		}
		.bio-line {
			font-size: 0.78rem;
			gap: 0.3rem 0.5rem;
		}
		.profile-nav {
			margin-bottom: 1rem;
			gap: 0.5rem;
			font-size: 0.78rem;
		}
		.research-grid {
			gap: 0.8rem;
		}
		.section-foot .row {
			gap: 0.4rem;
		}
		.section-controls button {
			font-size: 0.75rem;
		}
		.disclosure > summary {
			min-height: 44px;
		}
	}
	@media (max-width: 380px) {
		.section-controls {
			width: 100%;
			justify-content: space-between;
		}
		.research-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
