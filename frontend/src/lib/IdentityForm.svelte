<script lang="ts">
 import { untrack } from 'svelte';
 import ImagePicker from '#lib/ImagePicker.svelte';
 import { toast } from '#lib/ui/toast.svelte.ts';
 let { title, initialName, initialImage, fallbackName = '', fallbackImage = '', optional = false, save }: { title: string; initialName: string; initialImage: string; fallbackName?: string; fallbackImage?: string; optional?: boolean; save: (name: string,image: string) => Promise<void> } = $props();
 let name=$state(untrack(() => initialName)), image=$state(untrack(() => initialImage)), busy=$state(false), preparingImage=$state(false);
 let savedName=$state(untrack(() => initialName)), savedImage=$state(untrack(() => initialImage));
 const dirty=$derived(name!==savedName || image!==savedImage);
 const id=untrack(() => `identity-${Math.random().toString(36).slice(2)}`);
 async function submit(e: SubmitEvent) {
  e.preventDefault();if(preparingImage || busy)return;busy=true;const n=name.trim(),img=image.trim();
  try{await save(n,img);savedName=n;savedImage=img;name=n;image=img;toast.good(`${title} saved.`);}catch(e){toast.error(e);}finally{busy=false;}
 }
</script>
<form class="card stack" onsubmit={submit}>
 <h3>{title}</h3><label for={id}>{optional ? 'Team name' : 'Organization name'}</label><input id={id} bind:value={name} placeholder={fallbackName} required={!optional} maxlength="80" />
 <ImagePicker bind:busy={preparingImage} {fallbackImage} bind:value={image} name={name || fallbackName} label={optional ? 'Team image' : 'Organization image'} />
 {#if optional}<p class="muted small-text">Leave the name or image blank to use your organization’s. This changes your team’s identity in this sport.</p>{/if}
 <div class="row"><button class="primary small" disabled={busy || preparingImage || !dirty}>{preparingImage ? 'Preparing image…' : busy ? 'Saving…' : 'Save identity'}</button><button type="button" class="quiet small" disabled={busy || preparingImage || !dirty} onclick={() => {name=savedName;image=savedImage;}}>Undo changes</button></div>
</form>
