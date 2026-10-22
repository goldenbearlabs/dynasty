<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { inbox } from '#lib/inbox.svelte.ts';
	import NotificationInbox from '#lib/NotificationInbox.svelte';
	import Icon from '#lib/ui/Icon.svelte';

	let { accountId }: { accountId: string } = $props();
	let dialog = $state<HTMLDialogElement>();
	let open = $state(false);
	function close() {
		dialog?.close();
	}
	function show() {
		dialog?.showModal();
		open = true;
		void inbox.refresh();
	}
	afterNavigate(close);
	$effect(() => {
		void accountId;
		close();
	});
	$effect(() => {
		if (!open) return;
		const previous = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		return () => {
			document.body.style.overflow = previous;
		};
	});
</script>

<button
	class="mail-button"
	class:has-unread={inbox.unread > 0}
	aria-label="Notifications{inbox.unread ? `, ${inbox.unread} unread` : ''}"
	title="Notifications"
	aria-haspopup="dialog"
	aria-controls="notifications-panel"
	aria-expanded={open}
	onclick={show}
>
	<Icon name="mail" size={23} />
	{#if inbox.unread}<span class="unread-badge" aria-hidden="true"
			>{inbox.unread > 99 ? '99+' : inbox.unread}</span
		>{/if}
</button>
<dialog
	bind:this={dialog}
	id="notifications-panel"
	aria-label="Notifications"
	onclose={() => (open = false)}
	onpointerdown={(event) => {
		if (event.target !== dialog) return;
		const rect = dialog.getBoundingClientRect();
		if (
			event.clientX < rect.left ||
			event.clientX > rect.right ||
			event.clientY < rect.top ||
			event.clientY > rect.bottom
		)
			close();
	}}
>
	<div class="panel-top">
		<span class="eyebrow">Your league inbox</span><button
			class="quiet close"
			aria-label="Close notifications"
			onclick={close}><Icon name="x" size={19} /></button
		>
	</div>
	<div class="panel-body">
		{#if open}<NotificationInbox {accountId} compact onopen={close} />{/if}
	</div>
</dialog>

<style>
	.mail-button {
		flex-shrink: 0;
		position: relative;
		display: grid;
		place-items: center;
		width: 2.75rem;
		height: 2.75rem;
		padding: 0;
		border: 1px solid var(--rule);
		border-radius: 10px;
		background: var(--surface);
		color: var(--ink-soft);
	}
	.mail-button:hover,
	.mail-button[aria-expanded='true'] {
		color: var(--brand);
		background: var(--brand-soft);
		border-color: var(--brand);
	}
	.has-unread {
		color: var(--ink);
	}
	.unread-badge {
		position: absolute;
		top: -0.3rem;
		right: -0.35rem;
		min-width: 1.15rem;
		height: 1.15rem;
		padding: 0 0.25rem;
		display: grid;
		place-items: center;
		border-radius: 999px;
		background: var(--bad);
		color: white;
		box-shadow: 0 0 0 2px var(--surface);
		font: 750 0.65rem/1 var(--mono);
	}
	dialog {
		position: fixed;
		inset: max(0.75rem, env(safe-area-inset-top)) 1rem auto auto;
		margin: 0;
		padding: 0;
		width: min(33rem, calc(100vw - 2rem));
		max-width: none;
		max-height: calc(100dvh - 1.5rem);
		border: 1px solid var(--rule);
		border-radius: 16px;
		background: var(--surface);
		color: var(--ink);
		box-shadow: 0 16px 70px #0003;
		overflow: hidden;
	}
	dialog[open] {
		display: flex;
		flex-direction: column;
	}
	dialog::backdrop {
		background: rgb(15 23 42 / 0.3);
	}
	.panel-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.35rem 0.55rem 0.35rem 1rem;
		border-bottom: 1px solid var(--rule);
		flex-shrink: 0;
	}
	.panel-top .eyebrow {
		font-size: 0.65rem;
	}
	.close {
		display: grid;
		place-items: center;
		width: 2.75rem;
		height: 2.75rem;
		padding: 0;
	}
	.panel-body {
		overflow-y: auto;
		overscroll-behavior: contain;
		scrollbar-width: thin;
	}
	@media (max-width: 640px) {
		dialog {
			inset: max(0.5rem, env(safe-area-inset-top)) 0.5rem auto;
			width: calc(100vw - 1rem);
			max-height: calc(100dvh - 1rem - env(safe-area-inset-top) - env(safe-area-inset-bottom));
			border-radius: 12px;
		}
	}
</style>
