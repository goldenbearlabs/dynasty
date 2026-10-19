<script lang="ts">
	// Set who starts for one franchise in one league. The league's starting
	// slots are fixed rows with the bench below them: drag a player onto a
	// slot, or tap him and then tap where he goes. A daily league moves a day
	// at a time and shows that day's game; a weekly one shows the week's
	// games, and where a slot counts only some of them the manager taps the
	// one that counts.
	import { autoSetLineup, getLineup, setLineup, getRosterPerformance, type Competition, type LeagueRoster, type RosterPerformance, type Franchise, type League, type Lineup, type LineupGame, type LineupPlayer, type Slot } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import { columnValue, hockeyScoringColumns, statColumns, statValue, sumStats, type StatColumn } from '#lib/statLine.ts';
	import { rosterGroup, rosterGroupOrder } from '#lib/rosterGroups.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import { dayLabel, points, shiftDay, timeOfDay, sportsToday } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import WatchStar from '#lib/WatchStar.svelte';
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import PlayerActions from '#lib/PlayerActions.svelte';
	import { dropPlayer } from '#lib/dropPlayer.ts';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';

	type Props = {
		league: League;
		franchise: Franchise;
		/** Whether the viewer may change this lineup, and whether they are a commissioner. */
		canEdit: boolean;
		commissioner?: boolean;
		roster?: LeagueRoster;
		competition?: Competition;
		/** The day to open on; today when left out. Kept up to date as the viewer moves. */
		day?: string;
	};
	let { league, franchise, canEdit, commissioner = false, roster, competition, day = $bindable() }: Props = $props();

	// View changes the statistics in these same starter and bench rows.
	let statsView = $state('today');
	let performance = $state<RosterPerformance>();
	let statsLoading = $state(false);
	let statsError = $state('');
	let statsRequest = 0;
	const statsEnabled = $derived(!!roster && !!competition);
	async function loadStats() {
		if (!roster || !competition) return;
		const token = ++statsRequest;
		statsLoading = true;
		try {
			const historical = statsView.startsWith('season:');
			const found = await getRosterPerformance(league.id,franchise.id,historical ? 'season' : statsView,historical ? statsView.slice(7) : undefined,statsView === 'today' ? day ?? sportsToday() : undefined);
			if (token === statsRequest) { performance = found; statsError = ''; }
		} catch(e) { if(token === statsRequest) statsError = String(e); }
		finally { if(token === statsRequest) statsLoading = false; }
	}
	$effect(() => { void [statsView,day,league.id,franchise.id,roster?.players,competition]; loadStats(); return () => { statsRequest++; }; });
	const performanceFor = (id: string) => statsLoading || statsError ? undefined : performance?.players.find(p => p.player_id === id);
	const statAbbreviations: Record<string,string> = {
		pts:'PTS',reb:'REB',ast:'AST',stl:'STL',blk:'BLK',tpm:'3PM',tpa:'3PA',tov:'TO',fgm:'FGM',fga:'FGA',ftm:'FTM',fta:'FTA',
		goals:'G',assists:'A',shots:'SOG',hits:'HIT',blocks:'BLK',pp_points:'PPP',pp_goals:'PPG',plus_minus:'+/-',pim:'PIM',saves:'SV',goals_against:'GA',goalie_wins:'W',shutouts:'SO',
		pass_yds:'PYD',pass_td:'PTD',interceptions:'INT',rush_yds:'RYD',rush_td:'RTD',receptions:'REC',rec_yds:'RECYD',rec_td:'RECTD',fumbles_lost:'FUM',
		bat_ab:'AB',bat_h:'B H',bat_r:'R',bat_hr:'HR',bat_rbi:'RBI',bat_sb:'SB',bat_bb:'B BB',bat_so:'B K',pit_ip:'IP',pit_h:'P H',pit_er:'ER',pit_bb:'P BB',pit_so:'P K',pit_w:'W',pit_sv:'SV',pit_gs:'GS'
	};
	const performanceColumns = $derived.by(() => {
		const definitions = (competition?.stats ?? []).map(s => ({...s}));
		const known = new Set(definitions.map(s => s.key));
		for (const player of performance?.players ?? []) for(const key of Object.keys(player.stats)) {
			if(!known.has(key)) { definitions.push({key,label:key.replaceAll('_',' ')}); known.add(key); }
		}
		return [
			{key:'__gp',keys:[],label:'GP',title:'Games played in the selected view'},
			...definitions.map(s => ({key:s.key,keys:[s.key],label:statAbbreviations[s.key] ?? s.key.toUpperCase(),title:s.label})),
			{key:'__fpg',keys:[],label:'FP/G',title:'Fantasy points per game in the selected view'}
		];
	});
	function viewedStat(player: LineupPlayer, col: StatColumn): number | undefined {
		const data = performanceFor(player.player_id);
		if(!data) return undefined;
		if(col.key === '__gp') return data.games;
		if(!data.games) return undefined;
		return col.key === '__fpg' ? data.points_per_game : data.stats[col.key];
	}

	const sport = $derived(league.competition);
	let override = $state(false); // the commissioner's override of the locks

	let lineup = $state<Lineup>();
	// One seat for each place in the lineup, in the league's slot order, and who is in it.
	const seatSlots = $derived(lineup?.slots.flatMap((s) => Array.from({ length: s.count }, () => s)) ?? []);
	let seats = $state<(string | null)[]>([]);
	// The game picked for each player in a slot that counts only some games a
	// week: AUTO counts his last game of the week with a stat, a day counts
	// the game played then, and '' (from before auto) his next game.
	const AUTO = 'auto';
	let picked = $state<Record<string, string>>({});
	const savedPick = (p: LineupPlayer) => (p.counts_last ? AUTO : p.counts_from);

	function show(loaded: Lineup) {
		lineup = loaded;
		const slots = loaded.slots.flatMap((s) => Array.from({ length: s.count }, () => s));
		const next: (string | null)[] = slots.map(() => null);
		for (const p of loaded.players) {
			const i = slots.findIndex((s, i) => s.name === p.slot && next[i] === null);
			if (i >= 0) next[i] = p.player_id;
		}
		seats = next;
		picked = Object.fromEntries(loaded.players.map((p) => [p.player_id, savedPick(p)]));
		moving = undefined;
	}
	$effect(() => {
		getLineup(league.id, franchise.id, day).then(show, toast.error);
	});
	// Points and locks follow the games, unless a change is still being saved.
	onScoresChange(
		() => [sport],
		() => {
			if (statsEnabled) loadStats();
			if (lineup && !dirty && !saving) getLineup(league.id, franchise.id, lineup.day).then(show, toast.error);
		}
	);

	const byId = $derived(new Map(lineup?.players.map((p) => [p.player_id, p]) ?? []));
	// The slot each player is in, as the seats now have it; '' is the bench.
	const slotOf = (id: string) => {
		const i = seats.indexOf(id);
		return i < 0 ? '' : seatSlots[i].name;
	};
	// The bench, by kind of player in the order the slots put them (skaters
	// before goalies, hitters before pitchers), then by name.
	const kindOf = (positions: string[]) => statColumns(sport, positions).map((c) => c.key).join();
	const kinds = $derived([...new Set(lineup?.slots.map((s) => kindOf(s.positions)) ?? [])]);
	const bench = $derived(
		(lineup?.players ?? [])
			.filter((p) => !seats.includes(p.player_id))
			.toSorted((a, b) => kinds.indexOf(kindOf(a.positions)) - kinds.indexOf(kindOf(b.positions)) || a.full_name.localeCompare(b.full_name))
	);
	const dirty = $derived(
		lineup?.players.some((p) => slotOf(p.player_id) !== p.slot || picked[p.player_id] !== savedPick(p)) ?? false
	);
	const weekly = $derived(lineup !== undefined && lineup.day !== lineup.last_day);
	const limited = $derived(lineup?.slots.filter((s) => s.games_per_week > 0) ?? []);

	// ---- moving players ----
	const fits = (player: LineupPlayer, slot: Slot) =>
		player.starter_eligible && (slot.positions.includes('*') || player.positions.some((p) => slot.positions.includes(p)));
	const frozen = (player: LineupPlayer | undefined) =>
		!!player && (!canEdit || ((lineup!.locked !== '' || player.locked) && !override));
	// The player being moved, by drag or by tap.
	let moving = $state<string>();
	let over = $state<number | 'bench'>();
	function canPlace(id: string, target: number | 'bench'): boolean {
		const player = byId.get(id);
		if (!player || frozen(player)) return false;
		if (target === 'bench') return seats.includes(id);
		const occupant = seats[target];
		if (occupant === id || !fits(player, seatSlots[target])) return false;
		return !occupant || !frozen(byId.get(occupant));
	}
	// Puts a player in a seat or on the bench. Whoever held the seat takes his
	// old one if he fits it, or goes to the bench.
	function place(id: string, target: number | 'bench') {
		if (!canPlace(id, target)) return;
		const from = seats.indexOf(id);
		if (target === 'bench') {
			seats[from] = null;
		} else {
			const occupant = seats[target];
			seats[target] = id;
			if (from >= 0) seats[from] = occupant && fits(byId.get(occupant)!, seatSlots[from]) ? occupant : null;
			// Anyone landing in a slot that counts some games starts on auto.
			for (const [player, seat] of [[id, target], [occupant, from]] as const) {
				if (player && seat >= 0 && seats[seat] === player && seatSlots[seat].games_per_week > 0 && byId.get(player)?.slot !== seatSlots[seat].name) {
					picked[player] = AUTO;
				}
			}
		}
		moving = undefined;
		queueSave();
	}
	// Counts one game for a player, or goes back to auto.
	function pick(id: string, choice: string) {
		picked[id] = choice;
		queueSave();
	}
	const toggleMove = (id: string) => (moving = moving === id ? undefined : id);
	function dragStart(e: DragEvent, id: string) {
		moving = id;
		e.dataTransfer?.setData('text/plain', id);
		if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
	}
	function dragOver(e: DragEvent, target: number | 'bench') {
		if (!moving || !canPlace(moving, target)) return;
		e.preventDefault();
		over = target;
	}
	function drop(e: DragEvent, target: number | 'bench') {
		e.preventDefault();
		if (moving) place(moving, target);
		over = undefined;
	}
	const dragEnd = () => ((moving = undefined), (over = undefined));

	// ---- games ----
	const begun = (game: LineupGame) => new Date(game.starts_at).getTime() <= Date.now();
	// How many games a week count in the slot a player is in; 0 is all of them.
	// A doubled matchup counts twice as many over the whole of it.
	const limit = (player: LineupPlayer) =>
		(lineup!.slots.find((s) => s.name === slotOf(player.player_id))?.games_per_week ?? 0) * (lineup!.doubled ? 2 : 1);
	// Whether a game is one that counts, going by the lineup as it is now arranged.
	function counts(player: LineupPlayer, game: LineupGame): boolean {
		const n = limit(player);
		if (n === 0) return true;
		const choice = picked[player.player_id];
		const unsaved = slotOf(player.player_id) !== player.slot || choice !== savedPick(player);
		if (!unsaved) return game.counts;
		if (choice === AUTO) return false; // nothing played yet counts for a player just put on auto
		const from = choice || player.games.find((g) => !begun(g))?.day || '';
		return player.games.filter((g) => g.day >= from).slice(0, n).includes(game);
	}
	// On auto, the game that will count if he plays in it: his last still to come.
	function projected(player: LineupPlayer, game: LineupGame): boolean {
		if (limit(player) === 0 || picked[player.player_id] !== AUTO || begun(game)) return false;
		const ahead = player.games.filter((g) => !begun(g) && g.day >= (slotOf(player.player_id) === player.slot ? player.counts_from : ''));
		return ahead.at(-1) === game;
	}
	// Points from the games that count, as the lineup is now arranged.
	const total = (player: LineupPlayer) => player.games.reduce((n, g) => n + (counts(player, g) ? g.points : 0), 0);
	const pickable = (player: LineupPlayer, game: LineupGame) => limit(player) > 0 && !frozen(player) && !begun(game);
	const played = (game: LineupGame) => game.status !== 'scheduled';
	// The games a row's stats come from: those played, and for a starter in a
	// slot that counts only some, only the ones that count. A bench player in
	// such a league has none: no game of his counts.
	const shownGames = (player: LineupPlayer, starting: boolean) =>
		player.games.filter((g) => played(g) && (limited.length === 0 || (starting && counts(player, g))));
	// His projected points in the games still to come that would count: for
	// a starter in a slot that counts only some, the picked one or on auto
	// his last; for anyone else, all of them.
	const forecast = (player: LineupPlayer, starting: boolean) =>
		player.games
			.filter((g) => !played(g) && (limited.length === 0 || !starting || counts(player, g) || projected(player, g)))
			.reduce((n, g) => n + g.projected, 0);

	// ---- stat columns ----
	function groupColumns(group: string): StatColumn[] {
		if (sport === 'nhl') {
			const scored = hockeyScoringColumns(group === 'Goalies', league.settings.scoring);
			return statsEnabled ? [performanceColumns[0], ...scored, performanceColumns.at(-1)!] : scored;
		}
		if (!statsEnabled) return statColumns(sport, group === 'Goalies' ? ['G'] : group === 'Pitchers' ? ['SP'] : sport === 'nhl' ? ['C'] : sport === 'mlb' ? ['OF'] : ['PG']);
		return performanceColumns.filter(c => {
			if (c.key.startsWith('__')) return true;
			if (sport === 'mlb') return c.key.startsWith(group === 'Pitchers' ? 'pit_' : 'bat_');
			return true;
		});
	}
	const columns = (player: LineupPlayer) => groupColumns(rosterGroup(sport, player.positions, slotOf(player.player_id)));
	const tableGroups = $derived(rosterGroupOrder(sport).map(label => ({
		label, columns: groupColumns(label),
		seats: seatSlots.map((slot,index) => ({slot,index})).filter(({slot}) => rosterGroup(sport,slot.positions,slot.name) === label),
		bench: bench.filter(p => rosterGroup(sport,p.positions) === label)
	})).filter(group => group.seats.length || group.bench.length));
	// Who a feed has starting in goal or on the mound, shown on games still to come.
	const starts = {
		confirmed: { tone: 'good', label: 'Starting', title: 'Confirmed to start' },
		expected: { tone: 'gold', label: 'Probable', title: 'Expected to start' },
		other: { tone: 'bad', label: 'Not starting', title: 'A teammate is named to start' }
	};
	const startOf = (game: LineupGame) => (game.starter && game.status === 'scheduled' ? starts[game.starter] : undefined);
	const versus = (game: LineupGame) => `${game.at_home ? 'vs' : '@'} ${game.opponent}`;
	const when = (game: LineupGame) =>
		game.status === 'live' ? 'Live' : game.status === 'final' ? 'Final' : `${weekly ? `${weekday(game.day)} ` : ''}${timeOfDay(game.starts_at)}`;

	// ---- days ----
	const today = () => new Date().toLocaleDateString('en-CA'); // close enough to pick a column to highlight
	// The week around a daily lineup's day, for the day strip.
	const week = $derived.by(() => {
		if (!lineup || weekly) return [];
		const names = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
		const back = (new Date(lineup.day + 'T12:00').getDay() - names.indexOf(league.settings.lineup.week_start) + 7) % 7;
		return Array.from({ length: 7 }, (_, i) => shiftDay(lineup!.day, i - back));
	});
	const weekday = (d: string) => new Date(d + 'T12:00').toLocaleDateString(undefined, { weekday: 'short' });
	const dayNumber = (d: string) => new Date(d + 'T12:00').getDate();

	// ---- dropping ----
	// A commissioner dropping someone else's player does so past the rules.
	const owner = $derived((page.data.me as Franchise | null | undefined)?.id === franchise.id);
	async function release(player: LineupPlayer) {
		if (await dropPlayer(league, franchise.id, player.player_id, player.full_name, !owner)) {
			show(await getLineup(league.id, franchise.id, lineup!.day));
			await invalidateAll();
		}
	}

	// ---- saving ----
	// Every change saves itself a moment later. A change made while one is
	// saving is saved straight after it; a refusal puts the lineup back.
	let saving = $state(false);
	let saved = $state(false); // shows "Saved" for a moment
	let again = false;
	let timer: ReturnType<typeof setTimeout> | undefined;
	function queueSave() {
		clearTimeout(timer);
		timer = setTimeout(save, 300);
	}
	async function save() {
		if (!lineup || !dirty) return;
		if (saving) {
			again = true;
			return;
		}
		saving = true;
		const shown = lineup.day;
		try {
			await setLineup(league.id, {
				day: shown,
				entries: seats.flatMap((id, i) =>
					id ? [{ slot: seatSlots[i].name, player_id: id, counts_from: picked[id] === AUTO ? '' : (picked[id] ?? ''), auto: picked[id] === AUTO }] : []
				),
				franchise_id: franchise.id,
				force: override
			});
			if (!again) {
				show(await getLineup(league.id, franchise.id, shown));
				saved = true;
				setTimeout(() => (saved = false), 1800);
			}
		} catch (e) {
			toast.error(e);
			again = false;
			show(await getLineup(league.id, franchise.id, shown).catch(() => lineup!));
		} finally {
			saving = false;
			if (again) {
				again = false;
				save();
			}
		}
	}

	// Sets the best lineup for every day left this week, or for the week.
	let autoSetting = $state(false);
	async function autoSet() {
		if (!lineup) return;
		autoSetting = true;
		try {
			await autoSetLineup(league.id, { day: lineup.day, franchise_id: franchise.id });
			show(await getLineup(league.id, franchise.id, lineup.day));
			toast.good(weekly ? 'Lineup set for the week.' : 'Lineup set for every day left this week.');
		} catch (e) {
			toast.error(e);
		} finally {
			autoSetting = false;
		}
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && (moving = undefined)} />

