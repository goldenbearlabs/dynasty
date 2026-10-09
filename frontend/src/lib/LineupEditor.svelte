<script lang="ts">
	// Set who starts for one franchise in one league. A daily league moves a
	// day at a time; a weekly one shows the whole week as a grid of games,
	// where a slot that counts only some games lets the manager pick which.
	import { getLineup, setLineup, type Franchise, type League, type Lineup, type LineupGame, type LineupPlayer } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import { dayLabel, points, shiftDay, timeOfDay } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	type Props = {
		league: League;
		franchise: Franchise;
		/** Whether the viewer may change this lineup, and whether they are a commissioner. */
		canEdit: boolean;
		commissioner?: boolean;
		/** The day to open on; today when left out. Kept up to date as the viewer moves. */
		day?: string;
	};
	let { league, franchise, canEdit, commissioner = false, day = $bindable() }: Props = $props();

	const sport = $derived(league.competition);
	let override = $state(false); // the commissioner's override of the locks

	let lineup = $state<Lineup>();
	// The slot chosen for each player in the form; '' is the bench.
	let chosen = $state<Record<string, string>>({});
	// The game picked for each player in a slot that counts only some games a
	// week, as the day it is played; '' leaves it to his next game.
	let picked = $state<Record<string, string>>({});

	function show(loaded: Lineup) {
		lineup = loaded;
		chosen = Object.fromEntries(loaded.players.map((p) => [p.player_id, p.slot]));
		picked = Object.fromEntries(loaded.players.map((p) => [p.player_id, p.counts_from]));
	}
	$effect(() => {
		getLineup(league.id, franchise.id, day).then(show, toast.error);
	});
	// Points and locks follow the games, unless there are unsaved changes to keep.
	onScoresChange(
		() => [sport],
		() => {
			if (lineup && !dirty) getLineup(league.id, franchise.id, lineup.day).then(show, toast.error);
		}
	);

	const weekly = $derived(lineup !== undefined && lineup.day !== lineup.last_day);
	const dirty = $derived(
		lineup?.players.some((p) => chosen[p.player_id] !== p.slot || picked[p.player_id] !== p.counts_from) ?? false
	);
	const filled = (slot: string) => Object.values(chosen).filter((s) => s === slot).length;
	const open = $derived(lineup ? lineup.slots.reduce((n, s) => n + Math.max(0, s.count - filled(s.name)), 0) : 0);
	const fits = (player: LineupPlayer, positions: string[]) =>
		positions.includes('*') || player.positions.some((p) => positions.includes(p));
	const frozen = (player: LineupPlayer) => !canEdit || ((lineup!.locked !== '' || player.locked) && !override);
	// How many games a week count in the slot a player is in; 0 is all of them.
	const limit = (player: LineupPlayer) => lineup!.slots.find((s) => s.name === chosen[player.player_id])?.games_per_week ?? 0;
	const limited = $derived(lineup?.slots.filter((s) => s.games_per_week > 0) ?? []);
	const begun = (game: LineupGame) => new Date(game.starts_at).getTime() <= Date.now();
	// Whether a game is one that counts, going by what is chosen in the form.
	function counts(player: LineupPlayer, game: LineupGame): boolean {
		const n = limit(player);
		if (n === 0) return true;
		const unsaved = chosen[player.player_id] !== player.slot || picked[player.player_id] !== player.counts_from;
		if (!unsaved) return game.counts;
		const from = picked[player.player_id] || player.games.find((g) => !begun(g))?.day || '';
		return player.games.filter((g) => g.day >= from).slice(0, n).includes(game);
	}
	const pickable = (player: LineupPlayer, game: LineupGame) => limit(player) > 0 && !frozen(player) && !begun(game);

	// Starters first, in slot order, then the bench.
	const ordered = $derived.by(() => {
		if (!lineup) return [];
		const rank = (p: LineupPlayer) => {
			const i = lineup!.slots.findIndex((s) => s.name === chosen[p.player_id]);
			return i < 0 ? lineup!.slots.length : i;
		};
		return lineup.players.toSorted((a, b) => rank(a) - rank(b) || a.full_name.localeCompare(b.full_name));
	});

	// ---- days ----
	const today = () => new Date().toLocaleDateString('en-CA'); // close enough to pick a column to highlight
	// The seven days a weekly lineup covers, or the week around a daily one.
	const week = $derived.by(() => {
		if (!lineup) return [];
		let first = lineup.day;
		if (!weekly) {
			const names = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
			const back = (new Date(first + 'T12:00').getDay() - names.indexOf(league.settings.lineup.week_start) + 7) % 7;
			first = shiftDay(first, -back);
		}
		return Array.from({ length: 7 }, (_, i) => shiftDay(first, i));
	});
	const weekday = (d: string) => new Date(d + 'T12:00').toLocaleDateString(undefined, { weekday: 'short' });
	const dayNumber = (d: string) => new Date(d + 'T12:00').getDate();

	async function save() {
		if (!lineup) return;
		try {
			await setLineup(league.id, {
				day: lineup.day,
				entries: Object.entries(chosen)
					.filter(([, slot]) => slot !== '')
					.map(([player_id, slot]) => ({ slot, player_id, counts_from: picked[player_id] ?? '' })),
				franchise_id: franchise.id,
				force: override
			});
			show(await getLineup(league.id, franchise.id, lineup.day));
			toast.good('Lineup saved. It stays in force until you change it.');
		} catch (e) {
			toast.error(e);
		}
	}

	function describe(game: LineupGame, withDay = weekly): string {
		const versus = `${game.at_home ? 'vs' : '@'} ${game.opponent}`;
		if (game.status === 'live') return `${versus} · live`;
		if (game.status === 'final') return `${versus} · final`;
		return `${versus} · ${withDay ? `${weekday(game.day)} ` : ''}${timeOfDay(game.starts_at)}`;
	}
	// What a game's cell says in the week grid: the score once there is one, the time before.
	const cell = (game: LineupGame) => (game.status === 'scheduled' ? timeOfDay(game.starts_at) : points(game.points));
