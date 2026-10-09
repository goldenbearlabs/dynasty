import { getFranchise } from '#lib/api.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => getFranchise(params.slug);
