<script lang="ts">
 import type { ProspectRankingFields } from '#lib/api.ts';
 let { player }: { player: ProspectRankingFields } = $props();
</script>

{#if player.prospect_rank != null}
 <a class="pill gold" href={player.prospect_rank_url} target="_blank" rel="noopener noreferrer"
  title="MLB Pipeline Top 100 prospects · {player.prospect_rank_year}{player.prospect_eta ? ` · MLB ETA ${player.prospect_eta}` : ''}">
  MLB Pipeline #{player.prospect_rank} · {player.prospect_rank_year}
 </a>
{/if}
