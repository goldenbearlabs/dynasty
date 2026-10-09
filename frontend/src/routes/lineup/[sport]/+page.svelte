<script lang="ts">
	// Set who starts. Each main-roster player gets a slot or the bench; the
	// lineup then stays in force until it is changed again.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getLineup, setLineup, type Lineup, type LineupGame, type LineupPlayer } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { dayLabel, points, shiftDay, timeOfDay } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const sport = $derived(page.params.sport!);
	const league = $derived(data.dynasty?.leagues.find((l) => l.competition === sport));
	// Whose lineup: ?franchise=<slug>, or the signed-in manager's.
	const franchise = $derived(
		data.dynasty?.franchises.find((f) => f.slug === (page.url.searchParams.get('franchise') ?? data.me?.slug))
	);
	const day = $derived(page.url.searchParams.get('day') ?? undefined);

	const mine = $derived(franchise !== undefined && franchise.id === data.me?.id);
	const canEdit = $derived(mine || data.me?.is_commissioner === true);
	let override = $state(false); // the commissioner's override of the locks

	let lineup = $state<Lineup>();
	// The slot chosen for each player in the form; '' is the bench.
	let chosen = $state<Record<string, string>>({});

	function show(loaded: Lineup) {
		lineup = loaded;
		chosen = Object.fromEntries(loaded.players.map((p) => [p.player_id, p.slot]));
	}
	$effect(() => {
		if (league && franchise) getLineup(league.id, franchise.id, day).then(show, toast.error);
	});

	// Points and locks follow the games, unless there are unsaved changes to keep.
	onScoresChange(
		() => [sport],
		() => {
			if (league && franchise && lineup && !dirty) getLineup(league.id, franchise.id, lineup.day).then(show, toast.error);
		}
	);

	const weekly = $derived(lineup !== undefined && lineup.day !== lineup.last_day);
	const dirty = $derived(lineup?.players.some((p) => chosen[p.player_id] !== p.slot) ?? false);
	const filled = (slot: string) => Object.values(chosen).filter((s) => s === slot).length;
	const fits = (player: LineupPlayer, positions: string[]) =>
		positions.includes('*') || player.positions.some((p) => positions.includes(p));
	const frozen = (player: LineupPlayer) => !canEdit || ((lineup!.locked !== '' || player.locked) && !override);

	// Starters first, in slot order, then the bench.
	const ordered = $derived.by(() => {
		if (!lineup) return [];
		const rank = (p: LineupPlayer) => {
			const i = lineup!.slots.findIndex((s) => s.name === chosen[p.player_id]);
			return i < 0 ? lineup!.slots.length : i;
		};
		return lineup.players.toSorted((a, b) => rank(a) - rank(b) || a.full_name.localeCompare(b.full_name));
	});

	function go(to: string | undefined) {
		const query = new URLSearchParams(page.url.search);
		if (to) query.set('day', to);
		else query.delete('day');
		goto(`?${query}`);
	}

	async function save() {
		if (!league || !franchise || !lineup) return;
		try {
			await setLineup(league.id, {
				day: lineup.day,
				entries: Object.entries(chosen)
					.filter(([, slot]) => slot !== '')
					.map(([player_id, slot]) => ({ slot, player_id })),
				franchise_id: franchise.id,
				force: override
			});
			show(await getLineup(league.id, franchise.id, lineup.day));
			toast.good('Lineup saved. It stays in force until you change it.');
		} catch (e) {
			toast.error(e);
		}
	}

	function describe(game: LineupGame): string {
		const versus = `${game.at_home ? 'vs' : '@'} ${game.opponent}`;
		if (game.status === 'live') return `${versus} · live`;
		if (game.status === 'final') return `${versus} · final`;
		const when = weekly ? `${dayLabel(game.starts_at.slice(0, 10)).split(',')[0]} ` : '';
		return `${versus} · ${when}${timeOfDay(game.starts_at)}`;
	}
</script>

<svelte:head><title>Lineup</title></svelte:head>

