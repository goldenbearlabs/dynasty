<script lang="ts">
	// Adding to a full list: choose who to drop to make room, and add and drop
	// as one move. The player dropped goes on waivers as any drop does.
	import type { League, LeagueRoster, List } from '#lib/api.ts';
	import Headshot from '#lib/ui/Headshot.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';

	let {
		roster,
		league,
		list,
		adding,
		onconfirm,
		oncancel
	}: {
		roster: LeagueRoster;
		league: League;
		list: List;
		/** The name of the player being added. */
		adding: string;
		onconfirm: (dropId: string) => void;
		oncancel: () => void;
	} = $props();

	const listName = $derived(list === 'reserve' ? 'reserve list' : 'main roster');
	const limit = $derived(list === 'reserve' ? roster.limits.reserve : roster.limits.main);
	const players = $derived(roster.players.filter((p) => p.list === list).toSorted((a, b) => a.full_name.localeCompare(b.full_name)));
	const waivers = $derived(league.settings.waivers);
	let drop = $state('');
	const dropped = $derived(players.find((p) => p.player_id === drop));

	let dialog: HTMLDialogElement;
	$effect(() => {
		dialog.showModal();
	});
</script>

<dialog bind:this={dialog} aria-labelledby="drop-to-add" oncancel={oncancel}>
	<div class="body">
		<h2 id="drop-to-add">Make room for {adding}</h2>
		<p class="muted small-text">
			Your {league.name} {listName} is full ({players.length}/{limit}). Choose who to drop: he leaves as {adding} arrives, in one move.
		</p>
		<ul>
			{#each players as p (p.player_id)}
				<li>
					<label class:chosen={drop === p.player_id}>
						<input type="radio" name="drop" value={p.player_id} bind:group={drop} />
						<Headshot name={p.full_name} src={p.headshot_url} size={28} />
						<span class="who">
							<strong>{p.full_name}</strong>
							<span class="muted">{p.positions.join('/')}{#if p.team_abbrev} · <TeamLogo competition={league.competition} abbrev={p.team_abbrev} size={16} />{/if}</span>
						</span>
					</label>
				</li>
			{/each}
		</ul>
		<p class="small-text waivers">
			{#if waivers.mode === 'none'}
				{dropped?.full_name ?? 'The player you drop'} becomes a free agent straight away.
			{:else}
				{dropped?.full_name ?? 'The player you drop'} goes on waivers for {waivers.hours} hours, where any manager can claim him, before he becomes a free agent.
			{/if}
		</p>
		<div class="actions">
			<button class="quiet" onclick={() => (dialog.close(), oncancel())}>Cancel</button>
			<button class="primary" disabled={!drop} onclick={() => (dialog.close(), onconfirm(drop))}>
				{dropped ? `Add ${adding}, drop ${dropped.full_name}` : 'Choose who to drop'}
			</button>
		</div>
	</div>
</dialog>

<style>
	dialog {
		width: min(30rem, calc(100vw - 2rem));
		padding: 0;
		border: 1px solid var(--rule-strong);
		border-radius: var(--radius);
		background: var(--surface);
		color: var(--ink);
		box-shadow: var(--shadow);
	}
	dialog::backdrop {
		background: rgb(0 0 0 / 0.55);
	}
	.body {
		display: grid;
		gap: 0.7rem;
		padding: 1.2rem 1.3rem;
	}
	h2 {
		font-size: 1.15rem;
	}
	ul {
		max-height: 45dvh;
		margin: 0;
		padding: 0;
		overflow-y: auto;
		list-style: none;
		border: 1px solid var(--rule);
		border-radius: var(--radius-small);
	}
	li + li {
		border-top: 1px solid var(--rule);
	}
	label {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.45rem 0.7rem;
		cursor: pointer;
	}
	label:hover {
		background: var(--surface-2);
	}
	label.chosen {
		background: color-mix(in srgb, var(--bad) 12%, transparent);
	}
	.who {
		display: grid;
		font-size: 0.85rem;
	}
	.who .muted {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.72rem;
	}
	.waivers {
		padding: 0.5rem 0.7rem;
		border-radius: var(--radius-small);
		background: var(--surface-2);
	}
	.actions {
		display: flex;
		justify-content: end;
		gap: 0.5rem;
	}
</style>
