<script lang="ts">
	// The trade machine: up to four teams, each sending players and picks to
	// the others. A sport filter shows that league's players and draft picks
	// together for every team. Below, what each team receives, what it does
	// to the rosters, and the offer to send.
	import { untrack } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { getFranchise, proposeTrade, type Franchise, type FranchiseDetail, type List, type TradeDrop } from '#lib/api.ts';
	import TradeDrops from '#lib/TradeDrops.svelte';
	import TradeAsset from '#lib/TradeAsset.svelte';
	import TradeTeam from '#lib/TradeTeam.svelte';
	import { assetsOf, toItem, type Asset } from '#lib/tradeAssets.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const most = 4; // teams in one trade, the proposer's included
	const me = $derived(data.me);
	const franchises = $derived(data.dynasty?.franchises ?? []);
	const leagues = $derived(data.dynasty?.leagues ?? []);
	const others = $derived(franchises.filter((f) => f.id !== me?.id));
	const byId = (id: string) => franchises.find((f) => f.id === id);

	// The other teams, in the order they joined. The first can be preselected: /trades/new?with=their-slug
	let partners = $state<string[]>(
		untrack(() => {
			const slug = page.url.searchParams.get('with');
			const first = franchises.find((f) => f.slug === slug && f.id !== data.me?.id) ?? others[0];
			return first ? [first.id] : [];
		})
	);
	const parties = $derived([byId(me?.id ?? ''), ...partners.map(byId)].filter((f): f is Franchise => !!f));
	const addable = $derived(others.filter((f) => !partners.includes(f.id)));
	function addTeam(id: string) {
		if (id && partners.length < most - 1) partners = [...partners, id];
	}
	function removeTeam(id: string) {
		partners = partners.filter((p) => p !== id);
		if (active === id) active = me?.id ?? '';
	}

	// Each team's holdings, loaded once.
	let details = $state<Record<string, FranchiseDetail>>({});
	$effect(() => {
		for (const party of parties) {
			if (details[party.id]) continue;
			getFranchise(party.slug).then((d) => (details[party.id] = d), toast.error);
		}
	});

	// What is chosen, by asset: who sends it and who gets it.
	const chosen = new SvelteMap<string, { from: string; to: string }>();
	// A team leaving the trade takes its assets with it, and anything headed its way comes back.
	$effect(() => {
		const ids = parties.map((p) => p.id);
		untrack(() => {
			for (const [key, c] of chosen) if (!ids.includes(c.from) || !ids.includes(c.to)) chosen.delete(key);
		});
	});
	// /trades/new?with=slug&player=id starts with that player coming to you.
	let preselected = false;
	$effect(() => {
		const partner = partners[0];
		const detail = partner ? details[partner] : undefined;
		if (preselected || !detail || !me) return;
		preselected = true;
		const asset = assetsOf(detail, franchises).find((a) => a.playerId === page.url.searchParams.get('player'));
		if (asset) chosen.set(asset.key, { from: partner, to: me.id });
	});
	// What a team's ticked asset goes to first: the proposer's go to the first partner, everyone else's to the proposer.
	const defaultTo = (id: string) => (id === me?.id ? (partners[0] ?? '') : (me?.id ?? ''));

	// Every asset in play, by key, so the offer can describe what is chosen.
	const assets = $derived(new Map(Object.values(details).flatMap((d) => assetsOf(d, franchises).map((a) => [a.key, a] as [string, Asset]))));
	const lines = $derived([...chosen].flatMap(([key, c]) => (assets.has(key) ? [{ key, ...c, asset: assets.get(key)! }] : [])));
	const offerItems = $derived(lines.map((l) => toItem(l.asset, l.from, l.to)));
	// A team in the trade that neither sends nor gets anything.
	const idle = $derived(parties.filter((p) => !lines.some((l) => l.from === p.id || l.to === p.id)));

	// ---- the sport filter, shared by every team ----
	let sport = $state('');
	const sportTabs = $derived([
		{ value: '', label: 'All' },
		...leagues
			.map((l) => l.competition)
			.filter((c) => Object.values(details).some((d) => d.rosters.some((r) => r.competition === c && r.players.length) || d.picks.some((p) => p.competitions.includes(c))))
			.map((c) => ({ value: c, label: c.toUpperCase(), sport: c }))
	]);

	// ---- on a phone, one team at a time ----
	let active = $state(untrack(() => data.me?.id ?? ''));
	const teamTabs = $derived(
		parties.map((p) => ({ value: p.id, label: p.id === me?.id ? 'You' : p.name, count: lines.filter((l) => l.from === p.id).length }))
	);

	// What the trade does to each roster it touches, league by league. An
	// arriving player keeps the list he was on.
	type Change = { name: string; list: List; before: number; after: number; max: number };
	const impact = $derived.by(() => {
		const touched = [...new Set(lines.map((l) => l.asset.leagueId).filter((id): id is string => !!id))];
		return touched.flatMap((league) => {
			const changes: Change[] = [];
			let competition = '';
			for (const party of parties) {
				const roster = details[party.id]?.rosters.find((r) => r.league_id === league);
				if (!roster) continue;
				competition = roster.competition;
				for (const list of ['main', 'reserve'] as const) {
					const moved = (dir: 'from' | 'to') => lines.filter((l) => l[dir] === party.id && l.asset.leagueId === league && l.asset.list === list).length;
					const before = roster.players.filter((p) => p.list === list).length;
					const after = before - moved('from') + moved('to');
					if (after !== before) changes.push({ name: party.id === me?.id ? 'You' : party.name, list, before, after, max: roster.limits[list] });
				}
			}
			return changes.length ? [{ league, competition, changes }] : [];
		});
	});
	const over = $derived(impact.some((l) => l.changes.some((c) => c.after > c.max)));

	let note = $state('');
	let busy = $state(false);
	let drops = $state<TradeDrop[]>([]);
	let dropsReady = $state(false);
	async function propose() {
		if (!me) return;
		busy = true;
		try {
			await proposeTrade(note, offerItems, drops);
			await invalidateAll();
			toast.good(`Offer sent to ${parties.filter((p) => p.id !== me.id).map((p) => p.name).join(', ')}.`);
			await goto('/trades');
		} catch (e) {
			toast.error(e);
		} finally {
			busy = false;
		}
	}
	const label = (id: string) => (id === me?.id ? 'You' : (byId(id)?.name ?? ''));
