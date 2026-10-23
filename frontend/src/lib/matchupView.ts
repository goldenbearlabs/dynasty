import type { GameSummary, Lineup, LineupGame, MatchupScorer, Period, ScoredGame } from '#lib/api.ts';
import { sumStats } from '#lib/statLine.ts';
import { sportsToday, timeOfDay } from '#lib/ui/time.ts';

export type MatchupGame = {
	day: string; starts_at: string; status: string; opponent: string;
	stats: Record<string, number>; points: number; counted: boolean;
	label: string; gameId?: string;
	/** Whether his team is at home; unknown for a scored game no longer on his lineup. */
	at_home?: boolean;
	/** Projected fantasy points if it is still to be played and will count; otherwise 0. */
	projected: number;
	/** What was expected of him in it, before it started, whether or not it counts. */
	expected: number;
};
export type MatchupRow = {
	id: string; name: string; headshot: string; positions: string[]; slot: string;
	/** Counted fantasy points; for a bench player, what his games would have been worth. */
	points: number; games: MatchupGame[]; selection: string; earlier: boolean;
	/** Projected fantasy points from the games in view still to be played that will count. */
	projected: number;
	/** Whether any game behind his points has started. */
	started: boolean;
	/** What was expected of him in the games behind his points; with settled, once they are all final. */
	pregame: number;
	settled: boolean;
	/** Before any game of his counts: the projection of his next start, to show in place of a zero. */
	upcoming: number;
	bench?: boolean;
	empty?: boolean;
};

/** Official contributions determine points; the lineup adds selected and upcoming games. */
export function matchupRows(scored: MatchupScorer[], lineup: Lineup | undefined, period: Period,
	wholePeriod: boolean, day: string, weekly: boolean): MatchupRow[] {
	const first = wholePeriod ? period.starts_on : weekly ? (lineup?.day ?? day) : day;
	const last = wholePeriod ? period.ends_on : weekly ? (lineup?.last_day ?? day) : day;
	const inRange = (d: string) => d >= first && d <= last && d >= period.starts_on && d <= period.ends_on;
	const listed = lineup?.players ?? [];
	const ids = [...new Set([...listed.map(p => p.player_id), ...scored.filter(p => p.game_lines?.some(g => inRange(g.day))).map(p => p.player_id)])];
	const rows: MatchupRow[] = ids.map(id => {
		const player = listed.find(p => p.player_id === id);
		const contribution = scored.find(p => p.player_id === id);
		const official = (contribution?.game_lines ?? []).filter(g => inRange(g.day));
		const raw = (player?.games ?? []).filter(g => inRange(g.day));
		const matches = (g: LineupGame, line: ScoredGame) => g.game_id ? g.game_id === line.game_id :
			g.day === line.day && new Date(g.starts_at).getTime() === new Date(line.starts_at).getTime();
		// On auto, the game expected to count is his last still to come.
		const benched = !!player && !player.slot;
		const autoGame = !benched && player?.counts_last ? raw.filter(g => g.status === 'scheduled').at(-1) : undefined;
		const games: MatchupGame[] = raw.map(g => {
			const line = official.find(line => matches(g, line));
			const capped = lineup?.starts?.made.some(s => s.player_id === id && s.day === g.day && !s.counts);
			const hasStats = Object.keys(g.stats ?? {}).length > 0;
			const selected = g.counts && !capped;
			return {
				...g, opponent: `${g.at_home ? 'vs' : '@'} ${g.opponent}`, counted: !!line,
				points: line?.points ?? (benched ? g.points : 0), gameId: line?.game_id ?? g.game_id,
				projected: g.status === 'scheduled' && !benched && !capped && (g.counts || g === autoGame) ? g.projected : 0,
				expected: g.expected ?? 0,
				label: line ? 'Counted' : benched ? 'Bench · Not counted' : capped ? 'Start cap reached' : player?.counts_last && !hasStats && g.status !== 'final'
					? 'Auto candidate' : selected ? (g.status === 'scheduled' ? 'Selected' : 'Selected · Awaiting scored stats') : 'Not selected'
			};
		});
		for (const line of official) {
			if (raw.some(g => matches(g, line))) continue;
			games.push({ ...line, gameId: line.game_id, opponent: `${line.away} @ ${line.home}`, counted: true, label: 'Counted', projected: 0, expected: 0 });
		}
		games.sort((a, b) => a.starts_at.localeCompare(b.starts_at));
		// The games behind his points: those that count, or on the bench every one he played.
		const scoring = games.filter((g) => g.status !== 'scheduled' && (benched ? Object.keys(g.stats ?? {}).length > 0 : g.counted));
		const projected = games.reduce((total, game) => total + game.projected, 0);
		const next = games.find((g) => g.status === 'scheduled' && (g.label.startsWith('Selected') || g.label === 'Auto candidate'))
			?? games.find((g) => g.status === 'scheduled');
		return {
			started: scoring.length > 0,
			pregame: scoring.reduce((total, game) => total + game.expected, 0),
			settled: scoring.length > 0 && scoring.every((g) => g.status === 'final'),
			upcoming: projected || (!benched && next ? next.expected : 0),
			id, name: player?.full_name ?? contribution!.full_name, headshot: player?.headshot_url ?? contribution!.headshot_url,
			positions: player?.positions ?? official[0]?.positions ?? [], slot: benched ? 'BN' : player?.slot ?? official[0]?.slot ?? '',
			points: benched ? games.reduce((total, game) => total + game.points, 0) : official.reduce((total, game) => total + game.points, 0), games,
			projected,
			selection: player?.counts_last ? 'Auto · last games with stats' : player?.counts_from ? `Picked from ${player.counts_from}` : '',
			bench: benched,
			earlier: !player || (benched && official.length > 0)
		};
	});
	const ordered: MatchupRow[] = [];
	const used = new Set<string>();
	for (const slot of lineup?.slots ?? []) {
		for(let i=0; i<slot.count; i++) {
			const row = rows.find(row => !row.bench && !row.earlier && row.slot === slot.name && !used.has(row.id));
			if(row) { ordered.push(row); used.add(row.id); }
			else ordered.push({id:`empty-${slot.name}-${i}`, name:'Empty', headshot:'', positions:slot.positions, slot:slot.name, points:0, projected:0, started:false, pregame:0, settled:false, upcoming:0, games:[], selection:'', earlier:false, empty:true});
		}
	}
	return [...ordered,...rows.filter(row => !used.has(row.id) && !row.bench),...rows.filter(row => row.bench)];
}

