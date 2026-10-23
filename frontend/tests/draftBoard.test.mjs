import assert from 'node:assert/strict';
import test from 'node:test';
import { layoutDraftBoard } from '../src/lib/draftBoard.ts';

const pick = (position, round, owner, current = owner) => ({
	id: `pick-${position}`, position, round, original_franchise_id: owner, current_franchise_id: current
});

test('snake rounds align team columns and keep the real pick positions unchanged', () => {
	const picks = [pick(1, 1, 'A'), pick(2, 1, 'B'), pick(3, 1, 'C'),
		pick(4, 2, 'C'), pick(5, 2, 'B'), pick(6, 2, 'A'),
		pick(7, 3, 'A'), pick(8, 3, 'B'), pick(9, 3, 'C')];
	const before = structuredClone(picks);
	const board = layoutDraftBoard(picks);
	assert.deepEqual(board.columns.map((c) => c.franchiseId), ['A', 'B', 'C']);
	assert.deepEqual(board.rounds.map((r) => r.picks.map((p) => p?.position)), [[1, 2, 3], [6, 5, 4], [7, 8, 9]]);
	assert.deepEqual(picks, before);
});

test('linear drafts and traded picks retain their columns and current owners', () => {
	const board = layoutDraftBoard([pick(1, 1, 'B', 'A'), pick(2, 1, 'A'), pick(3, 2, 'B'), pick(4, 2, 'A', 'B')]);
	assert.deepEqual(board.columns.map((c) => c.franchiseId), ['B', 'A']);
	assert.deepEqual(board.rounds[1].picks.map((p) => p?.position), [3, 4]);
	assert.equal(board.rounds[1].picks[1]?.current_franchise_id, 'B');
});

test('incomplete and supplemental rounds preserve every pick without shifting teams', () => {
	const picks = [pick(1, 1, 'A'), pick(2, 1, 'B'), pick(3, 2, 'B'), pick(4, 3, 'A'), pick(5, 3, 'A')];
	const board = layoutDraftBoard(picks.toReversed());
	assert.deepEqual(board.rounds.map((r) => r.picks.map((p) => p?.position ?? null)), [[1, 2, null], [null, 3, null], [4, null, 5]]);
	assert.equal(board.rounds.flatMap((r) => r.picks.filter(Boolean)).length, picks.length);
	assert.deepEqual(layoutDraftBoard([]), { columns: [], rounds: [] });
});
