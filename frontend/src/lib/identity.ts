import type { Franchise, TeamIdentity } from './api.ts';
export function teamIdentity(franchise: Franchise | undefined, teams: TeamIdentity[] = [], leagueID?: string) {
 const team = teams.find(t => t.franchise_id === franchise?.id && t.league_id === leagueID);
 return { name: team?.name || franchise?.name || '', image_url: team?.image_url || franchise?.image_url || '' };
}
