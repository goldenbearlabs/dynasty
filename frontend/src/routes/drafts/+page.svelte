<script lang="ts">
 import { untrack } from 'svelte';
 import { goto } from '$app/navigation';
 import { page } from '$app/state';
 import DraftCard from '#lib/DraftCard.svelte';
 import Empty from '#lib/ui/Empty.svelte';
 import Icon from '#lib/ui/Icon.svelte';
 import Tabs from '#lib/ui/Tabs.svelte';
 import Rankings from './Rankings.svelte';
 import type { PageProps } from './$types';

 let { data }: PageProps = $props();
 type Section = 'drafts' | 'rankings';
 let tab = $state<Section>(untrack(() => page.url.searchParams.get('tab') === 'rankings' ? 'rankings' : 'drafts'));
 const tabs = [
  { value: 'drafts' as const, label: 'Draft rooms' },
  { value: 'rankings' as const, label: 'Pre-draft rankings' }
 ];
 const active = $derived(data.drafts.filter(d => d.status === 'live' || d.status === 'paused'));
 const scheduled = $derived(data.drafts.filter(d => d.status === 'scheduled' && !d.is_placeholder).toSorted((a,b) => a.year-b.year || a.name.localeCompare(b.name)));
 const rankingDrafts = $derived(data.drafts.filter(d => !d.is_placeholder || d.status !== 'scheduled'));
 function show(section: Section) {
  tab = section;
  const query = new URLSearchParams(page.url.search);
  if (section === 'rankings') query.set('tab',section); else query.delete('tab');
  void goto(`?${query}`,{replaceState:true});
 }
</script>

<svelte:head><title>Drafts</title></svelte:head>

<div class="stack">
 <div class="spread">
  <div><h1>Drafts</h1><p class="muted small-text">Join a draft or prepare your personal player rankings.</p></div>
  {#if data.me?.is_commissioner}<a class="button" href="/commissioner?section=drafts"><Icon name="plus" size={16} /> Manage drafts</a>{/if}
 </div>
 {#if data.me && data.dynasty}<Tabs {tabs} bind:value={() => tab, show} label="Drafts section" />{/if}

 {#if tab === 'rankings' && data.me && data.dynasty}
  <Rankings dynasty={data.dynasty} drafts={rankingDrafts} />
 {:else}
  {#if data.me && data.dynasty}
   <section class="card rankings-entry" aria-labelledby="rankings-heading">
    <div class="ranking-icon"><Icon name="queue" size={24} /></div>
    <div class="grow"><h2 id="rankings-heading">Build your draft board</h2><p class="muted small-text">Save a private list of players in your preferred order, then import it into My queue in the draft room. Your queue supplies your auto-pick order.</p><p class="muted small-text">Startup boards combine every league in the draft. Rookie boards rank one league.</p></div>
    <button class="primary" onclick={() => show('rankings')}>My pre-draft rankings <Icon name="right" size={16} /></button>
   </section>
  {/if}

  {#if active.length > 0}
   <section class="stack tight"><h2>In progress</h2><div class="list">{#each active as draft (draft.id)}<DraftCard {draft} />{/each}</div></section>
  {/if}
  <section class="stack tight">
   <h2>Scheduled drafts</h2>
   {#if scheduled.length === 0}
    <Empty icon="draft" title="No drafts scheduled">Drafts appear here when the commissioner schedules them.{#if data.me && data.dynasty} You can build a league ranking while you wait.{/if}</Empty>
   {:else}
    <div class="list">{#each scheduled as draft (draft.id)}<DraftCard {draft} />{/each}</div>
   {/if}
  </section>
 {/if}
</div>

<style>
 .list{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(100%,17rem),1fr));gap:.8rem}
 h2{font-size:1.1rem}
 .rankings-entry{display:flex;flex-wrap:wrap;align-items:center;gap:1rem;border-left:3px solid var(--brand)}
 .ranking-icon{color:var(--brand);display:flex}
 .grow{flex:1 1 20rem;display:grid;gap:.3rem}
 .rankings-entry button{white-space:normal}
</style>
