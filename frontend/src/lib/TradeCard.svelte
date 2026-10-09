<script lang="ts">
	// One trade: what each side receives, where it stands, and the buttons
	// this viewer is allowed to press.
	import { answerTrade, ruleOnTrade, type Franchise, type Session, type Trade } from '#lib/api.ts';
	import TradeAsset from '#lib/TradeAsset.svelte';
	import Crest from '#lib/ui/Crest.svelte';
	import { ago } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	type Props = {
		trade: Trade;
		franchises: Franchise[];
		me: Session | null;
		/** Called after the trade changes. */
		onchange: () => void;
	};
	let { trade, franchises, me, onchange }: Props = $props();

	const name = (id: string | null) => franchises.find((f) => f.id === id)?.name ?? '';
	const mine = $derived(trade.parties.find((p) => p.franchise_id === me?.id));
	const waitingOn = $derived(trade.parties.filter((p) => !p.accepted_at).map((p) => name(p.franchise_id)));
	const open = $derived(trade.status === 'proposed' || trade.status === 'accepted');

	const status = {
		proposed: { label: 'Offer', tone: 'brand' },
		accepted: { label: 'Awaiting commissioner', tone: 'gold' },
		executed: { label: 'Completed', tone: 'good' },
		reversed: { label: 'Reversed', tone: '' },
		rejected: { label: 'Declined', tone: '' },
		cancelled: { label: 'Withdrawn', tone: '' }
	} as const;

	const run = (work: Promise<void>, done: string) =>
		work.then(() => {
			toast.good(done);
			onchange();
		}, toast.error);

	function reverse() {
		if (confirm('Reverse this trade? Every player and pick goes back to where it came from.')) {
			run(ruleOnTrade(trade.id, 'reverse'), 'Trade reversed.');
		}
	}
</script>

<article class="card flush">
	<header class="spread">
		<span class="row">
			<span class="pill {status[trade.status].tone}">{status[trade.status].label}</span>
			<span class="muted small-text">Proposed by {name(trade.proposed_by)} · {ago(trade.created_at)}</span>
		</span>
		{#if trade.status === 'proposed' && waitingOn.length > 0}
			<span class="muted small-text">Waiting on {waitingOn.join(', ')}</span>
		{/if}
	</header>

	<div class="sides">
		{#each trade.parties as party (party.franchise_id)}
			<section>
				<h3 class="row">
					<Crest name={name(party.franchise_id)} size={24} />
					{name(party.franchise_id)} <span class="muted">receives</span>
				</h3>
				<ul>
					{#each trade.items.filter((i) => i.to_franchise === party.franchise_id) as item (item.id)}
						<li>
							<TradeAsset
								player={item.player_name}
								positions={item.player_positions}
								headshot={item.player_headshot}
								sport={item.competition}
								draft={item.draft_name}
								round={item.pick_round}
								via={item.pick_original_franchise_id !== item.from_franchise ? name(item.pick_original_franchise_id) : ''}
							/>
						</li>
					{:else}
						<li class="muted small-text">Nothing</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>

	{#if trade.note}<p class="note">“{trade.note}”</p>{/if}

	{#if me && (open || trade.status === 'executed')}
		<footer class="row">
			{#if trade.status === 'proposed' && mine && !mine.accepted_at}
				<button class="primary" onclick={() => run(answerTrade(trade.id, 'accept'), 'Trade accepted.')}>Accept</button>
				<button onclick={() => run(answerTrade(trade.id, 'reject'), 'Offer declined.')}>Decline</button>
			{/if}
			{#if open && trade.proposed_by === me.id}
				<button class="quiet" onclick={() => run(answerTrade(trade.id, 'cancel'), 'Offer withdrawn.')}>Withdraw offer</button>
			{/if}
			{#if me.is_commissioner}
				<span class="pill gold commissioner">Commissioner</span>
				{#if trade.status === 'accepted'}
					<button class="primary" onclick={() => run(ruleOnTrade(trade.id, 'approve'), 'Trade approved.')}>Approve</button>
				{/if}
				{#if open}
					<button class="quiet danger" onclick={() => run(ruleOnTrade(trade.id, 'veto'), 'Trade vetoed.')}>Veto</button>
				{:else}
					<button class="quiet danger" onclick={reverse}>Reverse</button>
				{/if}
			{/if}
		</footer>
	{/if}
</article>

<style>
	header,
	footer,
	.note {
		padding: 0.75rem 1.1rem;
	}
	header {
		border-bottom: 1px solid var(--rule);
		background: var(--surface-2);
	}
	.sides {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
	}
	section {
		padding: 1rem 1.1rem;
	}
	section + section {
		border-left: 1px solid var(--rule);
	}
	@media (max-width: 640px) {
		section + section {
			border-left: none;
			border-top: 1px solid var(--rule);
		}
	}
	h3 {
		gap: 0.5rem;
		margin-bottom: 0.7rem;
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.6rem;
	}
	.note {
		border-top: 1px solid var(--rule);
		color: var(--ink-soft);
		font-style: italic;
	}
	footer {
		border-top: 1px solid var(--rule);
	}
	.commissioner {
		margin-left: auto;
	}
</style>
