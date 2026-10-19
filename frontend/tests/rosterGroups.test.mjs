import test from 'node:test';
import assert from 'node:assert/strict';
import {rosterGroup,rosterGroupOrder} from '../src/lib/rosterGroups.ts';
test('sport groups keep ordinary lineups together and separate distinct stat tables',()=>{
 for(const sport of ['nba','wnba','cbb','nfl']) assert.equal(rosterGroup(sport,['G']),'Lineup');
 assert.deepEqual(rosterGroupOrder('nhl'),['Skaters','Goalies']);
 assert.equal(rosterGroup('nhl',['G']),'Goalies');
 assert.equal(rosterGroup('nhl',['C','L']),'Skaters');
 assert.deepEqual(rosterGroupOrder('mlb'),['Hitters','Pitchers']);
 assert.equal(rosterGroup('mlb',['SP']),'Pitchers');
 assert.equal(rosterGroup('mlb',['RP']),'Pitchers');
 assert.equal(rosterGroup('mlb',['OF']),'Hitters');
 assert.equal(rosterGroup('mlb',['SP','DH'],'SP'),'Pitchers');
 assert.equal(rosterGroup('mlb',['SP','DH'],'UTIL'),'Hitters');
});
