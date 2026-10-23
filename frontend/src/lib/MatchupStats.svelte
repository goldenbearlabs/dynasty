<script lang="ts">
	import type { GameSummary } from '#lib/api.ts';
	import { gameState as stateOf, gameSummary as summaryOf, rowStats, visibleGames, type MatchupGame, type MatchupRow } from '#lib/matchupView.ts';
	import { columnValue, hockeyScoringColumns, statColumns, statValue } from '#lib/statLine.ts';
	import { rosterGroup, rosterGroupOrder } from '#lib/rosterGroups.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { dayLabel, points } from '#lib/ui/time.ts';

	// picking: whether the league's slots count only some games, so a game is picked and can be looked into.
	// scores: the real games by id, for the clock and score of each.
	let { rows, sport, scoring, showOther = false, picking = true, scores = {} }: { rows: MatchupRow[]; sport: string; scoring: Record<string, number>; showOther?: boolean; picking?: boolean; scores?: Record<string, GameSummary> } = $props();
	let expanded = $state<string[]>([]);
	let benchClosed = $state<string[]>([]); // groups whose bench is collapsed
	const columns = (positions: string[]) => sport === 'nhl' ? hockeyScoringColumns(positions.includes('G'), scoring) : sport === 'nfl' ? [
		['pass_yds','PYD','Passing yards'], ['pass_td','PTD','Passing touchdowns'], ['interceptions','INT','Interceptions'],
		['rush_yds','RYD','Rushing yards'], ['rush_td','RTD','Rushing touchdowns'],
		['receptions','REC','Receptions'], ['rec_yds','RECYD','Receiving yards'], ['rec_td','RECTD','Receiving touchdowns'],
		['fumbles_lost','FUM','Fumbles lost']
	].map(([key,label,title]) => ({key,keys:[key],label,title})) : statColumns(sport,positions);
	const groups = $derived.by(() => {
		const grouped: { key: string; label: string; columns: ReturnType<typeof columns>; rows: MatchupRow[] }[] = [];
		for (const row of rows) {
			const cols = columns(row.positions);
			const key = rosterGroup(sport,row.positions,row.bench ? '' : row.slot);
			let group = grouped.find(g => g.key === key);
			if (!group) {
				group = {key, columns: cols, rows: [], label: key === 'Lineup' ? '' : key};
				grouped.push(group);
			}
			group.rows.push(row);
		}
		return grouped.sort((a,b) => rosterGroupOrder(sport).indexOf(a.key)-rosterGroupOrder(sport).indexOf(b.key));
	});
	const visible = (row: MatchupRow) => visibleGames(row, showOther);
	const statsFor = rowStats;
	const shortDate = (day: string) => new Date(`${day}T12:00:00`).toLocaleDateString(undefined,{month:'short',day:'numeric'});
	const gameState = (game: MatchupGame) => stateOf(game, scores);
	const gameSummary = (row: MatchupRow) => summaryOf(row, scores, showOther, picking);
	function toggle(id: string) { expanded = expanded.includes(id) ? expanded.filter(p => p !== id) : [...expanded,id]; }
	function toggleBench(key: string) { benchClosed = benchClosed.includes(key) ? benchClosed.filter(k => k !== key) : [...benchClosed,key]; }
</script>

