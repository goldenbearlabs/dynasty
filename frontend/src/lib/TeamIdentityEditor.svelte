<script lang="ts">
 import { invalidateAll } from '$app/navigation';
 import { setOrganizationIdentity, setTeamIdentity, type Franchise, type League, type TeamIdentity } from '#lib/api.ts';
 import IdentityForm from '#lib/IdentityForm.svelte';
 let { franchise, leagues, identities }: { franchise: Franchise; leagues: League[]; identities: TeamIdentity[] } = $props();
</script>
<details class="identity-editor panel">
 <summary><strong>Customize organization & teams</strong><span class="muted small-text">Your names and logos across every sport</span></summary>
 <div class="stack body"><p class="muted small-text">Build an organization identity, then give each sport its own team name and image. Names are visible throughout the dynasty; your existing links stay the same.</p>
  <IdentityForm title="Organization" initialName={franchise.name} initialImage={franchise.image_url} save={async (name,img) => {await setOrganizationIdentity(franchise.id,name,img);await invalidateAll();}} />
  <div class="team-grid">{#each leagues as l (l.id)}{@const identity=identities.find(i => i.franchise_id===franchise.id && i.league_id===l.id)}<IdentityForm title={l.name} initialName={identity?.name ?? ''} initialImage={identity?.image_url ?? ''} fallbackName={franchise.name} fallbackImage={franchise.image_url} optional save={async (name,img) => {await setTeamIdentity(franchise.id,l.id,name,img);await invalidateAll();}} />{/each}</div>
 </div>
</details>
<style>
 summary{padding:1rem;cursor:pointer;display:flex;flex-wrap:wrap;gap:.5rem 1rem;justify-content:space-between}.body{padding:1rem;border-top:1px solid var(--rule)}.team-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,24rem),1fr));gap:1rem}
</style>
