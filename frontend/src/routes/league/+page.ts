import { getActivity, getOdds, getOverall, getStandings } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent }) => {
	const { dynasty } = await parent();
	if (!dynasty) return { activity: [], standings: [], overall: [], odds: undefined };

	const [activity, standings, overall, odds] = await Promise.all([
		getActivity(),
		getStandings(),
		getOverall(),
		getOdds().catch(() => undefined) // the page stands without them
	]);
	return { activity, standings, overall, odds };
};
