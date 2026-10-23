// Sends old My Leagues bookmarks to the active league or account guidance.
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent }) => {
	const { me } = await parent();
	redirect(307, me?.id ? '/league' : '/account');
};
