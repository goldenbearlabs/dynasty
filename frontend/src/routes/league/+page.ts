import { getActivity, getOverall, getStandings } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, depends }) => {
	depends('app:league-overview');
	const { dynasty } = await parent();
	if (!dynasty) return { activity: [], standings: [], overall: [] };

	const [activity, standings, overall] = await Promise.all([
		getActivity(),
		getStandings(),
		getOverall()
	]);
	return { activity, standings, overall };
};
