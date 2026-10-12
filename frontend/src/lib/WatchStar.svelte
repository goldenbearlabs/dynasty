<script lang="ts">
 import { watchlist } from '#lib/watchlist.svelte.ts';
 import { toast } from '#lib/ui/toast.svelte.ts';
 let { playerID, playerName }: { playerID: string; playerName: string } = $props();
 const saved = $derived(watchlist.has(playerID));
 const label = $derived(`${saved ? 'Remove' : 'Add'} ${playerName} ${saved ? 'from' : 'to'} your watchlist`);
</script>
{#if watchlist.account}
 <button type="button" class="watch-star" class:saved aria-label={label} title={watchlist.error || label} aria-pressed={saved} disabled={watchlist.busy || !watchlist.loaded} onclick={() => watchlist.toggle(playerID).catch(toast.error)}>
  <svg width="18" height="18" viewBox="0 0 24 24" fill={saved ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.7" stroke-linejoin="round" aria-hidden="true"><path d="m12 3 2.8 5.7 6.3.9-4.55 4.45 1.07 6.28L12 17.36l-5.62 2.97 1.07-6.28L2.9 9.6l6.3-.9Z" /></svg>
 </button>
{/if}
<style>
 .watch-star{display:inline-flex;align-items:center;justify-content:center;padding:.3rem;border:0;background:transparent;color:var(--ink-faint);vertical-align:middle;flex:none}.watch-star:hover,.saved{color:var(--gold)}.watch-star:focus-visible{outline:2px solid var(--brand);outline-offset:2px}
</style>