</script>

<svelte:head><title>New trade</title></svelte:head>

{#if !me}
	<p role="alert">Sign in to propose a trade.</p>
{:else if others.length === 0}
	<p class="muted">There is nobody to trade with yet.</p>
{:else}
	<div class="stack builder" style:--teams={parties.length}>
		<header class="spread">
			<div>
				<h1>New trade</h1>
				<p class="muted small-text">Tick what each team sends. {parties.length > 2 ? 'Each ticked item says where it goes.' : 'Up to four teams can take part.'}</p>
			</div>
			{#if partners.length < most - 1 && addable.length}
				<label class="add">
					<Icon name="plus" size={14} />
					<select aria-label="Add a team to the trade" value="" onchange={(e) => { addTeam(e.currentTarget.value); e.currentTarget.value = ''; }}>
						<option value="">Add a team</option>
						{#each addable as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
					</select>
				</label>
			{/if}
		</header>

		<div class="filters">
			<Tabs tabs={sportTabs} bind:value={sport} label="Sport" />
		</div>
		<div class="team-tabs"><Tabs tabs={teamTabs} bind:value={active} label="Team" /></div>

		<div class="columns">
			{#each parties as party (party.id)}
				{@const wants = tradeMarket.data.wants.filter((w) => w.franchise_id === party.id && (!sport || w.competition === sport))}
				<section class="card flush column" class:inactive={active !== party.id} aria-label="{label(party.id)} sends">
					<header class="column-head">
						<Crest src={party.image_url} name={party.name} size={22} />
						<strong>{party.id === me.id ? 'You' : party.name}</strong>
						<span class="muted small-text">send {lines.filter((l) => l.from === party.id).length}</span>
						{#if party.id === me.id}
							<span></span>
						{:else if partners.length > 1}
							<button class="quiet small remove" aria-label="Take {party.name} out of the trade" onclick={() => removeTeam(party.id)}><Icon name="x" size={14} /></button>
						{:else}
							<select class="swap" aria-label="Trade with" value={party.id} onchange={(e) => (partners = [e.currentTarget.value])}>
								{#each others as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
							</select>
						{/if}
					</header>
					{#if wants.length}
						<p class="wants">
							<strong>Wants</strong>
							{#each wants as w (w.id)}<span>{#if !sport}<SportBadge sport={w.competition} /> {/if}{w.positions.join(', ')}{#if w.note} “{w.note}”{/if}</span>{/each}
						</p>
					{/if}
					<!-- svelte-ignore a11y_no_noninteractive_tabindex: the list scrolls with the keyboard too -->
					<div class="scroll-body" tabindex="0" role="region" aria-label="{party.name}: players and picks">
						{#if details[party.id]}
							<TradeTeam detail={details[party.id]} {franchises} {sport} {parties} {chosen} defaultTo={defaultTo(party.id)} me={me.id} />
						{:else}
							<p class="muted small-text loading">Loading…</p>
						{/if}
					</div>
				</section>
			{/each}
		</div>

		<!-- On a phone, the offer so far stays in reach while choosing. -->
		{#if lines.length}
			<a class="offer-bar" href="#offer"><strong>{lines.length} in the offer</strong><span>Review <Icon name="down" size={14} /></span></a>
		{/if}

		<section class="card stack offer" id="offer" aria-labelledby="offer-heading">
			<h2 id="offer-heading">The offer</h2>
			{#if lines.length === 0}
				<p class="muted small-text">Tick players and picks above. They can come from any sport.</p>
			{:else}
				<div class="receives">
					{#each parties as party (party.id)}
						{@const getting = lines.filter((l) => l.to === party.id)}
						<div class="receiver">
							<h3 class="row"><Crest src={party.image_url} name={party.name} size={20} /> {label(party.id)} <span class="muted">{party.id === me.id ? 'get' : 'gets'}</span></h3>
							{#if getting.length === 0}
								<p class="muted small-text">Nothing yet.</p>
							{:else}
								<ul>
									{#each getting as line (line.key)}
										<li>
											<TradeAsset {...line.asset.display} />
											{#if parties.length > 2}<span class="muted small-text from">from {label(line.from)}</span>{/if}
											<button class="quiet small" aria-label="Take {line.asset.name} out of the offer" onclick={() => chosen.delete(line.key)}><Icon name="x" size={14} /></button>
										</li>
									{/each}
								</ul>
							{/if}
						</div>
					{/each}
				</div>
				{#if idle.length}
					<p class="small-text warn">{idle.map((p) => label(p.id)).join(' and ')} {idle.length === 1 && idle[0].id !== me.id ? 'is' : 'are'} not sending or getting anything yet.</p>
				{/if}
			{/if}

			{#if impact.length > 0}
				<ul class="impact">
					{#each impact as league (league.league)}
						<li>
							<SportBadge sport={league.competition} solid />
							<div class="changes">
								{#each league.changes as change (change.name + change.list)}
									<span class:over={change.after > change.max}>
										{change.name} {change.list}: {change.before} → <strong>{change.after}</strong>/{change.max}
									</span>
								{/each}
							</div>
						</li>
					{/each}
				</ul>
				{#if over}
					<p class="muted small-text">Each manager chooses drops from their own overfull list. The others choose theirs when accepting.</p>
				{/if}
			{/if}

			{#if offerItems.length}<TradeDrops items={offerItems} {franchises} owner={me.id} bind:drops onchange={(ready) => (dropsReady = ready)} />{/if}

			<label class="field">Note (optional) <input bind:value={note} placeholder="Say something to sell it" /></label>
			<div class="row">
				<button class="primary" disabled={busy || lines.length === 0 || idle.length > 0 || !dropsReady} onclick={propose}>
					Send offer{parties.length > 2 ? ` to ${parties.length - 1} teams` : ''}
				</button>
				<a class="button quiet" href="/trades">Cancel</a>
			</div>
		</section>
	</div>
{/if}

<style>
	.builder {
		gap: 0.6rem;
		min-width: 0;
		grid-template-columns: minmax(0, 1fr);
	}
	.add {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--brand);
	}
	.add select {
		font-size: 0.8rem;
	}
	/* Every team side by side; each list scrolls on its own, tall enough to see a roster whole. */
	.columns {
		display: grid;
		grid-template-columns: repeat(var(--teams), minmax(0, 1fr));
		gap: 0.6rem;
		align-items: start;
	}
	.column {
		display: grid;
		grid-template-rows: auto auto 1fr;
		min-width: 0;
	}
	.column-head {
		display: grid;
		grid-template-columns: auto auto 1fr auto;
		align-items: center;
		gap: 0.45rem;
		padding: 0.4rem 0.6rem;
		border-bottom: 1px solid var(--rule);
		background: var(--surface-2);
	}
	.column-head strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.swap {
		max-width: 9rem;
		padding: 0.15rem 0.3rem;
		font-size: 0.72rem;
	}
	.remove {
		padding: 0.15rem;
	}
	.wants {
		display: flex;
		flex-wrap: wrap;
		gap: 0.15rem 0.6rem;
		margin: 0;
		padding: 0.3rem 0.6rem;
		border-bottom: 1px solid var(--rule);
		font-size: 0.7rem;
		color: var(--ink-soft);
	}
	.scroll-body {
		max-height: max(22rem, calc(100dvh - 17rem));
		overflow-y: auto;
		overscroll-behavior: contain;
		scrollbar-width: thin;
	}
	.loading {
		padding: 0.6rem;
	}
	.team-tabs,
	.offer-bar {
		display: none;
	}
	.receives {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr));
		gap: 0.8rem 1.2rem;
	}
	.receiver h3 {
		gap: 0.4rem;
		margin-bottom: 0.3rem;
		font-size: 0.9rem;
	}
	.receiver ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.3rem;
	}
	.receiver li {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.receiver li :global(.asset) {
		flex: 1;
	}
	.from {
		white-space: nowrap;
	}
	.warn {
		color: var(--gold);
	}
	.impact {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.4rem;
	}
	.impact li {
		display: flex;
		align-items: baseline;
		gap: 0.6rem;
	}
	.changes {
		display: flex;
		flex-wrap: wrap;
		gap: 0.15rem 1rem;
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
	}
	.over {
		color: var(--bad);
	}
	/* A phone: one team's list at a time, picked from tabs, and the offer a tap away. */
	@media (max-width: 760px) {
		.columns {
			grid-template-columns: minmax(0, 1fr);
		}
		.column.inactive {
			display: none;
		}
		.team-tabs {
			display: block;
		}
		.scroll-body {
			max-height: calc(100dvh - 19rem);
			min-height: 16rem;
		}
		.offer-bar {
			position: sticky;
			bottom: calc(4.3rem + env(safe-area-inset-bottom));
			z-index: 5;
			display: flex;
			justify-content: space-between;
			align-items: center;
			padding: 0.55rem 0.85rem;
			border-radius: var(--radius);
			background: var(--brand);
			color: var(--brand-ink);
			font-size: 0.85rem;
			box-shadow: 0 4px 14px rgb(0 0 0 / 0.25);
		}
		.offer-bar:hover {
			text-decoration: none;
		}
		.offer-bar span {
			display: inline-flex;
			align-items: center;
			gap: 0.2rem;
		}
	}
</style>
