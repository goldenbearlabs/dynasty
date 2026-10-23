import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The front door sends signed-in accounts into the app, whether or not they have a league.
export const load: PageLoad = async ({ parent, url }) => {
	const { me } = await parent();
	if (me?.id) {
		const next = url.searchParams.get('next');
		redirect(307, next?.startsWith('/') && !next.startsWith('//') ? next : `/franchise/${me.slug}`);
	}
	if (me?.account_id) redirect(307, '/leagues');
};
