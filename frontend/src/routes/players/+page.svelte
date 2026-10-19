<script lang="ts">
 import PlayerSearch from '#lib/PlayerSearch.svelte';
	// The player pool. Signed-in managers can add free agents from here; a
	// commissioner can also place a player on any roster.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
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
		...data.competitions.map((c) => ({ value: c.key, label: c.name, count: c.players, sport: c.key }))
	]);

	const leagues = $derived(data.dynasty?.leagues ?? []);
	const leagueFor = (key: string) => leagues.find((l) => l.competition === key);

	// Commissioner controls: whose roster to add to, and whether to override the rules.
	let actingFor = $state(untrack(() => data.me?.id ?? ''));
	let override = $state(false);
	let version = $state(0);

	// An add to a full list of your own asks who to drop to make room.
	let making = $state<{ player: Player; list: List }>();
	const full = (player: Player, list: List) => {
		const roster = data.myTeam?.rosters.find((r) => r.competition === player.competition);
		if (!roster || override || actingFor !== data.me?.id) return false;
		return roster.players.filter((p) => p.list === list).length >= (list === 'reserve' ? roster.limits.reserve : roster.limits.main);
	};

	async function add(player: Player, list: List, drop?: string) {
		if (!drop && full(player, list)) {
			making = { player, list };
			return;
		}
		try {
			await changeRoster(leagueFor(player.competition)!.id, 'add', {
				player_id: player.id,
				list,
				drop_player_id: drop,
				franchise_id: actingFor,
				force: override
			});
			toast.good(drop ? `Added ${player.full_name}. The player you dropped is on waivers.` : `Added ${player.full_name}.`);
			version++;
			await invalidateAll(); // your rosters, and how full they are
		} catch (e) {
			toast.error(e);
		}
	}
</script>

<svelte:head><title>Players</title></svelte:head>

<div class="stack">
	<header class="stack tight">
		<h1>Players</h1>
		<Tabs {tabs} bind:value={competition} label="Sport" />
	</header>

	<PlayerSearch competitions={data.competitions} />

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

	<PlayerList {competition} leagueId={leagueFor(competition)?.id} {version} action={data.me ? action : undefined} />
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

{#snippet action(player: Player)}
	{#if player.waiver_until && !override}
		<a class="button small" href="/waivers?competition={player.competition}">Claim</a>
	{:else if leagueFor(player.competition)}
		<button class="small" onclick={() => add(player, 'main')}>Add</button>
		<button class="small quiet" onclick={() => add(player, 'reserve')}>Reserve</button>
	{/if}
{/snippet}

<style>
	.commissioner {
		padding: 0.7rem 1rem;
	}
</style>
