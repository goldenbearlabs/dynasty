// Loads only the signed-in account's league memberships for the My Leagues page.

import { getMyLeagues } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => ({ myLeagues: await getMyLeagues() });