const weekday = (day: string) => new Date(`${day}T12:00:00`).toLocaleDateString(undefined, { weekday: 'short' });

/** Where a game stands: its start time, or the score with the clock once it starts. scores: the real games by id. */
export function gameState(game: MatchupGame, scores: Record<string, GameSummary>): string {
	const live = game.gameId ? scores[game.gameId] : undefined;
	const status = live?.status ?? game.status;
	if (!live || status === 'scheduled') return `${game.day === sportsToday() ? '' : `${weekday(game.day)} `}${timeOfDay(game.starts_at)}`;
	// From the player's side when known, otherwise away – home as the game is listed.
	const [ours, theirs] = game.at_home === undefined ? [live.away_score, live.home_score] : game.at_home ? [live.home_score, live.away_score] : [live.away_score, live.home_score];
	const result = status === 'final' && game.at_home !== undefined ? (ours > theirs ? 'W ' : ours < theirs ? 'L ' : 'T ') : '';
	return `${result}${ours}–${theirs} · ${live.detail || (status === 'final' ? 'Final' : 'Live')}`;
}

/** The games a row shows: on the bench all of them; otherwise the ones that count, are picked, or with showOther every one. */
export const visibleGames = (row: MatchupRow, showOther = false) =>
	row.games.filter((g) => row.bench || showOther || g.counted || g.label.startsWith('Selected') || g.label === 'Auto candidate');

/** One line on a row's games: the opponent and where the game stands, or how many there are. picking: slots count only some games. */
export function gameSummary(row: MatchupRow, scores: Record<string, GameSummary>, showOther = false, picking = true): string {
	const games = visibleGames(row, showOther);
	const auto = !row.bench && row.selection.startsWith('Auto');
	if (games.length === 1) return `${games[0].opponent} · ${gameState(games[0], scores)}${auto ? ' · Auto' : picking && games[0].label.startsWith('Selected') ? ' · Picked' : ''}`;
	const playing = games.find((g) => g.gameId && scores[g.gameId]?.status === 'live');
	if (games.length > 1) return `${games.length} games${playing ? ` · ${playing.opponent} ${gameState(playing, scores)}` : ''}${auto ? ' · Auto' : ''}`;
	return row.bench ? 'No game in this view' : auto ? 'Auto · waiting for a game' : 'No selected game';
}

/** A row's stat line from the games that count; for a bench player, all of his. */
export function rowStats(row: MatchupRow) {
	const games = row.games.filter((g) => row.bench || g.counted);
	return { line: sumStats(games.map((g) => g.stats)), played: games.some((g) => Object.keys(g.stats ?? {}).length > 0) };
}
