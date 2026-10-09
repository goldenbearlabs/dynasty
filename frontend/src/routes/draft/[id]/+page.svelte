<script lang="ts">
	// The draft room. Everything here is driven by the live draft state; a
	// pick made anywhere shows up on every screen at once.
	import { tick, untrack } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import {
		controlDraft,
		deleteDraft,
		getQueue,
		getRankings,
		importRanking,
		makePick,
		passPick,
		setDraftClock,
		setQueue,
		type DraftAction,
		type List,
		type Player,
		type QueuedPlayer,
		type RankingSummary
	} from '#lib/api.ts';
	import { DraftRoom } from '#lib/draftRoom.svelte.ts';
	import PlayerList from '#lib/PlayerList.svelte';
	import Countdown from '#lib/ui/Countdown.svelte';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Splitter from '#lib/ui/Splitter.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';
	import Board from './Board.svelte';
	import PickOrder from './PickOrder.svelte';
	import Research from './Research.svelte';
	import Teams from './Teams.svelte';
	import Queue from './Queue.svelte';

	let { data }: PageProps = $props();

	const id = $derived(page.params.id!);
	let room = $state<DraftRoom>();
	$effect(() => {
		const opened = new DraftRoom(id);
		room = opened;
		return () => opened.close();
	});

	const draft = $derived(room?.state?.draft);
	const picks = $derived(room?.state?.picks ?? []);
	const franchises = $derived(data.dynasty?.franchises ?? []);
	const franchise = (franchiseId: string) => franchises.find((f) => f.id === franchiseId);
	const me = $derived(data.me);
	const commissioner = $derived(me?.is_commissioner === true);

	const sports = $derived(
		(data.dynasty?.leagues ?? [])
			.filter((l) => room?.state?.league_ids.includes(l.id))
			.map((l) => l.competition)
	);
	const made = $derived(picks.filter((p) => p.player_id));
	const ordered = $derived(picks.toSorted((a, b) => a.position - b.position));
	const upcoming = $derived(ordered.filter((p) => !p.player_id && !p.skipped_at && !p.passed_at));
	const nextMine = $derived(upcoming.find((p) => p.current_franchise_id === me?.id));
	const picksAway = $derived(
		nextMine ? upcoming.filter((p) => p.position < nextMine.position).length : 0
	);
	const revision = $derived(
		made.map((p) => `${p.id}:${p.player_id}:${p.current_franchise_id}`).join(',')
	);
	let selectedId = $state<string>();
	let picking = $state(false);
	let savingQueue = $state(false);

	function research(playerId: string) {
		selectedId = playerId;
		if (window.matchMedia('(max-width: 640px)').matches) {
			void tick().then(() => {
				document.getElementById('draft-player-research')?.scrollIntoView({
					behavior: 'smooth',
					block: 'start'
				});
			});
		}
	}

	// Whose pick a click on "Draft" would use.
	const onClock = $derived(picks.find((p) => p.id === room?.state?.on_clock_pick_id));
	const myMakeUp = $derived(
		picks.find((p) => !p.player_id && p.skipped_at && !p.passed_at && p.current_franchise_id === me?.id)
	);
	const myTurn = $derived(onClock !== undefined && onClock.current_franchise_id === me?.id);
	const target = $derived.by(() => {
		if (draft?.status !== 'live' || !me) return undefined;
		if (myTurn) return onClock;
		if (myMakeUp) return myMakeUp;
		return commissioner ? onClock : undefined;
	});
	const pickingFor = $derived(
		target && target.current_franchise_id !== me?.id
			? franchise(target.current_franchise_id)
			: undefined
	);

	// ---- the pool and my queue follow the picks ----
	let queue = $state<QueuedPlayer[]>([]);
	let version = $state(0); // bumping it reloads the player list
	$effect(() => {
		void revision;
		untrack(() => version++);
		if (!me) {
			queue = [];
			return;
		}
		let active = true;
		getQueue(id).then(
			(q) => {
				if (active) queue = q;
			},
			(e) => {
				if (active) toast.error(e);
			}
		);
		return () => {
			active = false;
		};
	});

	let sport = $state('');
	const sportTabs = $derived([
		{ value: '', label: 'All' },
		...sports.map((s) => ({ value: s, label: s.toUpperCase(), sport: s }))
	]);

	// The rest of the app (the live banner, the drafts list) follows the status.
	let lastStatus: string | undefined;
	$effect(() => {
		const status = draft?.status;
		if (lastStatus && status && status !== lastStatus) invalidateAll();
		lastStatus = status;
	});

	// ---- actions ----
	// A rookie draft holds its picks as rights, to be signed afterwards, and lets a pick be passed.
	const rookie = $derived(draft?.kind === 'seasonal');
	async function pass() {
		if (!onClock || !confirm(`Pass pick #${onClock.position}? It cannot be made later.`)) return;
		try {
			await passPick(id);
		} catch (e) {
			toast.error(e);
		}
	}

	async function pick(player: Player, list: List) {
		if (!target || picking || player.owner_slug || made.some((p) => p.player_id === player.id))
			return;
		const pickId = target.id;
		const ownerName = pickingFor?.name ?? 'You';
		picking = true;
		try {
			await makePick(id, { player_id: player.id, list, pick_id: pickId });
			toast.good(`${ownerName} drafted ${player.full_name}.`);
			await invalidateAll();
		} catch (e) {
			toast.error(e);
		} finally {
			picking = false;
		}
	}

	// ---- pane sizes ----
	// Each manager arranges the room for themselves: the sizes are kept in
	// this browser. They apply to the wide layout; narrower screens stack.
	const usual = { side: 272, board: 38, pool: 60, queue: 34 }; // px, then % of the parent pane
	const limits = { side: [200, 560], board: [15, 72], pool: [25, 80], queue: [15, 78] } as const;
	type Pane = keyof typeof usual;
	let sizes = $state({ ...usual });
	try {
		const kept = JSON.parse(localStorage.getItem('draft-room-panes') ?? '{}');
		for (const pane of Object.keys(usual) as Pane[]) if (typeof kept[pane] === 'number') sizes[pane] = kept[pane];
	} catch {
		// storage is unavailable or holds something else: the usual sizes stand
	}
	function resize(pane: Pane, value: number) {
		sizes[pane] = Math.round(Math.min(limits[pane][1], Math.max(limits[pane][0], value)) * 10) / 10;
		try {
			localStorage.setItem('draft-room-panes', JSON.stringify(sizes));
		} catch {
			// not kept, but the room still resizes
		}
	}
	let workspace = $state<HTMLElement>();
	let mainPane = $state<HTMLElement>();
	let scouting = $state<HTMLElement>();
	let teamPane = $state<HTMLElement>();
	// Where the pointer is within a pane, as a percentage across or down it.
	const across = (el: HTMLElement | undefined, x: number) => (el ? ((x - el.getBoundingClientRect().left) / el.clientWidth) * 100 : 50);
	const down = (el: HTMLElement | undefined, y: number) => (el ? ((y - el.getBoundingClientRect().top) / el.clientHeight) * 100 : 50);

	// My pre-draft rankings for the leagues this draft covers, the ones made for it first.
	let rankings = $state<RankingSummary[]>([]);
	$effect(() => {
		if (me) getRankings().then((r) => (rankings = r), () => (rankings = []));
	});
	const importable = $derived(
		rankings
			.filter((r) => r.players > 0 && room?.state?.league_ids.includes(r.league_id))
			.toSorted((a, b) => Number(b.draft_id === id) - Number(a.draft_id === id))
	);
	async function loadRanking(rankingId: string) {
		if (!rankingId || savingQueue) return;
		savingQueue = true;
		try {
			const { added } = await importRanking(id, rankingId);
			queue = await getQueue(id);
			toast.good(
				added > 0
					? `Added ${added} ${added === 1 ? 'player' : 'players'} to the end of your queue.`
					: 'Nothing to add: everyone on that ranking is taken or already queued.'
			);
		} catch (e) {
			toast.error(e);
		} finally {
			savingQueue = false;
		}
	}

	async function saveQueue(playerIds: string[]) {
		if (savingQueue) return;
		savingQueue = true;
		try {
			await setQueue(id, playerIds);
			queue = await getQueue(id);
		} catch (e) {
			toast.error(e);
		} finally {
			savingQueue = false;
		}
	}

	const control = (action: DraftAction) => controlDraft(id, action).catch(toast.error);

	function finish() {
		if (confirm('Finish the draft now? Picks that have not been made are forfeited.'))
			control('finish');
	}

	async function remove() {
		if (!confirm('Delete this draft?')) return;
		try {
			await deleteDraft(id);
			await invalidateAll();
			await goto('/drafts');
		} catch (e) {
			toast.error(e);
		}
	}

	let clockSeconds = $state<number>();
	const saveClock = () =>
		setDraftClock(id, clockSeconds ?? 0)
			.then(() => toast.good('Pick clock updated. It applies from the next pick.'))
			.catch(toast.error);

	const statusPill = {
		scheduled: { label: 'Not started', tone: '' },
		live: { label: 'Live', tone: 'bad' },
		paused: { label: 'Paused', tone: 'gold' },
		complete: { label: 'Complete', tone: 'good' }
	} as const;
