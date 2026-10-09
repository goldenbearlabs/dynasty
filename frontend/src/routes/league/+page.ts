import { getActivity, getOverall, getStandings } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent }) => {
	const { dynasty, myTeam } = await parent();
	if (!dynasty) return { activity: [], mine: null, standings: [], overall: [] };

	const [activity, standings, overall] = await Promise.all([
		getActivity(),
		getStandings(),
		getOverall()
	]);
	return { activity, mine: myTeam, standings, overall };
};
