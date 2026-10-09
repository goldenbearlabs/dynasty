<script lang="ts">
	// Every rule of one league. The setup wizard and the commissioner page
	// both use this form; it edits the settings object it is given in place.
	import type { Competition, LeagueSettings } from '#lib/api.ts';

	type Props = {
		settings: LeagueSettings;
		competition: Competition;
		/** The dynasty's other sports, which players could carry over into. */
		others: Competition[];
	};
	let { settings = $bindable(), competition, others }: Props = $props();

	const ANY = '*';
	const starters = $derived(settings.lineup.slots.reduce((sum, slot) => sum + (slot.count || 0), 0));

	function togglePosition(positions: string[], position: string) {
		const at = positions.indexOf(position);
		if (at >= 0) positions.splice(at, 1);
		else positions.push(position);
	}

	// A blank scoring box means the stat is not scored at all.
	function setPoints(stat: string, value: string) {
		if (value === '') delete settings.scoring[stat];
		else settings.scoring[stat] = Number(value);
	}

	function setContinuity(into: string) {
		settings.continuity = into ? { into, land_on: settings.continuity?.land_on ?? 'reserve' } : null;
	}
</script>

<fieldset>
	<legend>Rosters</legend>
	<div class="row end">
		<label class="field">Main roster size <input type="number" min="1" bind:value={settings.roster.main} /></label>
		<label class="field">Reserve list size <input type="number" min="0" bind:value={settings.roster.reserve} /></label>
		<label class="field">
			Who can be on reserve
			<select bind:value={settings.roster.reserve_eligibility}>
				<option value="prospects_only">Prospects only</option>
				<option value="anyone">Anyone</option>
				{#if competition.conferences?.length}<option value="prospects_or_ineligible">Prospects or outside starting conferences</option>{/if}
			</select>
		</label>
		{#if settings.roster.reserve_eligibility === 'anyone'}
			<label class="field">
				Days locked on reserve
				<input type="number" min="0" max="365" bind:value={settings.roster.reserve_lock_days} />
			</label>
		{/if}
	</div>
</fieldset>

<fieldset>
	<legend>Lineup</legend>
	<div class="row end">
		<label class="field">
			Lineups are set
			<select bind:value={settings.lineup.period}>
				<option value="day">Daily</option>
				<option value="week">Weekly</option>
			</select>
		</label>
		{#if settings.lineup.period === 'week'}
			<label class="field">
				A week starts on
				<select bind:value={settings.lineup.week_start}>
					{#each ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'] as weekday (weekday)}
						<option value={weekday}>{weekday[0].toUpperCase() + weekday.slice(1)}</option>
					{/each}
				</select>
			</label>
		{/if}
		<label class="field">
			A starter locks
			<select bind:value={settings.lineup.lock}>
				<option value="game_start">When his game starts</option>
				<option value="period_start">When the day or week starts</option>
			</select>
		</label>
	</div>

	{#if competition.conferences?.length}
        <h3>Starting conferences</h3>
        <p class="muted small-text">These conferences can start and appear in research. Other players remain available for drafts and reserves. Select none to allow all conferences.</p>
        <div class="positions" role="group" aria-label="Starting conferences">
          {#each competition.conferences as conference (conference.id)}
            <label class="check small-text">
              <input type="checkbox" checked={(settings.lineup.conferences ?? competition.defaults.lineup.conferences ?? []).includes(conference.id)} onchange={() => {
                settings.lineup.conferences ??= [...(competition.defaults.lineup.conferences ?? [])];
                togglePosition(settings.lineup.conferences, conference.id);
              }} /> {conference.name}
            </label>
          {/each}
        </div>
    {/if}

    <h3>Starting slots <span class="muted">({starters} starters)</span></h3>
	{#each settings.lineup.slots as slot, i (i)}
		<div class="row end slot">
			<label class="field">Slot <input class="slot-name" bind:value={slot.name} /></label>
			<label class="field">Count <input type="number" min="1" bind:value={slot.count} /></label>
			{#if settings.lineup.period === 'week'}
				<label class="field" title="How many of a player's games count each week in this slot. 0 counts them all.">
					Games a week (0 = all)
					<input type="number" min="0" max="7" bind:value={slot.games_per_week} />
				</label>
			{/if}
			<div class="positions" role="group" aria-label="Positions for {slot.name}">
				{#each [ANY, ...competition.positions] as position (position)}
					<label class="check small-text position">
						<input
							type="checkbox"
							checked={slot.positions.includes(position)}
							onchange={() => togglePosition(slot.positions, position)}
						/>
						{position === ANY ? 'Any' : position}
					</label>
				{/each}
			</div>
			<button type="button" class="small quiet danger" onclick={() => settings.lineup.slots.splice(i, 1)}>Remove</button>
		</div>
	{/each}
	<button type="button" class="small" onclick={() => settings.lineup.slots.push({ name: '', positions: [], count: 1, games_per_week: 0 })}>
		Add a slot
	</button>
</fieldset>

<fieldset>
	<legend>Scoring</legend>
	<p class="muted small-text">Points per stat. Leave a box empty to not score that stat.</p>
	<div class="stats">
		{#each competition.stats as stat (stat.key)}
			<label class="field">
				{stat.label}
				<input
					type="number"
					step="any"
					value={settings.scoring[stat.key] ?? ''}
					oninput={(e) => setPoints(stat.key, e.currentTarget.value)}
				/>
			</label>
		{/each}
	</div>
</fieldset>

<fieldset>
	<legend>Season format</legend>
	<div class="row end">
		<label class="field">
			The title goes to
			<select bind:value={settings.format.type}>
				<option value="total_points">Most total points</option>
				<option value="head_to_head">Head-to-head playoff winner</option>
			</select>
		</label>
		{#if settings.format.type === 'head_to_head'}
			<label class="field">Days per matchup <input type="number" min="1" bind:value={settings.format.matchup_days} /></label>
			<label class="field">Playoff teams <input type="number" min="2" bind:value={settings.format.playoff_teams} /></label>
		{/if}
	</div>
</fieldset>

<fieldset>
	<legend>Free agency</legend>
	<div class="row end">
		<label class="field">
			Between drafts
			<select bind:value={settings.free_agency.mode}>
				<option value="open">Managers can add free agents</option>
				<option value="closed">No adds; drafts and trades only</option>
			</select>
		</label>
		<label class="check">
			<input type="checkbox" bind:checked={settings.free_agency.new_entrants_draft_only} />
			New players must go through the next draft first
		</label>
		<label class="field">
			Acquisitions per week (0 = no limit)
			<input type="number" min="0" max="100" bind:value={settings.free_agency.weekly_limit} />
		</label>
	</div>
</fieldset>

<fieldset>
	<legend>Waivers</legend>
	<div class="row end">
		<label class="field">
			A dropped player
			<select bind:value={settings.waivers.mode}>
				<option value="none">Is a free agent straight away</option>
				<option value="rolling">Goes on waivers: claims by rolling priority</option>
				<option value="faab">Goes on waivers: claims by blind bid (FAAB)</option>
			</select>
		</label>
		{#if settings.waivers.mode !== 'none'}
			<label class="field">Days on waivers <input type="number" min="1" max="14" bind:value={settings.waivers.days} /></label>
		{/if}
		{#if settings.waivers.mode === 'faab'}
			<label class="field">Budget per season <input type="number" min="0" bind:value={settings.waivers.budget} /></label>
		{/if}
	</div>
</fieldset>

<fieldset>
	<legend>Seasonal draft</legend>
	<div class="row end">
		<label class="field">Rounds <input type="number" min="1" bind:value={settings.draft.rounds} /></label>
		<label class="field">
			Order
			<select bind:value={settings.draft.order}>
				<option value="linear">Same order every round</option>
				<option value="snake">Snake</option>
			</select>
		</label>
		<label class="field">
			Seconds per pick (0 = no clock)
			<input type="number" min="0" bind:value={settings.draft.pick_clock_seconds} />
		</label>
		<label class="field">
			Future years of picks that can be traded
			<input type="number" min="0" bind:value={settings.draft.future_years} />
		</label>
	</div>
</fieldset>

<fieldset>
	<legend>Trades</legend>
	<div class="row end">
		<label class="field">Deadline (optional) <input type="date" bind:value={settings.trades.deadline} /></label>
		<label class="field">
			Approval
			<select bind:value={settings.trades.approval}>
				<option value="none">Final when both sides accept</option>
				<option value="commissioner">Commissioner must approve</option>
			</select>
		</label>
	</div>
</fieldset>

{#if others.length > 0}
	<fieldset>
		<legend>Continuity</legend>
		<div class="row end">
			<label class="field">
				When a player moves up, his franchise keeps him in
				<select value={settings.continuity?.into ?? ''} onchange={(e) => setContinuity(e.currentTarget.value)}>
					<option value="">No other league (he is released)</option>
					{#each others as other (other.key)}
						<option value={other.key}>{other.name}</option>
					{/each}
				</select>
			</label>
			{#if settings.continuity}
				<label class="field">
					He lands on the
					<select bind:value={settings.continuity.land_on}>
						<option value="reserve">Reserve list</option>
						<option value="main">Main roster</option>
					</select>
				</label>
			{/if}
		</div>
	</fieldset>
{/if}

<style>
	fieldset {
		border: none;
		border-top: 1px solid var(--rule);
		margin: 0;
		padding: 1.1rem 0 1.3rem;
	}
	fieldset:first-of-type {
		border-top: none;
		padding-top: 0.3rem;
	}
	legend {
		float: left; /* sits inside the fieldset, above its content */
		width: 100%;
		padding: 0;
		margin-bottom: 0.8rem;
		font: 700 1rem/1.2 var(--display);
	}
	legend + * {
		clear: both;
	}
	h3 {
		margin: 1.1rem 0 0.2rem;
		font: 600 0.8rem var(--body);
		color: var(--ink-soft);
	}
	.slot {
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--rule);
	}
	.slot:last-of-type {
		margin-bottom: 0.7rem;
	}
	.slot-name {
		width: 6rem;
	}
	.positions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		flex: 1 1 16rem;
		padding-bottom: 0.3rem;
	}
	.position {
		gap: 0.3rem;
		padding: 0.2rem 0.5rem;
		border: 1px solid var(--rule);
		border-radius: 999px;
		background: var(--surface-2);
	}
	.position:has(:checked) {
		border-color: var(--brand);
		background: var(--brand-soft);
		color: var(--brand);
		font-weight: 600;
	}
	.position input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}
	.position:has(:focus-visible) {
		outline: 2px solid var(--brand);
		outline-offset: 2px;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(10.5rem, 1fr));
		gap: 0.7rem 0.9rem;
	}
</style>
