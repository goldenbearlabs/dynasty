<script lang="ts">
	import { getRosterPerformance, type Competition, type League, type LeagueRoster, type RosterPerformance } from '#lib/api.ts';
	import { onScoresChange } from '#lib/socket.svelte.ts';
	import InjuryBadge from '#lib/ui/InjuryBadge.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';
	import { statValue } from '#lib/statLine.ts';

	let { league, roster, franchiseId, competition }: { league: League; roster: LeagueRoster; franchiseId: string; competition: Competition } = $props();
	let period = $state('current');
	let list = $state('all');
	let result = $state<RosterPerformance>();
	let loading = $state(true);
	let error = $state('');
	let sort = $state('points');
	let ascending = $state(false);
	let request = 0;
	async function load() {
		const token = ++request;
		loading = true;
		try {
			const historical = period.startsWith('season:');
			const found = await getRosterPerformance(league.id,franchiseId,historical ? 'season' : period,historical ? period.slice(7) : undefined);
			if (token === request) { result = found; error = ''; }
		} catch (e) { if (token === request) error = String(e); }
		finally { if (token === request) loading = false; }
	}
	$effect(() => { void [league.id,franchiseId,period,roster.players]; load(); return () => { request++; }; });
	onScoresChange(() => [league.competition],load);
	const columns = $derived.by(() => {
		const definitions = competition.stats.map(s => ({key:s.key,label:s.label}));
		const known = new Set(definitions.map(s => s.key));
		for (const player of result?.players ?? []) for (const key of Object.keys(player.stats)) {
			if (!known.has(key)) { definitions.push({key,label:key.replaceAll('_',' ')}); known.add(key); }
		}
		return definitions;
	});
	const abbreviations: Record<string,string> = {
		pts:'PTS',reb:'REB',ast:'AST',stl:'STL',blk:'BLK',tpm:'3PM',tpa:'3PA',tov:'TO',fgm:'FGM',fga:'FGA',ftm:'FTM',fta:'FTA',
		goals:'G',assists:'A',shots:'SOG',hits:'HIT',blocks:'BLK',pp_points:'PPP',pim:'PIM',saves:'SV',goals_against:'GA',goalie_wins:'W',shutouts:'SO',
		pass_yds:'PASS YD',pass_td:'PASS TD',interceptions:'INT',rush_yds:'RUSH YD',rush_td:'RUSH TD',receptions:'REC',rec_yds:'REC YD',rec_td:'REC TD',fumbles_lost:'FUM',
		bat_ab:'AB',bat_h:'B H',bat_r:'R',bat_hr:'HR',bat_rbi:'RBI',bat_sb:'SB',bat_bb:'B BB',bat_so:'B K',
		pit_ip:'IP',pit_h:'P H',pit_er:'ER',pit_bb:'P BB',pit_so:'P K',pit_w:'W',pit_sv:'SV',pit_gs:'GS'
	};
	const players = $derived.by(() => {
		const rows = roster.players.filter(p => list === 'all' || p.list === list).map(player => ({...player,performance:result?.players.find(p => p.player_id === player.player_id)}));
		return rows.toSorted((a,b) => {
			if(sort === 'name') return (ascending ? 1 : -1) * a.full_name.localeCompare(b.full_name);
			const value = (p: typeof a) => {
				const data = p.performance;
				if(!data || !data.games) return null;
				if(sort === 'overall_rank') return data.overall_rank;
				if(sort === 'games') return data.games;
				if(sort === 'points') return data.points;
				if(sort === 'points_per_game') return data.points_per_game;
				return data.stats[sort] ?? null;
			};
			const av=value(a),bv=value(b);
			if(av === null || bv === null) return av === bv ? a.full_name.localeCompare(b.full_name) : av === null ? 1 : -1;
			return (ascending ? av-bv : bv-av) || a.full_name.localeCompare(b.full_name);
		});
	});
	function order(key: string) {
		if(sort === key) ascending=!ascending;
		else {sort=key;ascending=key === 'name' || key === 'overall_rank';}
	}
	const sortLabel = (key: string) => sort === key ? ascending ? ' ↑' : ' ↓' : '';
</script>

