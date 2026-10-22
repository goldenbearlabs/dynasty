<script lang="ts">
	import { goto } from '$app/navigation';
	import { getNotifications, type InboxNotification, type NotificationPage } from '#lib/api.ts';
	import { inbox } from '#lib/inbox.svelte.ts';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Empty from '#lib/ui/Empty.svelte';
	import { ago, clockTime } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let kind = $state('');
	let unreadOnly = $state(false);
	let page = $state(1);
	let result = $state<NotificationPage>();
	let loading = $state(false);
	let error = $state('');
	let busy = $state(false);
	let refresh = $state(0);
	let request = 0;
	let shownAccount = '';
	const tabs = [
		{ value: '', label: 'All' },
		{ value: 'roster', label: 'Roster moves' },
		{ value: 'trade', label: 'Trade offers' },
		{ value: 'target', label: 'Targets' },
	];
	const icons: Record<string, IconName> = { roster: 'players', trade: 'trade', target: 'target' };
	const labels: Record<string, string> = {
		roster: 'Roster move',
		trade: 'Trade offer',
		target: 'Player target',
	};
	const pages = $derived(result ? Math.max(1, Math.ceil(result.total / result.per_page)) : 1);
	$effect(() => {
		void kind;
		void unreadOnly;
		page = 1;
	});
	$effect(() => {
		const account = data.me?.id;
		const filter = { kind, unread: unreadOnly ? 'true' : undefined, page };
		void inbox.revision;
		void refresh;
		if (shownAccount !== account) {
			shownAccount = account ?? '';
			result = undefined;
			error = '';
		}
		if (!account) return;
		const current = ++request;
		loading = true;
		let active = true;
		getNotifications(filter)
			.then((value) => {
				if (!active || current !== request) return;
				result = value;
				error = '';
				const max = Math.max(1, Math.ceil(value.total / value.per_page));
				if (page > max) page = max;
			})
			.catch((e) => {
				if (active && current === request)
					error = e instanceof Error ? e.message : 'Could not load notifications.';
			})
			.finally(() => {
				if (active && current === request) loading = false;
			});
		return () => {
			active = false;
		};
	});
	async function mark(item: InboxNotification) {
		if (busy) return;
		busy = true;
		try {
			await inbox.read(item.id, !item.read_at);
		} catch (e) {
			toast.error(e);
		} finally {
			busy = false;
		}
	}
	async function markAll() {
		busy = true;
		try {
			await inbox.readAll();
		} catch (e) {
			toast.error(e);
		} finally {
			busy = false;
		}
	}
	async function open(event: MouseEvent, item: InboxNotification) {
		if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
		event.preventDefault();
		try {
			if (!item.read_at) await inbox.read(item.id);
		} catch (e) {
			toast.error(e);
		}
		await goto(item.href);
	}
</script>

<svelte:head><title>Inbox</title></svelte:head>

