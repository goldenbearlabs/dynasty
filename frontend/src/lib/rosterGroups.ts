/** Keep each sport's lineup together, separating positions with different box scores. */
export function rosterGroup(sport: string, positions: string[], slot = ''): string {
	if (sport === 'nhl') {
		return slot === 'G' || (!slot && positions.includes('G')) ? 'Goalies' : 'Skaters';
	}
	if (sport === 'mlb') {
		const pitcher = ['SP', 'RP', 'P'].includes(slot) || (!slot &&
			positions.some(p => ['SP', 'RP', 'P'].includes(p)) &&
			!positions.some(p => ['DH', 'C', '1B', '2B', '3B', 'SS', 'OF', 'LF', 'CF', 'RF'].includes(p)));
		return pitcher ? 'Pitchers' : 'Hitters';
	}
	return 'Lineup';
}

export const rosterGroupOrder = (sport: string) =>
	sport === 'nhl' ? ['Skaters', 'Goalies'] : sport === 'mlb' ? ['Hitters', 'Pitchers'] : ['Lineup'];
