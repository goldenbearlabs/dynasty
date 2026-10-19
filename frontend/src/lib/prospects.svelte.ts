// The ranked prospects, by player, loaded the first time a tag asks.
import { getProspectRankings, type ProspectRanking } from '#lib/api.ts';

let ranked = $state<Map<string, ProspectRanking>>(new Map());
let asked = false;

/** A player's prospect ranking, or undefined; loads the list on first use. */
export function prospectRank(playerId: string): ProspectRanking | undefined {
	if (!asked) {
		asked = true;
		getProspectRankings().then(
			(list) => (ranked = new Map(list.map((r) => [r.player_id, r]))),
			() => (asked = false) // try again next time
		);
	}
	return ranked.get(playerId);
}
