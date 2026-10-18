<script lang="ts">
	// The commissioner's deadline for season rosters: by then every manager
	// sets who is on the main roster and who on reserve, or the lists lock
	// as they stand.
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { setRosterDeadline, type Season } from '#lib/api.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { clockTime, timeLeft } from '#lib/ui/time.ts';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let { season, onsaved }: { season: Season; onsaved?: () => unknown } = $props();

	// A datetime-local field speaks local time without a zone: "2026-10-26T21:00".
	const toField = (d: Date) => {
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};
	let when = $state(untrack(() => (season.roster_deadline ? toField(new Date(season.roster_deadline)) : '')));
	const passed = $derived(!!season.roster_deadline && new Date(season.roster_deadline).getTime() <= Date.now());

	// Quick picks: a day or three from now, or the evening before the season starts.
	function inHours(hours: number) {
		const d = new Date(Date.now() + hours * 3600_000);
		d.setMinutes(0, 0, 0);
		when = toField(d);
	}
	function eveOfSeason() {
		const d = new Date(season.starts_on + 'T21:00');
		d.setDate(d.getDate() - 1);
		when = toField(d);
	}

	let saving = $state(false);
	async function save(deadline: string | null) {
		saving = true;
		try {
			await setRosterDeadline(season.id, deadline && new Date(deadline).toISOString());
			toast.good(deadline ? 'Deadline set. Managers can see it on their teams.' : 'Deadline cleared.');
			await onsaved?.();
			await invalidateAll();
		} catch (e) {
			toast.error(e);
		} finally {
			saving = false;
		}
	}
</script>

<div class="deadline">
	<div>
		<strong>Season roster deadline</strong>
		<p class="muted small-text">
			{#if season.roster_deadline && passed}
				<Icon name="lock" size={12} /> Passed {clockTime(season.roster_deadline)}: rosters not set by then are locked as they stood.
			{:else if season.roster_deadline}
				<Icon name="clock" size={12} /> {clockTime(season.roster_deadline)} · {timeLeft(season.roster_deadline)} left. Any roster not set by then locks as it stands.
			{:else}
				No deadline: managers can set their season roster until they choose to. Set one and they see a countdown.
			{/if}
		</p>
	</div>
	<div class="row end">
		<label class="field">Lock rosters at <input type="datetime-local" bind:value={when} /></label>
		<span class="picks">
			<button class="quiet small" onclick={() => inHours(24)}>In 24 hours</button>
			<button class="quiet small" onclick={() => inHours(72)}>In 3 days</button>
			<button class="quiet small" onclick={eveOfSeason}>Eve of the season</button>
		</span>
		<button class="primary" disabled={!when || saving} onclick={() => save(when)}>{season.roster_deadline ? 'Change deadline' : 'Set deadline'}</button>
		{#if season.roster_deadline}<button class="quiet" disabled={saving} onclick={() => save(null)}>Clear</button>{/if}
	</div>
</div>

<style>
	.deadline {
		display: grid;
		gap: 0.6rem;
		padding: 0.8rem 0.9rem;
		border: 1px solid var(--rule);
		border-left: 3px solid var(--gold);
		border-radius: var(--radius-small);
		background: var(--surface-2);
	}
	.deadline p {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		margin-top: 0.2rem;
	}
	.picks {
		display: flex;
		flex-wrap: wrap;
		gap: 0.2rem;
	}
</style>
