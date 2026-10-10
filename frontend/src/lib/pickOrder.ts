import type { PickSlot } from './api.ts';

/** Only complete rounds with the same teams can be filled from a first-round order. */
export function roundTeams(slots: PickSlot[]): string[] | null {
 const rounds = [...new Set(slots.map(p => p.round))].sort((a,b) => a-b);
 if (!rounds.length || rounds.some((r,i) => r !== i+1)) return null;
 const first = slots.filter(p => p.round === 1).map(p => p.original_franchise_id);
 if (!first.length || new Set(first).size !== first.length) return null;
 if (rounds.some(round => {
  const teams = slots.filter(p => p.round === round).map(p => p.original_franchise_id);
  return teams.length !== first.length || new Set(teams).size !== first.length || teams.some(id => !first.includes(id));
 })) return null;
 return first;
}

export function inferPickOrder(slots: PickSlot[]): 'snake' | 'linear' {
 const first = slots.filter(p => p.round === 1).map(p => p.original_franchise_id);
 const second = slots.filter(p => p.round === 2).map(p => p.original_franchise_id);
 return second.length && second.length === first.length && second.every((id,i) => id === first[i]) ? 'linear' : 'snake';
}

/** Reorder existing picks; each pick keeps its ID, original team and current owner. */
export function fillPickOrder(slots: PickSlot[], teams: string[], order: 'snake' | 'linear'): PickSlot[] {
 const existing = roundTeams(slots);
 if (!existing || teams.length !== existing.length || new Set(teams).size !== existing.length || teams.some(id => !existing.includes(id))) {
  throw new Error('Automatic ordering needs one pick per team in every round. Use individual pick edits for a custom draft.');
 }
 return [...new Set(slots.map(p => p.round))].sort((a,b) => a-b).flatMap(round => {
  const sequence = order === 'snake' && round % 2 === 0 ? [...teams].reverse() : teams;
  const picks = new Map(slots.filter(p => p.round === round).map(p => [p.original_franchise_id,p]));
  return sequence.map(id => ({...picks.get(id)!}));
 });
}
