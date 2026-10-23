import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The front door sends managers into their league and other accounts to invite guidance.
export const load: PageLoad = async ({ parent, url }) => {
	const { me } = await parent();
	if (me?.id) {
		const next = url.searchParams.get('next');
		redirect(307, next?.startsWith('/') && !next.startsWith('//') ? next : `/franchise/${me.slug}`);
	}
	if (me?.account_id) redirect(307, '/account');
};
