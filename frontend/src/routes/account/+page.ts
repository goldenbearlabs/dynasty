// Keeps managers in their league while leaving a landing page for accounts without one.
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent }) => {
	const { me } = await parent();
	if (me?.id) redirect(307, '/league');
};
