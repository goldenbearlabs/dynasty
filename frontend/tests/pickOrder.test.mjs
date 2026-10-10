import test from 'node:test';
import assert from 'node:assert/strict';
import { fillPickOrder, inferPickOrder, roundTeams } from '../src/lib/pickOrder.ts';

const slots = Array.from({length:4},(_,i) => ['a','b','c'].map(team => ({id:`${i+1}-${team}`,round:i+1,original_franchise_id:team,current_franchise_id:team === 'b' && i === 1 ? 'c' : team}))).flat();
const teams = round => round.map(p => p.original_franchise_id);

test('snake alternates all rounds from the chosen first-round order', () => {
 const result = fillPickOrder(slots,['c','a','b'],'snake');
 assert.deepEqual([1,2,3,4].map(r => teams(result.filter(p => p.round === r))), [['c','a','b'],['b','a','c'],['c','a','b'],['b','a','c']]);
 assert.equal(inferPickOrder(result),'snake');
});
test('linear uses the same order in every round', () => {
 const result = fillPickOrder(slots,['b','c','a'],'linear');
 assert.deepEqual([1,2,3,4].map(r => teams(result.filter(p => p.round === r))), Array(4).fill(['b','c','a']));
 assert.equal(inferPickOrder(result),'linear');
});
test('traded picks keep their identities and owners; inputs are unchanged', () => {
 const original = structuredClone(slots);
 const result = fillPickOrder(slots,['c','a','b'],'snake');
 assert.deepEqual(result.find(p => p.id === '2-b'),original.find(p => p.id === '2-b'));
 assert.deepEqual(slots,original);
 assert.deepEqual(new Set(result.map(p => p.id)),new Set(slots.map(p => p.id)));
});
test('custom incomplete rounds and repeated teams are refused', () => {
 assert.equal(roundTeams(slots.slice(1)),null);
 assert.throws(() => fillPickOrder(slots.slice(1),['a','b','c'],'snake'));
 assert.throws(() => fillPickOrder(slots,['a','a','c'],'snake'));
});
