<script lang="ts">
	import type { MatchupRow } from '#lib/matchupView.ts';
	import { columnValue, statColumns, statValue, sumStats } from '#lib/statLine.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel, points, timeOfDay } from '#lib/ui/time.ts';

	let { rows, sport, showOther = false }: { rows: MatchupRow[]; sport: string; showOther?: boolean } = $props();
	let expanded = $state<string[]>([]);
	const columns = (positions: string[]) => sport === 'nfl' ? [
		['pass_yds','PYD','Passing yards'], ['pass_td','PTD','Passing touchdowns'], ['interceptions','INT','Interceptions'],
		['rush_yds','RYD','Rushing yards'], ['rush_td','RTD','Rushing touchdowns'],
		['receptions','REC','Receptions'], ['rec_yds','RECYD','Receiving yards'], ['rec_td','RECTD','Receiving touchdowns'],
		['fumbles_lost','FUM','Fumbles lost']
	].map(([key,label,title]) => ({key,keys:[key],label,title})) : statColumns(sport,positions);
	const groups = $derived.by(() => {
		const grouped: { key: string; label: string; columns: ReturnType<typeof columns>; rows: MatchupRow[] }[] = [];
		for (const row of rows) {
			const cols = columns(row.positions);
			const key = cols.map(c => c.key).join(',');
			let group = grouped.find(g => g.key === key);
			if (!group) {
				group = {key, columns: cols, rows: [], label: sport === 'nhl' ? row.positions.includes('G') ? 'Goalies' : 'Skaters' : sport === 'mlb' ? cols[0]?.key === 'pit_ip' ? 'Pitchers' : 'Hitters' : ''};
				grouped.push(group);
			}
			group.rows.push(row);
		}
		return grouped;
	});
	const visible = (row: MatchupRow) => row.games.filter(g => showOther || g.counted || g.label.startsWith('Selected') || g.label === 'Auto candidate');
	const statsFor = (row: MatchupRow) => {
		const games = row.games.filter(g => g.counted);
		return { line: sumStats(games.map(g => g.stats)), played: games.some(g => Object.keys(g.stats ?? {}).length > 0) };
	};
	const shortDate = (day: string) => new Date(`${day}T12:00:00`).toLocaleDateString(undefined,{month:'short',day:'numeric'});
	function gameSummary(row: MatchupRow): string {
		const games = visible(row);
		const auto = row.selection.startsWith('Auto');
		if (games.length === 1) return `${shortDate(games[0].day)} ${games[0].opponent}${auto ? ' · Auto' : games[0].label.startsWith('Selected') ? ' · Picked' : ''}`;
		if (games.length > 1) return `${games.length} games${auto ? ' · Auto' : ''}`;
		return auto ? 'Auto · waiting for a game' : 'No selected game';
	}
	function toggle(id: string) { expanded = expanded.includes(id) ? expanded.filter(p => p !== id) : [...expanded,id]; }
</script>

