<script lang="ts">
 import { untrack } from 'svelte';
 let { label, help, active = false, ascending = false, onsort, numeric = false, sticky = false }: {
  label: string; help: string; active?: boolean; ascending?: boolean; onsort?: () => void; numeric?: boolean; sticky?: boolean;
 } = $props();
 let open = $state(false);
 let left = $state(0);
 let top = $state(0);
 const id = untrack(() => `column-${Math.random().toString(36).slice(2)}`);
 function show(event: MouseEvent | FocusEvent) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  left = Math.max(8, Math.min(rect.left, window.innerWidth - 296));
  top = Math.min(rect.bottom + 8, window.innerHeight - 152);
  open = true;
 }
</script>
<th class:num={numeric} class:sticky aria-sort={onsort ? active ? ascending ? 'ascending' : 'descending' : 'none' : undefined}>
 <button class="column-label" class:active onclick={onsort} onmouseenter={show} onmouseleave={() => open = false} onfocus={show} onblur={() => open = false} onkeydown={(e) => { if (e.key === 'Escape') open = false; }} aria-describedby={open ? id : undefined}>
  {label}<span aria-hidden="true" class="hint">{active ? ascending ? '↑' : '↓' : onsort ? '↕' : 'ⓘ'}</span>
 </button>
 {#if open}<span {id} role="tooltip" class="tooltip" style:left="{left}px" style:top="{top}px">{help}{#if onsort}<small>Click the column name to sort; click again to reverse.</small>{/if}</span>{/if}
</th>
<style>
 th.sticky { position: sticky; left: 0; z-index: 2; background: var(--surface); min-width: 14rem; }
 th.num { text-align: right; }
 .column-label { padding: 0; border: 0; border-radius: 0; background: none; color: inherit; font: inherit; text-align: inherit; display: inline-flex; gap: 0.4rem; align-items: center; }
 .column-label:hover, .column-label.active { color: var(--brand); }
 .hint { color: var(--ink-faint); font-size: 0.8rem; }
 .tooltip { position: fixed; z-index: 100; width: 18rem; max-width: calc(100vw - 1rem); padding: 0.75rem; background: var(--ink); color: var(--surface); border-radius: var(--radius-small); box-shadow: 0 6px 24px #0003; font: 400 0.78rem/1.5 var(--body); letter-spacing: 0; text-transform: none; white-space: normal; text-align: left; pointer-events: none; }
 .tooltip small { display: block; margin-top: 0.5rem; opacity: 0.7; }
</style>
