<script lang="ts">
	import NotificationInbox from '#lib/NotificationInbox.svelte';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
</script>

<svelte:head><title>Inbox</title></svelte:head>
<NotificationInbox accountId={data.me?.id ?? ''} />
