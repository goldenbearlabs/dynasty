<script lang="ts">
 let { designation }: { designation?: string } = $props();
 const labels: Record<string,string> = {IR:'Injured reserve',IL7:'7-day injured list',IL10:'10-day injured list',IL15:'15-day injured list',IL60:'60-day injured list',OUT:'Out on the injury report'};
</script>
{#if designation}<span class="pill bad" title={labels[designation] ?? designation}>{designation === 'OUT' ? 'Out' : designation}</span>{/if}
