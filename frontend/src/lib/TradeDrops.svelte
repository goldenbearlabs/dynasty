<script lang="ts">
	import { getFranchise, type Franchise, type FranchiseDetail, type OfferItem, type TradeDrop } from '#lib/api.ts';

	type Props = {
		items: OfferItem[];
		franchises: Franchise[];
		owner: string;
		drops: TradeDrop[];
		onchange?: (ready: boolean) => void;
	};
	let { items, franchises, owner, drops = $bindable([]), onchange }: Props = $props();
	let details = $state<FranchiseDetail[]>([]);
	let error = $state('');
	let loading = $state(true);

	$effect(() => {
		const ids = [...new Set(items.flatMap(i => [i.from_franchise_id, i.to_franchise_id]))];
		let active = true;
		loading = true;
		Promise.all(ids.map(id => getFranchise(franchises.find(f => f.id === id)!.slug))).then(
			result => {
				if (active) { details = result; error = ''; loading = false; }
			},
			e => {
				if (active) { error = String(e); loading = false; }
			}
		);
		return () => { active = false; };
	});

	// Incoming players keep their current list. Picks and unsigned rights do
	// not consume main or reserve capacity.
	const groups = $derived.by(() => {
		const mine = details.find(d => d.franchise.id === owner);
		if (!mine) return [];
		return mine.rosters.flatMap(roster => {
			const leagueItems = items.filter(i => i.league_id === roster.league_id);
			if (!leagueItems.some(i => i.from_franchise_id === owner || i.to_franchise_id === owner)) return [];
			return (['main', 'reserve'] as const).flatMap(list => {
				const out = leagueItems.filter(i => i.from_franchise_id === owner &&
					roster.players.some(p => p.player_id === i.player_id && p.list === list)).length;
				const incoming = leagueItems.filter(i => i.to_franchise_id === owner &&
					details.find(d => d.franchise.id === i.from_franchise_id)?.rosters
						.find(r => r.league_id === roster.league_id)?.players
						.some(p => p.player_id === i.player_id && p.list === list)).length;
				const required = Math.max(0,
					roster.players.filter(p => p.list === list).length - out + incoming - roster.limits[list]);
				const candidates = roster.players.filter(p => p.list === list &&
					!leagueItems.some(i => i.player_id === p.player_id));
				return required ? [{ roster, list, required, candidates }] : [];
			});
		});
	});
	const chosen = (league: string, list: string) => drops.filter(d => d.league_id === league && d.list === list);

	$effect(() => {
		if (loading || error) { onchange?.(false); return; }
		const valid = drops.filter(d => groups.some(g => g.roster.league_id === d.league_id &&
			g.list === d.list && g.candidates.some(p => p.player_id === d.player_id)));
		if (valid.length !== drops.length) drops = valid;
		onchange?.(groups.every(g => chosen(g.roster.league_id, g.list).length === g.required));
	});

	function toggle(league: string, list: string, player: string, name: string) {
		if (drops.some(d => d.league_id === league && d.player_id === player)) {
			drops = drops.filter(d => d.league_id !== league || d.player_id !== player);
		} else {
			drops = [...drops, { franchise_id: owner, league_id: league, player_id: player, list, name }];
		}
	}
</script>

<div class="stack tight">
	{#if loading}
		<p class="muted">Checking roster space…</p>
	{:else if error}
		<p role="alert">{error}</p>
	{:else}
		{#each groups as group (group.roster.league_id + group.list)}
			<fieldset>
				<legend>{group.roster.name} · {group.list === 'main' ? 'Main roster' : 'Reserve list'}: select {group.required} to drop</legend>
				<p class="muted small-text">Selected {chosen(group.roster.league_id, group.list).length} of {group.required}</p>
				<div class="players">
					{#each group.candidates as player (player.player_id)}
						{@const checked = drops.some(d => d.league_id === group.roster.league_id && d.player_id === player.player_id)}
						<label class="row">
							<input type="checkbox" {checked}
								disabled={!checked && chosen(group.roster.league_id, group.list).length >= group.required}
								onchange={() => toggle(group.roster.league_id, group.list, player.player_id, player.full_name)} />
							{player.full_name}
						</label>
					{/each}
				</div>
			</fieldset>
		{:else}
			<p class="muted small-text">Your roster has room for this trade.</p>
		{/each}
		{#if groups.length}
			<p class="muted small-text">Selected players are released only when the trade completes. Normal waiver rules apply.</p>
		{/if}
	{/if}
</div>

<style>
	fieldset {
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
		padding: .75rem;
	}
	legend { font-weight: 600; }
	.players {
		max-height: 16rem;
		overflow-y: auto;
		display: grid;
		gap: .5rem;
		padding: .5rem 0;
	}
</style>