</script>

<svelte:head><title>{draft?.name ?? 'Draft'}</title></svelte:head>

{#if !draft}<p class="muted">Joining the draft room…</p>
{:else}
	<div class="draft-room">
		<header class="room-head">
			<div class="title">
				<div class="row">
					<span class="pill {statusPill[draft.status].tone}">{statusPill[draft.status].label}</span
					><span class="muted small-text">{made.length}/{picks.length} picks</span
					>{#if !room?.connected}<span class="pill gold">Reconnecting…</span>{/if}
				</div>
				<h1>{draft.name}</h1>
			</div>
			<div class="on-clock" class:mine={myTurn}>
				{#if onClock && draft.status !== 'complete'}<Crest
						src={franchise(onClock.current_franchise_id)?.image_url}
 name={franchise(onClock.current_franchise_id)?.name ?? ''}
						size={30}
					/>
					<div>
						<p class="eyebrow">{myTurn ? 'Your turn' : 'On the clock'} · #{onClock.position}</p>
						<strong>{franchise(onClock.current_franchise_id)?.name}</strong>
					</div>
				{:else}<div>
						<p class="eyebrow">{draft.status === 'complete' ? 'Draft complete' : 'Draft room'}</p>
						<strong
							>{draft.status === 'scheduled' ? 'Waiting to start' : 'Waiting for picks'}</strong
						>
					</div>{/if}
				<div class="clock">
					{#if draft.status === 'paused'}<span class="pill gold">Paused</span
						>{:else if draft.status === 'live' && draft.clock_expires_at}<Countdown
							until={draft.clock_expires_at}
						/>{:else if draft.status === 'live'}<span class="muted small-text">No clock</span>{/if}
				</div>
			</div>
			{#if me}<div class="next-pick">
					<p class="eyebrow">Your next pick</p>
					{#if nextMine && draft.status !== 'complete'}<strong
							>{myTurn
								? 'You’re up'
								: `${picksAway} ${picksAway === 1 ? 'pick' : 'picks'} away`}</strong
						><span>R{nextMine.round} · #{nextMine.position}</span>
						{#if rookie && draft.status === 'live' && (myTurn || commissioner) && onClock}
							<button class="small quiet pass" title="Give up this pick. It cannot be made later." onclick={pass}>
								{myTurn ? 'Pass this pick' : `Pass for ${franchise(onClock.current_franchise_id)?.name ?? 'them'}`}
							</button>
						{/if}{:else}<strong
							>{myMakeUp && draft.status !== 'complete'
								? 'Skipped pick owed'
								: 'No picks remaining'}</strong
						>{/if}
				</div>{/if}
			{#if commissioner}
				<details class="admin">
					<summary>Manage draft</summary>
					<div class="admin-controls">
						{#if draft.status === 'scheduled'}<button
								class="small primary"
								onclick={() => control('start')}><Icon name="play" size={14} />Start draft</button
							><button class="small quiet danger" onclick={remove}>Delete</button>
						{:else if draft.status === 'live'}<button class="small" onclick={() => control('pause')}
								><Icon name="pause" size={14} />Pause</button
							>
						{:else if draft.status === 'paused'}<button
								class="small primary"
								onclick={() => control('resume')}><Icon name="play" size={14} />Resume</button
							>{/if}
						{#if draft.status === 'live' || draft.status === 'paused'}<button
								class="small"
								onclick={() => control('undo')}
								disabled={!made.length}>Undo last pick</button
							><button class="small quiet danger" onclick={finish}>Finish draft</button>{/if}
						{#if draft.status !== 'complete'}<label class="row clock-set"
								><span class="muted small-text">Seconds per pick</span><input
									type="number"
									min="0"
									placeholder={String(draft.pick_clock_seconds)}
									bind:value={clockSeconds}
								/><button
									class="small"
									disabled={clockSeconds === undefined || clockSeconds === null}
									onclick={saveClock}>Set</button
								></label
							>{/if}
						{#if draft.status === 'scheduled'}<details class="order-editor">
								<summary>Edit pick order</summary><PickOrder draftId={id} {picks} {franchises} />
							</details>{/if}
					</div>
				</details>
			{/if}
		</header>
		{#if myMakeUp && !myTurn && draft.status === 'live'}<p class="makeup">
				<span class="pill gold">Skipped</span> Pick #{myMakeUp.position} is owed to you. Select a player
				below to make it up.
			</p>{/if}
		<div
			class="workspace"
			bind:this={workspace}
			style:--side="{sizes.side}px"
			style:--board="{sizes.board}%"
			style:--pool="{sizes.pool}%"
			style:--queue="{sizes.queue}%"
		>
			<div class="main-workspace" bind:this={mainPane}>
				<section class="board-pane" aria-label="Draft progress">
					<div class="section-heading">
						<h2>Draft board</h2>
						<div class="upcoming" aria-label="Upcoming picks">
							{#each upcoming.slice(0, 5) as p (p.id)}<span
									class:you={p.current_franchise_id === me?.id}
									><b>#{p.position}</b>
									{p.current_franchise_id === me?.id
										? 'You'
										: franchise(p.current_franchise_id)?.name}</span
								>{/each}
						</div>
					</div>
					<Board
						{picks}
						{franchises}
						onClockId={room?.state?.on_clock_pick_id ?? null}
						myId={me?.id}
						showSport={sports.length > 1}
						onselect={research}
					/>
				</section>
				<Splitter
					orientation="horizontal"
					label="Resize the draft board"
					onmove={(_, y) => resize('board', down(mainPane, y))}
					onnudge={(d) => resize('board', sizes.board + 2 * d)}
					onreset={() => resize('board', usual.board)}
				/>
				<div class="scouting" bind:this={scouting}>
					<section class="pool-pane" id="draft-player-pool" aria-label="Available players">
						<div class="section-heading">
							<h2>Available players</h2>
							<span class="hint">Click a name to research</span>
						</div>
						<div class="pool-body">
							{#if sports.length > 1}<Tabs
									tabs={sportTabs}
									bind:value={sport}
									label="Player sport"
								/>{/if}
							<PlayerList
								competition={sport}
								draftId={id}
								{version}
								compact
								byPoints
								statColumns
								onselect={(player) => research(player.id)}
								{selectedId}
								action={me && draft.status !== 'complete' ? queueAction : undefined}
							/>
						</div>
					</section>
					<Splitter
						orientation="vertical"
						label="Resize the player pool"
						onmove={(x) => resize('pool', across(scouting, x))}
						onnudge={(d) => resize('pool', sizes.pool + 2 * d)}
						onreset={() => resize('pool', usual.pool)}
					/>
					<Research {selectedId} competitions={data.competitions} action={researchAction} />
				</div>
			</div>
			<Splitter
				orientation="vertical"
				label="Resize the team panel"
				onmove={(x) => resize('side', workspace ? workspace.getBoundingClientRect().right - x : usual.side)}
				onnudge={(d) => resize('side', sizes.side - 16 * d)}
				onreset={() => resize('side', usual.side)}
			/>
			<aside class="team-workspace" bind:this={teamPane}>
				<Teams {franchises} myId={me?.id} {revision} onselect={research} />
				{#if me}<Splitter
						orientation="horizontal"
						label="Resize my queue"
						onmove={(_, y) => resize('queue', 100 - down(teamPane, y))}
						onnudge={(d) => resize('queue', sizes.queue - 2 * d)}
						onreset={() => resize('queue', usual.queue)}
					/>{/if}
				{#if me}<section class="queue-pane" aria-label="My draft queue">
						<div class="section-heading">
							<h2>My queue <span class="muted">{queue.length}</span></h2>
							{#if importable.length > 0 && draft.status !== 'complete'}
								<select
									class="import"
									aria-label="Add one of my rankings to the queue"
									disabled={savingQueue}
									onchange={(e) => {
										loadRanking(e.currentTarget.value);
										e.currentTarget.value = '';
									}}
								>
									<option value="">Add a ranking…</option>
									{#each importable as r (r.id)}<option value={r.id}>{r.name} ({r.players})</option>{/each}
								</select>
							{:else}
								<span class="hint">Auto-pick order</span>
							{/if}
						</div>
						<div class="queue-body">
							<Queue
								{queue}
								onchange={saveQueue}
								onselect={research}
								editable={draft.status !== 'complete'}
								disabled={savingQueue}
							/>
						</div>
					</section>{/if}
			</aside>
		</div>
	</div>
{/if}

{#snippet queueAction(player: Player)}
	{#if queue.some((q) => q.player_id === player.id)}<span class="queued">Queued</span>{:else}<button
			class="quiet small"
			disabled={savingQueue}
			aria-label={`Queue ${player.full_name}`}
			onclick={() => saveQueue([...queue.map((q) => q.player_id), player.id])}
			><Icon name="plus" size={12} />Queue</button
		>{/if}
{/snippet}

{#snippet researchAction(player: Player)}
	{@const taken = !!player.owner_slug || made.some((p) => p.player_id === player.id)}
	{#if taken}<span class="pill">{player.owner_name || 'Already drafted'}</span>
	{:else if me && draft?.status !== 'complete'}
		{#if target}<p class="target-note">
				{pickingFor ? `Picking for ${pickingFor.name}` : myTurn ? 'Your pick' : 'Make-up pick'} · R{target.round}
				· #{target.position}
			</p>{/if}
		<button
			class="small primary"
			disabled={!target || picking || !room?.connected}
			onclick={() => pick(player, rookie ? 'rights' : 'main')}>{picking ? 'Drafting…' : rookie ? 'Draft' : 'Draft to main'}</button
		>
		{#if !rookie}<button
				class="small"
				disabled={!target || picking || !room?.connected}
				onclick={() => pick(player, 'reserve')}>To reserve</button
			>{/if}
		{@render queueAction(player)}
		{#if !target}<p class="target-note muted">
				{draft?.status === 'paused'
					? 'Draft paused'
					: draft?.status === 'scheduled'
						? 'Draft has not started'
						: 'Queue this player while you wait for your pick.'}
			</p>{/if}
	{/if}
{/snippet}

<style>
	.draft-room {
		display: grid;
		gap: 0.7rem;
		min-width: 0;
	}
	.room-head {
		display: flex;
		align-items: center;
		gap: 1.1rem;
		padding-bottom: 0.65rem;
		border-bottom: 1px solid var(--rule);
	}
	.title {
		display: grid;
		gap: 0.3rem;
		flex: 1;
		min-width: 0;
	}
	.title .row {
		gap: 0.35rem;
	}
	h1 {
		font-size: 1.3rem;
	}
	.on-clock {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.5rem 0.65rem;
		background: var(--surface);
		border-left: 2px solid var(--rule-strong);
	}
	.on-clock.mine {
		background: var(--brand-soft);
		border-color: var(--brand);
	}
	.on-clock strong {
		font-size: 0.85rem;
	}
	.on-clock .eyebrow {
		font-size: 0.6rem;
	}
	.clock {
		margin-left: 0.5rem;
	}
	.next-pick {
		display: grid;
		gap: 0.15rem;
		border-left: 1px solid var(--rule);
		padding-left: 1rem;
	}
	.next-pick strong {
		font: 750 1rem var(--display);
		color: var(--brand);
	}
	.next-pick span {
		font-size: 0.7rem;
		color: var(--ink-soft);
	}
	.next-pick .eyebrow {
		font-size: 0.6rem;
	}
	.admin {
		position: relative;
		flex: none;
		font-size: 0.72rem;
	}
	.admin summary {
		cursor: pointer;
		color: var(--ink-soft);
	}
	.admin-controls {
		position: absolute;
		z-index: 25;
		right: 0;
		top: calc(100% + 0.5rem);
		width: 32rem;
		max-width: calc(100vw - 2rem);
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		padding: 0.85rem;
		background: var(--surface);
		border: 1px solid var(--rule-strong);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.15);
		max-height: 70dvh;
		overflow: auto;
	}
	.clock-set {
		width: 100%;
		gap: 0.5rem;
	}
	.clock-set input {
		width: 4rem;
		padding: 0.25rem;
	}
	.order-editor {
		width: 100%;
	}
	.order-editor > summary {
		padding-bottom: 0.7rem;
	}
	.workspace {
		display: grid;
		/* main, the bar that resizes, the team panel */
		grid-template-columns: minmax(0, 1fr) 0.75rem var(--side);
		height: calc(100dvh - 150px);
		min-height: 480px;
	}
	.main-workspace {
		display: grid;
		grid-template-rows: minmax(120px, var(--board)) 0.75rem minmax(0, 1fr);
		min-width: 0;
		min-height: 0;
	}
	.board-pane {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		background: var(--surface);
		border-block: 1px solid var(--rule);
	}
	.section-heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.55rem 0.7rem;
		border-bottom: 1px solid var(--rule);
		flex: none;
		min-width: 0;
	}
	h2 {
		font-size: 0.9rem;
		white-space: nowrap;
	}
	.pass {
		margin-top: 0.2rem;
		padding: 0.15rem 0.4rem;
		font-size: 0.72rem;
	}
	.import {
		max-width: 11rem;
		padding: 0.2rem 0.4rem;
		font-size: 0.75rem;
	}
	.hint {
		font-size: 0.65rem;
		color: var(--ink-soft);
	}
	.upcoming {
		display: flex;
		gap: 0.5rem;
		overflow: hidden;
		font-size: 0.6rem;
		color: var(--ink-soft);
	}
	.upcoming span {
		white-space: nowrap;
	}
	.upcoming b {
		font-family: var(--mono);
	}
	.upcoming .you {
		color: var(--brand);
		font-weight: 700;
	}
	.scouting {
		display: grid;
		grid-template-columns: minmax(180px, var(--pool)) 0.5rem minmax(180px, 1fr);
		background: var(--surface);
		min-height: 0;
		min-width: 0;
		border-block: 1px solid var(--rule);
	}
	.pool-pane {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		background: var(--surface);
	}
	.pool-body {
		overflow: auto;
		min-height: 0;
		padding: 0.45rem 0.65rem;
	}
	.pool-body :global(.tabs) {
		margin-bottom: 0.4rem;
	}
	.team-workspace {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		background: var(--surface);
		border-block: 1px solid var(--rule);
	}
	.team-workspace :global(.splitter) {
		height: 0.5rem;
	}
	.queue-pane {
		display: flex;
		flex-direction: column;
		min-height: 0;
		flex: 0 0 var(--queue);
		border-top: 1px solid var(--rule);
	}
	.queue-body {
		overflow: auto;
		min-height: 0;
		padding: 0.4rem;
	}
	.queued {
		font-size: 0.65rem;
		color: var(--ink-soft);
	}
	.target-note {
		font-size: 0.72rem;
		flex-basis: 100%;
	}
	.makeup {
		font-size: 0.78rem;
		padding: 0.4rem 0.65rem;
		background: var(--gold-soft);
	}
	@media (max-width: 1199px) {
		/* Stacked: the panes take their own heights and are not resized. */
		.workspace :global(.splitter) {
			display: none;
		}
		.workspace {
			grid-template-columns: minmax(0, 1fr);
			gap: 0.75rem;
			height: auto;
			min-height: 0;
		}
		.main-workspace {
			grid-template-rows: 250px auto;
			gap: 0.75rem;
		}
		.scouting {
			grid-template-columns: minmax(0, 1.5fr) minmax(240px, 1fr);
		}
		.scouting {
			height: 480px;
		}
		.team-workspace {
			display: grid;
			grid-template-columns: 1fr 1fr;
			height: 320px;
		}
		.queue-pane {
			border-top: 0;
			border-left: 1px solid var(--rule);
		}
		.room-head {
			flex-wrap: wrap;
		}
		.title {
			flex-basis: 40%;
		}
		.on-clock {
			flex: 1;
		}
		.admin {
			margin-left: auto;
		}
	}
	@media (max-width: 640px) {
		.room-head {
			gap: 0.5rem;
		}
		.title {
			flex-basis: 100%;
		}
		.on-clock {
			padding: 0.4rem;
		}
		.on-clock strong {
			font-size: 0.75rem;
		}
		.next-pick {
			padding-left: 0.5rem;
		}
		.next-pick strong {
			font-size: 0.85rem;
		}
		.scouting {
			grid-template-columns: 1fr;
			height: auto;
		}
		.pool-pane {
			max-height: 440px;
			min-height: 250px;
		}
		.scouting :global(.research) {
			border-left: 0;
			border-top: 1px solid var(--rule);
			max-height: 420px;
			min-height: 180px;
		}
		.team-workspace {
			grid-template-columns: 1fr;
			height: auto;
		}
		.team-workspace :global(.teams) {
			max-height: 400px;
		}
		.queue-pane {
			border-left: 0;
			border-top: 1px solid var(--rule);
			max-height: 300px;
		}
		.upcoming {
			display: none;
		}
		.section-heading {
			padding-inline: 0.6rem;
		}
	}
</style>
