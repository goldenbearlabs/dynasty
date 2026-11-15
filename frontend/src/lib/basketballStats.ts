import type { LineupGame, LineupPlayer, RosterPerformancePlayer } from '#lib/api.ts';
import { sumStats } from '#lib/statLine.ts';

export const isBasketball = (competition: string) => ['nba', 'cbb', 'wnba'].includes(competition);
export const defaultRosterStatsView = (competition: string) => isBasketball(competition) ? 'matchup' : 'today';

/** Keep the whole box score from the game with the most fantasy points. */
export function bestBasketballGame<T extends { status: string; points: number; stats: Record<string, number> }>(games: T[]): T | undefined {
	return games.reduce<T | undefined>((best, game) => {
		const appeared = game.stats?.min !== undefined ? game.stats.min > 0 : Object.keys(game.stats ?? {}).length > 0;
		if (game.status === 'scheduled' || !appeared) return best;
		return !best || game.points > best.points ? game : best;
	}, undefined);
}

export function basketballMatchupGames(player: LineupPlayer): LineupGame[] {
	if (player.slot) return player.games.filter(game => game.status !== 'scheduled' && (game.counts || game.locked_in));
	const best = bestBasketballGame(player.games);
	return best ? [best] : [];
}

/** The My Team matchup view uses scoring selections, rather than real weekly totals. */
export function basketballMatchupPerformance(player: LineupPlayer): RosterPerformancePlayer {
	const games = basketballMatchupGames(player);
	const points = games.reduce((total, game) => total + game.points, 0);
	return {
		player_id: player.player_id, games: games.length, stats: sumStats(games.map(game => game.stats)),
		points, points_per_game: games.length ? points / games.length : 0,
		overall_rank: null, position_ranks: {}
	};
}
