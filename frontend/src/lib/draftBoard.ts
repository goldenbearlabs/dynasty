type PickSlot = { id: string; round: number; position: number; original_franchise_id: string };

// Original owners define stable columns even when picks have been traded.
// Read positions to discover the order; never change the real pick sequence.
// Repeated owners and incomplete rounds retain every pick and empty slot.
export function layoutDraftBoard<T extends PickSlot>(picks: readonly T[]) {
	const columns: { key: string; franchiseId: string }[] = [];
	const known = new Set<string>();
	const rounds = new Map<number, Map<string, T>>();
	const occurrences = new Map<number, Map<string, number>>();
	for (const pick of picks.toSorted((a, b) => a.position - b.position)) {
		if (!rounds.has(pick.round)) {
			rounds.set(pick.round, new Map());
			occurrences.set(pick.round, new Map());
		}
		const counts = occurrences.get(pick.round)!;
		const count = counts.get(pick.original_franchise_id) ?? 0;
		counts.set(pick.original_franchise_id, count + 1);
		const key = `${pick.original_franchise_id}:${count}`;
		if (!known.has(key)) {
			known.add(key);
			columns.push({ key, franchiseId: pick.original_franchise_id });
		}
		rounds.get(pick.round)!.set(key, pick);
	}
	return {
		columns,
		rounds: [...rounds].map(([round, slots]) => ({
			round,
			picks: columns.map((column) => slots.get(column.key) ?? null)
		}))
	};
}
