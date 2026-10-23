// Dropping a player, the same from anywhere: one confirmation that says
// where he goes next, then the drop.
import { changeRoster, type League } from '#lib/api.ts';
import { toast } from '#lib/ui/toast.svelte.ts';

/** The question to ask before dropping, or releasing an unsigned pick. */
export function dropQuestion(league: League, name: string, unsigned = false): string {
	const waivers = league.settings.waivers;
	const next =
		waivers.mode === 'none'
			? 'He becomes a free agent straight away.'
			: `He goes on waivers for ${waivers.hours} hours, where any manager can claim him, before he becomes a free agent.`;
	return `${unsigned ? 'Release' : 'Drop'} ${name}? ${next}`;
}

/** Asks, then drops. Reports whether he was dropped. force: the commissioner, past the rules. */
export async function dropPlayer(league: League, franchiseId: string, playerId: string, name: string, force = false): Promise<boolean> {
	if (!confirm(dropQuestion(league, name))) return false;
	try {
		await changeRoster(league.id, 'drop', { player_id: playerId, franchise_id: franchiseId, force });
		toast.good(league.settings.waivers.mode === 'none' ? `Dropped ${name}.` : `Dropped ${name}. He is on waivers.`);
		return true;
	} catch (e) {
		toast.error(e);
		return false;
	}
}
