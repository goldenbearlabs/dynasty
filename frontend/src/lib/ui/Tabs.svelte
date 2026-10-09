<script lang="ts" generics="T extends string">
	// A row of choices where one is selected.
	type Tab = { value: T; label: string; count?: number; sport?: string };
	let { tabs, value = $bindable(), label }: { tabs: Tab[]; value: T; label: string } = $props();
</script>

<div class="tabs" role="tablist" aria-label={label}>
	{#each tabs as tab (tab.value)}
		<button
			role="tab"
			aria-selected={value === tab.value}
			class:selected={value === tab.value}
			data-sport={tab.sport}
			onclick={() => (value = tab.value)}
		>
			{tab.label}
			{#if tab.count !== undefined}<span class="count">{tab.count.toLocaleString()}</span>{/if}
		</button>
	{/each}
</div>

<style>
	.tabs {
		display: flex;
		gap: 0.25rem;
		overflow-x: auto;
		border-bottom: 1px solid var(--rule);
		scrollbar-width: none;
	}
	button {
		border: 0;
		border-bottom: 2px solid transparent;
		background: transparent;
		color: var(--ink-soft);
		padding: 0.5rem 0.8rem;
		border-radius: 0;
	}
	button:hover:not(.selected) {
		color: var(--ink);
	}
	button.selected {
		color: var(--sport, var(--ink));
		border-bottom-color: var(--sport, var(--brand));
	}
	.count {
		font: 600 0.72rem var(--mono);
		color: var(--ink-faint);
	}
</style>
