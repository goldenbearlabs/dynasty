<script lang="ts">
	// The trade machine: choose a partner, tick what each side sends, see
	// what it does to both rosters, and send the offer.
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { getFranchise, proposeTrade, type FranchiseDetail, type List } from '#lib/api.ts';
	import AssetPicker from '#lib/AssetPicker.svelte';
	import { assetsOf, toItem, type Asset } from '#lib/tradeAssets.ts';
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

		return my.rosters.flatMap((roster, i) => {
			const league = roster.league_id;
			if (![...give, ...get].some((a) => a.leagueId === league)) return [];
			const sides: Side[] = [];
			for (const [detail, out, incoming] of [
				[my, give, get],
				[their, get, give]
			] as const) {
				for (const list of ['main', 'reserve'] as const) {
					const before = detail.rosters[i].players.filter((p) => p.list === list).length;
					const after = before - count(out, league, list) + count(incoming, league, list);
					if (after !== before) sides.push({ name: detail.franchise.name, list, before, after, max: roster.limits[list] });
				}
			}
			return [{ sport: roster.competition, league: roster.name, sides }];
		});
	});

	async function propose() {
		if (!me || !theirs) return;
		busy = true;
		try {
			await proposeTrade(note, [
				...give.map((a) => toItem(a, me.id, theirs!.franchise.id)),
				...get.map((a) => toItem(a, theirs!.franchise.id, me.id))
			]);
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
				<h2 class="row"><Crest name={me.name} size={28} /> You send</h2>
				{#if mine}<AssetPicker detail={mine} {franchises} selected={sending} />{:else}<p class="muted">Loading…</p>{/if}
			</section>
			<section class="stack tight">
				<h2 class="row">
					{#if theirs}<Crest name={theirs.franchise.name} size={28} /> {theirs.franchise.name} sends{:else}They send{/if}
				</h2>
				{#if theirs}<AssetPicker detail={theirs} {franchises} selected={receiving} />{:else}<p class="muted">Loading…</p>{/if}
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
						A side over its limit must make room before the trade can be accepted. You can still send the offer.
					</p>
				{/if}
			{/if}

			<label class="field">Note (optional) <input bind:value={note} placeholder="Say something to sell it" /></label>
			<div class="row">
				<button class="primary" disabled={busy || give.length + get.length === 0} onclick={propose}>Send offer</button>
				<a class="button quiet" href="/trades">Cancel</a>
			</div>
		</section>
	</div>
{/if}

<style>
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
