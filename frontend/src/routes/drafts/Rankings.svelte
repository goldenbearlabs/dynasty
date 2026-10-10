<script lang="ts">
	// A manager's pre-draft rankings: private ranked lists of players, one
	// league each, which can be loaded into the queue once the draft is on.
	import {
		createRanking,
		deleteRanking,
		getRanking,
		getRankings,
		updateRanking,
		type Competition,
		type DraftSummary,
		type Dynasty,
		type Player,
		type Ranking,
		type RankingSummary
	} from '#lib/api.ts';
	import PlayerList from '#lib/PlayerList.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { dynasty, drafts, competitions }: { dynasty: Dynasty; drafts: DraftSummary[]; competitions: Competition[] } = $props();

	let rankings = $state<RankingSummary[]>([]);
	let open = $state<Ranking>(); // the ranking being edited
	let busy = $state(false);

	const league = (id: string) => dynasty.leagues.find((l) => l.id === id)!;
	// The drafts a ranking for this league could be for: unfinished ones that cover it.
	const draftsFor = (leagueId: string) =>
		drafts
			.filter((d) => d.status !== 'complete' && d.competitions.includes(league(leagueId).competition))
			.toSorted((a, b) => a.year - b.year);

	const refresh = () => getRankings().then((r) => (rankings = r), toast.error);
	refresh();

	// ---- a new ranking ----
	let fresh = $state({ name: '', league_id: '', draft_id: '' });
	let rankingType = $state<'league' | 'startup'>('league');
	const startupDrafts = $derived(drafts.filter(d => d.kind === 'startup' && d.status !== 'complete'));
	const freshLeagues = $derived(dynasty.leagues);
	const linkedDraft = $derived(drafts.find(d => d.id === open?.draft_id));
	function selectStartup(id: string) { fresh.draft_id = id; }

	async function create(event: SubmitEvent) {
		event.preventDefault();
		try {
			const draft = drafts.find((d) => d.id === fresh.draft_id);
			open = await createRanking({
				name: fresh.name.trim() || (draft ? `${draft.name} board` : `${league(fresh.league_id).name} board`),
				league_id: rankingType === 'startup' ? undefined : fresh.league_id,
				draft_id: fresh.draft_id || null
			});
			fresh = { name: '', league_id: '', draft_id: '' };
			rankingType = 'league';
			refresh();
		} catch (e) {
			toast.error(e);
		}
	}

	// ---- editing one ----
	async function edit(id: string) {
		try {
			open = await getRanking(id);
		} catch (e) {
			toast.error(e);
		}
	}
	// Every change is saved as it is made, like a draft queue.
	async function save(change: { name?: string; draft_id?: string | null; player_ids?: string[] }) {
		if (!open || busy) return;
		busy = true;
		try {
			open = await updateRanking(open.id, { name: open.name, draft_id: open.draft_id, ...change });
			refresh();
		} catch (e) {
			toast.error(e);
			open = await getRanking(open.id).catch(() => open);
		} finally {
			busy = false;
		}
	}
	const ids = $derived(open?.players.map((p) => p.player_id) ?? []);
	function move(i: number, to: number) {
		if (to < 0 || to >= ids.length) return;
		const next = [...ids];
		next.splice(to, 0, ...next.splice(i, 1));
		save({ player_ids: next });
	}
	async function remove() {
		if (!open || !confirm(`Delete "${open.name}"?`)) return;
		try {
			await deleteRanking(open.id);
			open = undefined;
			refresh();
		} catch (e) {
			toast.error(e);
		}
	}
</script>