{#if !league || !franchise}
	<Empty icon="shield" title="No lineup to show"><a href="/">Sign in</a> to set your lineup.</Empty>
{:else}
	<div class="stack" data-sport={sport}>
		<header class="spread">
			<div>
				<p class="row eyebrow"><SportBadge {sport} solid /> {franchise.name}</p>
				<h1>{league.name} lineup</h1>
			</div>
			{#if lineup}
				<nav class="row days" aria-label="Day">
					<button aria-label="Earlier" onclick={() => go(shiftDay(lineup!.day, -1))}><Icon name="left" size={16} /></button>
					<strong class="when">
						{weekly ? `${dayLabel(lineup.day)} – ${dayLabel(lineup.last_day)}` : dayLabel(lineup.day)}
					</strong>
					<button aria-label="Later" onclick={() => go(shiftDay(lineup!.last_day, 1))}><Icon name="right" size={16} /></button>
					<button class="quiet" onclick={() => go(undefined)}>Today</button>
				</nav>
			{/if}
		</header>

		{#if !lineup}
			<p class="muted">Loading…</p>
		{:else if lineup.players.length === 0}
			<Empty icon="players" title="Nobody on the main roster">
				Only main-roster players can start. <a href="/players?competition={sport}">Find players</a>.
			</Empty>
		{:else}
			{#if lineup.locked}<p class="card note"><Icon name="lock" size={16} /> {lineup.locked}</p>{/if}

			<div class="row slots">
				{#each lineup.slots as slot (slot.name)}
					<span class="pill" class:bad={filled(slot.name) > slot.count} class:good={filled(slot.name) === slot.count}>
						{slot.name} {filled(slot.name)}/{slot.count}
					</span>
				{/each}
				{#if data.me?.is_commissioner}
					<label class="check override">
						<input type="checkbox" bind:checked={override} />
						<span class="pill gold">Commissioner</span> Override locks
					</label>
				{/if}
			</div>

			<div class="card flush scroll">
				<table>
					<thead>
						<tr><th>Slot</th><th>Player</th><th class="wide">{weekly ? 'Games' : 'Game'}</th><th class="num">Points</th></tr>
					</thead>
					<tbody>
						{#each ordered as player (player.player_id)}
							<tr class:bench={!chosen[player.player_id]}>
								<td class="slot">
									<select aria-label="Slot for {player.full_name}" bind:value={chosen[player.player_id]} disabled={frozen(player)}>
										<option value="">Bench</option>
										{#each lineup.slots.filter((s) => fits(player, s.positions)) as slot (slot.name)}
											<option value={slot.name}>{slot.name}</option>
										{/each}
									</select>
								</td>
								<td>
									<div class="player">
										<Headshot name={player.full_name} src={player.headshot_url} />
										<div>
											<strong>{player.full_name}</strong>
											{#if player.locked}<span class="pill"><Icon name="lock" size={11} /> Locked</span>{/if}
											<div class="muted small-text">
												{player.positions.join('/')} · {player.team_abbrev}
												<span class="narrow">{player.games.map(describe).join(', ')}</span>
											</div>
										</div>
									</div>
								</td>
								<td class="wide muted small-text">
									{#each player.games as game (game.starts_at)}<div>{describe(game)}</div>{:else}No game{/each}
								</td>
								<td class="num total">{player.games.length > 0 ? points(player.points) : ''}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			{#if canEdit}
				<div class="row">
					<button class="primary" disabled={!dirty} onclick={save}>Save lineup</button>
					{#if dirty}<button class="quiet" onclick={() => show(lineup!)}>Discard changes</button>{/if}
					<span class="muted small-text">A lineup stays in force until you change it.</span>
				</div>
			{/if}
		{/if}
	</div>
{/if}

<style>
	.eyebrow {
		gap: 0.5rem;
		margin-bottom: 0.35rem;
	}
	.days {
		gap: 0.4rem;
	}
	.days button {
		padding: 0.45rem 0.6rem;
	}
	.when {
		min-width: 8.5rem;
		text-align: center;
	}
	.note {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.7rem 1rem;
	}
	.slots {
		gap: 0.35rem;
	}
	.override {
		margin-left: auto;
	}
	.slot {
		width: 7rem;
	}
	.slot select {
		width: 100%;
	}
	tr.bench td {
		background: var(--surface-2);
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.player .pill {
		margin-left: 0.3rem;
	}
	.total {
		font: 700 1rem var(--display);
	}
	.narrow {
		display: none;
	}
	@media (max-width: 640px) {
		.wide {
			display: none;
		}
		.narrow {
			display: block;
		}
	}
</style>
