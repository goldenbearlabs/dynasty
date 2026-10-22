<script lang="ts">
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import WatchStar from '#lib/WatchStar.svelte';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	// A searchable, paged list of players. Used wherever players are picked
	// from: the player browser, free agency and the draft room.
	import { untrack, type Snippet } from 'svelte';
	import {
		getPlayers,
		getStatSeasons,
		type Competition,
		type Franchise,
		type Player,
		type PlayerPage,
		type StatSeason,
	} from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { age } from '#lib/ui/time.ts';

	type Props = {
		workspace?: boolean;
		franchises?: Franchise[];
		/** Show one sport, or every sport when empty. */
		competition?: string;
		/**
		 * Offer more filters, for these sports: position and an age range, and
		 * when every sport is shown, a choice of one of them.
		 */
		competitions?: Competition[];
		/** Leave the choice of sport to the caller, e.g. to its own tabs. */
		noLeaguePicker?: boolean;
		/** Offer an "available only" switch for this league. */
		leagueId?: string;
		/** Show only players this draft can still pick. */
		draftId?: string;
		/** What to show in the last column for a player nobody owns. */
		action?: Snippet<[Player]>;
		/** Change this to reload the current page, e.g. after a roster move. */
		version?: number;
		/**
		 * Rank by a season's fantasy points: highest first, with controls for the
		 * season and the order. One sport is ordered by its points; a list of
		 * several sports by the index that makes them comparable.
		 */
		byPoints?: boolean;
		/**
		 * In a compact list, give the season's points and index columns of
		 * their own, and put the team and age under the name.
		 */
		statColumns?: boolean;
		/** Compact rows with a name button that opens inline research. */
		compact?: boolean;
		onselect?: (player: Player) => void;
		selectedId?: string;
	};
	let {
		workspace = false,
		franchises = [],
		competition: only = '',
		competitions = [],
		noLeaguePicker = false,
		leagueId,
		draftId,
		action,
		version = 0,
		byPoints = false,
		statColumns = false,
		compact = false,
		onselect,
		selectedId,
	}: Props = $props();

	// The sport shown: the one asked for, or the one picked from the filters.
	let picked = $state('');
	const competition = $derived(only || picked);
	// One sport offers its own positions; several sports offer all of theirs.
	const positions = $derived(
		competition
			? (competitions.find((c) => c.key === competition)?.positions ?? [])
			: [...new Set(competitions.flatMap((c) => c.positions))].sort(),
	);
	let position = $state('');
	let minAge = $state<number | null>(null);
	let maxAge = $state<number | null>(null);

	let status = $state('');
	let search = $state('');
	let query = $state(''); // search, applied after a pause in typing
	let availableOnly = $state(false);
	let ownership = $state<string[]>(untrack(() => (workspace ? ['available'] : [])));
	let owners = $state<string[]>([]);
	let perGame = $state(false);
	const statKeys = $derived(
		competition ? (competitions.find((c) => c.key === competition)?.stats ?? []) : [],
	);
	let chosenStats = $state<string[] | null>(null);
	let mobileStats = $state(false);
	let moreFilters = $state(false);
	const displayStats = $derived(
		statKeys.filter((stat) =>
			chosenStats ? chosenStats.includes(stat.key) : statKeys.indexOf(stat) < 6,
		),
	);
	const statValue = (player: Player, key: string) => {
		const n = player.season_stats?.[key];
		return n == null || (perGame && !player.season_games)
			? '—'
			: (perGame ? n / player.season_games : n).toLocaleString(undefined, {
					maximumFractionDigits: 1,
				});
	};
	function toggleOwnership(value: string) {
		ownership = ownership.includes(value)
			? ownership.filter((v) => v !== value)
			: [...ownership, value];
		page = 1;
	}
	const usual = () =>
		workspace ? 'points' : !byPoints ? 'name' : competition ? 'points' : 'index';
	let order = $state(untrack(usual));
	let ascending = $state(false);
	const textSort = (key: string) => ['name', 'position', 'team'].includes(key);
	const sorted = (key: string) => workspace && order === key;
	const sortDirection = (key: string) =>
		sorted(key) ? (ascending ? 'ascending' : 'descending') : 'none';
	function sortColumn(key: string) {
		ascending = order === key ? !ascending : textSort(key);
		order = key;
		chose = true;
		page = 1;
	}
	let chose = false; // whether the order was picked by hand, and so is kept
	let seasonBack = $state(0);
	let seasons = $state<StatSeason[]>([]);
	if (untrack(() => byPoints))
		getStatSeasons().then(
			(s) => (seasons = s),
			() => {},
		);
	// One sport offers its own seasons by name; several sports can only count back.
	const seasonChoices = $derived.by(() => {
		if (competition)
			return seasons.filter((s) => s.competition === competition).map((s) => s.label);
		const deepest = Math.max(
			0,
			...Object.values(Object.groupBy(seasons, (s) => s.competition)).map((list) => list!.length),
		);
		return Array.from({ length: Math.min(deepest, 10) }, (_, back) =>
			back === 0 ? 'Latest season' : `${back} ${back === 1 ? 'season' : 'seasons'} back`,
		);
	});
	const ranked = $derived(order === 'points' || order === 'index');
	// Whether the season's figures get columns: always in the full list, on request in the compact one.
	const columns = $derived(workspace || (ranked && (!compact || statColumns)));
	// What follows a player's team under his name.
	const under = (player: Player) =>
		[
			age(player.birth_date) && `age ${age(player.birth_date)}`,
			!competition && ranked && player.season,
		]
			.filter(Boolean)
			.join(' · ');
	const fantasyPoints = (n: number) => Math.round(n).toLocaleString();
	let page = $state(1);

	let result = $state<PlayerPage>();
	let error = $state('');
	let loading = $state(true);

	const pages = $derived(result ? Math.max(1, Math.ceil(result.total / result.per_page)) : 1);

	let typing: ReturnType<typeof setTimeout>;
	function onSearch() {
		clearTimeout(typing);
		typing = setTimeout(() => {
			query = search.trim();
			page = 1;
		}, 250);
	}

	// Changing sport starts again from the first page.
	let shown = '';
	$effect.pre(() => {
		if (competition !== shown) {
			shown = competition;
			position = ''; // positions belong to a sport
			chosenStats = null;
			if (order.startsWith('stat:')) {
				order = 'points';
				chose = false;
				ascending = false;
			}
			page = 1;
			seasonBack = 0;
			if (!chose || (order === 'prospect_rank' && competition !== 'mlb' && competition !== ''))
				order = usual();
		}
	});

	let latest = 0;
	$effect(() => {
		void version;
		const request = ++latest;
		loading = true;
		getPlayers({
			competition,
			ownership: workspace ? ownership.join(',') : undefined,
			owners: workspace ? owners.join(',') : undefined,
			status,
			q: query,
			position: position || undefined,
			min_age: minAge ?? undefined,
			max_age: maxAge ?? undefined,
			page,
			draft_id: draftId,
			sort: order.startsWith('stat:') ? 'stat' : order,
			stat_key: order.startsWith('stat:') ? order.slice(5) : undefined,
			ascending: workspace ? String(ascending) : undefined,
			per_game: workspace ? String(perGame) : undefined,
			season_back: (ranked || workspace) && seasonBack > 0 ? seasonBack : undefined,
			available_in: availableOnly ? leagueId : undefined,
		})
			.then((data) => {
				if (request !== latest) return; // a newer request has replaced this one
				result = data;
				error = '';
			})
			.catch((e: Error) => {
				if (request === latest) error = e.message;
			})
			.finally(() => {
				if (request === latest) loading = false;
			});
	});
