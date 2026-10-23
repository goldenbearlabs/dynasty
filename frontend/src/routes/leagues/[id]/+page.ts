// Resolves a league through the current account's membership, not a public ID lookup.

import { getMyLeague } from '#lib/api.ts';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	const league = await getMyLeague(params.id);
	if (league.ready) redirect(307, '/league');
	return { league };
};
