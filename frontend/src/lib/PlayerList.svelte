<script lang="ts">
	// A searchable, paged list of players. Used wherever players are picked
	// from: the player browser, free agency and the draft room.
	import { untrack, type Snippet } from 'svelte';
	import { getPlayers, getStatSeasons, type Player, type PlayerPage, type StatSeason } from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { age } from '#lib/ui/time.ts';

	type Props = {
		/** Show one sport, or every sport when empty. */
		competition?: string;
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
		competition = '',
		leagueId,
		draftId,
		action,
		version = 0,
		byPoints = false,
		statColumns = false,
		compact = false,
		onselect,
		selectedId
	}: Props = $props();

	let status = $state('');
	let search = $state('');
	let query = $state(''); // search, applied after a pause in typing
	let availableOnly = $state(false);
	const usual = () => (!byPoints ? 'name' : competition ? 'points' : 'index');
	let order = $state(untrack(usual));
	let chose = false; // whether the order was picked by hand, and so is kept
	let seasonBack = $state(0);
	let seasons = $state<StatSeason[]>([]);
	if (untrack(() => byPoints)) getStatSeasons().then((s) => (seasons = s), () => {});
	// One sport offers its own seasons by name; several sports can only count back.
	const seasonChoices = $derived.by(() => {
		if (competition) return seasons.filter((s) => s.competition === competition).map((s) => s.label);
		const deepest = Math.max(0, ...Object.values(Object.groupBy(seasons, (s) => s.competition)).map((list) => list!.length));
		return Array.from({ length: Math.min(deepest, 10) }, (_, back) =>
			back === 0 ? 'Latest season' : `${back} ${back === 1 ? 'season' : 'seasons'} back`
		);
	});
	const ranked = $derived(order !== 'name');
	// Whether the season's figures get columns: always in the full list, on request in the compact one.
	const columns = $derived(ranked && (!compact || statColumns));
	const under = (player: Player) =>
		[player.team_abbrev || player.team_name || 'No team', age(player.birth_date) && `age ${age(player.birth_date)}`, !competition && ranked && player.season]
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
			page = 1;
			seasonBack = 0;
			if (!chose) order = usual();
		}
	});

	let latest = 0;
	$effect(() => {
		void version;
		const request = ++latest;
		loading = true;
		getPlayers({
			competition,
			status,
			q: query,
			page,
			draft_id: draftId,
			sort: ranked ? order : undefined,
			season_back: ranked && seasonBack > 0 ? seasonBack : undefined,
			available_in: availableOnly ? leagueId : undefined
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

<div class="stack tight" class:compact>
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
		<select aria-label="Status" bind:value={status} onchange={() => (page = 1)}>
			<option value="">Any status</option>
			<option value="active">Active</option>
			<option value="prospect">Prospects</option>
			<option value="inactive">Inactive</option>
		</select>
		{#if byPoints}
			<select
				aria-label="Order"
				bind:value={order}
				onchange={() => {
					chose = true;
					page = 1;
				}}
			>
				<option value="points">By fantasy points</option>
				<option value="index">By league index</option>
				<option value="name">By name</option>
			</select>
			{#if ranked && seasonChoices.length > 0}
				<select aria-label="Season" bind:value={seasonBack} onchange={() => (page = 1)}>
					{#each seasonChoices as label, back (back)}<option value={back}>{label}</option>{/each}
				</select>
			{/if}
		{/if}
		{#if leagueId && !draftId}
			<label class="check">
				<input type="checkbox" bind:checked={availableOnly} onchange={() => (page = 1)} />
				Available only
			</label>
		{/if}
	</div>

	{#if error}
		<p role="alert">Could not load players: {error}</p>
	{:else if result}
		{#if result.players.length === 0}
			<Empty icon="search" title="No players match">Try a different search, sport or status.</Empty>
		{:else}
			<div class="panel scroll" class:loading>
				<table>
					<thead>
						<tr>
							<th>Player</th>
							<th>Pos</th>
							{#if !compact}<th class="wide">Team</th><th class="wide num">Age</th>{/if}
							{#if columns}<th class="num">Points</th><th class="num" title="100 is the average rostered player; 15 is one standard deviation">Index</th>{/if}
							<th class="num"
								>{result.total.toLocaleString()} {result.total === 1 ? 'player' : 'players'}</th
							>
						</tr>
					</thead>
					<tbody>
						{#each result.players as player (player.id)}
							<tr class:selected={selectedId === player.id}>
								<td>
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
													>{:else}<strong>{player.full_name}</strong>{/if}
												{#if !competition}<SportBadge sport={player.competition} />{/if}
												{#if player.class}<span class="pill">{player.class}</span>{/if}
												{#if player.status === 'prospect'}<span class="pill gold">Prospect</span
													>{/if}
												{#if player.status === 'inactive'}<span class="pill">Inactive</span>{/if}
												{#if player.waiver_until}<span class="pill brand">Waivers</span>{/if}
											</div>
											{#if compact && statColumns}<div class="muted small-text">{under(player)}</div>
											{:else if compact}<div class="muted small-text">
													{player.team_abbrev || player.team_name || 'No team'} · {player.status}
													{#if ranked && player.season}
														· <strong>{fantasyPoints(player.season_points)}</strong> pts
														{#if player.season_index}· <span title="League index: 100 is the average rostered player, 15 is one standard deviation">index <strong>{Math.round(player.season_index)}</strong></span>{/if}
														{#if !competition}· {player.season}{/if}
													{/if}
												</div>{:else if player.note}<div class="muted small-text">
													{player.note}
												</div>{/if}
										</div>
									</div>
								</td>
								<td>{player.positions.join('/')}</td>
								{#if !compact}<td class="wide muted" title={player.team_name}>{player.team_name}</td
									><td class="wide num">{age(player.birth_date)}</td>{/if}
								{#if columns}<td class="num">{player.season ? fantasyPoints(player.season_points) : ''}</td
									><td class="num">{player.season_index ? Math.round(player.season_index) : ''}</td>{/if}
								<td class="actions">
									{#if player.owner_slug}
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
