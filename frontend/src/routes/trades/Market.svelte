<script lang="ts">
	// Browse league listings first, with focused views for managing your
	// block, team needs, targets, and interest in your own players.
	import {
		getFranchise,
		type Competition,
		type Dynasty,
		type Franchise,
		type LeagueRoster,
		type MarketPlayer,
		type RosterPlayer,
	} from '#lib/api.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';

	let {
		dynasty,
		me,
		rosters,
		competitions,
	}: {
		dynasty: Dynasty;
		me: Franchise | null;
		rosters: LeagueRoster[];
		competitions: Competition[];
	} = $props();

	const market = $derived(tradeMarket.data);
	const franchise = (id: string) => dynasty.franchises.find((f) => f.id === id);

	// ---- around the league, one sport or all ----
	let sport = $state('');
	let section = $state('league');
	let search = $state('');
	let manager = $state('');
	let showEmpty = $state(false);
	let pickSearch = $state('');
	let targetError = $state('');
	let targetLoading = $state(false);
	$effect(() => {
		void section;
		pickSearch = '';
	});
	const sportTabs = $derived([
		{ value: '', label: 'All' },
		...dynasty.leagues.map((l) => ({ value: l.competition, label: l.name, sport: l.competition })),
	]);
	const managers = $derived(
		dynasty.franchises
			.map((f) => ({
				franchise: f,
				block: market.block.filter(
					(b) => b.franchise_id === f.id && (!sport || b.competition === sport),
				),
				wants: market.wants.filter(
					(w) => w.franchise_id === f.id && (!sport || w.competition === sport),
				),
			}))
			.toSorted(
				(a, b) =>
					Number(b.franchise.id !== me?.id) - Number(a.franchise.id !== me?.id) ||
					b.block.length + b.wants.length - (a.block.length + a.wants.length),
			),
	);

	// ---- picking several players from a roster at once ----
	const pickToggle = (list: string[], id: string) =>
		list.includes(id) ? list.filter((x) => x !== id) : [...list, id];

	// ---- your block: adding players ----
	let addLeague = $state('');
	let addPlayers = $state<string[]>([]);
	let addNote = $state('');
	const addable = $derived(
		rosters
			.find((r) => r.league_id === addLeague)
			?.players.filter((p) => p.list !== 'rights' && !tradeMarket.onBlock(addLeague, p.player_id))
			.toSorted((a, b) => a.full_name.localeCompare(b.full_name)) ?? [],
	);
	async function add() {
		await tradeMarket.blockMany(addLeague, addPlayers, addNote);
		addPlayers = [];
		addNote = '';
	}
	const myBlock = $derived(market.block.filter((b) => b.franchise_id === me?.id));
	// Notes being edited, by league and player.
	let notes = $state<Record<string, string>>({});
	const key = (p: { league_id: string; player_id: string }) => `${p.league_id}/${p.player_id}`;

	// ---- your wants, a league at a time ----
	const myLeagues = $derived(
		dynasty.leagues.filter((l) => rosters.some((r) => r.league_id === l.id)),
	);
	let wantLeague = $state('');
	// Opens on a league you have already said something about, if any.
	const wantFor = $derived(
		myLeagues.find((l) => l.id === wantLeague) ??
			myLeagues.find((l) =>
				market.wants.some((w) => w.franchise_id === me?.id && w.league_id === l.id),
			) ??
			myLeagues[0],
	);
	const myWants = $derived(
		market.wants.filter((w) => w.franchise_id === me?.id && w.league_id === wantFor?.id),
	);
	// A new want being written.
	let wantPositions = $state<string[]>([]);
	let wantNote = $state('');
	async function addWant() {
		if (!wantFor) return;
		await tradeMarket.addWant(wantFor.id, wantPositions, wantNote);
		wantPositions = [];
		wantNote = '';
	}
	const positionsOf = (competition: string) =>
		competitions.find((c) => c.key === competition)?.positions ?? [];
	const toggle = (p: string) =>
		(wantPositions = wantPositions.includes(p)
			? wantPositions.filter((x) => x !== p)
			: [...wantPositions, p]);

	// ---- interest in your players, grouped by player ----
	const interest = $derived(
		Object.values(
			Object.groupBy(market.interest, (t) => key(t)) as Record<string, typeof market.interest>,
		).map((ts) => ({ player: ts[0], by: ts.map((t) => t.by_franchise_id) })),
	);
	// ---- your targets: several of one manager's players at once ----
	let targetLeague = $state('');
	let targetFrom = $state('');
	let targetPicks = $state<string[]>([]);
	let targetRoster = $state<RosterPlayer[]>([]);
	$effect(() => {
		const league = targetLeague;
		const slug = franchise(targetFrom)?.slug;
		targetPicks = [];
		targetRoster = [];
		targetError = '';
		targetLoading = false;
		if (!league || !slug) return;
		let active = true;
		targetLoading = true;
		getFranchise(slug)
			.then(
				(detail) => {
					if (active)
						targetRoster = detail.rosters.find((r) => r.league_id === league)?.players ?? [];
				},
				() => {
					if (active) targetError = 'Could not load this roster. Choose the team again to retry.';
				},
			)
			.finally(() => {
				if (active) targetLoading = false;
			});
		return () => (active = false);
	});
	const targetable = $derived(
		targetRoster
			.filter((p) => !tradeMarket.targeted(targetLeague, p.player_id))
			.toSorted((a, b) => a.full_name.localeCompare(b.full_name)),
	);
	async function addTargets() {
		await tradeMarket.targetMany(targetLeague, targetPicks);
		targetPicks = [];
	}
	const visiblePicks = (players: RosterPlayer[]) =>
		players.filter(
			(p) =>
				!pickSearch.trim() || p.full_name.toLowerCase().includes(pickSearch.trim().toLowerCase()),
		);
	const proposeTo = (id: string) => `/trades/new?with=${franchise(id)?.slug ?? ''}`;
	const sections = $derived([
		{
			value: 'league',
			label: 'League block',
			short: 'League',
			icon: 'players' as const,
			count: market.block.length,
		},
		{
			value: 'block',
			label: 'My block',
			short: 'My block',
			icon: 'trade' as const,
			count: myBlock.length,
		},
		{
			value: 'needs',
			label: 'Team needs',
			short: 'Needs',
			icon: 'search' as const,
			count: market.wants.filter((w) => w.franchise_id === me?.id).length,
		},
		{
			value: 'targets',
			label: 'My targets',
			short: 'Targets',
			icon: 'target' as const,
			count: market.targets.length,
		},
		{
			value: 'interest',
			label: 'Interest',
			short: 'Interest',
			icon: 'bell' as const,
			count: interest.length,
		},
	]);
	const needle = $derived(search.trim().toLowerCase());
	const matchingManagers = $derived(
		managers.filter(
			(m) =>
				(!manager || m.franchise.id === manager) &&
				(showEmpty || m.block.length || m.wants.length) &&
				(!needle ||
					[
						m.franchise.name,
						...m.block.flatMap((b) => [b.full_name, b.note, ...b.positions]),
						...m.wants.flatMap((w) => [w.note, ...w.positions]),
					].some((v) => v.toLowerCase().includes(needle))),
		),
	);
	const tradeFor = (p: MarketPlayer, owner: string) => `${proposeTo(owner)}&player=${p.player_id}`;
