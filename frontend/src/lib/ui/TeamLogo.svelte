<script lang="ts">
	// A real team's logo, found by its sport and abbreviation, with the
	// abbreviation beside it unless asked not to. A team without a logo
	// shows its abbreviation alone.
	import { page } from '$app/state';
	import type { ProTeam } from '#lib/api.ts';

	let {
		competition,
		abbrev,
		size = 24,
		label = true
	}: { competition: string; abbrev: string; size?: number; label?: boolean } = $props();

	const team = $derived((page.data.teams as ProTeam[] | undefined)?.find((t) => t.competition === competition && t.abbrev === abbrev));
	let broken = $state(false);
	// The NHL's logos come wide, with room around the crest, and in a light
	// and a dark version: drawn larger so the crest matches the others, and
	// the dark version under a dark theme.
	const padded = $derived(team?.logo_url.includes('assets.nhle.com') ?? false);
	const dark = $derived(team?.logo_url.includes('_light.') ? team.logo_url.replace('_light.', '_dark.') : '');
	const height = $derived(padded ? Math.round(size * 1.5) : size);
</script>

{#if abbrev}
	<span class="team" title={team?.name ?? abbrev}>
		{#if team?.logo_url && !broken}
			<picture style:margin-block={padded ? `${(size - height) / 2}px` : undefined}>
				{#if dark}<source srcset={dark} media="(prefers-color-scheme: dark)" />{/if}
				<img
					src={team.logo_url}
					alt={label ? '' : (team.name ?? abbrev)}
					height={height}
					width={padded ? Math.round(height * 1.5) : size}
					style:margin-inline={padded ? `${-height * 0.22}px` : undefined}
					loading="lazy"
					onerror={() => (broken = true)}
				/>
			</picture>
		{/if}
		{#if label || !team?.logo_url || broken}<span>{abbrev}</span>{/if}
	</span>
{/if}

<style>
	.team {
		display: inline-flex;
		align-items: center;
		gap: 0.3em;
		vertical-align: middle;
		white-space: nowrap;
	}
	picture {
		display: inline-flex;
		flex: none;
	}
	img {
		object-fit: contain;
	}
</style>
