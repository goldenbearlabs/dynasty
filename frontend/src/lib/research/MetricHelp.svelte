<script lang="ts">
 import { untrack } from 'svelte';
 let { label, help }: { label: string; help: string } = $props();
 const id = untrack(() => `metric-${Math.random().toString(36).slice(2)}`);
 let open = $state(false), left = $state(0), top = $state(0);
 function show(event: MouseEvent | FocusEvent) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  left = Math.max(8,Math.min(rect.left,window.innerWidth-296));
  top = Math.max(8,Math.min(rect.bottom+8,window.innerHeight-330));
  open = true;
 }
</script>
<button type="button" aria-label="Explain {label}" aria-describedby={open ? id : undefined} onmouseenter={show} onmouseleave={() => open=false} onfocus={show} onblur={() => open=false} onkeydown={(e) => { if(e.key==='Escape') open=false; }}>{label} <span aria-hidden="true">ⓘ</span></button>
{#if open}<span {id} role="tooltip" style:left="{left}px" style:top="{top}px">{help}</span>{/if}
<style>
 button{border:0;background:none;padding:0;color:var(--ink-soft);font:inherit;text-align:left;cursor:help}[role=tooltip]{position:fixed;width:18rem;max-width:calc(100vw - 1rem);max-height:calc(100vh - 1rem);overflow:auto;padding:.8rem;border-radius:8px;background:var(--ink);color:var(--surface);font-size:.78rem;line-height:1.5;z-index:100;box-shadow:0 6px 24px #0003;pointer-events:none}
</style>