{#each groups as group (group.key)}
	{#if groups.length > 1}<h3 class="group-title">{group.label}</h3>{/if}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex: The stat table can be scrolled horizontally with the keyboard. -->
	<div class="stat-scroll" role="region" aria-label={group.label ? `${group.label} player stats` : 'Player stats'} tabindex="0">
		<table>
			<thead><tr><th scope="col" class="player-cell">Player / game</th>{#each group.columns as col (col.key)}<th scope="col" class="num" title={col.title}>{col.label}</th>{/each}<th scope="col" class="num fp" title="Counted fantasy points">FP</th></tr></thead>
			<tbody>
				<tr class="section-row"><th colspan={group.columns.length+2}>Starting lineup</th></tr>
				{#each group.rows as row, index (row.id)}
					{@const benchOpen = !benchClosed.includes(group.key)}
					{#if row.bench && (index === 0 || !group.rows[index-1].bench)}<tr class="section-row"><th colspan={group.columns.length+2}><button class="bench-toggle" aria-expanded={benchOpen} onclick={() => toggleBench(group.key)}><Icon name={benchOpen ? 'down' : 'right'} size={11} /> Bench · {group.rows.filter(p => p.bench).length}</button></th></tr>{/if}
					{#if !row.bench || benchOpen}
					{@const stats = statsFor(row)}
					{@const open = expanded.includes(row.id)}
					<tr class="player-row" class:bench-row={row.bench}>
						<th scope="row" class="player-cell">
							<div class="player-identity">{#if picking && !row.empty}<button class="expand" aria-label={`${open ? 'Hide' : 'Show'} games for ${row.name}`} aria-expanded={open} onclick={() => toggle(row.id)}><Icon name={open ? 'down' : 'right'} size={12} /></button>{/if}<span class="slot">{row.slot}</span>{#if row.empty}<span class="muted">Empty</span>{:else}<a href="/player/{row.id}" title={row.name}>{row.name}</a>{/if}</div>
							<div class="game-summary" class:flat={!picking} title={`${gameSummary(row)}${row.earlier ? ' · Started earlier' : ''}`}>{row.empty ? 'No starter selected' : gameSummary(row)}{#if row.earlier} · Earlier{/if}</div>
						</th>
						{#each group.columns as col (col.key)}<td class="num" title={col.title}>{stats.played ? statValue(columnValue(stats.line,col)) : '—'}</td>{/each}
						<td class="num fp player-points" title={row.bench ? 'Bench points, not counted toward the matchup' : undefined}>{#if row.empty}—{:else if row.points || !row.projected}{points(row.points)}{/if}{#if row.projected}<span class="proj" title="Projected for games still to come, from recent seasons and the betting line">{row.points ? '+' : ''}{points(row.projected)}</span>{/if}</td>
					</tr>
					{#if open && picking}
						{#each visible(row) as game, i (i)}
							{@const played = Object.keys(game.stats ?? {}).length > 0}
							<tr class="game-line" class:uncounted={!game.counted}>
								<td class="player-cell game-cell"><div>{#if game.gameId}<a href="/game/{game.gameId}" title={dayLabel(game.day)}>{shortDate(game.day)} {game.opponent}</a>{:else}{shortDate(game.day)} {game.opponent}{/if}</div><small class:counted={game.counted}>{game.label.replace('Selected · Awaiting scored stats','Awaiting scored stats')}{` · ${gameState(game)}`}</small></td>
								{#each group.columns as col (col.key)}<td class="num">{played ? statValue(columnValue(game.stats,col)) : '—'}</td>{/each}
								<td class="num fp">{game.counted || (row.bench && played) ? points(game.points) : game.projected ? '' : '—'}{#if game.projected}<span class="proj" title="Projected">{points(game.projected)}</span>{/if}</td>
							</tr>
						{:else}<tr class="game-line"><td colspan={group.columns.length+2} class="no-games">No games in this view.</td></tr>{/each}
					{/if}
					{/if}
				{/each}
			</tbody>
		</table>
	</div>
{/each}

<style>
	.section-row th { padding:.25rem .45rem; background:var(--surface-2); color:var(--ink-soft); text-align:left; font-size:.58rem; }
	.bench-toggle { display:inline-flex; align-items:center; gap:.25rem; padding:0; border:0; background:transparent; color:inherit; font:inherit; cursor:pointer; }
	.bench-row .slot { color:var(--ink-faint); }
	.bench-row .fp { color:var(--ink-soft); font-weight:500; }
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
	.game-summary.flat { padding-left: 0; }
	.proj { display: block; font-style: italic; font-weight: 500; color: var(--ink-soft); }
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
