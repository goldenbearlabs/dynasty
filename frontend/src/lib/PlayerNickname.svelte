<script lang="ts">
 import { untrack } from 'svelte';
 import { invalidateAll } from '$app/navigation';
 import { getPlayerNickname, setPlayerNickname } from '#lib/api.ts';
 import { toast } from '#lib/ui/toast.svelte.ts';
 let { franchiseID, playerID, playerName, initialNickname = '', load = false, editable = true }: { franchiseID: string; playerID: string; playerName: string; initialNickname?: string; load?: boolean; editable?: boolean } = $props();
 let nickname=$state(untrack(() => initialNickname)), draft=$state(''), editing=$state(false),busy=$state(false),error=$state('');
 $effect(() => {const f=franchiseID,p=playerID; let active=true; if(load)getPlayerNickname(f,p).then(r => {if(active){nickname=r.nickname;editing=false;}},e=>{if(active)error=e instanceof Error?e.message:'Could not load nickname.';});return()=>{active=false;};});
 async function save(e:SubmitEvent){e.preventDefault();busy=true;try{await setPlayerNickname(franchiseID,playerID,draft.trim());nickname=draft.trim();editing=false;await invalidateAll();toast.good(nickname?'Nickname saved.':'Nickname removed.');}catch(e){toast.error(e);}finally{busy=false;}}
</script>
<div class="nickname">
 {#if nickname}<span class="pill brand">“{nickname}”</span>{/if}
 {#if editing}<form class="row" onsubmit={save}><input aria-label="Nickname for {playerName}" bind:value={draft} maxlength="40" placeholder="Nickname" /><button class="small" disabled={busy}>{busy?'Saving…':'Save'}</button><button class="small quiet" type="button" disabled={busy} onclick={() => editing=false}>Cancel</button></form><span class="muted small-text">Your organization’s nickname. Clear the field to remove it.</span>
 {:else if editable}<button type="button" class="nickname-button" onclick={() => {draft=nickname;editing=true;}}>{nickname?'Edit nickname':'Set nickname'}</button>{/if}
 {#if error}<span class="small-text" role="alert">{error}</span>{/if}
</div>
<style>
 .nickname{display:flex;flex-wrap:wrap;align-items:center;gap:.4rem}.nickname-button{border:0;background:none;padding:.2rem 0;color:var(--ink-soft);font-size:.75rem}.nickname-button:hover{color:var(--brand)}form{flex-wrap:wrap}input{max-width:14rem;min-width:0}
</style>
