export type Column = { key: string; label: string; help: string };
export const basics: Column[] = [
 { key: 'games', label: 'Games', help: 'Imported appearances in this season. Research uses season stats, regardless of whether the player started in a fantasy lineup.' },
 { key: 'points', label: 'Fantasy points', help: 'Season stats multiplied by current fantasy league rules, or labelled sport defaults when the dynasty has no league for this sport.' },
 { key: 'points_per_game', label: 'FP / game', help: 'Total fantasy points divided by games played. Helps compare production rates within the same scoring system.' }
];
export const advanced: Column[] = [
 { key: 'league_index', label: 'League+', help: 'Production rate compared with the same league AND season. 100 is average; 115 is one standard deviation above average. Useful across different scoring systems. Requires a qualified sample with variation.' },
 { key: 'position_index', label: 'Position+', help: 'Production rate compared with eligible peers in this league-season and comparison position. Average 100; one standard deviation 15.' },
 { key: 'percentile', label: 'Percentile', help: 'FP/game rank among qualified players in the same league-season, from 0 to 100. Tied rates share their average rank.' },
 { key: 'points_above_replacement', label: 'Above replacement', help: '(FP/game − positional replacement FP/game) × games. Positive values measure season production above an estimated replacement player at the comparison position. Position filters restrict the eligible comparison positions.' },
 { key: 'par_per_game', label: 'PAR / game', help: 'FP/game above the replacement baseline. Replacement depth comes from manager count and starting lineup slots, or your manual replacement rank.' },
 { key: 'win_share_added', label: 'Win share (est.)', help: 'Custom model estimate of fantasy win share added over replacement. Assumes normal, independent scores and one game per starter. Not measured wins, game-level variance, or real-world Win Shares.' },
 { key: 'availability', label: 'Games coverage %', help: 'Games played divided by the highest qualified games count in the same league-season. Measures recorded appearances, not injuries.' },
 { key: 'production_share', label: 'Production share %', help: 'Share of all positive season fantasy points among qualified players in the same league-season. Negative totals contribute zero.' }
];
export const formatValue = (value: unknown, digits = 1) => typeof value === 'number' && Number.isFinite(value) ? value.toLocaleString(undefined, { maximumFractionDigits: digits }) : '—';
