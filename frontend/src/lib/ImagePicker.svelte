<script lang="ts">
 import { untrack } from 'svelte';
 import Crest from '#lib/ui/Crest.svelte';
 let { value = $bindable(''), name, fallbackImage = '', busy = $bindable(false), label = 'Image' }: { value: string; name: string; fallbackImage?: string; busy?: boolean; label?: string } = $props();
 const id = untrack(() => `image-${Math.random().toString(36).slice(2)}`);
 let error = $state('');
 async function upload(file: File | undefined) {
  if (!file) return;
  if (!file.type.startsWith('image/') || file.type === 'image/svg+xml') { error='Choose a PNG, JPEG, WebP or GIF image.'; return; }
  if (file.size > 10*1024*1024) { error='Choose an image smaller than 10 MB.'; return; }
  busy=true; error=''; let url='';
  try {
   url=URL.createObjectURL(file); const img=new Image(); img.src=url; await img.decode();
   const canvas=document.createElement('canvas'); const scale=Math.min(1,256/Math.max(img.width,img.height));
   canvas.width=Math.max(1,Math.round(img.width*scale)); canvas.height=Math.max(1,Math.round(img.height*scale));
   const context=canvas.getContext('2d'); if(!context)throw new Error('Image upload is unavailable in this browser.');
   context.drawImage(img,0,0,canvas.width,canvas.height); let encoded=canvas.toDataURL('image/png');
   if (encoded.length>200000) { context.fillStyle='#fff';context.globalCompositeOperation='destination-over';context.fillRect(0,0,canvas.width,canvas.height);encoded=canvas.toDataURL('image/jpeg',.85); }
   if (encoded.length>200000)throw new Error('This image is too complex. Try a smaller image.');
   value=encoded;
  } catch(e) { error=e instanceof Error ? e.message : 'Could not read that image.'; }
  finally { if(url)URL.revokeObjectURL(url); busy=false; }
 }
</script>
<div class="picker">
 <Crest {name} src={value || fallbackImage} size={64} />
 <div class="stack tight grow"><label for={id}>{label} URL</label><input id={id} type="url" disabled={busy} value={value.startsWith('data:') ? '' : value} placeholder="https://example.com/logo.png" oninput={(e) => {value=e.currentTarget.value;error='';}} />
  <div class="row"><label class="upload">{busy ? 'Preparing image…' : 'Upload image'}<input type="file" accept="image/png,image/jpeg,image/webp,image/gif" disabled={busy} onchange={(e) => {void upload(e.currentTarget.files?.[0]);e.currentTarget.value='';}} /></label>{#if value}<button type="button" class="small quiet" disabled={busy} onclick={() => value=''}>Remove image</button>{/if}{#if !value && fallbackImage}<span class="muted small-text">Using organization image</span>{/if}{#if value.startsWith('data:')}<span class="muted small-text">Uploaded image ready</span>{/if}</div>
  <p class="muted small-text">Upload or paste an image URL. Uploads are resized for a crisp team logo.</p>{#if error}<p role="alert">{error}</p>{/if}
 </div>
</div>
<style>
 .picker{display:flex;align-items:flex-start;gap:1rem}.grow{flex:1;min-width:0}.upload{position:relative;overflow:hidden;display:inline-flex;padding:.4rem .7rem;border:1px solid var(--rule);border-radius:6px;font-size:.85rem;cursor:pointer}.upload:focus-within{outline:2px solid var(--brand)}.upload input{position:absolute;inset:0;opacity:0;width:100%;cursor:pointer}input[type=url]{width:100%}
</style>