{#if !data.me}
	<Empty icon="bell" title="Your league inbox"
		><a href="/">Sign in</a> to see your notifications.</Empty
	>
{:else}
	<div class="stack inbox">
		<header class="spread inbox-header">
			<div class="stack tight">
				<div class="row">
					<h1>Inbox</h1>
					{#if inbox.unread}<span class="unread-count">{inbox.unread} unread</span>{/if}
				</div>
				<p class="muted small-text">
					Roster moves around the league. Offers and interest just for you.
				</p>
			</div>
			<button class="small quiet" disabled={busy || !inbox.unread} onclick={markAll}
				><Icon name="check" size={16} />Mark all read</button
			>
		</header>
		<div class="stack tight controls">
			<Tabs {tabs} bind:value={kind} label="Notification type" />
			<div class="spread">
				<label class="check"><input type="checkbox" bind:checked={unreadOnly} />Unread only</label
				><button
					class="small quiet"
					disabled={loading}
					onclick={() => {
						refresh++;
						void inbox.refresh();
					}}><Icon name="refresh" size={14} />Refresh</button
				>
			</div>
		</div>
		{#if error}<div class="card stack tight" role="alert">
				<p>{error}</p>
				<button class="small" onclick={() => refresh++}>Try again</button>
			</div>{/if}
		{#if !result && loading}<p class="muted" role="status">Loading your inbox…</p>
		{:else if result}
			{#if !result.notifications.length}
				<Empty icon="bell" title={unreadOnly ? 'All caught up' : 'Nothing here yet'}
					>{unreadOnly
						? 'You have no unread notifications in this view.'
						: 'New roster moves, trade offers, and interest in your players will appear here.'}</Empty
				>
			{:else}
				<div class="messages" aria-busy={loading}>
					{#each result.notifications as item (item.id)}
						<article class="message" class:unread={!item.read_at}>
							<div
								class="event-icon"
								class:trade={item.kind === 'trade'}
								class:target={item.kind === 'target'}
							>
								<Icon name={icons[item.kind]} size={19} />
							</div>
							<div class="message-body">
								<div class="metadata">
									<span>{labels[item.kind]}</span>{#if item.competition}<SportBadge
											sport={item.competition}
										/>{/if}<time datetime={item.created_at} title={clockTime(item.created_at)}
										>{ago(item.created_at)}</time
									>{#if !item.read_at}<span class="unread-dot" aria-label="Unread"></span>{/if}
								</div>
								<a class="message-title" href={item.href} onclick={(event) => open(event, item)}
									>{item.title}</a
								>
								{#if item.body}<p class="muted small-text">{item.body}</p>{/if}
								{#if item.current_waiver_until}<a
										class="waiver pill gold"
										href="/waivers?competition={item.competition}"
										>On waivers · clears {clockTime(item.current_waiver_until)}</a
									>
								{:else if item.waiver_until}<span class="muted waiver-history"
										>Waiver period ended</span
									>{/if}
							</div>
							<button
								class="quiet read-button"
								disabled={busy}
								aria-label="Mark {item.read_at ? 'unread' : 'read'}: {item.title}"
								title={item.read_at ? 'Mark unread' : 'Mark read'}
								onclick={() => mark(item)}
								><Icon name={item.read_at ? 'bell' : 'check'} size={17} /></button
							>
						</article>
					{/each}
				</div>
				{#if pages > 1}<nav class="spread pagination" aria-label="Inbox pages">
						<button class="small" disabled={page <= 1 || loading} onclick={() => page--}
							>Previous</button
						><span class="muted small-text">{page} / {pages}</span><button
							class="small"
							disabled={page >= pages || loading}
							onclick={() => page++}>Next</button
						>
					</nav>{/if}
			{/if}
		{/if}
	</div>
{/if}

<style>
	.inbox {
		max-width: 58rem;
		margin-inline: auto;
	}
	.inbox-header {
		align-items: flex-start;
		gap: 1rem;
	}
	.inbox-header h1 {
		line-height: 1.1;
	}
	.unread-count {
		border-radius: 20px;
		padding: 0.25rem 0.6rem;
		background: var(--brand-soft);
		color: var(--brand);
		font-size: 0.75rem;
		font-weight: 600;
	}
	.controls {
		gap: 0.65rem;
	}
	.controls .check {
		font-size: 0.85rem;
	}
	.messages {
		border: 1px solid var(--rule);
		border-radius: 12px;
		overflow: hidden;
		background: var(--surface);
	}
	.message {
		display: grid;
		grid-template-columns: 2.3rem minmax(0, 1fr) 2.75rem;
		gap: 0.85rem;
		padding: 1rem;
		border-bottom: 1px solid var(--rule);
	}
	.message:last-child {
		border-bottom: 0;
	}
	.message.unread {
		background: color-mix(in srgb, var(--brand-soft) 30%, var(--surface));
		box-shadow: inset 3px 0 var(--brand);
	}
	.event-icon {
		display: grid;
		place-items: center;
		width: 2.3rem;
		height: 2.3rem;
		border-radius: 10px;
		background: var(--surface-2);
		color: var(--ink-soft);
	}
	.event-icon.trade {
		color: #3276d2;
		background: color-mix(in srgb, #3276d2 10%, var(--surface));
	}
	.event-icon.target {
		color: var(--brand);
		background: var(--brand-soft);
	}
	.message-body {
		min-width: 0;
		display: grid;
		gap: 0.3rem;
	}
	.metadata {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem 0.65rem;
		align-items: center;
		color: var(--ink-faint);
		font-size: 0.7rem;
	}
	.metadata time {
		margin-left: auto;
		white-space: nowrap;
	}
	.message-title {
		color: var(--ink);
		font-size: 0.92rem;
		line-height: 1.45;
		overflow-wrap: anywhere;
	}
	.unread .message-title {
		font-weight: 650;
	}
	.message-title:hover {
		color: var(--brand);
	}
	.unread-dot {
		height: 6px;
		width: 6px;
		background: var(--brand);
		border-radius: 50%;
	}
	.read-button {
		width: 2.75rem;
		height: 2.75rem;
		padding: 0;
		display: grid;
		place-items: center;
		align-self: center;
	}
	.waiver {
		justify-self: start;
		white-space: normal;
		line-height: 1.5;
	}
	.waiver-history {
		font-size: 0.72rem;
	}
	@media (max-width: 640px) {
		.inbox-header {
			flex-wrap: wrap;
			gap: 0.65rem;
		}
		.inbox-header > button {
			margin-left: auto;
			min-height: 2.75rem;
		}
		.message {
			grid-template-columns: 1.9rem minmax(0, 1fr) 2.75rem;
			gap: 0.5rem;
			padding: 0.8rem 0.65rem;
		}
		.event-icon {
			width: 1.9rem;
			height: 1.9rem;
			border-radius: 8px;
		}
		.message-title {
			font-size: 0.86rem;
		}
		.metadata {
			font-size: 0.65rem;
			gap: 0.25rem 0.45rem;
		}
		.message p {
			font-size: 0.77rem;
		}
		.metadata time {
			margin-left: 0;
		}
	}
</style>
