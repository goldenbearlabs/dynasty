import { redirect } from '@sveltejs/kit';
import { getBootstrap, getCompetitions, getDrafts, getDynasty, getFranchise, getSession, getTeams, getTrades } from '#lib/api.ts';
import type { LayoutLoad } from './$types';

// Rendered entirely in the browser; the Go server only serves static files and JSON.
export const ssr = false;
export const prerender = false;

// Everything every page needs: the sports, the league structure, the drafts
// and trades, the real teams' logos, and who is signed in. Pages refresh it with invalidateAll()
// after changing something.
export const load: LayoutLoad = async ({ url }) => {
	const [competitions, bootstrap, me] = await Promise.all([getCompetitions(), getBootstrap(), getSession()]);
	const path = url.pathname;
	const entry = path === '/' || path === '/register' || path === '/verify' || path === '/reset-password' || path.startsWith('/join/') || (!bootstrap.configured && path === '/setup');
	if (!entry && !me?.account_id) redirect(307, `/?next=${encodeURIComponent(path + url.search)}`);
	if (me?.account_id && !me.id && !entry && path !== '/leagues' && !path.startsWith('/leagues/')) redirect(307, '/leagues');
	const [dynasty, drafts, trades, teams] = me?.id
		? await Promise.all([getDynasty(), getDrafts(), getTrades(), getTeams().catch(() => [])])
		: [null, [], [], []];
	const myTeam = me?.id ? await getFranchise(me.slug).catch(() => null) : null;
	return { competitions, configured: bootstrap.configured, dynasty, me, drafts, trades, myTeam, teams };
};
