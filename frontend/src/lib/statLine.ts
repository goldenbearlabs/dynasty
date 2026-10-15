// A player's game in a few words: "24 PTS · 8 REB · 5 AST". Each sport lists
// the stats worth a glance, in order; those he did not record are left out.

const glance: Record<string, [key: string, label: string][]> = {
	basketball: [['pts', 'PTS'], ['reb', 'REB'], ['ast', 'AST'], ['stl', 'STL'], ['blk', 'BLK'], ['tpm', '3PM'], ['tov', 'TO']],
	nhl: [
		['goals', 'G'], ['assists', 'A'], ['shots', 'SOG'], ['hits', 'HIT'], ['blocks', 'BLK'], ['pim', 'PIM'],
		['saves', 'SV'], ['goals_against', 'GA'], ['goalie_wins', 'W'], ['shutouts', 'SO']
	],
	nfl: [
		['pass_yds', 'PASS YDS'], ['pass_td', 'PASS TD'], ['interceptions', 'INT'], ['rush_yds', 'RUSH YDS'], ['rush_td', 'RUSH TD'],
		['receptions', 'REC'], ['rec_yds', 'REC YDS'], ['rec_td', 'REC TD'], ['fumbles_lost', 'FUM']
	],
	mlb: [
		['bat_h', 'H'], ['bat_r', 'R'], ['bat_hr', 'HR'], ['bat_rbi', 'RBI'], ['bat_sb', 'SB'], ['bat_bb', 'BB'], ['bat_so', 'K'],
		['pit_ip', 'IP'], ['pit_h', 'H'], ['pit_er', 'ER'], ['pit_bb', 'BB'], ['pit_so', 'K'], ['pit_w', 'W'], ['pit_sv', 'SV']
	]
};
const family: Record<string, string> = { nba: 'basketball', wnba: 'basketball', cbb: 'basketball' };

/** The stats he recorded, at most `most` of them, or '' before he has any. */
export function statLine(competition: string, stats: Record<string, number> | undefined, most = 5): string {
	if (!stats) return '';
	return (glance[family[competition] ?? competition] ?? [])
		.filter(([key]) => stats[key])
		.slice(0, most)
		.map(([key, label]) => `${Number.isInteger(stats[key]) ? stats[key] : stats[key].toFixed(1)} ${label}`)
		.join(' · ');
}
