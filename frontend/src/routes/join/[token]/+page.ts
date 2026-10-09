import { ApiError, getInvite } from '#lib/api.ts';
import type { PageLoad } from './$types';

// An invite link: find out whose it is, or that it is no longer good.
export const load: PageLoad = async ({ params }) => {
	try {
		return { invite: await getInvite(params.token), problem: '' };
	} catch (e) {
		if (e instanceof ApiError) return { invite: null, problem: e.message };
		throw e;
	}
};
