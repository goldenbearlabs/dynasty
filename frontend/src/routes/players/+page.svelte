<script lang="ts">
	// The player pool. Signed-in managers can add free agents from here; a
	// commissioner can also place a player on any roster.
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { changeRoster, type List, type Player } from '#lib/api.ts';
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

	async function add(player: Player, list: List) {
		try {
			await changeRoster(leagueFor(player.competition)!.id, 'add', {
				player_id: player.id,
				list,
				franchise_id: actingFor,
				force: override
			});
			toast.good(`Added ${player.full_name}.`);
			version++;
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
