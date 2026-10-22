// The stats worth a glance for a player, as columns: what a skater and a
// goalie, a hitter and a pitcher are judged by. Football keeps it short.

/** A column adds up one or more of the competition's stat keys. */
export type StatColumn = { key: string; keys: string[]; label: string; title: string };

const col = (keys: string | string[], label: string, title: string): StatColumn => {
	const list = typeof keys === 'string' ? [keys] : keys;
	return { key: list.join('+'), keys: list, label, title };
};

const basketball = [
	col('pts', 'PTS', 'Points'), col('reb', 'REB', 'Rebounds'), col('ast', 'AST', 'Assists'), col('stl', 'STL', 'Steals'),
	col('blk', 'BLK', 'Blocks'), col('tpm', '3PM', 'Three-pointers made'), col('tov', 'TO', 'Turnovers')
];
const skater = [
	col('goals', 'G', 'Goals'), col('assists', 'A', 'Assists'), col('shots', 'SOG', 'Shots on goal'), col('hits', 'HIT', 'Hits'),
	col('blocks', 'BLK', 'Blocked shots'), col('pp_points', 'PPP', 'Power-play points'), col('pim', 'PIM', 'Penalty minutes')
];
const goalie = [
	col('saves', 'SV', 'Saves'), col('goals_against', 'GA', 'Goals against'), col('goalie_wins', 'W', 'Wins'), col('shutouts', 'SO', 'Shutouts')
];
// Every football player alike: yards and touchdowns however they came.
const football = [
	col(['pass_yds', 'rush_yds', 'rec_yds'], 'YDS', 'Yards: passing, rushing and receiving'),
	col('receptions', 'REC', 'Receptions'),
	col(['pass_td', 'rush_td', 'rec_td'], 'TD', 'Touchdowns: passing, rushing and receiving'),
	col('fumbles_lost', 'FUM', 'Fumbles lost')
];
const hitter = [
	col('bat_h', 'H', 'Hits'), col('bat_r', 'R', 'Runs'), col('bat_hr', 'HR', 'Home runs'), col('bat_rbi', 'RBI', 'Runs batted in'),
	col('bat_sb', 'SB', 'Stolen bases'), col('bat_bb', 'BB', 'Walks'), col('bat_so', 'K', 'Strikeouts')
];
const pitcher = [
	col('pit_ip', 'IP', 'Innings pitched'), col('pit_h', 'H', 'Hits allowed'), col('pit_er', 'ER', 'Earned runs'), col('pit_bb', 'BB', 'Walks'),
	col('pit_so', 'K', 'Strikeouts'), col('pit_w', 'W', 'Wins'), col('pit_sv', 'SV', 'Saves')
];

/** The stat columns for a player in his sport, by what he plays. */
export function statColumns(competition: string, positions: string[]): StatColumn[] {
	const is = (...any: string[]) => positions.some((p) => any.includes(p));
	switch (competition) {
		case 'nba':
		case 'wnba':
		case 'cbb':
			return basketball;
		case 'nhl':
			return is('G') ? goalie : skater;
		case 'nfl':
			return football;
		case 'mlb':
			return is('P', 'SP', 'RP') && !is('DH', 'C', '1B', '2B', '3B', 'SS', 'OF', 'LF', 'CF', 'RF') ? pitcher : hitter;
		default:
			return [];
	}
}

/** NHL tables follow the league's active scoring categories, including deductions. */
export function hockeyScoringColumns(goalie: boolean, scoring: Record<string, number>): StatColumn[] {
	const columns = goalie ? [
		...statColumns('nhl', ['G'])
	] : [
		...statColumns('nhl', ['C']),
		col('pp_goals', 'PPG', 'Power-play goals'),
		col('plus_minus', '+/−', 'Plus/minus')
	];
	return columns.filter(column => Number.isFinite(scoring[column.key]) && scoring[column.key] !== 0);
}

/** The most columns any player in the sport has, so every row lines up. */
export const widestStats = (competition: string) =>
	Math.max(...['G', 'C', 'QB', 'P', 'SS'].map((p) => statColumns(competition, [p]).length));

/** A column's value in a stat line. */
export const columnValue = (line: Record<string, number>, column: StatColumn) =>
	column.keys.reduce((n, key) => n + (line[key] ?? 0), 0);

/** Adds up the stat lines of several games. */
export function sumStats(lines: Record<string, number>[]): Record<string, number> {
	const total: Record<string, number> = {};
	for (const line of lines) for (const [key, n] of Object.entries(line ?? {})) total[key] = (total[key] ?? 0) + n;
	return total;
}

/** A stat as a box score shows it: whole numbers plain, innings to a tenth. */
export const statValue = (n: number | undefined) => (n === undefined ? '0' : Number.isInteger(n) ? String(n) : n.toFixed(1));

/** Short names for every stat key, as a box score would print them. */
export const statAbbreviations: Record<string, string> = {
	pts: 'PTS', reb: 'REB', ast: 'AST', stl: 'STL', blk: 'BLK', tpm: '3PM', tpa: '3PA', tov: 'TO', fgm: 'FGM', fga: 'FGA', ftm: 'FTM', fta: 'FTA',
	goals: 'G', assists: 'A', shots: 'SOG', hits: 'HIT', blocks: 'BLK', pp_points: 'PPP', pp_goals: 'PPG', plus_minus: '+/-', pim: 'PIM', saves: 'SV', goals_against: 'GA', goalie_wins: 'W', shutouts: 'SO',
	pass_yds: 'PYD', pass_td: 'PTD', interceptions: 'INT', rush_yds: 'RYD', rush_td: 'RTD', receptions: 'REC', rec_yds: 'RECYD', rec_td: 'RECTD', fumbles_lost: 'FUM',
	bat_ab: 'AB', bat_h: 'H', bat_r: 'R', bat_hr: 'HR', bat_rbi: 'RBI', bat_sb: 'SB', bat_bb: 'BB', bat_so: 'K', pit_ip: 'IP', pit_h: 'H', pit_er: 'ER', pit_bb: 'BB', pit_so: 'K', pit_w: 'W', pit_sv: 'SV', pit_gs: 'GS'
};

/** The stats that scored, as "2 G, 1 A, 4 SOG": every one that is not zero
 * and is worth points under the league's scoring, in the scoring's order. */
export const scoringLine = (line: Record<string, number>, scoring: Record<string, number>) =>
	Object.keys(scoring)
		.filter((key) => scoring[key] !== 0 && line[key])
		.map((key) => `${statValue(line[key])} ${statAbbreviations[key] ?? key.toUpperCase()}`)
		.join(', ');
