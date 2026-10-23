import { getDraft } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => ({ state: await getDraft(params.id) });