{#if !open}
	<div class="stack">
		<p class="muted">
			Rank players ahead of a draft. A ranking is yours alone: nobody else can see it. When the draft starts, load it
			into your queue from the draft room. Startup rankings combine every sport in the draft into one board.
		</p>

		{#if rankings.length > 0}
			<div class="list">
				{#each rankings as r (r.id)}
					<button class="card ranking" data-sport={r.competition} onclick={() => edit(r.id)}>
						<span class="row">{#if r.league_id}<SportBadge sport={r.competition} solid />{:else}<span class="pill brand">All leagues</span>{/if} {#if r.draft_name}<span class="pill gold">{r.draft_name}</span>{/if}</span>
						<strong class="name">{r.name}</strong>
						<span class="muted small-text">{r.players} {r.players === 1 ? 'player' : 'players'} ranked</span>
					</button>
				{/each}
			</div>
		{:else}
			<Empty icon="queue" title="No rankings yet">Create your first board below. Rank players in order, then import the board into My queue in the draft room.</Empty>
		{/if}

		<form class="card stack tight" onsubmit={create}>
			<h3>Create a draft board</h3>
			<label class="field">Ranking for
				<select bind:value={rankingType} onchange={() => { fresh.league_id = ''; fresh.draft_id = ''; if (rankingType === 'startup' && startupDrafts.length === 1) selectStartup(startupDrafts[0].id); }}>
					<option value="league">League / rookie draft</option>
					<option value="startup">Startup draft</option>
				</select>
			</label>
			{#if rankingType === 'startup'}
				{#if startupDrafts.length === 0}<p class="muted small-text">The commissioner needs to create a startup draft before you can link a startup ranking.</p>{/if}
				<p class="muted small-text">Rank players from every league in one combined order, then use “Add a ranking…” in the startup draft’s queue to import your board.</p>
			{/if}
			<div class="row end">
				{#if rankingType === 'startup'}
					<label class="field">Startup draft
						<select value={fresh.draft_id} required onchange={e => selectStartup(e.currentTarget.value)}>
							<option value="">Choose a startup draft</option>
							{#each startupDrafts as d (d.id)}<option value={d.id}>{d.name} · {d.year}</option>{/each}
						</select>
					</label>
				{/if}
				{#if rankingType === 'league'}
				<label class="field">
					League
					<select bind:value={fresh.league_id} required onchange={() => { if (rankingType === 'league') fresh.draft_id = ''; }}>
						<option value="">Choose a league</option>
						{#each freshLeagues as l (l.id)}<option value={l.id}>{l.name}</option>{/each}
					</select>
				</label>
				{/if}
				{#if fresh.league_id && rankingType === 'league'}
					<label class="field">
						For which draft
						<select bind:value={fresh.draft_id}>
							<option value="">No particular draft</option>
							{#each draftsFor(fresh.league_id) as d (d.id)}<option value={d.id}>{d.name}</option>{/each}
						</select>
					</label>
				{/if}
				<label class="field grow">Name (optional) <input bind:value={fresh.name} placeholder="My board" maxlength="80" /></label>
				<button class="primary" disabled={rankingType === 'startup' ? !fresh.draft_id : !fresh.league_id}>Create</button>
			</div>
		</form>
	</div>
{:else}
	{@const sport = open.league_id ? league(open.league_id).competition : ''}
	{@const covered = sport ? [sport] : (linkedDraft?.competitions ?? dynasty.leagues.map((l) => l.competition))}
	{@const pool = competitions.filter((c) => covered.includes(c.key))}
	<div class="stack" data-sport={sport}>
		<div class="row end head">
			<button class="quiet" onclick={() => (open = undefined)}><Icon name="left" size={16} /> All rankings</button>
			<label class="field grow">
				Name
				<input value={open.name} maxlength="80" onchange={(e) => save({ name: e.currentTarget.value })} />
			</label>
			{#if open.league_id}
			<label class="field">
				For which draft
				<select value={open.draft_id ?? ''} onchange={(e) => save({ draft_id: e.currentTarget.value || null })}>
					<option value="">No particular draft</option>
					{#each draftsFor(open.league_id) as d (d.id)}<option value={d.id}>{d.name}</option>{/each}
				</select>
			</label>
			{:else}<span class="pill brand">All leagues · {linkedDraft?.name ?? 'Startup draft'}</span>{/if}
			<button class="quiet danger" onclick={remove}>Delete</button>
		</div>

		{#if linkedDraft?.kind === 'startup'}
			<p class="muted small-text">Startup draft board · <a href="/draft/{linkedDraft.id}">Open {linkedDraft.name}</a> and choose “Add a ranking…” in My queue to import this board.</p>
		{/if}
		<div class="editor">
			<section class="panel" aria-label="My ranking">
				<h3 class="eyebrow bar">{#if sport}<SportBadge {sport} />{:else}<span>All leagues</span>{/if} Ranked <span class="muted">{open.players.length}</span></h3>
				{#if open.players.length === 0}
					<p class="muted small-text none">Add players from the pool. The order here is the order they go into your queue.</p>
				{:else}
					<ol>
						{#each open.players as player, i (player.player_id)}
							<li class:taken={player.owner_name !== ''}>
								<span class="n">{i + 1}</span>
								<Headshot name={player.full_name} src={player.headshot_url} size={28} />
								<div class="who">
									<strong>{player.full_name}</strong>
									{#if !open.league_id}<SportBadge sport={player.competition} />{/if}
									<span class="muted small-text">
										{[player.positions.join('/'), player.team_abbrev].filter(Boolean).join(' · ')}
										{#if player.owner_name}· taken by {player.owner_name}{/if}
									</span>
								</div>
								<span class="moves">
									<button class="quiet small" aria-label="Move {player.full_name} to the top" disabled={busy || i === 0} onclick={() => move(i, 0)}>Top</button>
									<button class="quiet small" aria-label="Move {player.full_name} up" disabled={busy || i === 0} onclick={() => move(i, i - 1)}>
										<Icon name="up" size={14} />
									</button>
									<button
										class="quiet small"
										aria-label="Move {player.full_name} down"
										disabled={busy || i === open.players.length - 1}
										onclick={() => move(i, i + 1)}
									>
										<Icon name="down" size={14} />
									</button>
									<button
										class="quiet small danger"
										aria-label="Remove {player.full_name}"
										disabled={busy}
										onclick={() => save({ player_ids: ids.filter((id) => id !== player.player_id) })}
									>
										<Icon name="x" size={14} />
									</button>
								</span>
							</li>
						{/each}
					</ol>
				{/if}
			</section>

			<section aria-label="Player pool">
				<PlayerList competition={sport} competitions={pool} leagueId={open.league_id ?? undefined} draftId={linkedDraft?.status !== 'complete' ? linkedDraft?.id : undefined} compact byPoints statColumns action={add} />
			</section>
		</div>
	</div>
{/if}

{#snippet add(player: Player)}
	{#if ids.includes(player.id)}
		<span class="muted small-text">Ranked</span>
	{:else}
		<button class="small" disabled={busy} aria-label="Rank {player.full_name}" onclick={() => save({ player_ids: [...ids, player.id] })}>
			<Icon name="plus" size={12} /> Rank
		</button>
	{/if}
{/snippet}

<style>
	.list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 0.8rem;
	}
	.ranking {
		display: grid;
		gap: 0.45rem;
		justify-items: start;
		text-align: left;
		white-space: normal;
		font-weight: 400;
	}
	.ranking .row {
		gap: 0.4rem;
	}
	.name {
		font: 700 1.1rem/1.2 var(--display);
	}
	.grow {
		flex: 1 1 12rem;
	}
	.head {
		gap: 0.6rem 0.9rem;
	}
	.editor {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 1rem;
		align-items: start;
	}
	@media (max-width: 900px) {
		.editor {
			grid-template-columns: 1fr;
		}
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.8rem;
		background: var(--surface-2);
		border-bottom: 1px solid var(--rule);
	}
	.none {
		padding: 0.8rem;
	}
	ol {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.6rem;
		border-bottom: 1px solid var(--rule);
	}
	li:last-child {
		border-bottom: none;
	}
	li.taken .who {
		opacity: 0.5;
		text-decoration: line-through;
	}
	.n {
		width: 1.6rem;
		font: 700 0.8rem var(--mono);
		color: var(--ink-faint);
	}
	.who {
		display: grid;
		flex: 1;
		min-width: 0;
		line-height: 1.3;
	}
	.moves {
		white-space: nowrap;
	}
	.moves button {
		padding: 0.2rem 0.3rem;
	}
</style>
