<script lang="ts">
 import { goto } from '$app/navigation';
 import { getPlayers, type Competition, type Player } from '#lib/api.ts';
 import Headshot from '#lib/ui/Headshot.svelte';
 let { competitions }: { competitions: Competition[] } = $props();
 let query = $state(''), competition = $state(''), players = $state<Player[]>([]);
 let loading = $state(false), error = $state(''), total = $state(0), retry = $state(0);
 $effect(() => {
  const q = query.trim(), sport = competition; retry;
  let active = true;
  players = []; total = 0; error = ''; loading = !!q;
  const timer = setTimeout(async () => {
   if (!q) return;
   try { const result = await getPlayers({q, competition:sport, page:1}); if (active) { players = result.players.slice(0,8); total = result.total; } }
   catch (e) { if (active) error = e instanceof Error ? e.message : 'Search could not load.'; }
   finally { if (active) loading = false; }
  },250);
  return () => { active = false; clearTimeout(timer); };
 });
</script>
<section class="search-card" aria-label="Find a player">
 <div><h2>Find a player</h2><p class="muted small-text">One profile for their career stats, advanced research, and fantasy history. Searches every player, including prospects and reserve-only players.</p></div>
 <form onsubmit={(event) => { event.preventDefault(); if (players[0]) goto(`/player/${players[0].id}`); }}>
  <label class="query"><span class="sr-only">Player name</span><input type="search" bind:value={query} placeholder="Search a player’s name…" autocomplete="off" /></label>
  <label><span class="sr-only">Search sport</span><select bind:value={competition}><option value="">All sports</option>{#each competitions as c}<option value={c.key}>{c.name}</option>{/each}</select></label>
 </form>
 <div aria-live="polite" class="small-text muted">{#if loading}Searching…{:else if error}<span role="alert">{error}</span> <button class="small" onclick={() => retry++}>Retry</button>{:else if query.trim()}{total ? `${total.toLocaleString()} matches${total > 8 ? ' · Showing the first 8; narrow your search for more' : ''}` : 'No matches. Try a last name or another sport.'}{/if}</div>
 {#if players.length}<div class="results">{#each players as p (p.id)}<a href="/player/{p.id}" class="result"><Headshot name={p.full_name} src={p.headshot_url} size={34} /><div><strong>{p.full_name}</strong><span class="muted small-text">{p.competition.toUpperCase()} · {p.positions?.join(' / ')} · {p.team_abbrev || p.status}{p.owner_name ? ` · ${p.owner_name}` : ''}</span></div><span aria-hidden="true">→</span></a>{/each}</div>{/if}
</section>
<style>
 .search-card{padding:1.2rem;border:1px solid var(--rule);border-radius:12px;background:var(--surface);display:grid;gap:.7rem}h2{font-size:1.1rem}p{margin-top:.3rem}form{display:flex;gap:.6rem}.query{flex:1;min-width:0}input{width:100%}.results{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:.4rem}.result{display:flex;align-items:center;gap:.7rem;padding:.65rem;border:1px solid var(--rule);border-radius:8px;text-decoration:none}.result:hover{background:var(--brand-soft)}.result div{flex:1}.result span.muted{display:block}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}@media(max-width:600px){.results{grid-template-columns:1fr}form{flex-direction:column}}
</style>
