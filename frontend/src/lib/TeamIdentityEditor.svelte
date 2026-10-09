<script lang="ts">
 import { invalidateAll } from '$app/navigation';
 import { setOrganizationIdentity, setTeamIdentity, type Franchise, type League, type TeamIdentity } from '#lib/api.ts';
 import Icon from '#lib/ui/Icon.svelte';
 import IdentityForm from '#lib/IdentityForm.svelte';
 let { franchise, leagues, identities }: { franchise: Franchise; leagues: League[]; identities: TeamIdentity[] } = $props();
 let dialog: HTMLDialogElement;
</script>
<button class="quiet settings" aria-label="Team settings" title="Edit names and logos" onclick={() => dialog.showModal()}><Icon name="settings" size={20} /></button>
<dialog bind:this={dialog} aria-labelledby="settings-title">
 <div class="dialog-head"><h2 id="settings-title">Team settings</h2><button class="quiet" aria-label="Close team settings" onclick={() => dialog.close()}><Icon name="x" /></button></div>
 <div class="stack body"><p class="muted small-text">Build an organization identity, then give each sport its own team name and image. Names are visible throughout the dynasty; your existing links stay the same.</p>
  <IdentityForm title="Organization" initialName={franchise.name} initialImage={franchise.image_url} save={async (name,img) => {await setOrganizationIdentity(franchise.id,name,img);await invalidateAll();}} />
  <div class="team-grid">{#each leagues as l (l.id)}{@const identity=identities.find(i => i.franchise_id===franchise.id && i.league_id===l.id)}<IdentityForm title={l.name} initialName={identity?.name ?? ''} initialImage={identity?.image_url ?? ''} fallbackName={franchise.name} fallbackImage={franchise.image_url} optional save={async (name,img) => {await setTeamIdentity(franchise.id,l.id,name,img);await invalidateAll();}} />{/each}</div>
 </div>
</dialog>
<style>
 .settings{padding:.45rem;flex:none}dialog{width:min(54rem,calc(100vw - 2rem));max-height:85dvh;padding:0;border:1px solid var(--rule);border-radius:12px;background:var(--surface);color:var(--ink);overflow:auto}dialog::backdrop{background:rgb(0 0 0 / .5)}.dialog-head{display:flex;align-items:center;justify-content:space-between;padding:.75rem 1rem;border-bottom:1px solid var(--rule)}.dialog-head h2{font-size:1.2rem}.body{padding:1rem}.team-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,24rem),1fr));gap:1rem}
</style>
