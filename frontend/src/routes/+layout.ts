import { getCompetitions, getDrafts, getDynasty, getFranchise, getSession, getTeams, getTrades } from '#lib/api.ts';
import type { LayoutLoad } from './$types';

// Rendered entirely in the browser; the Go server only serves static files and JSON.
export const ssr = false;
export const prerender = false;

// Everything every page needs: the sports, the league structure, the drafts
// and trades, the real teams' logos, and who is signed in. Pages refresh it with invalidateAll()
// after changing something.
export const load: LayoutLoad = async () => {
	const [competitions, dynasty, me, drafts, trades, teams] = await Promise.all([
		getCompetitions(),
		getDynasty(),
		getSession(),
		getDrafts(),
		getTrades(),
		getTeams().catch(() => []) // logos are a nicety: abbreviations stand in without them
	]);
	const myTeam = me ? await getFranchise(me.slug).catch(() => null) : null;
	return { competitions, dynasty, me, drafts, trades, myTeam, teams };
};
