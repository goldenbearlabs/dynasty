import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The front door. A signed-in manager goes straight to their team.
export const load: PageLoad = async ({ parent }) => {
	const { me } = await parent();
	if (me) redirect(307, `/franchise/${me.slug}`);
};
