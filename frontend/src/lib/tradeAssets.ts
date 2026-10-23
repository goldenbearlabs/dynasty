// Turning a franchise's holdings into the list of things it could trade.

import type { Franchise, FranchiseDetail, List, OfferItem } from '#lib/api.ts';
import { draftTag } from '#lib/draftSlot.ts';

/** Where an asset sits on its team's list in the trade builder. */
export type Section = 'main' | 'reserve' | 'rights' | 'picks';

export type Asset = {
	/** Identifies the asset within a trade being built. */
	key: string;
	sport: string;
	/** Every sport it belongs to: a player's one, or each league a pick's draft covers. */
	sports: string[];
	section: Section;
	/** One short line for a thin row: the player's name or the pick, then what it is. */
	name: string;
	detail: string;
	tradeable: boolean;
	/** Set for a player. */
	leagueId?: string;
	playerId?: string;
	list?: List;
	/** Set for a pick. */
	pickId?: string;
	/** Props for <TradeAsset>. */
	display: {
		player?: string;
		positions?: string[];
		headshot?: string;
		sport: string;
		draft?: string;
		round?: number;
		via?: string;
		note?: string;
	};
};

export function assetsOf(detail: FranchiseDetail, franchises: Franchise[]): Asset[] {
	const name = (id: string) => franchises.find((f) => f.id === id)?.name ?? '';

	const players = detail.rosters.flatMap((roster) =>
		roster.players.map(
			(p): Asset => ({
				key: `player:${roster.league_id}:${p.player_id}`,
				sport: roster.competition,
				sports: [roster.competition],
				section: p.list,
				name: p.full_name,
				detail: [p.positions.join('/'), p.team_abbrev, p.status === 'prospect' ? draftTag(p) : ''].filter(Boolean).join(' · '),
				tradeable: true,
				leagueId: roster.league_id,
				playerId: p.player_id,
				list: p.list,
				display: {
					player: p.full_name,
					positions: p.positions,
					headshot: p.headshot_url,
					sport: roster.competition,
					note: p.list === 'reserve' ? '· reserve' : p.list === 'rights' ? '· unsigned pick' : ''
				}
			})
		)
	);
	const picks = detail.picks.map(
		(p): Asset => ({
			key: `pick:${p.id}`,
			sport: p.competitions[0] ?? '',
			sports: p.competitions,
			section: 'picks',
			name: `${p.year} round ${p.round}`,
			detail: [p.competitions.length > 1 ? p.draft_name : '', p.original_franchise_id !== detail.franchise.id ? `via ${name(p.original_franchise_id)}` : '', p.draft_status === 'scheduled' ? '' : 'draft under way'].filter(Boolean).join(' · '),
			// Once a draft starts its picks stay where they are.
			tradeable: p.draft_status === 'scheduled',
			pickId: p.id,
			display: {
				sport: p.competitions.length === 1 ? p.competitions[0] : '',
				draft: p.draft_name,
				round: p.round,
				via: p.original_franchise_id !== detail.franchise.id ? name(p.original_franchise_id) : '',
				note: p.draft_status === 'scheduled' ? '' : '· draft under way'
			}
		})
	);
	return [...players, ...picks];
}

/** The offer item that sends an asset from one franchise to another. */
export const toItem = (asset: Asset, from: string, to: string): OfferItem => ({
	from_franchise_id: from,
	to_franchise_id: to,
	league_id: asset.leagueId,
	player_id: asset.playerId,
	draft_pick_id: asset.pickId
});
