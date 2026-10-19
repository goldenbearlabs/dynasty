<script lang="ts">
	import { page } from '$app/state';
	import TradeFlag from '#lib/TradeFlag.svelte';
	import Icon from '#lib/ui/Icon.svelte';

	let { leagueId, playerId, ownerId, name, detail = '', onDrop }: {
		leagueId: string;
		playerId: string;
		ownerId: string;
		name: string;
		detail?: string;
		onDrop?: () => void | Promise<void>;
	} = $props();
	let dialog = $state<HTMLDialogElement>();
</script>

{#if page.data.me || onDrop}
	<button class="quiet player-actions" aria-label="Actions for {name}" title="Player actions" aria-haspopup="dialog" onclick={() => dialog?.showModal()}><Icon name="more" size={16} /></button>
	<dialog bind:this={dialog} aria-label="Actions for {name}">
		<div class="dialog-head">
			<div><h2>{name}</h2>{#if detail}<p class="muted small-text">{detail}</p>{/if}</div>
			<button class="quiet" aria-label="Close player actions" onclick={() => dialog?.close()}><Icon name="x" size={18} /></button>
		</div>
		<div class="body">
			<a href="/player/{playerId}">View player profile →</a>
			{#if page.data.me}
				<section>
					<h3>Trade interest</h3>
					<p class="muted small-text">{page.data.me.id === ownerId ? 'Let other managers know this player is available and see who is interested.' : 'Let this player’s manager know you are interested.'}</p>
					<TradeFlag {leagueId} {playerId} {ownerId} {name} />
				</section>
			{/if}
			{#if onDrop}
				<section>
					<h3>Roster action</h3>
					<p class="muted small-text">Remove this player from the roster. You’ll review the waiver details before confirming.</p>
					<button class="small quiet danger" onclick={() => { dialog?.close(); onDrop?.(); }}>Drop player</button>
				</section>
			{/if}
		</div>
	</dialog>
{/if}

<style>
	.player-actions { display: inline-flex; align-items: center; justify-content: center; padding: .2rem; vertical-align: middle; }
	dialog { width: min(25rem, calc(100vw - 2rem)); max-height: 85dvh; overflow: auto; padding: 0; border: 1px solid var(--rule); border-radius: 12px; background: var(--surface); color: var(--ink); text-align: left; white-space: normal; }
	dialog::backdrop { background: rgb(0 0 0 / .5); }
	.dialog-head { display: flex; align-items: start; justify-content: space-between; gap: 1rem; padding: 1rem; border-bottom: 1px solid var(--rule); }
	h2 { font-size: 1.1rem; }
	h3 { font-size: .85rem; margin-bottom: .35rem; }
	.body { display: grid; gap: 1rem; padding: 1rem; font-size: .85rem; }
	section { border-top: 1px solid var(--rule); padding-top: 1rem; }
	section p { margin-bottom: .6rem; }
</style>
