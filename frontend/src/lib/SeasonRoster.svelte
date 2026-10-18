<script lang="ts">
	// Setting a roster for the season the commissioner has opened: who is on
	// the main roster and who on reserve. It is done once, and saving asks
	// for a final confirmation that says what the choice locks in.
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { setSeasonRoster, type LeagueRoster, type RosterPlayer } from '#lib/api.ts';
	import Headshot from '#lib/ui/Headshot.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	import TeamLogo from '#lib/ui/TeamLogo.svelte';
	import { clockTime, dayLabel, timeLeft } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let {
		roster,
		franchiseId,
		force = false,
		oncancel
	}: {
		roster: LeagueRoster;
		franchiseId: string;
		/** A commissioner setting another franchise's roster, past the rules. */
		force?: boolean;
		oncancel?: () => void;
	} = $props();

	const season = $derived(roster.season!);
	const signed = $derived(roster.players.filter((p) => p.list !== 'rights'));
	// Who goes on reserve, as the manager arranges it, starting from the lists as
	// they are; everyone else is on the main roster.
	let reserve = $state(untrack(() => new Set(roster.players.filter((p) => p.list === 'reserve').map((p) => p.player_id))));
	const toReserve = (id: string) => (reserve = new Set([...reserve, id]));
	const toMain = (id: string) => (reserve = new Set([...reserve].filter((r) => r !== id)));

	const byName = (a: RosterPlayer, b: RosterPlayer) => a.full_name.localeCompare(b.full_name);
	const main = $derived(signed.filter((p) => !reserve.has(p.player_id)).toSorted(byName));
	const held = $derived(signed.filter((p) => reserve.has(p.player_id)).toSorted(byName));
	const mainOver = $derived(main.length - roster.limits.main);
	const reserveOver = $derived(held.length - roster.limits.reserve);
	const problem = $derived(
		mainOver > 0
			? `Move ${mainOver} more to the reserve list: the main roster holds ${roster.limits.main}.`
			: reserveOver > 0
				? `Move ${reserveOver} more to the main roster: the reserve list holds ${roster.limits.reserve}.`
				: ''
	);

	let confirming = $state<HTMLDialogElement>();
	let saving = $state(false);
	async function save() {
		saving = true;
		try {
			await setSeasonRoster(roster.league_id, { reserve: [...reserve], franchise_id: franchiseId, force });
			confirming?.close();
			toast.good(`Your ${season.year} ${roster.name} season roster is set.`);
			await invalidateAll();
		} catch (e) {
			toast.error(e);
		} finally {
			saving = false;
		}
	}
</script>

