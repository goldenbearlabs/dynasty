<script lang="ts">
 import { onMount } from 'svelte';
 import { page } from '$app/state';
 import type { PageProps } from './$types';
 import { watchlist } from '#lib/watchlist.svelte.ts';
 import WatchStar from '#lib/WatchStar.svelte';
 import PlayerSearch from '#lib/PlayerSearch.svelte';
 import Headshot from '#lib/ui/Headshot.svelte';
 import SportBadge from '#lib/ui/SportBadge.svelte';
 import Empty from '#lib/ui/Empty.svelte';
 import { toast } from '#lib/ui/toast.svelte.ts';
 let { data }: PageProps = $props();
 let sport = $state(page.url.searchParams.get('competition') || ''), search = $state(''), category = $state(''), ownership = $state('');
 let editing = $state(''), draftCategory = $state(''), draftNotes = $state('');
 const categories = $derived([...new Set(watchlist.entries.map(p => p.category).filter(Boolean))].sort());
 const visible = $derived(watchlist.entries.filter(p => (!sport || p.competition === sport) && (!category || (category === '\u0000' ? !p.category : p.category === category)) && (!ownership || (ownership === 'free' ? !p.owner_slug : ownership === 'mine' ? p.owner_slug === data.me?.slug : !!p.owner_slug && p.owner_slug !== data.me?.slug)) && (!search.trim() || `${p.full_name} ${p.team_name} ${p.notes}`.toLowerCase().includes(search.trim().toLowerCase()))));
 onMount(() => { if (watchlist.loaded) void watchlist.refresh(); });
 async function save() { try { await watchlist.edit(editing, draftCategory, draftNotes); editing = ''; } catch(e) { toast.error(e); } }
</script>
<svelte:head><title>Watchlist · Crossover Dynasty</title></svelte:head>
<div class="stack">
 <header class="spread"><div class="stack tight"><span class="eyebrow">{data.me?.name || 'My team'}</span><h1>Watchlist</h1><p class="muted">Keep an eye on anyone. Set your priority order, group players, and save scouting notes.</p></div>{#if data.me}<span class="pill gold">Private to your team · {watchlist.entries.length} players</span>{/if}</header>
 {#if !data.me}<Empty icon="lock" title="Sign in to view your watchlist"><a href="/">Sign in</a> to save players for your team.</Empty>
 {:else}
  <PlayerSearch competitions={data.competitions} />
  {#if watchlist.error}<div class="card"><p role="alert">{watchlist.error}</p><button disabled={watchlist.loading || watchlist.busy} onclick={() => watchlist.refresh()}>Retry</button></div>{/if}
  <div class="row filters">
   <input type="search" aria-label="Search watchlist" placeholder="Search saved players or notes" bind:value={search} />
   <select aria-label="Watchlist sport" bind:value={sport}><option value="">All sports</option>{#each data.competitions as c}<option value={c.key}>{c.name}</option>{/each}</select>
   <select aria-label="Watchlist group" bind:value={category}><option value="">All groups</option><option value={"\u0000"}>Ungrouped</option>{#each categories as group}<option value={group}>{group}</option>{/each}</select>
   <select aria-label="Watchlist ownership" bind:value={ownership}><option value="">Any owner</option><option value="free">Free agents</option><option value="mine">My players</option><option value="other">Other teams’ players</option></select>
   <button class="small quiet" disabled={watchlist.loading || watchlist.busy} onclick={() => watchlist.refresh()}>Refresh</button>
  </div>
  {#if watchlist.loading && !watchlist.loaded}<p role="status" class="muted">Loading your watchlist…</p>
  {:else if watchlist.loaded && !watchlist.entries.length}<Empty icon="players" title="Your watchlist starts with a star">Star a player in search, player lists, research, or on a team roster to save them here.</Empty>
  {:else if watchlist.loaded && !visible.length}<Empty icon="search" title="No saved players match"><button onclick={() => { search=''; sport=''; category=''; ownership=''; }}>Clear filters</button></Empty>
  {:else}
   <p class="muted small-text">{visible.length} shown · Players appear in your saved priority order. Arrow controls move a player one place in the full list.</p>
   <div class="stack tight">
    {#each visible as p (p.player_id)}
     {@const index = watchlist.entries.findIndex(e => e.player_id === p.player_id)}
     <article class="card entry">
      <div class="spread"><div class="row who"><span class="rank">{index+1}</span><Headshot name={p.full_name} src={p.headshot_url} size={40} /><div><div class="row"><a href="/player/{p.player_id}"><strong>{p.full_name}</strong></a><WatchStar playerID={p.player_id} playerName={p.full_name} /><SportBadge sport={p.competition} />{#if p.category}<span class="pill">{p.category}</span>{/if}</div><p class="muted small-text">{p.positions?.join(' / ')} · {p.team_name || 'No real-world team'} · {p.status} · {#if p.owner_slug}<a href="/franchise/{p.owner_slug}">{p.owner_name}</a>{:else}Free agent{/if}</p></div></div>
       <div class="row"><button class="small" aria-label="Move {p.full_name} up" disabled={watchlist.busy || index===0} onclick={() => watchlist.move(p.player_id,-1).catch(toast.error)}>↑</button><button class="small" aria-label="Move {p.full_name} down" disabled={watchlist.busy || index===watchlist.entries.length-1} onclick={() => watchlist.move(p.player_id,1).catch(toast.error)}>↓</button><button class="small" disabled={watchlist.busy} onclick={() => { editing=p.player_id; draftCategory=p.category; draftNotes=p.notes; }}>Group & notes</button></div>
      </div>
      {#if editing===p.player_id}<form class="stack tight" onsubmit={event => { event.preventDefault(); void save(); }}><label>Group<input bind:value={draftCategory} maxlength="60" list="watchlist-groups" placeholder="Trade targets, prospects, keepers…" /></label><label>Scouting notes<textarea bind:value={draftNotes} maxlength="2000" rows="3" placeholder="What are you watching for?"></textarea></label><div class="row"><button class="small primary" disabled={watchlist.busy}>Save</button><button type="button" class="small quiet" onclick={() => editing=''}>Cancel</button></div></form>{:else if p.notes}<p class="notes small-text">{p.notes}</p>{/if}
     </article>
    {/each}
   </div>
   <datalist id="watchlist-groups">{#each categories as group}<option value={group}></option>{/each}</datalist>
  {/if}
 {/if}
</div>
<style>
 .entry{display:grid;gap:.8rem}.who{flex-wrap:nowrap;min-width:0}.who>div{min-width:0}.rank{font:600 .85rem var(--mono);color:var(--ink-faint);min-width:1.6rem}.notes{white-space:pre-wrap;overflow-wrap:anywhere;color:var(--ink-soft);border-top:1px solid var(--rule);padding-top:.7rem}form label{display:grid;gap:.3rem}form input,textarea{width:100%}.filters input{flex:1;min-width:12rem}.entry strong{overflow-wrap:anywhere}
</style>
