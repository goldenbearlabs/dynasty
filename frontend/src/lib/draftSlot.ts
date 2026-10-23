import type { DraftSlot } from '#lib/api.ts';

/** A drafted player's slot as a short tag, "R1 P16 '26": round 1, 16th pick in it, the 2026 draft. Empty when undrafted. */
export const draftTag = (p: DraftSlot) =>
	p.draft_year && p.draft_round && p.draft_pick ? `R${p.draft_round} P${p.draft_pick} '${String(p.draft_year).slice(-2)}` : '';

/** The tag's longer reading, for a tooltip. */
export const draftTitle = (p: DraftSlot) =>
	p.draft_year && p.draft_round ? `Drafted ${p.draft_year}, round ${p.draft_round}, pick ${p.draft_pick} (${p.draft_overall} overall)` : '';
