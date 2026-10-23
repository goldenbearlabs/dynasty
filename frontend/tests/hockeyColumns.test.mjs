import test from 'node:test';
import assert from 'node:assert/strict';
import {hockeyScoringColumns} from '../src/lib/statLine.ts';
const scoring={goals:2,assists:1,shots:.5,hits:.1,blocks:.25,pp_points:1,saves:.25,goals_against:-1,goalie_wins:4,shutouts:3};
test('NHL defaults show only scored categories, with compact headings and full titles',()=>{
 const skaters=hockeyScoringColumns(false,scoring), goalies=hockeyScoringColumns(true,scoring);
 assert.deepEqual(skaters.map(c=>c.label),['G','A','SOG','HIT','BLK','PPP']);
 assert.deepEqual(goalies.map(c=>c.label),['SV','GA','W','SO']);
 assert.equal(goalies.find(c=>c.key==='goals_against').title,'Goals against');
 assert(skaters.every(c=>c.label.length<=3));
 assert(goalies.every(c=>c.label.length<=3));
});
test('custom NHL scoring adds enabled categories and excludes zero or missing weights',()=>{
 const custom={...scoring,shots:0,pim:-.2,pp_goals:2,plus_minus:.5,saves:0,shutouts:0};
 assert.deepEqual(hockeyScoringColumns(false,custom).map(c=>c.label),['G','A','HIT','BLK','PPP','PIM','PPG','+/−']);
 assert.deepEqual(hockeyScoringColumns(true,custom).map(c=>c.label),['GA','W']);
 assert.deepEqual(hockeyScoringColumns(false,{}),[]);
});
