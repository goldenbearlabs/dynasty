<script lang="ts">
	// One franchise's main roster and reserve list in every league, and its
	// draft picks.
	import { invalidateAll } from '$app/navigation';
	import { changeRoster, type LeagueRoster, type List, type RosterPlayer } from '#lib/api.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import TradeAsset from '#lib/TradeAsset.svelte';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { clockTime } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const mine = $derived(data.me?.id === data.franchise.id);
	const canEdit = $derived(mine || data.me?.is_commissioner === true);
	const lists: { key: List; title: string; other: List; move: string }[] = [
		{ key: 'main', title: 'Main roster', other: 'reserve', move: 'To reserve' },
		{ key: 'reserve', title: 'Reserve list', other: 'main', move: 'To main' }
	];
	const on = (roster: LeagueRoster, list: List) => roster.players.filter((p) => p.list === list);
	const franchiseName = (id: string) => data.dynasty?.franchises.find((f) => f.id === id)?.name ?? '';

	// A commissioner editing someone else's roster is overriding the rules.
	async function change(roster: LeagueRoster, action: 'drop' | 'move', player: RosterPlayer, list?: List) {
		const days = roster.limits.reserve_lock_days;
		if (mine && list === 'reserve' && days > 0 && player.status !== 'prospect') {
			if (!confirm(`${player.full_name} will be locked on the reserve list for ${days} ${days === 1 ? 'day' : 'days'}. Move him?`)) return;
		}
		try {
			await changeRoster(roster.league_id, action, {
				player_id: player.player_id,
				list,
				franchise_id: data.franchise.id,
				force: !mine
			});
			await invalidateAll();
			toast.good(action === 'drop' ? `Dropped ${player.full_name}.` : `Moved ${player.full_name}.`);
		} catch (e) {
			toast.error(e);
		}
	}
</script>

<svelte:head><title>{data.franchise.name}</title></svelte:head>

<div class="stack">
	<header class="head">
		<Crest name={data.franchise.name} size={64} />
		<div class="grow">
			<h1>{data.franchise.name}</h1>
			<div class="row muted">
				Managed by {data.franchise.manager_name}
				{#if mine}<span class="pill brand">You</span>{/if}
				{#if data.franchise.is_commissioner}<span class="pill gold">Commissioner</span>{/if}
			</div>
		</div>
		{#if data.me && !mine}
			<a class="button primary" href="/trades/new?with={data.franchise.slug}"><Icon name="trade" size={16} /> Propose a trade</a>
		{:else if mine}
			<a class="button" href="/trades/new"><Icon name="trade" size={16} /> New trade</a>
		{/if}
	</header>

	{#if canEdit && !mine}
		<p class="card small-text"><span class="pill gold">Commissioner</span> Changes you make here skip the roster rules.</p>
	{/if}

	{#each data.rosters as roster (roster.league_id)}
			<section class="panel" id={roster.competition} data-sport={roster.competition}>
			<div class="league">
				<div class="row">
					<SportBadge sport={roster.competition} solid />
					<h2>{roster.name}</h2>
					{#if roster.overage > 0}<span class="pill bad">Over the limit by {roster.overage}</span>{/if}
					<a class="button small" href="/lineup/{roster.competition}?franchise={data.franchise.slug}">
						{canEdit ? 'Set lineup' : 'See lineup'}
					</a>
				</div>
				<div class="meters">
					<Meter label="Main" value={on(roster, 'main').length} max={roster.limits.main} />
					<Meter label="Reserve" value={on(roster, 'reserve').length} max={roster.limits.reserve} />
				</div>
			</div>

			{#each lists as list (list.key)}
				{@const players = on(roster, list.key)}
				<h3 class="eyebrow list">{list.title}</h3>
				{#if players.length === 0}
					<p class="muted small-text none">Nobody yet.</p>
				{:else}
					<div class="scroll">
						<table>
							<tbody>
								{#each players as player (player.player_id)}
									<tr>
										<td>
											<div class="player">
												<Headshot name={player.full_name} src={player.headshot_url} />
												<div>
													<div class="row name">
														<strong>{player.full_name}</strong>
														{#if player.class}<span class="pill">{player.class}</span>{/if}
														{#if player.status === 'prospect'}<span class="pill gold">Prospect</span>{/if}
														{#if player.status === 'inactive'}<span class="pill">Inactive</span>{/if}
														{#if player.locked_until}<span class="pill">Locked until {clockTime(player.locked_until)}</span>{/if}
													</div>
													{#if player.note}<div class="muted small-text">{player.note}</div>{/if}
												</div>
											</div>
										</td>
										<td class="pos">{player.positions.join('/')}</td>
										<td class="team muted">{player.team_abbrev}</td>
										{#if canEdit}
											<td class="actions">
												<button class="small" disabled={mine && !!player.locked_until} onclick={() => change(roster, 'move', player, list.other)}>{list.move}</button>
												<button class="small quiet danger" onclick={() => change(roster, 'drop', player)}>Drop</button>
											</td>
										{/if}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			{/each}

			{#if mine}
				<p class="find small-text"><a href="/players?competition={roster.competition}">Find {roster.name} players to add</a></p>
			{/if}
		</section>
	{/each}

	<section class="panel" id="picks">
		<div class="league">
			<div class="row">
				<span class="pill gold">Picks</span>
				<h2>Draft picks</h2>
			</div>
		</div>
		{#if data.picks.length === 0}
			<p class="muted small-text none">No picks in upcoming drafts.</p>
		{:else}
			<ul class="picks">
				{#each data.picks as pick (pick.id)}
					<li>
						<TradeAsset
							draft={pick.draft_name}
							round={pick.round}
							sport={pick.competitions.length === 1 ? pick.competitions[0] : ''}
							via={pick.original_franchise_id !== data.franchise.id ? franchiseName(pick.original_franchise_id) : ''}
						/>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</div>

<style>
	.head {
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.head .row {
		gap: 0.4rem 0.6rem;
		margin-top: 0.3rem;
	}
	.head {
		flex-wrap: wrap;
	}
	.grow {
		flex: 1;
	}
	.picks {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		border-top: 1px solid var(--rule);
	}
	.picks li {
		padding: 0.7rem 1.2rem;
		border-bottom: 1px solid var(--rule);
	}

	section {
		scroll-margin-top: 1rem;
	}
	.league {
		display: grid;
		grid-template-columns: 1fr minmax(0, 22rem);
		align-items: center;
		gap: 0.7rem 1.5rem;
		padding: 0.75rem 1rem;
	}
	.meters {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1.2rem;
	}
	@media (max-width: 640px) {
		.league {
			grid-template-columns: 1fr;
		}
	}

	.list {
		padding: 0.4rem 1rem;
		background: var(--surface-2);
		border-block: 1px solid var(--rule);
	}
	.none,
	.find {
		padding: 0.65rem 1rem;
	}
	.find {
		border-top: 1px solid var(--rule);
	}
	td:first-child {
		padding-left: 1rem;
	}
	.player {
		display: flex;
		align-items: center;
		gap: 0.7rem;
	}
	.name {
		gap: 0.25rem 0.45rem;
	}
	.pos,
	.team {
		width: 4.5rem;
	}
</style>