<div class="stack tight roster-stats" data-sport={league.competition}>
	<div class="spread filters">
		<label class="row small-text">Stats period
			<select aria-label="Player stats period" bind:value={period}>
				<option value="current">Current season{result?.current_season ? ` · ${result.current_season}` : ''}</option>
				<option value="7">Last 7 days</option><option value="14">Last 14 days</option><option value="30">Last 30 days</option>
				{#if result?.seasons.length}<optgroup label="Past seasons">{#each result.seasons.filter(s => s.label !== result?.current_season) as season (season.label)}<option value={`season:${season.label}`}>{season.label}</option>{/each}</optgroup>{/if}
			</select>
		</label>
		<label class="row small-text">Players<select aria-label="Roster list" bind:value={list}><option value="all">All players</option><option value="main">Main roster</option><option value="reserve">Reserve list</option><option value="rights">Unsigned</option></select></label>
	</div>
	{#if error}<p role="alert">{error}</p><button class="small" onclick={load}>Try again</button>{/if}
	{#if loading}<p class="muted small-text" role="status">Loading stats and ranks…</p>
	{:else if result && !error}
		<p class="muted small-text context">{result.season || `Last ${result.window} days`}{#if result.source === 'games'} · Recorded games, {dayLabel(result.from_day)} – {dayLabel(result.to_day)}{:else} · Imported season totals{/if}. Ranks use total fantasy points across {result.ranked_players} players with games in this sport.</p>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex: The complete stat table supports horizontal keyboard scrolling. -->
		<div class="panel stat-scroll" role="region" aria-label="Roster player stats and ranks" tabindex="0">
			<table>
				<thead><tr>
					<th scope="col" class="player-name"><button onclick={() => order('name')}>Player{sortLabel('name')}</button></th>
					<th scope="col" class="num" aria-sort={sort === 'overall_rank' ? ascending ? 'ascending' : 'descending' : undefined}><button title="Overall rank by total fantasy points in this sport" onclick={() => order('overall_rank')}>Rank{sortLabel('overall_rank')}</button></th>
					<th scope="col">Position rank</th>
					<th scope="col" class="num"><button onclick={() => order('games')}>GP{sortLabel('games')}</button></th>
					<th scope="col" class="num emphasis"><button onclick={() => order('points')}>FP{sortLabel('points')}</button></th>
					<th scope="col" class="num emphasis"><button onclick={() => order('points_per_game')}>FP/G{sortLabel('points_per_game')}</button></th>
					{#each columns as column (column.key)}<th scope="col" class="num" title={column.label}><button onclick={() => order(column.key)}>{abbreviations[column.key] ?? column.key.replaceAll('_',' ').toUpperCase()}{sortLabel(column.key)}</button></th>{/each}
				</tr></thead>
				<tbody>
					{#each players as player (player.player_id)}
						{@const stats=player.performance}
						<tr>
							<th scope="row" class="player-name"><a href="/player/{player.player_id}">{player.full_name}</a><InjuryBadge designation={player.injury_designation} /><span class="player-meta">{player.positions.join('/')} · {player.list === 'main' ? 'Main' : player.list === 'reserve' ? 'Reserve' : 'Unsigned'}</span></th>
							<td class="num rank">{stats?.overall_rank ? `#${stats.overall_rank}` : '—'}</td>
							<td class="position-ranks">{#each Object.entries(stats?.position_ranks ?? {}) as [position,rank] (position)}<span>{position} #{rank}</span>{:else}—{/each}</td>
							<td class="num">{stats?.games || '—'}</td>
							<td class="num emphasis">{stats?.games ? points(stats.points) : '—'}</td>
							<td class="num emphasis">{stats?.games ? points(stats.points_per_game) : '—'}</td>
							{#each columns as column (column.key)}<td class="num">{stats?.games && stats.stats[column.key] !== undefined ? statValue(stats.stats[column.key]) : '—'}</td>{/each}
						</tr>
					{:else}<tr><td colspan={columns.length+6} class="empty">No players on this list.</td></tr>{/each}
				</tbody>
			</table>
		</div>
		<p class="muted small-text context">FP = total fantasy points; FP/G = fantasy points per game. Ties share a rank. — means no recorded data for the selected period. All production uses {league.name} scoring, including games played while benched or before joining this roster.</p>
	{/if}
</div>

<style>
	.roster-stats { min-width: 0; grid-template-columns: minmax(0,1fr); }
	.filters { gap: .5rem; }
	.filters label { gap: .4rem; }
	.filters select { font-size: .78rem; padding: .35rem .5rem; }
	.stat-scroll { overflow-x: auto; scrollbar-width: thin; }
	table { width: 100%; font-size: .75rem; font-variant-numeric: tabular-nums; }
	th,td { padding: .45rem .5rem; white-space: nowrap; }
	thead th { font-size: .65rem; }
	thead button { padding: 0; border: 0; background: transparent; font: inherit; color: inherit; }
	.player-name { position: sticky; left: 0; z-index: 1; background: var(--surface); min-width: 10rem; max-width: 14rem; text-align: left; text-transform: none; letter-spacing: 0; }
	thead .player-name { background: var(--surface-2); }
	.player-name a { color: var(--ink); font: 600 .78rem var(--body); }
	.player-meta { display: block; color: var(--ink-soft); font: 400 .65rem var(--body); margin-top: .15rem; }
	.position-ranks span { display: inline-block; font-size: .7rem; margin-right: .4rem; }
	.emphasis { font-weight: 750; background: var(--surface-2); }
	.rank { color: var(--brand); font-weight: 700; }
	.context { font-size: .67rem; }
	.empty { padding: .75rem; color: var(--ink-soft); }
	@media(max-width:640px) { .filters label { width: 100%; justify-content: space-between; } .player-name { min-width: 8rem; max-width: 10rem; } }
</style>