</script>

<div class="stack tight" data-sport={sport}>
	{#if !lineup}
		<p class="muted">Loading the lineup…</p>
	{:else}
		<div class="spread">
			<nav class="row days" aria-label={weekly ? 'Week' : 'Day'}>
				<button aria-label="Earlier" onclick={() => (day = shiftDay(lineup!.day, -1))}><Icon name="left" size={16} /></button>
				{#if weekly}
					<strong class="when">{dayLabel(lineup.day)} – {dayLabel(lineup.last_day)}</strong>
				{:else}
					{#each week as d (d)}
						<button class="day" class:on={d === lineup.day} class:today={d === today()} aria-pressed={d === lineup.day} onclick={() => (day = d)}>
							<span>{weekday(d)}</span><strong>{dayNumber(d)}</strong>
						</button>
					{/each}
				{/if}
				<button aria-label="Later" onclick={() => (day = shiftDay(lineup!.last_day, 1))}><Icon name="right" size={16} /></button>
				<button class="quiet" onclick={() => (day = undefined)}>{weekly ? 'This week' : 'Today'}</button>
			</nav>
			{#if commissioner}
				<label class="check">
					<input type="checkbox" bind:checked={override} />
					<span class="pill gold">Commissioner</span> Override locks
				</label>
			{/if}
		</div>

		{#if lineup.locked}<p class="card note"><Icon name="lock" size={16} /> {lineup.locked}</p>{/if}

		{#if lineup.players.length === 0}
			<Empty icon="players" title="Nobody on the main roster">
				Only main-roster players can start. <a href="/players?competition={sport}">Find players</a>.
			</Empty>
		{:else}
			<div class="row slots">
				{#each lineup.slots as slot (slot.name)}
					<span class="pill" class:bad={filled(slot.name) > slot.count} class:good={filled(slot.name) === slot.count}>
						{slot.name} {filled(slot.name)}/{slot.count}
					</span>
				{/each}
				{#if canEdit && open > 0}<span class="pill gold">{open} open {open === 1 ? 'slot' : 'slots'}</span>{/if}
				{#if limited.length > 0}
					<span class="muted small-text">
						{limited.length === lineup.slots.length ? 'Each starter' : `Each ${limited.map((s) => s.name).join(' and ')}`}
						scores in {limited[0].games_per_week === 1 ? 'one game' : `${limited[0].games_per_week} games`} this week:
						click the game you want.
					</span>
				{/if}
			</div>

			{#if lineup.starts}
				{@const starts = lineup.starts}
				<div class="card starts">
					<Meter label="Pitcher starts this week" value={starts.made.length} max={starts.limit} />
					<p class="muted small-text">
						{#if starts.made.length === 0}
							None yet. The first {starts.limit} of the week score; any after that score nothing.
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

			<div class="card flush scroll">
				<table class:grid={weekly}>
					<thead>
						<tr>
							<th>Slot</th>
							<th>Player</th>
							{#if weekly}
								{#each week as d (d)}<th class="daycol" class:today={d === today()}>{weekday(d)} {dayNumber(d)}</th>{/each}
							{:else}
								<th class="wide">Game</th>
							{/if}
							<th class="num">Points</th>
						</tr>
					</thead>
					<tbody>
						{#each ordered as player (player.player_id)}
							<tr class:bench={!chosen[player.player_id]}>
								<td class="slot">
									<select aria-label="Slot for {player.full_name}" bind:value={chosen[player.player_id]} disabled={frozen(player)}>
										<option value="">Bench</option>
										{#each lineup.slots.filter((s) => fits(player, s.positions)) as slot (slot.name)}
											<option value={slot.name}>{slot.name}</option>
										{/each}
									</select>
								</td>
								<td>
									<div class="player">
										<Headshot name={player.full_name} src={player.headshot_url} size={30} />
										<div>
											<strong>{player.full_name}</strong>
											{#if player.locked}<span class="pill"><Icon name="lock" size={11} /> Locked</span>{/if}
											<div class="muted small-text">
												{player.positions.join('/')} · {player.team_abbrev}
												{#if !weekly}<span class="narrow">{player.games.map((g) => describe(g)).join(', ')}</span>{/if}
											</div>
										</div>
									</div>
								</td>
								{#if weekly}
									{#each week as d (d)}
										<td class="daycol" class:today={d === today()}>
											{#each player.games.filter((g) => g.day === d) as game (game.starts_at)}
												{#if pickable(player, game)}
													<button
														class="game"
														class:counted={counts(player, game)}
														aria-pressed={counts(player, game)}
														title="Count this game for {player.full_name}"
														onclick={() => (picked[player.player_id] = game.day)}
													>
														<span>{game.at_home ? 'vs' : '@'} {game.opponent}</span><small>{cell(game)}</small>
													</button>
												{:else}
													<div class="game" class:counted={chosen[player.player_id] !== '' && counts(player, game)} class:skipped={chosen[player.player_id] !== '' && !counts(player, game)}>
														<span>{game.at_home ? 'vs' : '@'} {game.opponent}</span><small>{cell(game)}</small>
													</div>
												{/if}
											{/each}
										</td>
									{/each}
								{:else}
									<td class="wide muted small-text">
										{#each player.games as game (game.starts_at)}<div>{describe(game)}</div>{:else}No game{/each}
									</td>
								{/if}
								<td class="num total">{player.games.length > 0 ? points(player.points) : ''}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			{#if canEdit}
				<div class="row">
					<button class="primary" disabled={!dirty} onclick={save}>Save lineup</button>
					{#if dirty}<button class="quiet" onclick={() => show(lineup!)}>Discard changes</button>{/if}
					<span class="muted small-text">A lineup stays in force until you change it.</span>
				</div>
			{/if}
		{/if}
	{/if}
</div>

<style>
	.days {
		gap: 0.3rem;
	}
	.days button {
		padding: 0.4rem 0.55rem;
	}
	.when {
		min-width: 8.5rem;
		text-align: center;
		padding: 0 0.4rem;
	}
	.day {
		display: grid;
		gap: 0;
		justify-items: center;
		min-width: 2.7rem;
		line-height: 1.15;
		font-weight: 400;
	}
	.day span {
		font-size: 0.68rem;
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
	.note {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.7rem 1rem;
	}
	.slots {
		gap: 0.35rem;
	}
	.starts {
		display: grid;
		gap: 0.4rem;
		max-width: 38rem;
	}
	.slot {
		width: 6.5rem;
	}
	.slot select {
		width: 100%;
		padding: 0.35rem 0.4rem;
	}
	tr.bench td {
		background: var(--surface-2);
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		min-width: 11rem;
	}
	.total {
		font-weight: 650;
	}
	.narrow {
		display: none;
	}

	/* The week grid: a column per day, a cell per game. */
	.grid td,
	.grid th {
		padding: 0.35rem 0.4rem;
	}
	.daycol {
		width: 5.2rem;
		min-width: 4.6rem;
		text-align: center;
		vertical-align: middle;
	}
	th.daycol.today {
		color: var(--sport, var(--brand));
	}
	td.daycol.today {
		background: color-mix(in srgb, var(--sport, var(--brand)) 7%, transparent);
	}
	.game {
		display: grid;
		width: 100%;
		gap: 0;
		justify-items: center;
		padding: 0.25rem 0.2rem;
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
		background: var(--surface);
		font-size: 0.74rem;
		font-weight: 600;
		line-height: 1.2;
		white-space: nowrap;
	}
	.game small {
		font-weight: 400;
		font-size: 0.66rem;
		color: var(--ink-soft);
	}
	button.game:hover {
		border-color: var(--sport, var(--brand));
	}
	.game.counted {
		background: var(--sport, var(--brand));
		border-color: var(--sport, var(--brand));
		color: var(--surface);
	}
	.game.counted small {
		color: inherit;
	}
	.skipped {
		opacity: 0.45;
		text-decoration: line-through;
	}
	.game.skipped {
		text-decoration: none;
	}
	@media (max-width: 640px) {
		.wide {
			display: none;
		}
		.narrow {
			display: inline;
		}
		.narrow::before {
			content: '· ';
		}
	}
</style>
