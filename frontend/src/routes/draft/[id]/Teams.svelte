<script lang="ts">
	import { getFranchise, type Franchise, type FranchiseDetail } from '#lib/api.ts';
	import Headshot from '#lib/ui/Headshot.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	let {
		franchises,
		myId,
		revision,
		onselect
	}: { franchises: Franchise[]; myId?: string; revision: string; onselect: (id: string) => void } =
		$props();
	let selected = $state('');
	const owner = $derived(franchises.find((f) => f.id === (selected || myId)) ?? franchises[0]);
	let team = $state<FranchiseDetail>();
	let error = $state('');
	let retry = $state(0);
	$effect(() => {
		const slug = owner?.slug;
		void revision;
		void retry;
		error = '';
		if (!slug) return;
		let active = true;
		getFranchise(slug).then(
			(r) => {
				if (active) team = r;
			},
			(e) => {
				if (active) error = e.message;
			}
		);
		return () => {
			active = false;
		};
	});
	const showing = $derived(team?.franchise.id === owner?.id ? team : undefined);
</script>

<section class="teams" aria-label="Team rosters">
	<div class="section-title">
		<h2>Team rosters</h2>
		<select
			aria-label="View team roster"
			bind:value={() => selected || owner?.id || '', (v) => (selected = v)}
			>{#each franchises as f (f.id)}<option value={f.id}
					>{f.id === myId ? 'You · ' : ''}{f.name}</option
				>{/each}</select
		>
	</div>
	<div class="body">
		{#if error}<p role="alert">{error}</p>
			<button class="small" onclick={() => retry++}>Try again</button>
		{:else if !showing}<p class="muted small-text">Loading rosters…</p>
		{:else}
			{#each showing.rosters as roster (roster.league_id)}
				{@const main = roster.players.filter((p) => p.list === 'main')}
				{@const reserve = roster.players.filter((p) => p.list === 'reserve')}
				<details open class="roster" data-sport={roster.competition}>
					<summary
						><SportBadge sport={roster.competition} /><strong
							>{main.length}/{roster.limits.main}</strong
						><span>main · {reserve.length}/{roster.limits.reserve} reserve</span></summary
					>
					{#if roster.overage}<p class="warning">Over the roster limit by {roster.overage}</p>{/if}
					<div class="coverage">
						{#each [...new Set(main.flatMap((p) => p.positions))] as position (position)}<span
								>{position}
								<strong>{main.filter((p) => p.positions.includes(position)).length}</strong></span
							>{/each}
					</div>
					{#if !main.length}<p class="muted empty">Main roster is empty.</p>{/if}
					{#each main as player (player.player_id)}<button
							class="player"
							onclick={() => onselect(player.player_id)}
							><Headshot name={player.full_name} src={player.headshot_url} size={22} /><strong
								>{player.full_name}</strong
							><span>{player.positions.join('/')}</span></button
						>{/each}
					{#if reserve.length}<details class="reserve">
							<summary>Reserve list · {reserve.length}</summary
							>{#each reserve as player (player.player_id)}<button
									class="player"
									onclick={() => onselect(player.player_id)}
									><Headshot name={player.full_name} src={player.headshot_url} size={22} /><strong
										>{player.full_name}</strong
									><span>{player.positions.join('/')}</span></button
								>{/each}
						</details>{/if}
				</details>
			{/each}
		{/if}
	</div>
</section>

<style>
	.teams {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		flex: 1;
	}
	.section-title {
		padding: 0.6rem 0.7rem;
		border-bottom: 1px solid var(--rule);
		display: grid;
		gap: 0.5rem;
	}
	h2 {
		font-size: 0.9rem;
	}
	select {
		width: 100%;
		padding: 0.3rem 0.4rem;
		font-size: 0.75rem;
	}
	.body {
		overflow: auto;
		min-height: 0;
		padding: 0.45rem 0.7rem;
	}
	.roster {
		border-bottom: 1px solid var(--rule);
		padding-bottom: 0.4rem;
		margin-bottom: 0.4rem;
	}
	summary {
		cursor: pointer;
		font-size: 0.7rem;
		padding: 0.35rem 0;
	}
	.roster > summary strong {
		margin-left: 0.3rem;
	}
	.roster > summary > span {
		font-size: 0.63rem;
		color: var(--ink-soft);
	}
	.coverage {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		font-size: 0.6rem;
		color: var(--ink-soft);
		margin-bottom: 0.3rem;
	}
	.coverage span {
		padding: 0.1rem 0.3rem;
		background: var(--surface-2);
	}
	.coverage strong {
		color: var(--ink);
	}
	.player {
		display: flex;
		justify-content: start;
		width: 100%;
		gap: 0.4rem;
		padding: 0.25rem 0;
		border: 0;
		border-radius: 0;
		background: transparent;
		font-size: 0.72rem;
		white-space: normal;
		text-align: left;
	}
	.player strong {
		flex: 1;
		font-weight: 600;
	}
	.player > span:last-child {
		font-size: 0.6rem;
		color: var(--ink-soft);
	}
	.player:hover {
		color: var(--brand);
	}
	.reserve {
		margin-top: 0.35rem;
	}
	.reserve summary {
		font-size: 0.65rem;
		color: var(--ink-soft);
	}
	.empty {
		font-size: 0.72rem;
		padding: 0.3rem 0;
	}
	.warning {
		font-size: 0.7rem;
		color: var(--bad);
	}
</style>