{#each groups as group (group.key)}
	{#if groups.length > 1}<h3 class="group-title">{group.label}</h3>{/if}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex: The stat table can be scrolled horizontally with the keyboard. -->
	<div class="stat-scroll" role="region" aria-label={group.label ? `${group.label} player stats` : 'Player stats'} tabindex="0">
		<table>
			<thead><tr><th scope="col" class="player-cell">Player / game</th>{#each group.columns as col (col.key)}<th scope="col" class="num" title={col.title}>{col.label}</th>{/each}<th scope="col" class="num fp" title="Counted fantasy points">FP</th></tr></thead>
			<tbody>
				{#each group.rows as row (row.id)}
					{@const stats = statsFor(row)}
					{@const open = expanded.includes(row.id)}
					<tr class="player-row">
						<th scope="row" class="player-cell">
							<div class="player-identity"><button class="expand" aria-label={`${open ? 'Hide' : 'Show'} games for ${row.name}`} aria-expanded={open} onclick={() => toggle(row.id)}><Icon name={open ? 'down' : 'right'} size={12} /></button><span class="slot">{row.slot}</span><a href="/player/{row.id}" title={row.name}>{row.name}</a></div>
							<div class="game-summary" title={`${gameSummary(row)}${row.earlier ? ' · Started earlier' : ''}`}>{gameSummary(row)}{#if row.earlier} · Earlier{/if}</div>
						</th>
						{#each group.columns as col (col.key)}<td class="num" title={col.title}>{stats.played ? statValue(columnValue(stats.line,col)) : '—'}</td>{/each}
						<td class="num fp player-points">{points(row.points)}</td>
					</tr>
					{#if open}
						{#each visible(row) as game, i (i)}
							{@const played = Object.keys(game.stats ?? {}).length > 0}
							<tr class="game-line" class:uncounted={!game.counted}>
								<td class="player-cell game-cell"><div>{#if game.gameId}<a href="/game/{game.gameId}" title={dayLabel(game.day)}>{shortDate(game.day)} {game.opponent}</a>{:else}{shortDate(game.day)} {game.opponent}{/if}</div><small class:counted={game.counted}>{game.label.replace('Selected · Awaiting scored stats','Awaiting scored stats')}{game.status === 'live' ? ' · Live' : game.status === 'final' ? ' · Final' : ` · ${timeOfDay(game.starts_at)}`}</small></td>
								{#each group.columns as col (col.key)}<td class="num">{played ? statValue(columnValue(game.stats,col)) : '—'}</td>{/each}
								<td class="num fp">{game.counted ? points(game.points) : '—'}</td>
							</tr>
						{:else}<tr class="game-line"><td colspan={group.columns.length+2} class="no-games">No games in this view.</td></tr>{/each}
					{/if}
				{/each}
			</tbody>
		</table>
	</div>
{/each}

<style>
	.stat-scroll { overflow-x: auto; scrollbar-width: thin; }
	table { width: 100%; font-size: .72rem; border-collapse: collapse; font-variant-numeric: tabular-nums; }
	th, td { padding: .4rem .25rem; border-bottom: 1px solid var(--rule); white-space: nowrap; }
	thead th { background: var(--surface-2); color: var(--ink-soft); font-size: .6rem; padding-block: .35rem; font-weight: 650; }
	th.num, td.num { text-align: right; min-width: 1.85rem; }
	.player-cell { text-transform: none; letter-spacing: 0; position: sticky; left: 0; z-index: 1; background: var(--surface); text-align: left; width: 10.5rem; min-width: 10.5rem; max-width: 10.5rem; padding-left: .45rem; }
	.player-identity { display: flex; align-items: center; gap: .2rem; font-weight: 600; }
	.player-identity a { overflow: hidden; text-overflow: ellipsis; color: var(--ink); }
	.slot { flex: none; font-size: .55rem; color: var(--ink-soft); min-width: 1.05rem; }
	.expand { flex: none; padding: .15rem 0; border: 0; background: transparent; color: var(--ink-soft); }
	.game-summary { font-size: .6rem; color: var(--ink-soft); font-weight: 400; padding-left: 1rem; overflow: hidden; text-overflow: ellipsis; margin-top: .1rem; }
	.fp { position: sticky; right: 0; z-index: 1; background: var(--surface); border-left: 1px solid var(--rule); padding-inline: .45rem; font-weight: 750; min-width: 3rem; }
	thead .fp { background: var(--surface-2); }
	.game-line td { font-size: .67rem; background: var(--surface-2); padding-block: .25rem; }
	.game-cell { font-weight: 400; padding-left: 1.5rem; }
	.game-cell div { overflow: hidden; text-overflow: ellipsis; }
	.game-cell small { display: block; font-size: .55rem; color: var(--ink-soft); white-space: normal; }
	.game-cell .counted { color: var(--good); }
	.game-line .fp { font-weight: 600; }
	.uncounted td:not(.player-cell) { color: var(--ink-soft); }
	.group-title { margin: 0; padding: .35rem .5rem; font-size: .65rem; background: var(--surface-2); border-bottom: 1px solid var(--rule); }
	.no-games { color: var(--ink-soft); padding-left: 1rem; }
	@media(max-width:640px) { .player-cell { min-width: 9rem; max-width: 9rem; width: 9rem; } }
</style>
