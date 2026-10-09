<script lang="ts">
	// One league's waivers: who is on them and until when, the manager's own
	// claims, and the order claims are settled in.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { cancelWaiverClaim, claimWaiver, getWaivers, type Waivers } from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { clockTime } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const leagues = $derived((data.dynasty?.leagues ?? []).filter((l) => l.settings.waivers.mode !== 'none'));
	const tabs = $derived(leagues.map((l) => ({ value: l.competition, label: l.name, sport: l.competition })));
	let competition = $state(untrack(() => page.url.searchParams.get('competition') ?? ''));
	const league = $derived(leagues.find((l) => l.competition === competition) ?? leagues[0]);

	let waivers = $state<Waivers>();
	let error = $state('');
	async function load(leagueId: string) {
		try {
			waivers = await getWaivers(leagueId);
			error = '';
		} catch (e) {
			error = (e as Error).message;
		}
	}
	$effect(() => {
		if (league) load(league.id);
	});

	const faab = $derived(waivers?.rules.mode === 'faab');
	const mine = $derived(waivers?.standings.find((s) => s.franchise_id === data.me?.id));
	const pending = (playerId: string) => waivers?.claims.find((c) => c.status === 'pending' && c.player_id === playerId);
	const settled = $derived(waivers?.claims.filter((c) => c.status !== 'pending') ?? []);
	// Who the manager could release to make room.
	const roster = $derived(data.myTeam?.rosters.find((r) => r.league_id === league?.id)?.players ?? []);

	// What the manager has entered for each player, before claiming.
	let bids = $state<Record<string, number>>({});
	let drops = $state<Record<string, string>>({});

	async function act(change: Promise<void>, done: string) {
		try {
			await change;
			toast.good(done);
			await load(league!.id);
		} catch (e) {
			toast.error(e);
		}
	}
	const claim = (playerId: string, name: string) =>
		act(
			claimWaiver(league!.id, { player_id: playerId, bid: bids[playerId] ?? 0, drop_player_id: drops[playerId] || undefined }),
			`Claim in for ${name}.`
		);
</script>

<svelte:head><title>Waivers</title></svelte:head>

<div class="stack">
	<header class="stack tight">
		<h1>Waivers</h1>
		{#if leagues.length > 0}<Tabs {tabs} label="League" bind:value={() => league.competition, (v) => (competition = v)} />{/if}
	</header>

	{#if leagues.length === 0}
		<Empty icon="clock" title="No league uses waivers">A commissioner can turn them on in a league's rules.</Empty>
	{:else if error}
		<p role="alert">Could not load waivers: {error}</p>
	{:else if waivers}
		<p class="row muted small-text">
			<span>
				A dropped player is on waivers for {waivers.rules.days}
				{waivers.rules.days === 1 ? 'day' : 'days'}, then goes to
				{faab ? 'the highest bid; the waiver order breaks ties' : 'the claim highest in the waiver order'}.
			</span>
			{#if data.me && waivers.weekly_limit > 0}
				<span class="pill" class:bad={waivers.acquisitions >= waivers.weekly_limit}>
					{waivers.acquisitions} of {waivers.weekly_limit} acquisitions this week
				</span>
			{/if}
			{#if faab && mine}<span class="pill gold">{mine.budget_left} left to bid</span>{/if}
		</p>

		<section class="panel" data-sport={league.competition}>
			<h2 class="eyebrow">On waivers</h2>
			{#if waivers.players.length === 0}
				<p class="muted small-text none">Nobody is on waivers.</p>
			{:else}
				<div class="scroll">
					<table>
						<tbody>
							{#each waivers.players as player (player.player_id)}
								{@const live = pending(player.player_id)}
								<tr>
									<td>
										<div class="player">
											<Headshot name={player.full_name} src={player.headshot_url} />
											<div>
												<strong>{player.full_name}</strong>
												<div class="muted small-text">
													{[player.positions.join('/'), player.team_abbrev, `until ${clockTime(player.clears_at)}`]
														.filter(Boolean)
														.join(' · ')}
												</div>
											</div>
										</div>
									</td>
									{#if data.me}
										<td class="actions">
											{#if live}
												<span class="pill brand">Claimed{faab ? ` for ${live.bid}` : ''}</span>
												<button class="small quiet danger" onclick={() => act(cancelWaiverClaim(live.id), 'Claim withdrawn.')}>
													Withdraw
												</button>
											{:else}
												{#if faab}
													<input type="number" min="0" max={mine?.budget_left} placeholder="Bid" aria-label="Bid" bind:value={bids[player.player_id]} />
												{/if}
												<select aria-label="Player to drop" bind:value={drops[player.player_id]}>
													<option value="">Drop nobody</option>
													{#each roster as held (held.player_id)}
														<option value={held.player_id}>Drop {held.full_name}</option>
													{/each}
												</select>
												<button class="small primary" onclick={() => claim(player.player_id, player.full_name)}>Claim</button>
											{/if}
										</td>
									{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<div class="columns">
			<section class="panel">
				<h2 class="eyebrow">Waiver order</h2>
				<ol>
					{#each waivers.standings as s (s.franchise_id)}
						<li class="spread">
							<span><span class="muted">{s.priority}.</span> <a href="/franchise/{s.slug}">{s.name}</a></span>
							{#if faab}<span class="muted small-text">{s.budget_left} left</span>{/if}
						</li>
					{/each}
				</ol>
			</section>

			{#if data.me}
				<section class="panel">
					<h2 class="eyebrow">Your settled claims</h2>
					{#if settled.length === 0}
						<p class="muted small-text none">None yet.</p>
					{:else}
						<ol>
							{#each settled as c (c.id)}
								<li>
									<div class="spread">
										<strong>{c.player_name}</strong>
										<span class="pill" class:good={c.status === 'won'}>{c.status}{faab && c.bid ? ` · ${c.bid}` : ''}</span>
									</div>
									{#if c.reason}<div class="muted small-text">{c.reason}</div>{/if}
								</li>
							{/each}
						</ol>
					{/if}
				</section>
			{/if}
		</div>
	{/if}
</div>

<style>
	h2 {
		padding: 0.5rem 1rem;
		background: var(--surface-2);
		border-bottom: 1px solid var(--rule);
	}
	.none {
		padding: 0.65rem 1rem;
	}
	td:first-child {
		padding-left: 1rem;
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.actions select {
		max-width: 11rem;
	}
	.actions input {
		width: 4.5rem;
	}
	@media (max-width: 640px) {
		tr {
			display: grid;
		}
		td.actions {
			display: flex;
			flex-wrap: wrap;
			gap: 0.4rem;
			padding: 0 1rem 0.7rem;
			text-align: left;
			white-space: normal;
		}
		td.actions > * {
			margin: 0;
		}
		tr td:first-child {
			border-bottom: none;
		}
	}
	.columns {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
		gap: 1rem;
		align-items: start;
	}
	ol {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		padding: 0.55rem 1rem;
		border-bottom: 1px solid var(--rule);
	}
	li:last-child {
		border-bottom: none;
	}
</style>