{#snippet tile(game: LineupGame)}
	<span class="logo"><small>{game.at_home ? 'vs' : '@'}</small><TeamLogo competition={sport} abbrev={game.opponent} size={24} label={false} /></span>
	<span class="day">{weekday(game.day)} {dayNumber(game.day)}</span>
	<span class="time" class:live={game.status === 'live'}>{played(game) ? when(game) : timeOfDay(game.starts_at)}</span>
	{#if !played(game) && game.projected}<span class="proj" title="Projected fantasy points, from recent seasons and the betting line">{points(game.projected)} proj</span>{/if}
	{@render starter(game)}
{/snippet}

{#snippet starter(game: LineupGame)}
	{@const start = startOf(game)}
	{#if start}<span class="pill start {start.tone}" title={start.title}>{start.label}</span>{/if}
{/snippet}

{#snippet games(player: LineupPlayer, starting: boolean)}
	{#if player.games.length === 0}
		<span class="muted small-text">{weekly ? 'No games this week' : 'No game'}</span>
	{:else if weekly && limited.length > 0}
		<!-- The week's games as the opponents' logos, each with its day and time
		     beneath: the one that counts is filled in; tap another to count it instead. -->
		<div class="tiles">
			{#if starting && canEdit && !frozen(player)}
				<button
					class="tile auto"
					class:on={picked[player.player_id] === AUTO}
					aria-pressed={picked[player.player_id] === AUTO}
					title="Count his last game of the week that he plays in"
					onclick={() => pick(player.player_id, AUTO)}
				>
					<span class="logo"><strong>AUTO</strong></span><span class="day">Last</span><span class="time">he plays</span>
				</button>
			{/if}
			{#each player.games as game (game.starts_at)}
				{@const on = starting && counts(player, game)}
				{@const next = starting && !on && projected(player, game)}
				{#if starting && pickable(player, game)}
					<button
						class="tile"
						class:on
						class:next
						aria-pressed={on}
						title={next ? `Counts if he plays: ${weekday(game.day)} ${versus(game)}` : `Count ${weekday(game.day)} ${versus(game)} for ${player.full_name}`}
						onclick={() => pick(player.player_id, game.day)}
					>
						{@render tile(game)}
					</button>
				{:else}
					<span class="tile" class:on class:next class:done={played(game)} title="{weekday(game.day)} {versus(game)} · {when(game)}">
						{@render tile(game)}
					</span>
				{/if}
			{/each}
		</div>
	{:else}
		{#each player.games as game (game.starts_at)}
			<div class="game">
				<span class="opponent">{game.at_home ? 'vs' : '@'} <TeamLogo competition={sport} abbrev={game.opponent} size={28} /></span>
				<span class="status" class:live={game.status === 'live'}>{when(game)}</span>
				{@render starter(game)}
			</div>
		{/each}
	{/if}
{/snippet}

<!-- A row of column headings; the first in a section also names it. -->
{#snippet labels(cols: StatColumn[], title: string)}
	<li class="labels" aria-hidden="true">
		<span class="title">{title}</span>
		<span>{title ? (weekly ? 'This week' : 'Game') : ''}</span>
		<div class="stats">
			{#each cols as c (c.key)}<span title={c.title}>{c.label}</span>{/each}
		</div>
		<span class="pts" title="Fantasy points. In italics: projected for games to come, from recent seasons and the betting line.">FPTS</span>
	</li>
{/snippet}

{#snippet who(player: LineupPlayer, starting: boolean)}
	<!-- svelte-ignore a11y_no_static_element_interactions: dragging is a shortcut; the grip button is the tap and keyboard path -->
	<div
		class="who"
		class:picked={moving === player.player_id}
		class:movable={canEdit && !frozen(player)}
		draggable={canEdit && !frozen(player)}
		ondragstart={(e) => dragStart(e, player.player_id)}
		ondragend={dragEnd}
	>
		{#if canEdit}
			<button
				class="grip"
				disabled={frozen(player)}
				aria-pressed={moving === player.player_id}
				aria-label="Move {player.full_name}"
				title={frozen(player) ? `${player.full_name} is locked` : 'Drag, or tap and then tap where he goes'}
				onclick={() => toggleMove(player.player_id)}
			>
				<Icon name={frozen(player) && player.locked ? 'lock' : 'grip'} size={14} />
			</button>
		{/if}
		<Headshot name={player.full_name} src={player.headshot_url} size={24} />
		<div class="name">
			<span class="row tight">
				<a href="/player/{player.player_id}"><strong>{player.full_name}</strong></a>
				<WatchStar playerID={player.player_id} playerName={player.full_name} /><ProspectRank id={player.player_id} />
				<PlayerActions leagueId={league.id} playerId={player.player_id} ownerId={franchise.id} name={player.full_name} detail={`${player.positions.join('/')} · ${player.team_abbrev ?? ''}`} onDrop={canEdit ? () => release(player) : undefined} />
				{#if player.nickname}<span class="pill brand">“{player.nickname}”</span>{/if}
				<InjuryBadge designation={player.injury} />
				{#if player.locked && !canEdit}<Icon name="lock" size={12} />{/if}
			</span>
			<div class="player-meta"><span class="muted small-text">{player.positions.join('/')} · <TeamLogo competition={sport} abbrev={player.team_abbrev} size={18} />{#if player.eligibility_note} · {player.eligibility_note}{/if}</span>
			{#if statsEnabled}
				{@const stats = performanceFor(player.player_id)}
				<div class="period-ranks" aria-label="Ranks for {player.full_name}"><span title="Overall rank by total fantasy points">Rank {stats?.overall_rank ? `#${stats.overall_rank}` : '—'}</span>{#each Object.entries(stats?.position_ranks ?? {}) as [position,rank] (position)}<span>{position} #{rank}</span>{:else}<span>Position —</span>{/each}</div>
			{/if}
			</div>
		</div>
	</div>
	<div class="games">{@render games(player, starting)}</div>
	{@const shown = shownGames(player, starting)}
	{@const line = sumStats(shown.map((g) => g.stats))}
	<div class="stats" class:none={!statsEnabled && shown.length === 0}>
		{#each columns(player) as c (c.key)}
			{@const n = statsEnabled ? viewedStat(player,c) : columnValue(line, c)}
			<span class:zero={!n} title={c.title}>{#if statsEnabled || shown.length}<small>{c.label}</small>{statsEnabled && n === undefined ? '—' : c.key === '__fpg' && n !== undefined ? points(n) : statValue(n)}{/if}</span>
		{/each}
	</div>
	{@const ahead = !statsEnabled || statsView === 'today' ? forecast(player, starting) : 0}
	{@const viewed = performanceFor(player.player_id)}
	<div class="pts" class:none={statsEnabled ? !viewed?.games : shown.length === 0 && !ahead}>
		<small>FPTS</small>{#if statsEnabled}{viewed?.games ? points(viewed.points) : '—'}{:else if shown.length}{points(shown.reduce((n, g) => n + g.points, 0))}{:else if !ahead}–{/if}
		{#if ahead}<span class="proj" title="Projected for games to come in this lineup">{!statsEnabled && shown.length ? '+' : ''}{points(ahead)}</span>{/if}
	</div>
{/snippet}

<div class="stack tight" class:stats-enabled={statsEnabled} data-sport={sport}>
	{#if !lineup}
		<p class="muted">Loading the lineup…</p>
	{:else}
		{#if statsEnabled}
			<div class="spread stat-view-controls">
				<label class="row small-text">View
					<select aria-label="View" bind:value={statsView}>
						<option value="today">{day && day !== sportsToday() ? dayLabel(day) : 'Today'}</option><option value="7">Last 7 days</option><option value="14">Last 14 days</option><option value="30">Last 30 days</option>
						<option value="current">Current season{performance?.current_season ? ` · ${performance.current_season}` : ''}</option>
						{#if performance?.seasons.length}<optgroup label="Past seasons">{#each performance.seasons.filter(s => s.label !== performance?.current_season) as season (season.label)}<option value={`season:${season.label}`}>{season.label}</option>{/each}</optgroup>{/if}
					</select>
				</label>
				{#if statsLoading}<span class="muted small-text" role="status">Loading stats…</span>{:else if performance}<span class="muted stat-context">{statsView === 'today' ? dayLabel(performance.to_day) : performance.season || `${dayLabel(performance.from_day)} – ${dayLabel(performance.to_day)}`} · ranks by total FP</span>{/if}
			</div>
			{#if statsError}<p role="alert">{statsError}</p><button class="small" onclick={loadStats}>Retry stats</button>{/if}
		{/if}
		<div class="spread">
			<nav class="row days" aria-label={weekly ? 'Week' : 'Day'}>
				<button class="quiet step" aria-label={weekly ? 'Previous week' : 'Previous day'} onclick={() => (day = shiftDay(lineup!.day, -1))}><Icon name="left" size={16} /></button>
				{#if weekly}
					<strong class="range">{dayLabel(lineup.day)} – {dayLabel(lineup.last_day)}</strong>
				{:else}
					{#each week as d (d)}
						<button class="day" class:on={d === lineup.day} class:today={d === today()} aria-pressed={d === lineup.day} onclick={() => (day = d)}>
							<span>{weekday(d)}</span><strong>{dayNumber(d)}</strong>
						</button>
					{/each}
				{/if}
				<button class="quiet step" aria-label={weekly ? 'Next week' : 'Next day'} onclick={() => (day = shiftDay(lineup!.last_day, 1))}><Icon name="right" size={16} /></button>
				<button class="quiet small" onclick={() => (day = undefined)}>{weekly ? 'This week' : 'Today'}</button>
			</nav>
			<div class="row actions">
				{#if saving}<span class="muted small-text">Saving…</span>{:else if saved}<span class="saved small-text"><Icon name="check" size={14} /> Saved</span>{/if}
				{#if canEdit && !lineup.locked && lineup.players.length > 0}
					<button class="small" disabled={autoSetting || saving} title="Start the best players you have, keeping anyone who has locked" onclick={autoSet}>
						<Icon name="refresh" size={14} /> {autoSetting ? 'Setting…' : weekly ? 'Auto-set this week' : 'Auto-set rest of week'}
					</button>
				{/if}
				{#if commissioner}
					<label class="check small-text">
						<input type="checkbox" bind:checked={override} />
						<span class="pill gold">Commissioner</span> Override locks
					</label>
				{/if}
			</div>
		</div>

		{#if lineup.locked}<p class="card note small-text"><Icon name="lock" size={14} /> {lineup.locked}</p>{/if}
		{#if lineup.doubled}
			<p class="card note doubled small-text">
				<strong>Double games</strong> · {dayLabel(lineup.doubled.starts_on)} – {dayLabel(lineup.doubled.ends_on)}:
				{[limited.length > 0 && `${limited.length === lineup.slots.length ? 'each starter' : `each ${limited.map((s) => s.name).join(' and ')}`} counts ${limited[0].games_per_week * 2} games`, lineup.starts && `${lineup.starts.limit} pitcher starts`].filter(Boolean).join(' and ')}
				over the whole matchup.
			</p>
		{/if}

		{#if lineup.players.length === 0}
			<Empty icon="players" title="Nobody on the main roster">
				Only main-roster players can start. <a href="/players?competition={sport}">Find players</a>.
			</Empty>
		{:else}
			{#if limited.length > 0 && canEdit}
				<p class="muted small-text">
					{limited.length === lineup.slots.length ? 'Each starter' : `Each ${limited.map((s) => s.name).join(' and ')}`}
					scores in {lineup.doubled ? `${limited[0].games_per_week * 2} games this matchup` : limited[0].games_per_week === 1 ? 'one game a week' : `${limited[0].games_per_week} games a week`}. On <strong>Auto</strong> it is his last {lineup.doubled ? 'games of the matchup' : 'game of the week'}
					that he plays in, so a game he sits out falls back to the one before. Tap a game to count that one instead. Changes save as you make them.
				</p>
			{:else if canEdit && !lineup.locked}
				<p class="muted small-text">
					{weekly ? 'Each player locks when his game starts, and stays put for the rest of the week.' : 'Each player locks when his game starts.'}
					Drag players between slots and the bench, or tap one and then tap where he goes. Changes save as you make them.
				</p>
			{/if}

			{#if lineup.starts}
				{@const starts = lineup.starts}
				<div class="card starts">
					<Meter label={lineup.doubled ? 'Pitcher starts this matchup' : 'Pitcher starts this week'} value={starts.made.length} max={starts.limit} />
					<p class="muted small-text">
						{#if starts.made.length === 0}
							None yet. The first {starts.limit} of the {lineup.doubled ? 'matchup' : 'week'} score; any after that score nothing.
						{:else}
							{#each starts.made as start, i (start.player_id + start.day)}
								<span class:skipped={!start.counts}>{start.full_name} ({weekday(start.day)}, {start.innings.toFixed(1)} IP)</span>{i < starts.made.length - 1 ? ', ' : '.'}
							{/each}
							{#if starts.made.length > starts.limit}Starts past the limit score nothing.{/if}
						{/if}
						A reliever who opens a game uses a start only if he goes more than an inning.
					</p>
				</div>
			{/if}

			{#each tableGroups as group (group.label)}
			<section class="card flush lineup-list" aria-label={group.label} style:--stat-cols={group.columns.length}>
				<header class="head narrow"><span>{group.label === 'Lineup' ? 'Starters' : group.label}</span><span>{weekly ? 'This week' : 'Game'}</span></header>
				<ol class="rows">
					{@render labels(group.columns, group.label === 'Lineup' ? 'Starters' : `${group.label} · Starters`)}
					{#each group.seats as {index: i} (i)}
						{@const slot = seatSlots[i]}
						{@const player = seats[i] ? byId.get(seats[i]) : undefined}
						{@const target = !!moving && canPlace(moving, i)}
						<!-- svelte-ignore a11y_no_noninteractive_element_interactions: a drop zone; the tap path is the button inside -->
						<li
							class="seat"
							class:target
							class:dim={!!moving && !target && seats[i] !== moving}
							class:over={over === i}
							ondragover={(e) => dragOver(e, i)}
							ondragleave={() => over === i && (over = undefined)}
							ondrop={(e) => drop(e, i)}
						>
							<span class="slot" title={slot.positions.includes('*') ? 'Any position' : slot.positions.join(', ')}>{slot.name}</span>
							{#if player}
								{@render who(player, true)}
							{:else}
								<span class="empty">Empty</span>
							{/if}
							{#if target}<button class="drop" onclick={() => place(moving!, i)}>Move {byId.get(moving!)?.full_name} here</button>{/if}
						</li>
					{/each}
					<!-- svelte-ignore a11y_no_noninteractive_element_interactions: Bench drop zone has a button for keyboard access. -->
					<li class="bench-zone" class:target={!!moving && canPlace(moving, 'bench')} class:over={over === 'bench'} ondragover={(e) => dragOver(e, 'bench')} ondragleave={() => over === 'bench' && (over = undefined)} ondrop={(e) => drop(e, 'bench')}>
						<div class="bench-divider"><span>Bench · {group.bench.length}</span>{#if moving && canPlace(moving, 'bench')}<button class="small" onclick={() => place(moving!, 'bench')}>Move to bench</button>{/if}</div>
						<ul class="rows">
							{#each group.bench as player (player.player_id)}
								<li class="seat bench-seat"><span class="slot">BN</span>{@render who(player, false)}</li>
							{:else}<li class="muted small-text pad">Everyone is starting.</li>{/each}
						</ul>
					</li>
				</ol>
			</section>
			{/each}

		{/if}
	{/if}
</div>

<style>
	.bench-divider { display:flex; align-items:center; justify-content:space-between; padding:.3rem .55rem; background:var(--surface-2); border-block:1px solid var(--rule); font:600 .62rem var(--mono); text-transform:uppercase; }
	.name { font-size:.8rem; }
	.player-meta { display:flex; align-items:center; flex-wrap:wrap; gap:0 .45rem; }
	.name .row.tight :global(.watch-star) { padding:.1rem; }
	.name .small-text { font-size:.66rem; }
	.name .period-ranks { margin-top:0; font-size:.6rem; }
	.seat .empty { padding:.25rem .5rem; }

	.stat-view-controls { gap: .4rem; }
	.stat-view-controls label { gap: .4rem; }
	.stat-view-controls select { font-size: .8rem; padding: .35rem .5rem; }
	.stat-context { font-size: .68rem; }
	.period-ranks { display: flex; flex-wrap: wrap; gap: .25rem .5rem; font-size: .65rem; color: var(--brand); margin-top: .15rem; }
	.stats-enabled { min-width: 0; grid-template-columns: minmax(0,1fr); }
	.stats-enabled .lineup-list { overflow-x: auto; scrollbar-width: thin; }
	.stats-enabled .lineup-list > .rows { min-width: calc(30rem + var(--stat-cols) * 2.55rem); }
	@media(max-width:640px) { .stats-enabled .lineup-list > .rows { min-width: 0; } }

	/* ---- the day or week ---- */
	.days {
		gap: 0.25rem;
	}
	.step {
		padding: 0.35rem;
	}
	.range {
		min-width: 9rem;
		text-align: center;
		font-size: 0.9rem;
	}
	.day {
		display: grid;
		justify-items: center;
		min-width: 2.5rem;
		padding: 0.25rem 0.4rem;
		line-height: 1.1;
		font-weight: 400;
	}
	.day span {
		font-size: 0.62rem;
		color: var(--ink-soft);
	}
	.day.today {
		border-color: var(--sport, var(--brand));
	}
	.day.on {
		background: var(--sport, var(--brand));
		border-color: var(--sport, var(--brand));
		color: var(--surface);
	}
	.day.on span {
		color: inherit;
	}
	.check {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.note {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.6rem 0.9rem;
	}
	.note.doubled {
		display: block;
		border-left: 3px solid var(--gold);
	}
	.starts {
		display: grid;
		gap: 0.4rem;
		max-width: 38rem;
	}
	.skipped {
		opacity: 0.45;
		text-decoration: line-through;
	}

	/* ---- the slots and the bench ---- */
	/* Slot, player, game, a column per stat, fantasy points: every row shares
	   these, so the numbers line up down the table. */
	.lineup-list {
		--row: 2rem minmax(14rem, 1fr) minmax(6rem, 1fr) calc(var(--stat-cols) * 2.55rem) 3.6rem;
	}
	.head {
		display: grid;
		grid-template-columns: var(--row);
		align-items: center;
		gap: 0.45rem;
		padding: 0.45rem 0.9rem;
		background: var(--surface-2);
		border-bottom: 1px solid var(--rule);
		font: 600 0.66rem/1.2 var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.head span:first-child {
		grid-column: span 2;
	}
	.rows {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.seat,
	.labels {
		position: relative;
		display: grid;
		grid-template-columns: var(--row);
		align-items: center;
		gap: 0.45rem;
		min-height: 2.7rem;
		padding: 0.25rem 0.55rem;
		transition:
			background 120ms,
			opacity 120ms;
	}
	.seat + .seat {
		border-top: 1px solid var(--rule);
	}
	/* The column headings, above the first row of each kind of player. */
	.labels {
		min-height: 0;
		padding: 0 0.55rem;
		background: var(--surface-2);
		border-top: 1px solid var(--rule);
		border-bottom: 1px solid var(--rule-strong);
		font: 700 0.66rem/1 var(--mono);
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--ink-soft);
	}
	.rows > .labels:first-child {
		border-top: 0;
	}
	.labels .title {
		grid-column: span 2;
		color: var(--ink);
		letter-spacing: 0.07em;
	}
	/* The section bar repeats what the first heading row says: it stays only
	   where those rows are hidden, on a phone, or to offer the bench as a target. */
	@media (min-width: 641px) {
		.head.narrow {
			display: none;
		}
	}
	.labels + .seat {
		border-top: 0;
	}
	.slot {
		justify-self: start;
		min-width: 1.8rem;
		padding: 0.3rem 0.35rem;
		border-radius: var(--radius-small);
		background: color-mix(in srgb, var(--sport, var(--brand)) 14%, transparent);
		color: var(--sport, var(--brand));
		font: 700 0.68rem/1 var(--mono);
		text-align: center;
	}
	.bench-seat .slot {
		background: var(--surface-2);
		color: var(--ink-faint);
	}
	.bench-zone .seat {
		background: var(--surface);
		opacity: 0.85;
	}
	.empty {
		grid-column: span 4;
		padding: 0.55rem 0.7rem;
		border: 1px dashed var(--rule-strong);
		border-radius: var(--radius-small);
		color: var(--ink-faint);
		font-size: 0.8rem;
	}
	.seat.target {
		background: color-mix(in srgb, var(--sport, var(--brand)) 8%, transparent);
		box-shadow: inset 3px 0 0 var(--sport, var(--brand));
		cursor: pointer;
	}
	.seat.target .empty {
		border-color: var(--sport, var(--brand));
		color: var(--sport, var(--brand));
	}
	.seat.over,
	.bench-zone.over {
		background: color-mix(in srgb, var(--sport, var(--brand)) 16%, transparent);
	}
	.bench-zone.target {
		outline: 2px dashed var(--sport, var(--brand));
		outline-offset: -2px;
	}
	.seat.dim {
		opacity: 0.4;
	}
	/* While a player is being moved, a tap anywhere on a seat that takes him puts him there. */
	.drop {
		position: absolute;
		inset: 0;
		z-index: 1;
		border: 0;
		border-radius: 0;
		background: transparent;
		color: transparent;
	}
	.drop:focus-visible {
		outline: 2px solid var(--sport, var(--brand));
		outline-offset: -2px;
	}
	.who {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		min-width: 0;
	}
	.who.movable {
		cursor: grab;
	}
	.who.picked {
		border-radius: var(--radius-small);
		box-shadow: 0 0 0 2px var(--sport, var(--brand));
	}
	.grip {
		flex: none;
		padding: 0.3rem 0.15rem;
		border: 0;
		background: transparent;
		color: var(--ink-faint);
		cursor: grab;
	}
	.grip:disabled {
		cursor: not-allowed;
	}
	.grip[aria-pressed='true'] {
		color: var(--sport, var(--brand));
	}
	.name {
		display: grid;
		min-width: 0;
	}
	.name a {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.row.tight {
		flex-wrap: nowrap;
		gap: 0.3rem;
	}
	.games {
		display: grid;
		gap: 0.2rem;
		min-width: 0;
		font-size: 0.8rem;
	}
	.status {
		color: var(--ink-soft);
	}
	.live {
		color: var(--bad);
		font-weight: 650;
	}
	/* True columns: each stat and the points sit between lines that run the
	   full height of the row, headings included. */
	.stats {
		display: grid;
		grid-template-columns: repeat(var(--stat-cols), minmax(0, 1fr));
		align-self: stretch;
		margin-block: -0.25rem;
		text-align: center;
	}
	.stats > span {
		min-width: 0;
		white-space: nowrap;
		display: grid;
		place-items: center;
		border-left: 1px solid var(--rule);
	}
	.stats > span:last-child {
		border-right: 1px solid var(--rule);
	}
	.labels .stats {
		margin-block: 0;
	}
	.labels .stats > span,
	.labels .pts {
		padding-block: 0.5rem;
	}
	.labels .stats > span:last-child,
	.labels .stats > span {
		border-color: var(--rule-strong);
	}
	.seat .stats {
		font: 600 0.72rem var(--mono);
		font-variant-numeric: tabular-nums;
	}
	.stats .zero,
	.none {
		color: var(--ink-faint);
		font-weight: 400;
	}
	/* Each value's own label shows only where the label rows do not: on a phone. */
	.seat small {
		display: none;
	}
	.opponent {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-weight: 650;
	}
	.game {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.15rem 0.5rem;
	}
	/* A game as a tile: the opponent's logo, its day and time beneath. */
	.tiles {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
	}
	.tile {
		position: relative;
		display: grid;
		justify-items: center;
		gap: 0.05rem;
		min-width: 2.8rem;
		padding: 0.2rem 0.25rem;
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
		background: var(--surface);
		font: 600 0.68rem/1.25 var(--body);
		white-space: nowrap;
	}
	.tile .logo {
		display: grid;
		place-items: center;
		height: 26px;
	}
	.tile .logo small {
		position: absolute;
		top: 0.2rem;
		left: 0.3rem;
		display: block;
		font: 600 0.55rem var(--mono);
		color: var(--ink-faint);
	}
	.tile .time {
		font-weight: 400;
		color: var(--ink-soft);
	}
	.tile .proj {
		font-size: 0.62rem;
	}
	.tile.on .proj {
		color: var(--ink);
	}
	.tile.done:not(.on) {
		opacity: 0.5;
	}
	button.tile:hover {
		border-color: var(--sport, var(--brand));
	}
	.tile.on {
		border-color: var(--sport, var(--brand));
		background: color-mix(in srgb, var(--sport, var(--brand)) 18%, var(--surface));
		box-shadow: inset 0 -3px 0 var(--sport, var(--brand));
	}
	.tile.on .time {
		color: var(--ink);
	}
	/* On auto, the game that counts if he plays in it. */
	.tile.next {
		border-style: dashed;
		border-color: var(--sport, var(--brand));
	}
	.tile .start {
		padding: 0.15rem 0.3rem;
		font-size: 0.56rem;
	}
	.tile.auto {
		min-width: 3.4rem;
	}
	.tile.auto .logo strong {
		font: 700 0.72rem var(--mono);
		letter-spacing: 0.06em;
	}
	.pts {
		text-align: right;
		font: 700 0.9rem var(--mono);
	}
	.proj {
		display: block;
		font-style: italic;
		font-weight: 500;
		color: var(--ink-soft);
	}
	.labels .pts {
		font: inherit;
		color: var(--ink);
	}
	.pad {
		padding: 0.6rem 0.9rem;
	}
	.actions {
		gap: 0.5rem 0.8rem;
	}
	.saved {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		color: var(--good);
	}
	@media (max-width: 640px) {
		.head {
			grid-template-columns: 1fr auto;
		}
		.head span:first-child {
			grid-column: auto;
		}
		.head span:nth-child(2) {
			display: none;
		}
		.seat {
			grid-template-columns: 2.6rem minmax(0, 1fr) 3.4rem;
			gap: 0.3rem 0.6rem;
		}
		.labels {
			display: none;
		}
		/* With the label rows hidden, every player but the first keeps his divider. */
		.labels + .seat {
			border-top: 1px solid var(--rule);
		}
		.rows > .labels:first-child + .seat {
			border-top: 0;
		}
		.games {
			grid-column: 2 / 4;
			grid-row: 2;
		}
		.stats {
			grid-column: 2 / 4;
			grid-row: 3;
			display: flex;
			flex-wrap: wrap;
			gap: 0.15rem 0.7rem;
		}
		.stats.none {
			display: none;
		}
		.stats {
			margin-block: 0;
		}
		.stats > span,
		.stats > span:last-child {
			display: inline;
			border: 0;
		}
		.seat small {
			display: inline;
			margin-right: 0.2rem;
			font: 600 0.58rem var(--mono);
			color: var(--ink-faint);
		}
		.pts {
			grid-column: 3;
			grid-row: 1;
		}
		.pts small {
			display: block;
			margin: 0;
		}
		.empty {
			grid-column: span 2;
		}
	}
</style>
