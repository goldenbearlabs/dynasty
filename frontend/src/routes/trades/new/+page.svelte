<script lang="ts">
	// The trade machine: choose a partner, tick what each side sends, see
	// what it does to both rosters, and send the offer.
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { getFranchise, proposeTrade, type FranchiseDetail, type List, type TradeDrop } from '#lib/api.ts';
	import TradeDrops from '#lib/TradeDrops.svelte';
	import AssetPicker from '#lib/AssetPicker.svelte';
	import { assetsOf, toItem, type Asset } from '#lib/tradeAssets.ts';
	import { tradeMarket } from '#lib/tradeMarket.svelte.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const me = $derived(data.me);
	const franchises = $derived(data.dynasty?.franchises ?? []);
	const others = $derived(franchises.filter((f) => f.id !== me?.id));

	// The partner can be preselected: /trades/new?with=their-slug
	let partnerSlug = $state(untrack(() => page.url.searchParams.get('with') ?? others[0]?.slug ?? ''));
	let mine = $state<FranchiseDetail>();
	let theirs = $state<FranchiseDetail>();
	const sending = new SvelteSet<string>();
	const receiving = new SvelteSet<string>();
	let note = $state('');
	let busy = $state(false);
	let drops = $state<TradeDrop[]>([]);
	let dropsReady = $state(false);

	$effect(() => {
		if (me) getFranchise(me.slug).then((d) => (mine = d), toast.error);
	});
	$effect(() => {
		const slug = partnerSlug;
		theirs = undefined;
		receiving.clear();
		if (slug) getFranchise(slug).then((d) => slug === partnerSlug && (theirs = d), toast.error);
	});

	const chosen = (detail: FranchiseDetail | undefined, keys: SvelteSet<string>): Asset[] =>
		detail ? assetsOf(detail, franchises).filter((a) => keys.has(a.key)) : [];
	const give = $derived(chosen(mine, sending));
	const get = $derived(chosen(theirs, receiving));

	// What the trade does to each roster it touches, league by league. An
	// arriving player keeps the list he was on.
	type Side = { name: string; list: List; before: number; after: number; max: number };
	const impact = $derived.by(() => {
		const [my, their] = [mine, theirs];
		if (!my || !their) return [];
		const count = (assets: Asset[], league: string, list: List) =>
			assets.filter((a) => a.leagueId === league && a.list === list).length;

		return my.rosters.flatMap((roster) => {
			const league = roster.league_id;
			if (![...give, ...get].some((a) => a.leagueId === league)) return [];
			const sides: Side[] = [];
			for (const [detail, out, incoming] of [
				[my, give, get],
				[their, get, give]
			] as const) {
				for (const list of ['main', 'reserve'] as const) {
					const before = detail.rosters.find(r => r.league_id === league)!.players.filter((p) => p.list === list).length;
					const after = before - count(out, league, list) + count(incoming, league, list);
					if (after !== before) sides.push({ name: detail.franchise.name, list, before, after, max: roster.limits[list] });
				}
			}
			return [{ sport: roster.competition, league: roster.name, sides }];
		});
	});

	const offerItems = $derived(me && theirs ? [
	...give.map(a => toItem(a, me.id, theirs!.franchise.id)),
	...get.map(a => toItem(a, theirs!.franchise.id, me.id))
	] : []);
	async function propose() {
		if (!me || !theirs) return;
		busy = true;
		try {
			await proposeTrade(note, offerItems, drops);
			await invalidateAll();
			toast.good(`Offer sent to ${theirs.franchise.name}.`);
			await goto('/trades');
		} catch (e) {
			toast.error(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>New trade</title></svelte:head>

{#if !me}
	<p role="alert">Sign in to propose a trade.</p>
{:else if others.length === 0}
	<p class="muted">There is nobody to trade with yet.</p>
{:else}
	<div class="stack">
		<header class="spread">
			<h1>New trade</h1>
			<label class="row">
				<span class="muted">Trade with</span>
				<select bind:value={partnerSlug}>
					{#each others as f (f.id)}<option value={f.slug}>{f.name}</option>{/each}
				</select>
			</label>
		</header>

		<div class="sides">
			<section class="stack tight">
				<h2 class="row"><Crest src={me.image_url} name={me.name} size={28} /> You send</h2>
				{#if mine}<AssetPicker detail={mine} {franchises} selected={sending} counterpart={theirs?.franchise.id} />{:else}<p class="muted">Loading…</p>{/if}
			</section>
			<section class="stack tight">
				<h2 class="row">
					{#if theirs}<Crest src={theirs.franchise.image_url} name={theirs.franchise.name} size={28} /> {theirs.franchise.name} sends{:else}They send{/if}
				</h2>
				{#if theirs}
					{@const wants = tradeMarket.data.wants.filter((w) => w.franchise_id === theirs!.franchise.id)}
					{#if wants.length}
						<p class="wants small-text">
							<strong>Looking for</strong>
							{#each wants as w (w.league_id)}
								<span><SportBadge sport={w.competition} /> {w.positions.join(', ')}{#if w.note}{' '}“{w.note}”{/if}</span>
							{/each}
						</p>
					{/if}
					<AssetPicker detail={theirs} {franchises} selected={receiving} />
				{:else}<p class="muted">Loading…</p>{/if}
			</section>
		</div>

		<section class="card stack">
			<h2>The offer</h2>
			<p class="muted">
				{#if give.length + get.length === 0}
					Tick players and picks above. They can come from any sport.
				{:else}
					You send {give.length} and get {get.length}.
				{/if}
			</p>

			{#if impact.length > 0}
				<ul class="impact">
					{#each impact as league (league.league)}
						<li>
							<SportBadge sport={league.sport} solid />
							<div class="changes">
								{#each league.sides as side (side.name + side.list)}
									<span class:over={side.after > side.max}>
										{side.name} {side.list}: {side.before} → <strong>{side.after}</strong> of {side.max}
										{#if side.after > side.max}(over the limit){/if}
									</span>
								{/each}
							</div>
						</li>
					{/each}
				</ul>
				{#if impact.some((l) => l.sides.some((s) => s.after > s.max))}
					<p class="muted small-text">
						Each manager chooses drops from their own overfull list. Your partner will choose theirs when accepting.
					</p>
				{/if}
			{/if}

			{#if offerItems.length}<TradeDrops items={offerItems} {franchises} owner={me.id} bind:drops onchange={ready => dropsReady = ready} />{/if}

			<label class="field">Note (optional) <input bind:value={note} placeholder="Say something to sell it" /></label>
			<div class="row">
				<button class="primary" disabled={busy || give.length + get.length === 0 || !dropsReady} onclick={propose}>Send offer</button>
				<a class="button quiet" href="/trades">Cancel</a>
			</div>
		</section>
	</div>
{/if}

<style>
	.wants {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem 0.8rem;
		padding: 0.5rem 0.7rem;
		border-radius: var(--radius-small);
		background: var(--surface-2);
	}
	.sides {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 20rem), 1fr));
		gap: 1.25rem;
		align-items: start;
	}
	h2.row {
		gap: 0.55rem;
	}
	.impact {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.5rem;
	}
	.impact li {
		display: flex;
		align-items: baseline;
		gap: 0.7rem;
	}
	.changes {
		display: flex;
		flex-wrap: wrap;
		gap: 0.2rem 1.2rem;
		font-variant-numeric: tabular-nums;
	}
	.over {
		color: var(--bad);
	}
</style>
