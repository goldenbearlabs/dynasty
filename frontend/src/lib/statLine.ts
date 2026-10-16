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
