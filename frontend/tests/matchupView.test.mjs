import test from 'node:test';
import assert from 'node:assert/strict';
import { matchupRows } from '../src/lib/matchupView.ts';

const period = {starts_on:'2026-10-19',ends_on:'2026-11-01',double_games:false};
const game = (day, points=12, extra={}) => ({day,starts_at:`${day}T23:00:00Z`,status:'final',opponent:'BOS',at_home:true,points,stats:{pts:points},counts:true,...extra});
const official = (day,points=12,extra={}) => ({...game(day,points),game_id:`game-${day}`,slot:'PG',home:'NYK',away:'BOS',positions:['PG'],...extra});
const player = (extra={}) => ({player_id:'curry',full_name:'Curry',headshot_url:'',positions:['PG'],slot:'PG',games:[],...extra});
const scored = (lines) => ({player_id:'curry',full_name:'Curry',headshot_url:'',points:lines.reduce((n,g)=>n+g.points,0),game_lines:lines});
const lineup = (players,extra={}) => ({day:'2026-10-19',last_day:'2026-10-25',players,...extra});

test('daily stats use only the selected day and retain former starters', () => {
 const rows = matchupRows([scored([official('2026-10-19',8),official('2026-10-20',12)])],lineup([]),period,false,'2026-10-19',false);
 assert.equal(rows.length,1);
 assert.equal(rows[0].points,8);
 assert.equal(rows[0].earlier,true);
 assert.equal(rows[0].games[0].day,'2026-10-19');
});
test('basketball shows the picked game and rejects points from other games', () => {
 const p = player({counts_from:'2026-10-20',games:[game('2026-10-19',30,{counts:false}),game('2026-10-20',12),game('2026-10-23',0,{status:'scheduled',stats:{},counts:false})]});
 const row=matchupRows([scored([official('2026-10-20')])],lineup([p]),period,false,'2026-10-21',true)[0];
 assert.equal(row.points,12);
 assert.deepEqual(row.games.map(g=>g.label),['Not selected','Counted','Not selected']);
 assert.equal(row.games[0].points,0);
 assert.match(row.selection,/2026-10-20/);
});
test('NFL weekly view includes all counted games in the lineup week', () => {
 const row=matchupRows([scored([official('2026-10-22',10),official('2026-10-25',15),official('2026-10-29',20)])],lineup([]),period,false,'2026-10-25',true)[0];
 assert.equal(row.points,25);
 assert.equal(row.games.length,2);
});
test('full matchup totals include prior weeks without duplicating lineup games', () => {
 const row=matchupRows([scored([official('2026-10-20',12),official('2026-10-27',15)])],lineup([player({games:[game('2026-10-27',15)]})],{day:'2026-10-26',last_day:'2026-11-01'}),period,true,'2026-10-27',true)[0];
 assert.equal(row.points,27);
 assert.equal(row.games.length,2);
});
test('pitcher starts past the cap are visible as uncounted and add no points', () => {
 const p=player({positions:['SP'],games:[game('2026-10-20',12)]});
 const row=matchupRows([],lineup([p],{starts:{limit:4,made:[{player_id:'curry',day:'2026-10-20',counts:false}]}}),period,false,'2026-10-20',false)[0];
 assert.equal(row.points,0);
 assert.equal(row.games[0].label,'Start cap reached');
 assert.equal(row.games[0].points,0);
});
test('Auto candidates retain their status until they earn a counted contribution', () => {
 const p=player({counts_last:true,games:[game('2026-10-20',0,{status:'scheduled',stats:{},counts:false})]});
 const row=matchupRows([],lineup([p]),period,false,'2026-10-20',true)[0];
 assert.equal(row.games[0].label,'Auto candidate');
 assert.equal(row.points,0);
});

test('doubleheaders match distinct game IDs even when start timestamps coincide', () => {
 const first={...official('2026-10-20',10),game_id:'first'};
 const second={...official('2026-10-20',20),game_id:'second'};
 const p=player({games:[{...game('2026-10-20',10),game_id:'first'},{...game('2026-10-20',20),game_id:'second'}]});
 const row=matchupRows([scored([first,second])],lineup([p]),period,false,'2026-10-20',false)[0];
 assert.equal(row.points,30);
 assert.deepEqual(row.games.map(g=>g.points),[10,20]);
 assert.deepEqual(row.games.map(g=>g.gameId),['first','second']);
});
