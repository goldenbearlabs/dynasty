<script lang="ts">
	// The trade market: your block, what you are looking for, who is after
	// your players and who you are after, then every manager's block and
	// wants, so a trade can start from what the other side said they want.
	import { page } from '$app/state';
	import { getFranchise, type Competition, type Dynasty, type Franchise, type LeagueRoster, type MarketPlayer, type RosterPlayer } from '#lib/api.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import ProspectRank from '#lib/ui/ProspectRank.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';

	let { dynasty, me, rosters, competitions }: { dynasty: Dynasty; me: Franchise | null; rosters: LeagueRoster[]; competitions: Competition[] } = $props();

	const market = $derived(tradeMarket.data);
	const franchise = (id: string) => dynasty.franchises.find((f) => f.id === id);
	const leagueName = (id: string) => dynasty.leagues.find((l) => l.id === id)?.name ?? '';

	// ---- around the league, one sport or all ----
	let sport = $state('');
	const sportTabs = $derived([{ value: '', label: 'All' }, ...dynasty.leagues.map((l) => ({ value: l.competition, label: l.name, sport: l.competition }))]);
	const managers = $derived(
		dynasty.franchises
			.map((f) => ({
				franchise: f,
				block: market.block.filter((b) => b.franchise_id === f.id && (!sport || b.competition === sport)),
				wants: market.wants.filter((w) => w.franchise_id === f.id && (!sport || w.competition === sport))
			}))
			.toSorted((a, b) => Number(b.franchise.id !== me?.id) - Number(a.franchise.id !== me?.id) || b.block.length + b.wants.length - (a.block.length + a.wants.length))
	);

	// ---- picking several players from a roster at once ----
	const pickToggle = (list: string[], id: string) => (list.includes(id) ? list.filter((x) => x !== id) : [...list, id]);

	// ---- your block: adding players ----
	let addLeague = $state('');
	let addPlayers = $state<string[]>([]);
	let addNote = $state('');
	const addable = $derived(
		rosters
			.find((r) => r.league_id === addLeague)
			?.players.filter((p) => p.list !== 'rights' && !tradeMarket.onBlock(addLeague, p.player_id))
			.toSorted((a, b) => a.full_name.localeCompare(b.full_name)) ?? []
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
	const myLeagues = $derived(dynasty.leagues.filter((l) => rosters.some((r) => r.league_id === l.id)));
	let wantLeague = $state('');
	// Opens on a league you have already said something about, if any.
	const wantFor = $derived(
		myLeagues.find((l) => l.id === wantLeague) ??
			myLeagues.find((l) => market.wants.some((w) => w.franchise_id === me?.id && w.league_id === l.id)) ??
			myLeagues[0]
	);
	const myWants = $derived(market.wants.filter((w) => w.franchise_id === me?.id && w.league_id === wantFor?.id));
	// A new want being written.
	let wantPositions = $state<string[]>([]);
	let wantNote = $state('');
	async function addWant() {
		if (!wantFor) return;
		await tradeMarket.addWant(wantFor.id, wantPositions, wantNote);
		wantPositions = [];
		wantNote = '';
	}
	const positionsOf = (competition: string) => competitions.find((c) => c.key === competition)?.positions ?? [];
	const toggle = (p: string) => (wantPositions = wantPositions.includes(p) ? wantPositions.filter((x) => x !== p) : [...wantPositions, p]);

	// ---- interest in your players, grouped by player ----
	const interest = $derived(
		Object.values(
			Object.groupBy(market.interest, (t) => key(t)) as Record<string, typeof market.interest>
		).map((ts) => ({ player: ts[0], by: ts.map((t) => t.by_franchise_id) }))
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
		if (!league || !slug) return;
		let active = true;
		getFranchise(slug).then(
			(detail) => {
				if (active) targetRoster = detail.rosters.find((r) => r.league_id === league)?.players ?? [];
			},
			() => {}
		);
		return () => (active = false);
	});
	const targetable = $derived(
		targetRoster.filter((p) => !tradeMarket.targeted(targetLeague, p.player_id)).toSorted((a, b) => a.full_name.localeCompare(b.full_name))
	);
	async function addTargets() {
		await tradeMarket.targetMany(targetLeague, targetPicks);
		targetPicks = [];
	}
	const proposeTo = (id: string) => `/trades/new?with=${franchise(id)?.slug ?? ''}`;