{#snippet list(title: string, players: RosterPlayer[], limit: number, over: number, onReserve: boolean)}
	<section class="list" class:reserve={onReserve}>
		<header>
			<h3>{title}</h3>
			<span class="count" class:bad={over > 0} class:good={over === 0}>{players.length}/{limit}</span>
		</header>
		{#if players.length === 0}
			<p class="muted small-text none">Nobody here.</p>
		{:else}
			<ul>
				{#each players as p (p.player_id)}
					<li>
						<Headshot name={p.full_name} src={p.headshot_url} size={30} />
						<span class="who">
							<strong>{p.full_name}</strong>
							<span class="muted">{p.positions.join('/')}{#if p.team_abbrev} · <TeamLogo competition={roster.competition} abbrev={p.team_abbrev} size={18} />{/if}</span>
						</span>
						<InjuryBadge designation={p.injury_designation} />
						{#if onReserve}
							<button class="small" onclick={() => toMain(p.player_id)} title="Move {p.full_name} to the main roster"><Icon name="left" size={14} /> Main</button>
						{:else}
							<button class="small" onclick={() => toReserve(p.player_id)} title="Move {p.full_name} to the reserve list">Reserve <Icon name="right" size={14} /></button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/snippet}

<section class="setter card flush" aria-label="Set your season roster">
	<header class="intro">
		<div>
			<p class="eyebrow">{season.year} season · {season.started ? 'under way since' : 'starts'} {dayLabel(season.starts_on)}</p>
			<h2>Set your {roster.name} season roster</h2>
			<p class="muted small-text">
				Choose who is on your main roster and who goes on the reserve list for the season. Anyone can be on either list; only main-roster
				players can start. This is set once and holds for the season, so take your time.
			</p>
			{#if season.deadline}
				<p class="locks-at"><Icon name="clock" size={14} /> Lists lock automatically in <strong>{timeLeft(season.deadline)}</strong> ({clockTime(season.deadline)}) with the roster as it stands if it is not set.</p>
			{/if}
		</div>
		{#if oncancel}<button class="quiet small" onclick={oncancel}>Not now</button>{/if}
	</header>

	<div class="lists">
		{@render list('Main roster', main, roster.limits.main, mainOver, false)}
		{@render list('Reserve list', held, roster.limits.reserve, reserveOver, true)}
	</div>

	<footer>
		<span class="small-text" class:problem={!!problem}>{problem || `${main.length} on the main roster, ${held.length} on reserve.`}</span>
		<button class="primary" disabled={!!problem} onclick={() => confirming?.showModal()}>Review and save</button>
	</footer>
</section>

<dialog bind:this={confirming} aria-labelledby="season-roster-confirm">
	<div class="confirm">
		<h2 id="season-roster-confirm">Lock in your {season.year} {roster.name} roster?</h2>
		<p class="totals">
			<span><strong>{main.length}</strong> on the main roster</span>
			<span><strong>{held.length}</strong> on the reserve list</span>
		</p>
		<p><strong>This is a final decision.</strong> Once you save, everyone stays on the list he is on until the season ends: nobody moves between the main roster and the reserve list, except in these cases.</p>
		<ul class="rules">
			<li><Icon name="trophy" size={16} /><span><strong>Playoffs.</strong> When the playoffs come, you can add players from your reserve list to your playoff roster.</span></li>
			<li><Icon name="refresh" size={16} /><span><strong>Drops and free agents.</strong> You can drop a reserve player and add a free agent to the reserve list in his place.</span></li>
			<li><Icon name="trade" size={16} /><span><strong>Trades.</strong> Trades go through as normal.</span></li>
			<li>
				<Icon name="lock" size={16} />
				<span>
					<strong>Injuries.</strong> If a main-roster player is hurt, you can send him down to reserve and call someone up in his place.
					He is then locked on reserve for the rest of the season, even if he comes back healthy. He can still join your playoff roster.
				</span>
			</li>
		</ul>
		<div class="actions">
			<button class="quiet" disabled={saving} onclick={() => confirming?.close()}>Go back</button>
			<button class="primary" disabled={saving} onclick={save}>{saving ? 'Saving…' : 'Confirm and lock in'}</button>
		</div>
	</div>
</dialog>

<style>
	.setter {
		display: grid;
		border-top: 3px solid var(--sport, var(--brand));
	}
	.intro {
		display: flex;
		align-items: start;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.9rem 1rem 0.6rem;
	}
	.intro h2 {
		margin: 0.15rem 0 0.3rem;
		font-size: 1.15rem;
	}
	.intro p.muted {
		max-width: 46rem;
	}
	.locks-at {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin-top: 0.4rem;
		color: var(--gold);
		font-size: 0.85rem;
	}
	.lists {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		border-top: 1px solid var(--rule);
	}
	.list + .list {
		border-left: 1px solid var(--rule);
	}
	.list header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 1rem;
		background: var(--surface-2);
		border-bottom: 1px solid var(--rule);
	}
	.list h3 {
		font: 700 0.7rem var(--mono);
		letter-spacing: 0.07em;
		text-transform: uppercase;
		color: var(--ink-soft);
	}
	.count {
		font: 700 0.85rem var(--mono);
	}
	.count.good {
		color: var(--good);
	}
	.count.bad {
		color: var(--bad);
	}
	.list ul {
		list-style: none;
		margin: 0;
		padding: 0;
		max-height: 32rem;
		overflow-y: auto;
	}
	.list li {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.4rem 1rem;
		border-bottom: 1px solid var(--rule);
	}
	.list li:last-child {
		border-bottom: 0;
	}
	.list.reserve li {
		background: color-mix(in srgb, var(--surface-2) 60%, transparent);
	}
	.who {
		display: grid;
		flex: 1;
		min-width: 0;
		font-size: 0.85rem;
	}
	.who strong {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.who .muted {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
	}
	.none {
		padding: 0.8rem 1rem;
	}
	footer {
		position: sticky;
		bottom: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.7rem 1rem;
		background: var(--surface);
		border-top: 1px solid var(--rule);
	}
	.problem {
		color: var(--bad);
		font-weight: 600;
	}

	/* ---- the final confirmation ---- */
	dialog {
		max-width: min(36rem, calc(100vw - 2rem));
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
	.confirm {
		display: grid;
		gap: 0.8rem;
		padding: 1.3rem 1.4rem;
		font-size: 0.9rem;
		line-height: 1.45;
	}
	.confirm h2 {
		font-size: 1.2rem;
	}
	.totals {
		display: flex;
		gap: 1.5rem;
		padding: 0.6rem 0.8rem;
		border-radius: var(--radius-small);
		background: var(--surface-2);
	}
	.totals strong {
		font: 700 1.1rem var(--mono);
	}
	.rules {
		display: grid;
		gap: 0.6rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.rules li {
		display: flex;
		gap: 0.6rem;
		align-items: start;
	}
	.rules :global(svg) {
		flex: none;
		margin-top: 0.15rem;
		color: var(--sport, var(--brand));
	}
	.actions {
		display: flex;
		justify-content: end;
		gap: 0.5rem;
		margin-top: 0.3rem;
	}
	@media (max-width: 720px) {
		.lists {
			grid-template-columns: 1fr;
		}
		.list + .list {
			border-left: 0;
			border-top: 1px solid var(--rule);
		}
	}
</style>
