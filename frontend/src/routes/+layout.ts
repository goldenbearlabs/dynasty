import { getCompetitions, getDrafts, getDynasty, getFranchise, getSession, getTrades } from '#lib/api.ts';
import type { LayoutLoad } from './$types';

// Rendered entirely in the browser; the Go server only serves static files and JSON.
export const ssr = false;
export const prerender = false;

// Everything every page needs: the sports, the league structure, the drafts
// and trades, and who is signed in. Pages refresh it with invalidateAll()
// after changing something.
export const load: LayoutLoad = async () => {
	const [competitions, dynasty, me, drafts, trades] = await Promise.all([
		getCompetitions(),
		getDynasty(),
		getSession(),
		getDrafts(),
		getTrades()
	]);
	const myTeam = me ? await getFranchise(me.slug).catch(() => null) : null;
	return { competitions, dynasty, me, drafts, trades, myTeam };
};