</script>

{#snippet who(p: MarketPlayer, extra?: string)}
	<Headshot name={p.full_name} src={p.headshot_url} size={30} />
	<span class="who">
		<span><a href="/player/{p.player_id}"><strong>{p.full_name}</strong></a> <ProspectRank id={p.player_id} /></span>
		<span class="muted">
			<SportBadge sport={p.competition} />
			{p.positions.join('/')}{#if p.team_abbrev} · <TeamLogo competition={p.competition} abbrev={p.team_abbrev} size={16} />{/if}{#if p.list === 'reserve'} · reserve{/if}{#if extra} · {extra}{/if}
		</span>
	</span>
{/snippet}

<!-- A roster to tick several players from. -->
{#snippet checklist(players: RosterPlayer[], picked: string[], toggle: (id: string) => void, label: string)}
	{#if players.length}
		<ul class="checklist" aria-label={label}>
			{#each players as p (p.player_id)}
				<li>
					<label class:on={picked.includes(p.player_id)}>
						<input type="checkbox" checked={picked.includes(p.player_id)} onchange={() => toggle(p.player_id)} />
						<span>{p.full_name} <ProspectRank id={p.player_id} /></span>
						<span class="muted">{p.positions.join('/')}{#if p.list !== 'main'} · {p.list}{/if}</span>
					</label>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="muted small-text">Nobody left to pick.</p>
	{/if}
{/snippet}

<div class="market">
	{#if me}
		<section class="mine" aria-label="Your market">
			<!-- ---- your block ---- -->
			<div class="card part">
				<header>
					<h2><Icon name="trade" size={16} /> Your trade block</h2>
					<span class="muted small-text">Players you will listen to offers for. Everyone can see your block.</span>
				</header>
				{#if myBlock.length}
					<ul class="players">
						{#each myBlock as b (key(b))}
							<li>
								{@render who(b)}
								<input
									class="note"
									placeholder="Note: what you want for him"
									value={notes[key(b)] ?? b.note}
									oninput={(e) => (notes[key(b)] = e.currentTarget.value)}
									onchange={(e) => tradeMarket.block(b.league_id, b.player_id, true, e.currentTarget.value, b.full_name)}
								/>
								<button class="quiet small" title="Take him off your block" onclick={() => tradeMarket.block(b.league_id, b.player_id, false, '', b.full_name)}><Icon name="x" size={14} /></button>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="muted small-text">Nobody yet. Add players here, or tap <strong>Trade block</strong> next to anyone on your roster.</p>
				{/if}
				<div class="add">
					<select bind:value={addLeague} aria-label="League" onchange={() => (addPlayers = [])}>
						<option value="">Add players from…</option>
						{#each myLeagues as l (l.id)}<option value={l.id}>{l.name}</option>{/each}
					</select>
					{#if addLeague}
						{@render checklist(addable, addPlayers, (id) => (addPlayers = pickToggle(addPlayers, id)), 'Players to put on the block')}
						<div class="row">
							<input class="grow" placeholder="Note for them all (optional)" bind:value={addNote} />
							<button class="primary small" disabled={!addPlayers.length || tradeMarket.busy} onclick={add}>
								<Icon name="plus" size={14} /> Add {addPlayers.length || ''}
							</button>
						</div>
					{/if}
				</div>
			</div>

			<!-- ---- your wants ---- -->
			{#if wantFor}
				<div class="card part">
					<header>
						<h2><Icon name="search" size={16} /> What you're looking for</h2>
						<span class="muted small-text">As many as you like in each league, each with positions and a note. Every manager can see them.</span>
					</header>
					<div class="row">
						{#each myLeagues as l (l.id)}
							<button class="small league" class:on={l.id === wantFor.id} onclick={() => (wantLeague = l.id)}>
								<SportBadge sport={l.competition} solid={l.id === wantFor.id} />
								{#if market.wants.some((w) => w.franchise_id === me.id && w.league_id === l.id)}<Icon name="check" size={12} />{/if}
							</button>
						{/each}
					</div>
					{#if myWants.length}
						<ul class="wants mine-wants">
							{#each myWants as w (w.id)}
								<li>
									{#each w.positions as p (p)}<span class="chip on small">{p}</span>{/each}
									{#if w.note}<span class="quote">“{w.note}”</span>{/if}
									<button class="quiet small remove" title="Remove this want" disabled={tradeMarket.busy} onclick={() => tradeMarket.removeWant(w.id)}><Icon name="x" size={14} /></button>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="muted small-text">Nothing yet in {wantFor.name}.</p>
					{/if}
					<div class="chips" role="group" aria-label="Positions for a new want in {wantFor.name}">
						{#each positionsOf(wantFor.competition) as p (p)}
							<button class="chip" class:on={wantPositions.includes(p)} aria-pressed={wantPositions.includes(p)} onclick={() => toggle(p)}>{p}</button>
						{/each}
					</div>
					<div class="row">
						<input class="grow" placeholder="e.g. A young goalie, or picks for a veteran" bind:value={wantNote} />
						<button class="primary small" disabled={tradeMarket.busy || (!wantPositions.length && !wantNote.trim())} onclick={addWant}><Icon name="plus" size={14} /> Add want</button>
					</div>
				</div>
			{/if}

			<!-- ---- who is after whom ---- -->
			<div class="card part">
				<header>
					<h2><Icon name="target" size={16} /> Interest in your players</h2>
					<span class="muted small-text">Managers who have marked one of your players as a target.</span>
				</header>
				{#if interest.length}
					<ul class="players">
						{#each interest as { player, by } (key(player))}
							<li>
								{@render who(player, leagueName(player.league_id))}
								<span class="suitors">
									{#each by as id (id)}
										<a class="suitor" href={proposeTo(id)} title="Propose a trade to {franchise(id)?.name}">
											<Crest src={franchise(id)?.image_url} name={franchise(id)?.name ?? ''} size={22} />{franchise(id)?.name}
										</a>
									{/each}
								</span>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="muted small-text">Nobody has targeted your players yet.</p>
				{/if}
			</div>

			<div class="card part">
				<header>
					<h2><Icon name="target" size={16} /> Your targets</h2>
					<span class="muted small-text">Players you want. Only you and their manager can see these. Tap <strong>Target</strong> next to any player.</span>
				</header>
				{#if market.targets.length}
					<ul class="players">
						{#each market.targets as t (key(t))}
							<li>
								{@render who(t, franchise(t.owner_franchise_id)?.name)}
								<a class="button small" href={proposeTo(t.owner_franchise_id)}>Propose trade</a>
								<button class="quiet small" title="Drop the target" onclick={() => tradeMarket.target(t.league_id, t.player_id, false, t.full_name)}><Icon name="x" size={14} /></button>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="muted small-text">No targets yet.</p>
				{/if}
				<div class="add">
					<div class="row">
						<select bind:value={targetLeague} aria-label="League for new targets">
							<option value="">Add targets in…</option>
							{#each dynasty.leagues as l (l.id)}<option value={l.id}>{l.name}</option>{/each}
						</select>
						<select bind:value={targetFrom} aria-label="Manager" disabled={!targetLeague}>
							<option value="">From…</option>
							{#each dynasty.franchises.filter((f) => f.id !== me.id) as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
						</select>
					</div>
					{#if targetLeague && targetFrom}
						{@render checklist(targetable, targetPicks, (id) => (targetPicks = pickToggle(targetPicks, id)), 'Players to target')}
						<button class="primary small" disabled={!targetPicks.length || tradeMarket.busy} onclick={addTargets}>
							<Icon name="target" size={14} /> Target {targetPicks.length || ''}
						</button>
					{/if}
				</div>
			</div>
		</section>
	{/if}

	<!-- ---- around the league ---- -->
	<section class="league-wide" aria-label="Around the league">
		<div class="spread">
			<h2>Around the league</h2>
		</div>
		{#if dynasty.leagues.length > 1}<Tabs tabs={sportTabs} bind:value={sport} label="League" />{/if}
		<div class="managers">
			{#each managers as m (m.franchise.id)}
				<article class="card manager" class:me={m.franchise.id === me?.id}>
					<header class="spread">
						<a class="name" href="/franchise/{m.franchise.slug}"><Crest src={m.franchise.image_url} name={m.franchise.name} size={30} /><strong>{m.franchise.name}</strong></a>
						{#if me && m.franchise.id !== me.id}<a class="button small" href={proposeTo(m.franchise.id)}><Icon name="trade" size={14} /> Propose a trade</a>{/if}
						{#if m.franchise.id === me?.id}<span class="pill brand">You</span>{/if}
					</header>
					<h3>On the block</h3>
					{#if m.block.length}
						<ul class="players">
							{#each m.block as b (key(b))}
								<li class="stacked">
									<span class="line">{@render who(b)}</span>
									{#if b.note}<span class="quote">“{b.note}”</span>{/if}
								</li>
							{/each}
						</ul>
					{:else}
						<p class="muted small-text">Nobody listed.</p>
					{/if}
					<h3>Looking for</h3>
					{#if m.wants.length}
						<ul class="wants">
							{#each m.wants as w (w.id)}
								<li>
									<SportBadge sport={w.competition} />
									{#each w.positions as p (p)}<span class="chip on small">{p}</span>{/each}
									{#if w.note}<span class="quote">“{w.note}”</span>{/if}
								</li>
							{/each}
						</ul>
					{:else}
						<p class="muted small-text">Nothing said yet.</p>
					{/if}
				</article>
			{:else}
				<Empty icon="trade" title="No managers">Managers show up here once the dynasty has them.</Empty>
			{/each}
		</div>
	</section>
</div>

<style>
	.market {
		display: grid;
		gap: 1.5rem;
	}
	.mine {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 24rem), 1fr));
		gap: 0.8rem;
		align-items: start;
	}
	.part {
		display: grid;
		gap: 0.7rem;
	}
	.part header {
		display: grid;
		gap: 0.15rem;
	}
	h2 {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 1.05rem;
	}
	h3 {
		margin-top: 0.4rem;
		font: 700 0.66rem var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-soft);
	}
	.players,
	.wants {
		display: grid;
		gap: 0.1rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.players li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 0.6rem;
		padding: 0.45rem 0;
		border-top: 1px solid var(--rule);
	}
	.players li:first-child {
		border-top: 0;
	}
	.players li.stacked {
		display: grid;
		gap: 0.25rem;
	}
	.line {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.who {
		display: grid;
		flex: 1;
		min-width: 9rem;
		font-size: 0.85rem;
	}
	.who .muted {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.72rem;
	}
	.note {
		flex: 1 1 12rem;
		font-size: 0.8rem;
	}
	.add {
		display: grid;
		gap: 0.4rem;
		padding-top: 0.6rem;
		border-top: 1px solid var(--rule);
	}
	.add select,
	.add input {
		flex: 1 1 8rem;
		font-size: 0.82rem;
	}
	.add .row {
		gap: 0.4rem;
	}
	.checklist {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 13rem), 1fr));
		gap: 0.15rem 0.6rem;
		max-height: 15rem;
		overflow-y: auto;
		margin: 0;
		padding: 0.2rem;
		list-style: none;
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
	}
	.checklist label {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.25rem 0.3rem;
		border-radius: var(--radius-small);
		font-size: 0.8rem;
		cursor: pointer;
	}
	.checklist label.on {
		background: var(--brand-soft);
	}
	.checklist .muted {
		margin-left: auto;
		font-size: 0.7rem;
	}
	.mine-wants .remove {
		margin-left: auto;
		padding: 0.15rem;
	}
	.league {
		padding: 0.2rem 0.35rem;
		gap: 0.2rem;
	}
	.league.on {
		border-color: var(--sport, var(--brand));
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
	}
	.chip {
		padding: 0.2rem 0.55rem;
		border: 1px solid var(--rule-strong);
		border-radius: 999px;
		background: transparent;
		font: 700 0.72rem var(--mono);
		color: var(--ink-soft);
	}
	.chip.on {
		border-color: var(--brand);
		background: var(--brand-soft);
		color: var(--brand);
	}
	.chip.small {
		padding: 0.05rem 0.4rem;
		font-size: 0.65rem;
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.suitors {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
	}
	.suitor {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.15rem 0.5rem 0.15rem 0.2rem;
		border: 1px solid var(--rule);
		border-radius: 999px;
		font-size: 0.75rem;
		color: var(--ink);
	}
	.quote {
		font-size: 0.8rem;
		font-style: italic;
		color: var(--ink-soft);
	}
	.league-wide {
		display: grid;
		gap: 0.7rem;
	}
	.managers {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
		gap: 0.8rem;
		align-items: start;
	}
	.manager {
		display: grid;
		gap: 0.35rem;
	}
	.manager.me {
		border-color: var(--brand);
	}
	.name {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--ink);
	}
	.wants li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem;
		padding: 0.25rem 0;
	}
</style>
