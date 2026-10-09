<script lang="ts">
	// Tools for players the feeds miss, and for joining two rows that are one person.
	import {
		addPlayers,
		getMergeSuggestions,
		mergePlayers,
		type Competition,
		type MergeSuggestion,
		type NewPlayer
	} from '#lib/api.ts';
	import Empty from '#lib/ui/Empty.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { competitions }: { competitions: Competition[] } = $props();

	let csv = $state('');
	let suggestions = $state<MergeSuggestion[]>([]);

	const loadSuggestions = () =>
		getMergeSuggestions()
			.then((s) => (suggestions = s))
			.catch(toast.error);
	loadSuggestions();

	// One player per line: sport, name, positions, birth date, note.
	function parse(text: string): NewPlayer[] {
		return text
			.split('\n')
			.map((line) => line.split(',').map((cell) => cell.trim()))
			.filter((cells) => cells.some(Boolean))
			.map(([competition = '', full_name = '', positions = '', birth_date = '', ...note]) => ({
				competition: competition.toLowerCase(),
				full_name,
				positions: positions ? positions.split('/').map((p) => p.trim().toUpperCase()) : [],
				birth_date,
				note: note.join(', '),
				status: 'prospect'
			}));
	}

	async function add(event: SubmitEvent) {
		event.preventDefault();
		try {
			const { added } = await addPlayers(parse(csv));
			toast.good(`Added ${added} ${added === 1 ? 'player' : 'players'} as prospects.`);
			csv = '';
			await loadSuggestions();
		} catch (e) {
			toast.error(e);
		}
	}

	async function merge(s: MergeSuggestion) {
		try {
			await mergePlayers(s.player_id, s.prospect_id);
			toast.good(`Merged ${s.full_name}.`);
			await loadSuggestions();
		} catch (e) {
			toast.error(e);
		}
	}
</script>

<section class="stack">
	<form class="card stack tight" onsubmit={add}>
		<h2>Add players the feeds do not have</h2>
		<p class="muted">
			One per line: <code>sport, name, positions, birth date, note</code>. Sport is one of
			{competitions.map((c) => c.key).join(', ')}. Only sport and name are required. They are added as prospects.
		</p>
		<textarea
			rows="5"
			aria-label="Players to add"
			placeholder={'nba, Luka Example, G/F, 2008-05-17, Real Madrid\ncbb, Jordan Sample, PG, , Class of 2028'}
			bind:value={csv}
		></textarea>
		<div><button class="primary" disabled={!csv.trim()}>Add players</button></div>
	</form>

	<div class="stack tight">
		<h2>Possible duplicates</h2>
		{#if suggestions.length === 0}
			<Empty icon="players" title="No duplicates to review">
				A prospect shows up here when a player with the same name arrives in the same sport.
			</Empty>
		{:else}
			<p class="muted">
				Merging keeps the arrived player and moves the prospect's roster spot and history onto him. Names repeat, so check it is
				the same person first.
			</p>
			<div class="card flush scroll">
				<table>
					<thead><tr><th>Name</th><th>Prospect entry</th><th>Arrived player</th><th></th></tr></thead>
					<tbody>
						{#each suggestions as s (s.prospect_id + s.player_id)}
							<tr>
								<td><div class="row"><SportBadge sport={s.competition} /> <strong>{s.full_name}</strong></div></td>
								<td class="small-text">{s.prospect_note || 'No note'}</td>
								<td class="small-text">{[s.player_team, s.player_class, s.player_status].filter(Boolean).join(', ')}</td>
								<td class="actions"><button class="small" onclick={() => merge(s)}>Merge</button></td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</section>
