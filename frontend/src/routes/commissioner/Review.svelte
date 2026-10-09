<script lang="ts">
	// Rostered players who have dropped out of their competition's feed: a
	// college player who left without turning pro, a retirement. The
	// commissioner releases them, or carries them over where the league's
	// rules keep a franchise's rights.
	import { invalidateAll } from '$app/navigation';
	import { carryOver, changeRoster, getReviewQueue, type Dynasty, type ReviewItem } from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { dynasty }: { dynasty: Dynasty } = $props();

	let queue = $state<ReviewItem[]>([]);
	const load = () => getReviewQueue().then((q) => (queue = q), toast.error);
	load();

	const league = (item: ReviewItem) => dynasty.leagues.find((l) => l.id === item.league_id)!;
	// Where this league's rules carry a player on to, if anywhere.
	const destination = (item: ReviewItem) => {
		const into = league(item).settings.continuity?.into;
		return dynasty.leagues.find((l) => l.competition === into);
	};

	const run = (work: Promise<void>, done: string) =>
		work
			.then(load)
			.then(invalidateAll)
			.then(() => toast.good(done), toast.error);

	const release = (item: ReviewItem) =>
		run(
			changeRoster(item.league_id, 'drop', { player_id: item.player_id, franchise_id: item.franchise_id, force: true }),
			`Released ${item.full_name}.`
		);
</script>

<section class="stack tight">
	<p class="muted">
		These players are on a roster but no longer appear in their sport's feed. A college player who reaches the NBA moves on by
		himself; anyone listed here needs a decision.
	</p>

	{#if queue.length === 0}
		<Empty icon="check" title="Nobody to review">Every rostered player is still in his sport's feed.</Empty>
	{:else}
		<div class="card flush scroll">
			<table>
				<thead><tr><th>Player</th><th>Franchise</th><th></th></tr></thead>
				<tbody>
					{#each queue as item (item.league_id + item.player_id)}
						{@const into = destination(item)}
						<tr>
							<td>
								<div class="row">
									<SportBadge sport={item.competition} />
									<strong>{item.full_name}</strong>
									<span class="muted small-text">{item.positions.join('/')}</span>
								</div>
							</td>
							<td><a href="/franchise/{item.franchise_slug}">{item.franchise_name}</a></td>
							<td class="actions">
								{#if into}
									<button
										class="small"
										onclick={() => run(carryOver(item.league_id, item.player_id), `${item.franchise_name} keep ${item.full_name} in ${into.name}.`)}
									>
										Carry over to {into.name}
									</button>
								{/if}
								<button class="small quiet danger" onclick={() => release(item)}>Release</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
