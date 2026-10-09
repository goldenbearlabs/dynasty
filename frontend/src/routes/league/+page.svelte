<script lang="ts">
	// The league at a glance: your rosters, every franchise, the drafts, and
	// what just happened.
	import { invalidateAll } from '$app/navigation';
	import Activity from '#lib/Activity.svelte';
	import DraftCard from '#lib/DraftCard.svelte';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import Standings from '#lib/Standings.svelte';
	import Crest from '#lib/ui/Crest.svelte';
	import Meter from '#lib/ui/Meter.svelte';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const onList = (players: { list: string }[], list: string) => players.filter((p) => p.list === list).length;

	// Standings follow the games as they are played.
	onScoresChange(() => data.dynasty?.leagues.map((l) => l.competition) ?? [], invalidateAll);
</script>

<svelte:head><title>League · {data.dynasty?.name ?? 'Crossover Dynasty'}</title></svelte:head>

{#if data.dynasty}
	<div class="stack">
		<header>
			<p class="eyebrow">{data.dynasty.leagues.length}-sport dynasty · {data.dynasty.franchises.length} franchises</p>
			<h1>{data.dynasty.name}</h1>
		</header>

		<section class="stack tight">
			<h2>Standings</h2>
			<Standings
				identities={data.dynasty.team_identities}
 tables={data.standings}
				overall={data.overall}
				franchises={data.dynasty.franchises}
				leagues={data.dynasty.leagues}
			/>
		</section>

		{#if data.mine}
			<section class="stack tight">
				<div class="spread">
					<h2>Your rosters</h2>
					<a href="/franchise/{data.mine.franchise.slug}">Manage {data.mine.franchise.name}</a>
				</div>
				<div class="sports">
					{#each data.mine.rosters as roster (roster.league_id)}
						<a class="sport" data-sport={roster.competition} href="/lineup/{roster.competition}">
							<div class="spread">
								<SportBadge sport={roster.competition} solid />
								{#if roster.overage > 0}<span class="pill bad">Over by {roster.overage}</span>{/if}
							</div>
							<div class="row"><Crest name={roster.team_name} src={roster.image_url} size={30} /><strong class="league">{roster.team_name}</strong></div><span class="muted small-text">{roster.name}</span>
							<Meter label="Main" value={onList(roster.players, 'main')} max={roster.limits.main} />
							<Meter label="Reserve" value={onList(roster.players, 'reserve')} max={roster.limits.reserve} />
						</a>
					{/each}
				</div>
			</section>
		{:else}
			<p class="card signin">
				<strong>You are not signed in.</strong>
				<span><a href="/">Sign in</a> to manage a team. You can still look around.</span>
			</p>
		{/if}

		<section class="stack tight">
			<h2>Franchises</h2>
			<div class="franchises">
				{#each data.dynasty.franchises as f (f.id)}
					<a class="franchise" href="/franchise/{f.slug}">
						<Crest src={f.image_url} name={f.name} size={44} />
						<div>
							<strong>{f.name}</strong>
							<div class="muted small-text">{f.manager_name}</div>
						</div>
						<div class="row tags">
							{#if f.id === data.me?.id}<span class="pill brand">You</span>{/if}
							{#if f.is_commissioner}<span class="pill gold">Commissioner</span>{/if}
						</div>
					</a>
				{/each}
			</div>
		</section>

		{#if data.drafts.length > 0}
			<section class="stack tight">
				<div class="spread">
					<h2>Drafts</h2>
					<a href="/drafts">All drafts</a>
				</div>
				<div class="franchises">
					{#each data.drafts.slice(0, 3) as draft (draft.id)}<DraftCard {draft} />{/each}
				</div>
			</section>
		{/if}

		<section class="stack tight">
			<h2>Activity</h2>
			<Activity items={data.activity} />
		</section>
	</div>
{/if}

<style>
	header {
		display: grid;
		gap: 0.4rem;
	}
	a.sport, a.franchise {
		color: var(--ink);
		transition: border-color 0.12s;
	}
	a.sport:hover, a.franchise:hover {
		text-decoration: none;
		background: var(--surface-2);
	}

	.sports {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12.5rem, 1fr));
		gap: 0;
		border-block: 1px solid var(--rule);
		background: var(--surface);
	}
	.sport {
		display: grid;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-left: 2px solid var(--sport);
	}
	.league {
		font: 700 1.05rem/1.2 var(--display);
	}

	.signin {
		display: grid;
		gap: 0.2rem;
	}

	.franchises {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(17rem, 1fr));
		gap: 0.4rem;
	}
	.franchise {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: center;
		gap: 0.65rem;
		padding: 0.65rem 0;
		border-bottom: 1px solid var(--rule);
	}
	.tags {
		gap: 0.3rem;
		justify-content: end;
	}
</style>