</script>

{#snippet who(p: MarketPlayer, extra = '')}
	<div class="player-identity">
		<Headshot name={p.full_name} src={p.headshot_url} size={34} />
		<div class="who">
			<div class="player-name">
				<a href="/player/{p.player_id}">{p.full_name}</a><ProspectRank id={p.player_id} />
			</div>
			<div class="player-meta">
				<SportBadge sport={p.competition} /><span>{p.positions.join('/')}</span
				>{#if p.team_abbrev}<TeamLogo
						competition={p.competition}
						abbrev={p.team_abbrev}
						size={16}
					/>{/if}{#if p.list === 'reserve'}<span>Reserve</span>{/if}{#if extra}<span>{extra}</span
					>{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet checklist(
	players: RosterPlayer[],
	picked: string[],
	togglePick: (id: string) => void,
	label: string,
)}
	<label class="search-field"
		><Icon name="search" size={15} /><input
			type="search"
			aria-label="Search {label.toLowerCase()}"
			placeholder="Find a player…"
			bind:value={pickSearch}
		/></label
	>
	<ul class="checklist" aria-label={label}>
		{#each visiblePicks(players) as p (p.player_id)}<li>
				<label class:on={picked.includes(p.player_id)}
					><input
						type="checkbox"
						checked={picked.includes(p.player_id)}
						onchange={() => togglePick(p.player_id)}
					/><span class="pick-name">{p.full_name}<ProspectRank id={p.player_id} /></span><span
						class="muted pick-position"
						>{p.positions.join('/')}{#if p.list !== 'main'}
							· {p.list}{/if}</span
					></label
				>
			</li>
		{:else}<li class="empty-picks">
				{players.length ? 'No matching players.' : 'No eligible players left to select.'}
			</li>{/each}
	</ul>
{/snippet}

<div class="market">
	<nav class="market-nav" aria-label="Trade market sections">
		{#each sections as tab (tab.value)}<button
				class:active={section === tab.value}
				class:attention={tab.value === 'interest' && tab.count > 0}
				aria-pressed={section === tab.value}
				onclick={() => (section = tab.value)}
				><Icon name={tab.icon} size={16} /><span class="desktop-label">{tab.label}</span><span
					class="mobile-label">{tab.short}</span
				>{#if tab.count}<span class="nav-count">{tab.count}</span>{/if}</button
			>{/each}
	</nav>

	{#if !me}<Empty icon="trade" title="Find your next trade"
			><a href="/">Sign in</a> to browse blocks, share team needs, and manage your targets.</Empty
		>
	{:else if section === 'league'}
		<section class="league-wide" aria-label="Around the league">
			<header class="section-heading">
				<div>
					<h2>Around the league</h2>
					<p>See who’s available and what each team wants in return.</p>
				</div>
				<span class="muted result-count"
					>{matchingManagers.length} {matchingManagers.length === 1 ? 'team' : 'teams'}</span
				>
			</header>
			<div class="market-filters">
				<label class="search-field"
					><Icon name="search" size={16} /><input
						type="search"
						bind:value={search}
						placeholder="Search players, teams, or needs"
						aria-label="Search trade market"
					/></label
				>
				<select bind:value={manager} aria-label="Filter by fantasy team"
					><option value="">All teams</option>{#each dynasty.franchises as f (f.id)}<option
							value={f.id}>{f.name}</option
						>{/each}</select
				>
				<label class="check"
					><input type="checkbox" bind:checked={showEmpty} />Include inactive teams</label
				>
			</div>
			{#if dynasty.leagues.length > 1}<Tabs
					tabs={sportTabs}
					bind:value={sport}
					label="Trade market sport"
				/>{/if}
			{#if !tradeMarket.loaded}<div class="spread">
					<p class="muted" role="status">Loading the trade market…</p>
					<button class="small quiet" onclick={() => tradeMarket.refresh()}>Retry</button>
				</div>
			{:else if matchingManagers.length}
				<div class="managers">
					{#each matchingManagers as m (m.franchise.id)}
						<article class="manager-card" aria-label="{m.franchise.name} trade block">
							<header class="manager-heading">
								<a class="team-name" href="/franchise/{m.franchise.slug}"
									><Crest src={m.franchise.image_url} name={m.franchise.name} size={36} /><span
										><strong>{m.franchise.name}</strong><small
											>{m.block.length} on the block{m.franchise.id === me.id
												? ' · Your team'
												: ''}</small
										></span
									></a
								>
								{#if m.franchise.id !== me.id}<a
										class="button small quiet"
										href={proposeTo(m.franchise.id)}><Icon name="trade" size={15} />Trade</a
									>{:else}<button class="small quiet" onclick={() => (section = 'block')}
										>Manage</button
									>{/if}
							</header>
							<div class="card-section">
								<h3>On the block <span>{m.block.length}</span></h3>
								{#if m.block.length}<ul class="player-list">
										{#each m.block as b (key(b))}<li class="league-player">
												<div class="player-line">
													{@render who(b)}{#if m.franchise.id !== me.id}<button
															class="icon-button target-button"
															class:selected={tradeMarket.targeted(b.league_id, b.player_id)}
															aria-label="{tradeMarket.targeted(b.league_id, b.player_id)
																? 'Remove target'
																: 'Target'} {b.full_name}"
															aria-pressed={tradeMarket.targeted(b.league_id, b.player_id)}
															disabled={tradeMarket.busy}
															title={tradeMarket.targeted(b.league_id, b.player_id)
																? 'Remove target'
																: 'Add to your targets'}
															onclick={() =>
																tradeMarket.target(
																	b.league_id,
																	b.player_id,
																	!tradeMarket.targeted(b.league_id, b.player_id),
																	b.full_name,
																)}><Icon name="target" size={17} /></button
														>{/if}
												</div>
												{#if b.note}<p class="player-note">{b.note}</p>{/if}
											</li>{/each}
									</ul>
								{:else}<p class="empty-copy">No players listed.</p>{/if}
							</div>
							<div class="card-section needs-section">
								<h3>Looking for</h3>
								{#if m.wants.length}<ul class="needs-list">
										{#each m.wants as w (w.id)}<li>
												<div class="need-tags">
													<SportBadge sport={w.competition} />{#each w.positions as p (p)}<span
															class="position-chip">{p}</span
														>{/each}
												</div>
												{#if w.note}<p>{w.note}</p>{/if}
											</li>{/each}
									</ul>{:else}<p class="empty-copy">No needs shared yet.</p>{/if}
							</div>
						</article>
					{/each}
				</div>
			{:else}<div class="empty-state">
					<Icon name="search" size={26} />
					<h3>{search || manager || sport ? 'No matching teams' : 'The block is quiet'}</h3>
					<p>
						{search || manager || sport
							? 'Try another search, team, or sport.'
							: 'List a player or share what your team needs to get conversations started.'}
					</p>
					<button
						class="small"
						onclick={() => {
							if (search || manager || sport) {
								search = '';
								manager = '';
								sport = '';
							} else section = 'block';
						}}>{search || manager || sport ? 'Clear filters' : 'Manage my block'}</button
					>
				</div>{/if}
		</section>
	{:else if section === 'block'}
		<section class="personal-section" aria-label="Your trade block">
			<header class="section-heading">
				<div>
					<h2>Your trade block</h2>
					<p>Let the league know who you’re open to moving.</p>
				</div>
				<span class="visibility"><Icon name="players" size={13} />Visible to the league</span>
			</header>
			<details class="composer">
				<summary
					><span><Icon name="plus" size={16} />List players</span><Icon
						name="down"
						size={15}
					/></summary
				>
				<div class="composer-body">
					<label class="field"
						>Sport<select
							aria-label="Sport for new listings" bind:value={addLeague}
							onchange={() => {
								addPlayers = [];
								pickSearch = '';
							}}
							><option value="">Choose a sport…</option>{#each myLeagues as l (l.id)}<option
									value={l.id}>{l.name}</option
								>{/each}</select
						></label
					>{#if addLeague}{@render checklist(
							addable,
							addPlayers,
							(id) => (addPlayers = pickToggle(addPlayers, id)),
							'Players to put on the block',
						)}<label class="field"
							>What would you like in return?<input
								bind:value={addNote}
								maxlength="280"
								placeholder="Optional note for these players"
							/></label
						>
						<div class="composer-footer">
							<span>{addPlayers.length} selected</span><button
								class="primary small"
								disabled={!addPlayers.length || tradeMarket.busy}
								onclick={add}
								>List {addPlayers.length || ''}
								{addPlayers.length === 1 ? 'player' : 'players'}</button
							>
						</div>{/if}
				</div>
			</details>
			{#if myBlock.length}<ul class="personal-list">
					{#each myBlock as b (key(b))}<li class="owned-player">
							<div class="player-line">
								{@render who(b)}<button
									class="icon-button"
									aria-label="Remove {b.full_name} from your block"
									title="Remove from block"
									disabled={tradeMarket.busy}
									onclick={() =>
										tradeMarket.block(b.league_id, b.player_id, false, '', b.full_name)}
									><Icon name="x" size={16} /></button
								>
							</div>
							<label class="listing-note"
								><span>Asking for</span><input
									aria-label="Trade note for {b.full_name}"
									placeholder="Add a note about what you want in return"
									value={notes[key(b)] ?? b.note}
									maxlength="280"
									oninput={(e) => (notes[key(b)] = e.currentTarget.value)}
									onchange={(e) =>
										tradeMarket.block(
											b.league_id,
											b.player_id,
											true,
											e.currentTarget.value,
											b.full_name,
										)}
								/></label
							>
						</li>{/each}
				</ul>{:else}<div class="empty-state">
					<Icon name="trade" size={26} />
					<h3>Your block is empty</h3>
					<p>Use “List players” above to make players available for trade.</p>
				</div>{/if}
		</section>
	{:else if section === 'needs'}
		<section class="personal-section" aria-label="Your team needs">
			<header class="section-heading">
				<div>
					<h2>What your team needs</h2>
					<p>Share positions, player types, or picks you’re looking for.</p>
				</div>
				<span class="visibility"><Icon name="players" size={13} />Visible to the league</span>
			</header>
			{#if wantFor}<div class="league-switch" role="group" aria-label="Sport for team needs">
					{#each myLeagues as l (l.id)}<button
							class="small"
							class:active={l.id === wantFor.id}
							aria-pressed={l.id === wantFor.id}
							onclick={() => {
								wantLeague = l.id;
								wantPositions = [];
								wantNote = '';
							}}
							><SportBadge
								sport={l.competition}
							/>{#if market.wants.some((w) => w.franchise_id === me.id && w.league_id === l.id)}<Icon
									name="check"
									size={12}
								/>{/if}</button
						>{/each}
				</div>
				<div class="needs-editor">
					<h3>Add a need for {wantFor.name}</h3>
					<div class="chips" role="group" aria-label="Positions for a new want in {wantFor.name}">
						{#each positionsOf(wantFor.competition) as p (p)}<button
								class="chip"
								class:on={wantPositions.includes(p)}
								aria-pressed={wantPositions.includes(p)}
								onclick={() => toggle(p)}>{p}</button
							>{/each}
					</div>
					<label class="field"
						>Details<input
							bind:value={wantNote}
							maxlength="280"
							placeholder="e.g. A young goalie, or picks for a veteran"
						/></label
					>
					<div class="composer-footer">
						<span>Positions, a note, or both.</span><button
							class="primary small"
							disabled={tradeMarket.busy || (!wantPositions.length && !wantNote.trim())}
							onclick={addWant}><Icon name="plus" size={14} />Share need</button
						>
					</div>
				</div>
				{#if myWants.length}<ul class="saved-needs">
						{#each myWants as w (w.id)}<li>
								<div>
									<div class="need-tags">
										{#each w.positions as p (p)}<span class="position-chip">{p}</span>{/each}
									</div>
									{#if w.note}<p>{w.note}</p>{/if}
								</div>
								<button
									class="icon-button"
									aria-label="Remove need {w.note || w.positions.join('/')}"
									disabled={tradeMarket.busy}
									onclick={() => tradeMarket.removeWant(w.id)}><Icon name="x" size={16} /></button
								>
							</li>{/each}
					</ul>{:else}<p class="empty-copy">No needs shared for {wantFor.name} yet.</p>{/if}
			{:else}<p class="empty-copy">
					Your sport rosters will appear here once your league is set up.
				</p>{/if}
		</section>
	{:else if section === 'targets'}
		<section class="personal-section" aria-label="Your targets">
			<header class="section-heading">
				<div>
					<h2>Your targets</h2>
					<p>Players you’d like to bring to your team.</p>
				</div>
				<span class="visibility"><Icon name="lock" size={13} />You and their manager</span>
			</header>
			<details class="composer">
				<summary
					><span><Icon name="plus" size={16} />Add targets</span><Icon
						name="down"
						size={15}
					/></summary
				>
				<div class="composer-body">
					<div class="form-grid">
						<label class="field"
							>Sport<select aria-label="Sport for targets" bind:value={targetLeague}
								><option value="">Choose a sport…</option>{#each dynasty.leagues as l (l.id)}<option
										value={l.id}>{l.name}</option
									>{/each}</select
							></label
						><label class="field"
							>Team<select aria-label="Team for targets" bind:value={targetFrom} disabled={!targetLeague}
								><option value="">Choose a team…</option
								>{#each dynasty.franchises.filter((f) => f.id !== me.id) as f (f.id)}<option
										value={f.id}>{f.name}</option
									>{/each}</select
							></label
						>
					</div>
					{#if targetLoading}<p class="empty-copy" role="status">
							Loading roster…
						</p>{:else if targetError}<p role="alert">
							{targetError}
						</p>{:else if targetLeague && targetFrom}{@render checklist(
							targetable,
							targetPicks,
							(id) => (targetPicks = pickToggle(targetPicks, id)),
							'Players to target',
						)}
						<div class="composer-footer">
							<span>{targetPicks.length} selected</span><button
								class="primary small"
								disabled={!targetPicks.length || tradeMarket.busy}
								onclick={addTargets}
								>Target {targetPicks.length || ''}
								{targetPicks.length === 1 ? 'player' : 'players'}</button
							>
						</div>{/if}
				</div>
			</details>
			{#if market.targets.length}<ul class="personal-list">
					{#each market.targets as t (key(t))}<li class="target-row">
							{@render who(t, franchise(t.owner_franchise_id)?.name)}
							<div class="row-actions">
								<a class="button small" href={tradeFor(t, t.owner_franchise_id)}
									><Icon name="trade" size={14} />Propose trade</a
								><button
									class="icon-button"
									aria-label="Remove target {t.full_name}"
									disabled={tradeMarket.busy}
									onclick={() => tradeMarket.target(t.league_id, t.player_id, false, t.full_name)}
									><Icon name="x" size={16} /></button
								>
							</div>
						</li>{/each}
				</ul>{:else}<div class="empty-state">
					<Icon name="target" size={26} />
					<h3>No targets yet</h3>
					<p>Use the target icon on a league listing, or pick players with “Add targets”.</p>
				</div>{/if}
		</section>
	{:else if section === 'interest'}
		<section class="personal-section" aria-label="Interest in your players">
			<header class="section-heading">
				<div>
					<h2>Interest in your players</h2>
					<p>Start a conversation with teams targeting your players.</p>
				</div>
				<span class="visibility"><Icon name="lock" size={13} />Only shown to you</span>
			</header>
			{#if interest.length}<ul class="personal-list">
					{#each interest as { player, by } (key(player))}<li class="interest-row">
							{@render who(player)}
							<div class="suitors">
								<span class="interest-label">Interested teams</span>{#each by as id (id)}<a
										class="suitor"
										href={proposeTo(id)}
										title="Propose a trade to {franchise(id)?.name}"
										><Crest
											src={franchise(id)?.image_url}
											name={franchise(id)?.name ?? ''}
											size={22}
										/><span>{franchise(id)?.name}</span><Icon name="trade" size={14} /></a
									>{/each}
							</div>
						</li>{/each}
				</ul>{:else}<div class="empty-state">
					<Icon name="target" size={26} />
					<h3>No interest yet</h3>
					<p>When another manager targets one of your players, you’ll see their team here.</p>
				</div>{/if}
		</section>
	{/if}
</div>

<style>
	.market {
		display: grid;
		gap: 1.2rem;
		min-width: 0;
	}
	.market-nav {
		display: flex;
		gap: 0.3rem;
		padding: 0.3rem;
		border: 1px solid var(--rule);
		background: var(--surface);
		border-radius: 11px;
		overflow-x: auto;
		scrollbar-width: thin;
	}
	.market-nav button {
		display: flex;
		flex: 1 0 auto;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0.65rem 0.75rem;
		border: 0;
		border-radius: 7px;
		color: var(--ink-soft);
		background: transparent;
		font-size: 0.82rem;
	}
	.market-nav button.active {
		background: var(--brand-soft);
		color: var(--brand);
		font-weight: 650;
	}
	.nav-count {
		min-width: 1.2rem;
		padding: 0.1rem 0.3rem;
		background: var(--surface-2);
		border-radius: 5px;
		font: 650 0.65rem var(--mono);
	}
	.attention .nav-count {
		background: var(--bad);
		color: white;
	}
	.mobile-label {
		display: none;
	}
	.section-heading {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.6rem 1rem;
	}
	h2 {
		font-size: 1.15rem;
		letter-spacing: -0.025em;
	}
	.section-heading p {
		font-size: 0.82rem;
		color: var(--ink-soft);
		margin-top: 0.3rem;
	}
	.visibility,
	.result-count {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--ink-faint);
		font-size: 0.72rem;
	}
	.league-wide,
	.personal-section {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	.market-filters {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.6rem;
	}
	.search-field {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		border: 1px solid var(--rule-strong);
		border-radius: 8px;
		background: var(--surface);
		color: var(--ink-faint);
		padding-left: 0.7rem;
		flex: 1 1 15rem;
		min-width: 0;
	}
	.search-field:focus-within {
		outline: 2px solid var(--brand);
		outline-offset: 1px;
	}
	.search-field input {
		border: 0;
		background: transparent;
		padding-left: 0;
		min-width: 0;
		width: 100%;
		outline: none;
		font-size: 0.82rem;
	}
	.market-filters select {
		font-size: 0.82rem;
	}
	.market-filters .check {
		font-size: 0.75rem;
		color: var(--ink-soft);
	}
	.managers {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		align-items: start;
	}
	.manager-card {
		background: var(--surface);
		border: 1px solid var(--rule);
		border-radius: 12px;
		overflow: hidden;
		min-width: 0;
	}
	.manager-heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.7rem;
		padding: 1rem;
		background: var(--surface-2);
		border-bottom: 1px solid var(--rule);
	}
	.team-name {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		color: var(--ink);
		min-width: 0;
	}
	.team-name > span {
		display: grid;
		gap: 0.2rem;
		min-width: 0;
	}
	.team-name strong {
		font-size: 0.9rem;
		overflow-wrap: anywhere;
	}
	.team-name small {
		color: var(--ink-faint);
		font-size: 0.7rem;
	}
	.card-section {
		padding: 1rem;
	}
	h3 {
		font-size: 0.7rem;
		font-weight: 650;
		color: var(--ink-soft);
		text-transform: uppercase;
		letter-spacing: 0.055em;
	}
	h3 > span {
		margin-left: 0.35rem;
		color: var(--ink-faint);
	}
	.player-list,
	.personal-list,
	.needs-list,
	.saved-needs {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.player-list {
		margin-top: 0.4rem;
	}
	.league-player {
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--rule);
	}
	.league-player:last-child {
		padding-bottom: 0;
		border-bottom: 0;
	}
	.player-line {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.6rem;
		min-width: 0;
	}
	.player-identity {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		min-width: 0;
	}
	.who {
		min-width: 0;
		display: grid;
		gap: 0.3rem;
	}
	.player-name {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.35rem;
	}
	.player-name > a {
		font-weight: 650;
		font-size: 0.85rem;
		color: var(--ink);
		overflow-wrap: anywhere;
	}
	.player-name > a:hover {
		color: var(--brand);
	}
	.player-meta {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.25rem 0.45rem;
		color: var(--ink-faint);
		font-size: 0.7rem;
	}
	.player-note {
		margin: 0.55rem 0 0 2.7rem;
		padding: 0.45rem 0.65rem;
		border-left: 2px solid var(--rule-strong);
		background: var(--surface-2);
		color: var(--ink-soft);
		font-size: 0.78rem;
		line-height: 1.5;
		overflow-wrap: anywhere;
	}
	.icon-button {
		display: inline-grid;
		place-items: center;
		flex-shrink: 0;
		width: 2.25rem;
		height: 2.25rem;
		padding: 0;
		border: 1px solid var(--rule);
		background: transparent;
		border-radius: 8px;
		color: var(--ink-soft);
	}
	.target-button.selected {
		color: var(--brand);
		border-color: var(--brand);
		background: var(--brand-soft);
	}
	.needs-section {
		border-top: 1px solid var(--rule);
		background: color-mix(in srgb, var(--surface-2) 40%, var(--surface));
	}
	.needs-list {
		margin-top: 0.6rem;
		display: grid;
		gap: 0.75rem;
	}
	.need-tags {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem;
	}
	.needs-list p,
	.saved-needs p {
		font-size: 0.8rem;
		margin-top: 0.35rem;
		line-height: 1.5;
		color: var(--ink-soft);
		overflow-wrap: anywhere;
	}
	.position-chip {
		padding: 0.16rem 0.4rem;
		border: 1px solid var(--rule);
		border-radius: 5px;
		background: var(--surface);
		color: var(--ink-soft);
		font: 600 0.65rem var(--mono);
	}
	.empty-copy {
		margin-top: 0.5rem;
		font-size: 0.8rem;
		color: var(--ink-faint);
		line-height: 1.5;
	}
	.empty-state {
		display: grid;
		justify-items: center;
		text-align: center;
		gap: 0.7rem;
		padding: 2.5rem 1rem;
		border: 1px dashed var(--rule-strong);
		border-radius: 12px;
		color: var(--ink-faint);
	}
	.empty-state h3 {
		color: var(--ink);
		text-transform: none;
		letter-spacing: 0;
		font-size: 1rem;
	}
	.empty-state p {
		max-width: 28rem;
		font-size: 0.85rem;
		line-height: 1.6;
	}
	.personal-section {
		max-width: 58rem;
		width: 100%;
	}
	.composer,
	.needs-editor {
		border: 1px solid var(--rule);
		background: var(--surface);
		border-radius: 10px;
	}
	.composer summary {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.85rem 1rem;
		color: var(--brand);
		font-size: 0.85rem;
		font-weight: 650;
		cursor: pointer;
		list-style: none;
	}
	.composer summary::-webkit-details-marker {
		display: none;
	}
	.composer summary > span {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.composer[open] summary {
		border-bottom: 1px solid var(--rule);
	}
	.composer-body,
	.needs-editor {
		display: grid;
		gap: 0.8rem;
		padding: 1rem;
	}
	.field {
		display: grid;
		gap: 0.4rem;
		font-size: 0.75rem;
		color: var(--ink-soft);
	}
	.field input,
	.field select {
		min-width: 0;
		width: 100%;
		font-size: 0.82rem;
	}
	.composer-footer {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: center;
		gap: 0.6rem;
	}
	.composer-footer > span {
		font-size: 0.75rem;
		color: var(--ink-faint);
	}
	.personal-list {
		border: 1px solid var(--rule);
		border-radius: 10px;
		background: var(--surface);
		overflow: hidden;
	}
	.personal-list > li {
		padding: 1rem;
		border-bottom: 1px solid var(--rule);
	}
	.personal-list > li:last-child {
		border-bottom: 0;
	}
	.listing-note {
		display: grid;
		grid-template-columns: 4.5rem minmax(0, 1fr);
		align-items: center;
		gap: 0.6rem;
		margin: 0.7rem 0 0 2.7rem;
	}
	.listing-note > span {
		color: var(--ink-faint);
		font-size: 0.72rem;
	}
	.listing-note input {
		min-width: 0;
		font-size: 0.8rem;
		background: var(--surface-2);
	}
	.league-switch {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
	}
	.league-switch button {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.4rem 0.6rem;
		background: var(--surface);
	}
	.league-switch button.active {
		border-color: var(--brand);
		background: var(--brand-soft);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.chip {
		padding: 0.4rem 0.65rem;
		border: 1px solid var(--rule-strong);
		border-radius: 7px;
		background: var(--surface);
		color: var(--ink-soft);
		font-size: 0.75rem;
	}
	.chip.on {
		background: var(--brand-soft);
		border-color: var(--brand);
		color: var(--brand);
	}
	.saved-needs {
		display: grid;
		gap: 0.65rem;
	}
	.saved-needs > li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.7rem;
		padding: 1rem;
		background: var(--surface);
		border: 1px solid var(--rule);
		border-radius: 10px;
	}
	.form-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.7rem;
	}
	.target-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.row-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
	}
	.interest-row {
		display: grid;
		gap: 0.8rem;
	}
	.suitors {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		padding-left: 2.7rem;
	}
	.interest-label {
		width: 100%;
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--ink-faint);
	}
	.suitor {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.3rem 0.55rem;
		border: 1px solid var(--rule);
		border-radius: 7px;
		color: var(--ink);
		font-size: 0.78rem;
	}
	.suitor:hover {
		background: var(--brand-soft);
	}
	.checklist {
		list-style: none;
		margin: 0;
		padding: 0.25rem;
		max-height: 18rem;
		overflow: auto;
		border: 1px solid var(--rule);
		border-radius: 8px;
	}
	.checklist label {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		padding: 0.65rem 0.5rem;
		cursor: pointer;
		border-radius: 5px;
	}
	.checklist label.on {
		background: var(--brand-soft);
	}
	.pick-name {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.82rem;
	}
	.pick-position {
		font-size: 0.7rem;
	}
	.empty-picks {
		padding: 1rem;
		font-size: 0.8rem;
		color: var(--ink-faint);
	}
	@media (max-width: 1000px) {
		.managers {
			grid-template-columns: 1fr;
		}
	}
	@media (max-width: 640px) {
		.market {
			gap: 1rem;
		}
		.market-nav {
			gap: 0.15rem;
			padding: 0.25rem;
		}
		.market-nav button {
			padding: 0.6rem 0.35rem;
			gap: 0.2rem;
			font-size: 0.7rem;
		}
		.market-nav button :global(svg),
		.desktop-label {
			display: none;
		}
		.mobile-label {
			display: inline;
		}
		.nav-count {
			font-size: 0.6rem;
			min-width: 1rem;
			padding: 0.1rem 0.2rem;
		}
		h2 {
			font-size: 1.05rem;
		}
		.section-heading p {
			font-size: 0.78rem;
			line-height: 1.5;
		}
		.market-filters {
			gap: 0.6rem;
		}
		.market-filters .search-field {
			flex-basis: 100%;
		}
		.market-filters select {
			flex: 1;
			min-width: 0;
		}
		.market-filters .check {
			font-size: 0.7rem;
		}
		.manager-heading,
		.card-section,
		.personal-list > li,
		.composer-body,
		.needs-editor {
			padding: 0.8rem;
		}
		.manager-heading .button {
			padding: 0.4rem 0.6rem;
		}
		.target-row {
			flex-direction: column;
			align-items: stretch;
			gap: 0.8rem;
		}
		.row-actions {
			justify-content: flex-end;
		}
		.listing-note {
			grid-template-columns: 1fr;
			gap: 0.35rem;
			margin-left: 0;
		}
		.player-note {
			margin-left: 0;
		}
		.suitors {
			padding-left: 0;
		}
		.icon-button {
			width: 2.75rem;
			height: 2.75rem;
		}
		.form-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
