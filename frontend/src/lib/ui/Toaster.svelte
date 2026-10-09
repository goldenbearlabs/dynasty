<script lang="ts">
	import Icon from './Icon.svelte';
	import { toast } from './toast.svelte.ts';
</script>

<div class="toaster" aria-live="polite">
	{#each toast.items as item (item.id)}
		<div class="toast {item.kind}" role={item.kind === 'bad' ? 'alert' : 'status'}>
			<Icon name={item.kind === 'good' ? 'check' : 'x'} />
			<span>{item.text}</span>
			<button class="quiet small" aria-label="Dismiss" onclick={() => toast.dismiss(item.id)}>
				<Icon name="x" size={14} />
			</button>
		</div>
	{/each}
</div>

<style>
	.toaster {
		position: fixed;
		z-index: 50;
		right: 1rem;
		bottom: 1rem;
		display: grid;
		gap: 0.5rem;
		width: min(24rem, calc(100vw - 2rem));
	}
	@media (max-width: 860px) {
		.toaster {
			bottom: 4.75rem; /* clear of the tab bar */
		}
	}
	.toast {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.7rem 0.6rem 0.7rem 0.85rem;
		border-radius: var(--radius);
		border: 1px solid var(--rule-strong);
		background: var(--surface);
		color: var(--ink);
		box-shadow: 0 8px 30px rgb(0 0 0 / 0.18);
		font-weight: 500;
		animation: rise 0.18s ease-out;
	}
	.toast span {
		flex: 1;
	}
	.good :global(svg:first-child) {
		color: var(--good);
	}
	.bad :global(svg:first-child) {
		color: var(--bad);
	}
	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(6px);
		}
	}
</style>
