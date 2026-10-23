import { getMatchups } from '#lib/api.ts';
import { sportsToday } from '#lib/ui/time.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent }) => {
	const { dynasty } = await parent();
	const leagues = dynasty?.leagues.filter(l => l.settings.format.type === 'head_to_head') ?? [];
	const periods = await Promise.all(leagues.map(async league => {
		try { return { competition: league.competition, period: (await getMatchups(league.id)).period }; }
		catch { return { competition: league.competition, period: null }; }
	}));
	const today = sportsToday();
	const active = periods.find(p => p.period && p.period.starts_on <= today && p.period.ends_on >= today);
	return { defaultCompetition: active?.competition ?? periods.find(p => p.period)?.competition ?? leagues[0]?.competition ?? '' };
};
