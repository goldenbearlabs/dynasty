<script lang="ts">
	// Manual triggers for the data feeds, and how the recent runs went.
	import { getIngestRuns, startSync, type Competition, type IngestRun, type SyncJob } from '#lib/api.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { clockTime } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { competitions }: { competitions: Competition[] } = $props();

	let runs = $state<IngestRun[]>([]);

	const refresh = () =>
		getIngestRuns()
			.then((r) => (runs = r))
			.catch(toast.error);
	refresh();

	// Keep the table current while a sync is under way.
	$effect(() => {
		if (!runs.some((r) => r.status === 'running')) return;
		const timer = setInterval(refresh, 5000);
		return () => clearInterval(timer);
	});

	async function start(competition: Competition, job: SyncJob) {
		try {
			await startSync(competition.key, job);
			toast.good(`${competition.name} ${job} sync started.`);
			setTimeout(refresh, 500);
		} catch (e) {
			toast.error(e);
		}
	}

	const result = { ok: 'good', error: 'bad', running: 'brand' } as const;
</script>

<section class="stack">
	<p class="muted">
		Rosters, injury designations and season-by-season stat history refresh every night, prospects every week, and scores every 15 minutes for the
		sports this dynasty plays. Use these to refresh now.
	</p>

	<div class="feeds">
		{#each competitions as c (c.key)}
			<div class="card stack tight" data-sport={c.key}>
				<div class="spread">
					<SportBadge sport={c.key} solid />
					<span class="muted small-text">{c.players.toLocaleString()} players</span>
				</div>
				<strong>{c.name}</strong>
				<div class="row">
					<button class="small" onclick={() => start(c, 'rosters')}><Icon name="refresh" size={14} /> Rosters</button>
					<button class="small" onclick={() => start(c, 'injuries')}><Icon name="refresh" size={14} /> Injuries</button>
					<button class="small" onclick={() => start(c, 'games')}><Icon name="refresh" size={14} /> Scores</button>
					<button class="small" onclick={() => start(c, 'seasons')}><Icon name="refresh" size={14} /> Stat history</button>
					{#if c.has_prospects}
						<button class="small" onclick={() => start(c, 'prospects')}><Icon name="refresh" size={14} /> Prospects</button>
					{/if}
				</div>
			</div>
		{/each}
	</div>

	<div class="stack tight">
		<h2>Recent runs</h2>
		{#if runs.length === 0}
			<p class="muted">No runs yet.</p>
		{:else}
			<div class="card flush scroll">
				<table>
					<thead><tr><th>Started</th><th>Sport</th><th>Feed</th><th>Result</th><th class="num">Rows</th></tr></thead>
					<tbody>
						{#each runs.slice(0, 15) as run (run.id)}
							<tr>
								<td>{clockTime(run.started_at)}</td>
								<td><SportBadge sport={run.competition} /></td>
								<td>{run.job}</td>
								<td>
									<span class="pill {result[run.status]}">{run.status}</span>
									{#if run.error}<span class="small-text muted">{run.error}</span>{/if}
								</td>
								<td class="num">{run.rows_upserted.toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</section>

<style>
	.feeds {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(12rem, 1fr));
		gap: 0.8rem;
	}
</style>