</script>

{#snippet team(player: Player)}{#if player.team_abbrev}<TeamLogo
			competition={player.competition}
			abbrev={player.team_abbrev}
			size={18}
		/>{:else}{player.team_name || 'No team'}{/if}{/snippet}

{#snippet sortHeader(key: string, label: string, description = '')}
	{#if workspace}<button
			class="sort-header"
			class:active={sorted(key)}
			title={description || `Sort by ${label}`}
			onclick={() => sortColumn(key)}
			>{label}{#if sorted(key)}<Icon name={ascending ? 'up' : 'down'} size={12} />{/if}</button
		>{:else}{label}{/if}
{/snippet}

<div class="stack tight" class:compact class:workspace class:more-filters={moreFilters}>
	{#if workspace}<button
			class="small quiet mobile-toggle"
			aria-expanded={moreFilters}
			onclick={() => (moreFilters = !moreFilters)}
			>{moreFilters ? 'Fewer filters' : 'More filters'}</button
		>{/if}
	<div class="row filters">
		<label class="search">
			<Icon name="search" size={16} />
			<input
				type="search"
				placeholder="Search players"
				aria-label="Search players"
				bind:value={search}
				oninput={onSearch}
			/>
		</label>
		<select
			class="extra-filter"
			aria-label="Status"
			bind:value={status}
			onchange={() => (page = 1)}
		>
			<option value="">Any status</option>
			<option value="active">Active</option>
			<option value="prospect">Prospects</option>
			<option value="inactive">Inactive</option>
		</select>
		{#if byPoints || competition === 'mlb'}
			<select
				aria-label="Order"
				bind:value={order}
				onchange={() => {
					chose = true;
					ascending = textSort(order);
					page = 1;
				}}
			>
				{#if byPoints}<option value="points">By fantasy points</option><option value="index"
						>By league index</option
					>{/if}
				{#if competition === 'mlb' || !competition}<option value="prospect_rank"
						>By MLB Pipeline Top 100 rank</option
					>{/if}
				<option value="name">By name</option>
				{#if workspace}<option value="games">By games played</option><option value="position"
						>By position</option
					>{#each displayStats as stat}<option value="stat:{stat.key}"
							>By {stat.label.toLowerCase()}</option
						>{/each}{/if}
			</select>
			{#if (ranked || workspace) && seasonChoices.length > 0}
				<select aria-label="Season" bind:value={seasonBack} onchange={() => (page = 1)}>
					{#each seasonChoices as label, back (back)}<option value={back}>{label}</option>{/each}
				</select>
			{/if}
		{/if}
		{#if competitions.length > 0}
			{#if !only && !noLeaguePicker && competitions.length > 1}
				<select aria-label="League" bind:value={picked}>
					<option value="">All leagues</option>
					{#each competitions as c (c.key)}<option value={c.key}>{c.name}</option>{/each}
				</select>
			{/if}
			{#if positions.length > 0}
				<select
					class="extra-filter"
					aria-label="Position"
					bind:value={position}
					onchange={() => (page = 1)}
				>
					<option value="">Any position</option>
					{#each positions as p (p)}<option value={p}>{p}</option>{/each}
				</select>
			{/if}
			<span class="ages extra-filter">
				Age
				<input
					type="number"
					min="0"
					max="60"
					placeholder="min"
					aria-label="Youngest age"
					bind:value={minAge}
					onchange={() => (page = 1)}
				/>
				–
				<input
					type="number"
					min="0"
					max="60"
					placeholder="max"
					aria-label="Oldest age"
					bind:value={maxAge}
					onchange={() => (page = 1)}
				/>
			</span>
		{/if}
		{#if leagueId && !draftId && !workspace}
			<label class="check">
				<input type="checkbox" bind:checked={availableOnly} onchange={() => (page = 1)} />
				Available only
			</label>
		{/if}
	</div>

	{#if workspace}
		<div class="ownership-filters">
			<div class="row" role="group" aria-label="Ownership filters">
				<button
					class="small quiet"
					class:active={ownership.length === 0}
					onclick={() => {
						ownership = [];
						owners = [];
						page = 1;
					}}>All players</button
				>
				{#each [{ value: 'available', label: 'Available' }, { value: 'taken', label: 'All taken' }, { value: 'main', label: 'Rostered' }, { value: 'reserve', label: 'Reserve' }, { value: 'rights', label: 'Unsigned' }] as option}
					<button
						class="small quiet"
						class:active={ownership.includes(option.value)}
						aria-pressed={ownership.includes(option.value)}
						onclick={() => toggleOwnership(option.value)}>{option.label}</button
					>
				{/each}
			</div>
			<details class="team-filter">
				<summary>Fantasy teams{owners.length ? ` (${owners.length})` : ''}</summary>
				<div class="team-options">
					{#each franchises as f (f.id)}<label class="check"
							><input
								type="checkbox"
								value={f.slug}
								bind:group={owners}
								onchange={() => {
									if (ownership.length === 1 && ownership[0] === 'available') ownership = [];
									page = 1;
								}}
							/>{f.name}</label
						>{/each}
				</div>
			</details>
			{#if statKeys.length}
				<details class="team-filter">
					<summary>Stat columns</summary>
					<div class="team-options">
						{#each statKeys as stat}<label class="check"
								><input
									type="checkbox"
									checked={displayStats.some((s) => s.key === stat.key)}
									onchange={() => {
										const current = displayStats.map((s) => s.key);
										chosenStats = current.includes(stat.key)
											? current.filter((k) => k !== stat.key)
											: [...current, stat.key];
										if (order === `stat:${stat.key}` && !chosenStats.includes(stat.key)) {
											order = 'points';
											ascending = false;
											page = 1;
										}
									}}
								/>{stat.label}</label
							>{/each}
					</div>
				</details>
			{/if}
			<label class="check"
				><input type="checkbox" bind:checked={perGame} onchange={() => (page = 1)} />Per game</label
			>
			<label class="check mobile-toggle"
				><input type="checkbox" bind:checked={mobileStats} />Show detailed stats</label
			>
		</div>
		<div class="spread muted small-text">
			<span
				>{result ? `${result.total.toLocaleString()} players` : 'Loading players…'} · {ownership.length
					? 'Combine ownership filters'
					: 'All ownership statuses'}</span
			><span>+ Add · − Drop · ⇄ Trade</span>
		</div>
	{/if}

	{#if error}
		<p role="alert">Could not load players: {error}</p>
	{:else if result}
		{#if result.players.length === 0}
			<Empty icon="search" title="No players match">Try a different search, sport or status.</Empty>
		{:else}
			<div
				class="panel scroll"
				class:loading
				class:show-mobile-stats={mobileStats}
				aria-busy={loading}
			>
				<table>
					<thead>
						<tr>
							<th
								class:sorted={sorted('name')}
								aria-sort={workspace ? sortDirection('name') : undefined}
								>{@render sortHeader('name', 'Player')}</th
							>
							<th
								class:mobile-hide={workspace}
								class:sorted={sorted('position')}
								aria-sort={workspace ? sortDirection('position') : undefined}
								>{@render sortHeader('position', 'Pos')}</th
							>
							{#if !compact}<th
									class="wide"
									class:sorted={sorted('team')}
									aria-sort={workspace ? sortDirection('team') : undefined}
									>{@render sortHeader('team', 'Team')}</th
								><th
									class="wide num"
									class:sorted={sorted('age')}
									aria-sort={workspace ? sortDirection('age') : undefined}
									>{@render sortHeader('age', 'Age')}</th
								>{/if}
							{#if workspace}<th
									class="num mobile-hide"
									class:sorted={sorted('games')}
									aria-sort={sortDirection('games')}
									>{@render sortHeader('games', 'GP', 'Games played')}</th
								>{/if}
							{#if columns}<th
									class="num"
									class:sorted={sorted('points')}
									aria-sort={workspace ? sortDirection('points') : undefined}
									>{@render sortHeader(
										'points',
										workspace && perGame ? 'FP/G' : 'FP',
										workspace && perGame ? 'Fantasy points per game' : 'Fantasy points',
									)}</th
								><th
									class="num"
									class:sorted={sorted('index')}
									aria-sort={workspace ? sortDirection('index') : undefined}
									>{@render sortHeader(
										'index',
										'Index',
										'100 is the average rostered player; 15 is one standard deviation. Based on season totals.',
									)}</th
								>{/if}
							{#if workspace}{#each displayStats as stat}<th
										class="num raw-stat"
										class:sorted={sorted(`stat:${stat.key}`)}
										aria-sort={sortDirection(`stat:${stat.key}`)}
										>{@render sortHeader(
											`stat:${stat.key}`,
											stat.key.toUpperCase(),
											stat.label,
										)}</th
									>{/each}{/if}
							<th class="num"
								>{workspace ? 'Action' : `${result.total.toLocaleString()} players`}</th
							>
						</tr>
					</thead>
					<tbody>
						{#each result.players as player (player.id)}
							<tr class:selected={selectedId === player.id}>
								<td class:sorted={sorted('name')}>
									<div class="player">
										<Headshot
											name={player.full_name}
											src={player.headshot_url}
											size={compact ? 28 : 36}
										/>
										<div>
											<div class="name">
												{#if onselect}<button
														class="player-name"
														aria-pressed={selectedId === player.id}
														onclick={() => onselect(player)}>{player.full_name}</button
													>{:else}<a href="/player/{player.id}"
														><strong>{player.full_name}</strong></a
													>{/if}
												<WatchStar playerID={player.id} playerName={player.full_name} />
												{#if !competition}<SportBadge sport={player.competition} />{/if}
												{#if player.class}<span class="pill">{player.class}</span>{/if}
												<InjuryBadge designation={player.injury_designation} />
												<ProspectRank id={player.id} />
												{#if player.status === 'prospect'}<span class="pill gold">Prospect</span
													>{/if}
												{#if player.status === 'inactive'}<span class="pill">Inactive</span>{/if}
												{#if player.waiver_until}<span class="pill brand">Waivers</span>{/if}
											</div>
											{#if workspace}<div class="muted small-text player-detail">
													{player.positions.join('/')} · {@render team(
														player,
													)}{#if player.season && !competition}
														· {player.season}{/if} · {#if player.owner_slug}<a
															href="/franchise/{player.owner_slug}">{player.owner_name}</a
														>
														· {player.roster_list === 'main'
															? 'Rostered'
															: player.roster_list === 'reserve'
																? 'Reserve'
																: 'Unsigned'}{:else}{player.waiver_until
															? 'Waivers'
															: 'Available'}{/if}
												</div>
											{:else if compact && statColumns}<div class="muted small-text">
													{@render team(player)}{#if under(player)}
														· {under(player)}{/if}
												</div>
											{:else if compact}<div class="muted small-text">
													{@render team(player)} · {player.status}
													{#if ranked && player.season}
														· <strong>{fantasyPoints(player.season_points)}</strong>
														pts
														{#if player.season_index}· <span
																title="League index: 100 is the average rostered player, 15 is one standard deviation"
																>index <strong>{Math.round(player.season_index)}</strong></span
															>{/if}
														{#if !competition}· {player.season}{/if}
													{/if}
												</div>{:else if player.note}<div class="muted small-text">
													{player.note}
												</div>{/if}
										</div>
									</div>
								</td>
								<td class:mobile-hide={workspace} class:sorted={sorted('position')}
									>{player.positions.join('/')}</td
								>
								{#if !compact}<td
										class="wide muted"
										class:sorted={sorted('team')}
										title={player.team_name}>{player.team_name}</td
									><td class="wide num" class:sorted={sorted('age')}>{age(player.birth_date)}</td
									>{/if}
								{#if workspace}<td class="num mobile-hide" class:sorted={sorted('games')}
										>{player.season ? player.season_games : '—'}</td
									>{/if}
								{#if columns}<td class="num" class:sorted={sorted('points')}
										>{player.season && player.season_scored !== false
											? workspace && perGame
												? player.season_games
													? (player.season_points / player.season_games).toFixed(1)
													: '—'
												: fantasyPoints(player.season_points)
											: '—'}</td
									><td class="num" class:sorted={sorted('index')}
										>{player.season_index ? Math.round(player.season_index) : '—'}</td
									>{/if}
								{#if workspace}{#each displayStats as stat}<td
											class="num raw-stat"
											class:sorted={sorted(`stat:${stat.key}`)}>{statValue(player, stat.key)}</td
										>{/each}{/if}
								<td class="actions">
									{#if workspace && action}{@render action(player)}
									{:else if player.owner_slug}
										<a class="small-text" href="/franchise/{player.owner_slug}"
											>{player.owner_name}</a
										>
									{:else if action}
										{@render action(player)}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			{#if pages > 1}
				<nav class="spread" aria-label="Pages">
					<button disabled={page <= 1} onclick={() => page--}>Previous</button>
					<span class="muted small-text">Page {page} of {pages.toLocaleString()}</span>
					<button disabled={page >= pages} onclick={() => page++}>Next</button>
				</nav>
			{/if}
		{/if}
	{:else}
		<p class="muted">Loading players…</p>
	{/if}
</div>

<style>
	.sort-header {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: transparent;
		color: inherit;
		font: inherit;
		text-transform: inherit;
		letter-spacing: inherit;
		min-height: 1.8rem;
	}
	.sort-header:hover,
	.sort-header.active {
		color: var(--brand);
	}
	th.num .sort-header {
		justify-content: flex-end;
	}
	.workspace th.sorted {
		background: var(--brand-soft);
		color: var(--brand);
	}
	.workspace td.sorted {
		background: color-mix(in srgb, var(--brand-soft) 40%, var(--surface));
	}

	.mobile-toggle {
		display: none;
	}
	.ownership-filters {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.6rem 1rem;
	}
	.ownership-filters .row {
		gap: 0.25rem;
		flex-wrap: wrap;
	}
	.ownership-filters button.active {
		background: var(--brand-soft);
		color: var(--brand);
		border-color: var(--brand);
	}
	.team-filter {
		position: relative;
		font-size: 0.8rem;
	}
	.team-filter summary {
		cursor: pointer;
		padding: 0.5rem;
	}
	.team-options {
		position: absolute;
		z-index: 5;
		top: 100%;
		right: 0;
		min-width: 14rem;
		max-height: 20rem;
		overflow: auto;
		background: var(--surface);
		border: 1px solid var(--rule);
		border-radius: 8px;
		padding: 0.75rem;
		display: grid;
		gap: 0.6rem;
		box-shadow: 0 8px 24px #0002;
	}
	.workspace table {
		width: 100%;
	}
	.workspace .player {
		gap: 0.45rem;
	}
	.workspace .player-detail {
		font-size: 0.72rem;
		line-height: 1.4;
	}
	.workspace td {
		padding: 0.4rem 0.5rem;
	}
	.workspace .actions {
		position: sticky;
		right: 0;
		background: var(--surface);
	}
	.workspace th {
		white-space: nowrap;
	}
	@media (max-width: 640px) {
		.workspace .raw-stat,
		.workspace .mobile-hide {
			display: none;
		}
		.workspace .show-mobile-stats .raw-stat,
		.workspace .show-mobile-stats .mobile-hide {
			display: table-cell;
		}
		.workspace .mobile-toggle {
			display: flex;
		}
		.workspace .name {
			font-size: 0.8rem;
		}
		.workspace td,
		.workspace th {
			padding: 0.4rem 0.3rem;
		}
		.workspace .player :global(.headshot) {
			display: none;
		}
		.workspace .extra-filter {
			display: none;
		}
		.workspace.more-filters .extra-filter {
			display: initial;
		}
		.workspace.more-filters .ages {
			display: flex;
		}
		.workspace .filters {
			display: grid;
			grid-template-columns: 1fr 1fr;
		}
		.workspace .search {
			grid-column: 1/-1;
			min-width: 0;
		}
		.workspace .ages {
			grid-column: 1/-1;
		}
		.team-options {
			left: 0;
			right: auto;
		}
	}

	.search {
		flex: 1 1 14rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding-left: 0.7rem;
		background: var(--surface);
		border: 1px solid var(--rule-strong);
		border-radius: var(--radius-small);
		color: var(--ink-faint);
	}
	.search:focus-within {
		outline: 2px solid var(--brand);
		outline-offset: 1px;
	}
	.search input {
		flex: 1;
		border: none;
		background: transparent;
		padding-left: 0;
		outline: none;
	}
	.ages {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--ink-soft);
		font-size: 0.85em;
	}
	.ages input {
		width: 4.2rem;
	}

	.panel {
		transition: opacity 0.15s;
	}
	.loading {
		opacity: 0.55;
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.name {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.45rem;
	}
	th.num {
		text-align: right;
	}
	.player-name {
		padding: 0;
		border: 0;
		border-radius: 0;
		background: transparent;
		font-weight: 700;
		text-align: left;
		white-space: normal;
	}
	.player-name:hover {
		color: var(--brand);
		text-decoration: underline;
	}
	tr.selected td {
		background: var(--brand-soft);
	}
	.compact .player {
		gap: 0.45rem;
	}
	.compact .name {
		gap: 0.2rem;
	}
	.compact .small-text {
		font-size: 0.67rem;
		line-height: 1.2;
	}
	.compact .player-name {
		font-size: 0.76rem;
		line-height: 1.2;
	}
	.compact td {
		padding: 0.35rem 0.5rem;
		font-size: 0.8rem;
	}
	.compact th {
		padding: 0.45rem 0.5rem;
		letter-spacing: 0.02em;
	}
	.compact .search {
		flex-basis: 10rem;
	}
	.compact .filters {
		gap: 0.4rem;
	}
	.compact select {
		font-size: 0.8rem;
		padding: 0.4rem;
	}
	.compact .search input {
		font-size: 0.8rem;
		padding-block: 0.4rem;
	}

	@media (max-width: 640px) {
		.wide {
			display: none;
		}
	}
</style>
