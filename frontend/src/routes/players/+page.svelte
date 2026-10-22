<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import { dropPlayer } from '#lib/dropPlayer.ts';
	// The player pool. Signed-in managers can add free agents from here; a
	// commissioner can also place a player on any roster.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { snapshot, invalidateAll } from '$app/navigation';
	import { changeRoster, type List, type Player } from '#lib/api.ts';
	import DropToAdd from '#lib/DropToAdd.svelte';
	import PlayerList from '#lib/PlayerList.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let competition = $state(untrack(() => page.url.searchParams.get('competition') ?? ''));
	const tabs = $derived([
		{ value: '', label: 'All sports' },
		...data.competitions.map((c) => ({
			value: c.key,
			label: c.name,
			count: c.players,
			sport: c.key,
		})),
	]);

	const leagues = $derived(data.dynasty?.leagues ?? []);
	const leagueFor = (key: string) => leagues.find((l) => l.competition === key);

	// Commissioner controls: whose roster to add to, and whether to override the rules.
	let actingFor = $state(untrack(() => data.me?.id ?? ''));
	let override = $state(false);
	let version = $state(0);
	let busy = $state('');
	let adding = $state<Player>();
	let addDialog = $state<HTMLDialogElement>();
	function chooseAdd(player: Player) {
		adding = player;
		addDialog?.showModal();
	}
	async function drop(player: Player) {
		const league = leagueFor(player.competition);
		if (!league || !data.me || busy) return;
		busy = player.id;
		try {
			if (await dropPlayer(league, data.me.id, player.id, player.full_name)) {
				version++;
				await invalidateAll();
			}
		} finally {
			busy = '';
		}
	}

	// An add to a full list of your own asks who to drop to make room.
	let making = $state<{ player: Player; list: List }>();
	const full = (player: Player, list: List) => {
		const roster = data.myTeam?.rosters.find((r) => r.competition === player.competition);
		if (!roster || override || actingFor !== data.me?.id) return false;
		return (
			roster.players.filter((p) => p.list === list).length >=
			(list === 'reserve' ? roster.limits.reserve : roster.limits.main)
		);
	};

	async function add(player: Player, list: List, drop?: string) {
		if (!drop && full(player, list)) {
			making = { player, list };
			return;
		}
		if (busy) return;
		busy = player.id;
		try {
			await changeRoster(leagueFor(player.competition)!.id, 'add', {
				player_id: player.id,
				list,
				drop_player_id: drop,
				franchise_id: actingFor,
				force: override,
			});
			toast.good(
				drop
					? `Added ${player.full_name}. The player you dropped is on waivers.`
					: `Added ${player.full_name}.`,
			);
			version++;
			await invalidateAll(); // your rosters, and how full they are
		} catch (e) {
			toast.error(e);
		} finally {
			busy = '';
		}
	}
	snapshot({
		id: 'players-sport',
		capture: () => competition,
		restore: (value) => (competition = value),
	});
</script>

<svelte:head><title>Players</title></svelte:head>

<div class="stack">
	<header class="stack tight">
		<h1>Players</h1>
		<p class="muted small-text">Season stats, fantasy value, and roster moves in one place.</p>
		<Tabs {tabs} bind:value={competition} label="Sport" />
	</header>

	{#if data.me?.is_commissioner && data.dynasty}
		<div class="card row commissioner">
			<span class="pill gold">Commissioner</span>
			<label class="row">
				<span class="muted small-text">Add to</span>
				<select bind:value={actingFor}>
					{#each data.dynasty.franchises as f (f.id)}
						<option value={f.id}>{f.name}</option>
					{/each}
				</select>
			</label>
			<label class="check">
				<input type="checkbox" bind:checked={override} />
				Override free agency rules and roster limits
			</label>
		</div>
	{/if}

	<PlayerList
		workspace
		compact
		statColumns
		byPoints
		competitions={data.competitions}
		franchises={data.dynasty?.franchises ?? []}
		{competition}
		leagueId={leagueFor(competition)?.id}
		{version}
		action={data.me ? action : undefined}
	/>
</div>

{#if making}
	{@const roster = data.myTeam?.rosters.find((r) => r.competition === making!.player.competition)}
	{@const league = leagueFor(making.player.competition)}
	{#if roster && league}
		<DropToAdd
			{roster}
			{league}
			list={making.list}
			adding={making.player.full_name}
			onconfirm={(drop) => {
				const { player, list } = making!;
				making = undefined;
				add(player, list, drop);
			}}
			oncancel={() => (making = undefined)}
		/>
	{/if}
{/if}

<dialog bind:this={addDialog} aria-label="Add player">
	{#if adding}
		<div class="stack">
			<div class="spread">
				<h2>Add {adding.full_name}</h2>
				<button class="quiet" aria-label="Close" onclick={() => addDialog?.close()}
					><Icon name="x" /></button
				>
			</div>
			<p class="muted small-text">
				Choose where to add this player. League eligibility and roster limits apply.
			</p>
			<div class="row">
				<button
					class="primary"
					onclick={() => {
						addDialog?.close();
						add(adding!, 'main');
					}}>Add to roster</button
				><button
					onclick={() => {
						addDialog?.close();
						add(adding!, 'reserve');
					}}>Add to reserve</button
				>
			</div>
		</div>
	{/if}
</dialog>

{#snippet action(player: Player)}
	{#if player.owner_slug === data.me?.slug}
		<button
			class="roster-action drop"
			disabled={!!busy}
			aria-label="Drop {player.full_name}"
			title="Drop {player.full_name}"
			onclick={() => drop(player)}><span aria-hidden="true">−</span></button
		>
	{:else if player.owner_slug}
		<a
			class="button roster-action trade"
			aria-label="Trade for {player.full_name}"
			title="Trade for {player.full_name}"
			href="/trades/new?with={player.owner_slug}&player={player.id}"
			><Icon name="trade" size={18} /></a
		>
	{:else if player.waiver_until && !override}
		<a
			class="button roster-action add"
			aria-label="Claim {player.full_name} on waivers"
			title="Claim on waivers"
			href="/waivers?competition={player.competition}"><Icon name="plus" size={18} /></a
		>
	{:else if leagueFor(player.competition)}
		<button
			class="roster-action add"
			disabled={!!busy}
			aria-label="Add {player.full_name}"
			title="Add {player.full_name}"
			onclick={() => chooseAdd(player)}><Icon name="plus" size={18} /></button
		>
	{/if}
{/snippet}

<style>
	.roster-action {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.5rem;
		height: 2.5rem;
		padding: 0;
		border-radius: 8px;
		border: 1px solid currentColor;
	}
	.add {
		color: var(--good, #16834a);
		background: color-mix(in srgb, var(--good, #16834a) 10%, var(--surface));
	}
	.drop {
		color: var(--bad);
		background: color-mix(in srgb, var(--bad) 10%, var(--surface));
		font-size: 1.5rem;
	}
	.trade {
		color: #357cda;
		background: color-mix(in srgb, #357cda 10%, var(--surface));
	}
	dialog {
		width: min(28rem, calc(100vw - 2rem));
		padding: 1.25rem;
		border: 1px solid var(--rule);
		border-radius: 12px;
		background: var(--surface);
		color: var(--ink);
	}
	dialog::backdrop {
		background: #0007;
	}
	dialog h2 {
		font-size: 1.1rem;
	}

	.commissioner {
		padding: 0.7rem 1rem;
	}
</style>
