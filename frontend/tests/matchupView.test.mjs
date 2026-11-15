import test from 'node:test';
import assert from 'node:assert/strict';
import { matchupRows, rowStats, visibleGames, gameSummary } from '../src/lib/matchupView.ts';

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

test('games still to be played carry their projection only when they will count', () => {
 const ahead=(day,projected,extra={})=>game(day,0,{status:'scheduled',stats:{},projected,...extra});
 const daily=matchupRows([],lineup([player({games:[ahead('2026-10-20',7.5)]})]),period,false,'2026-10-20',false)[0];
 assert.equal(daily.projected,7.5);
 const picked=matchupRows([],lineup([player({games:[ahead('2026-10-20',7,{counts:false}),ahead('2026-10-23',9)]})]),period,false,'2026-10-21',true)[0];
 assert.equal(picked.projected,9);
 const auto=matchupRows([],lineup([player({counts_last:true,games:[ahead('2026-10-20',7,{counts:false}),ahead('2026-10-23',9,{counts:false})]})]),period,false,'2026-10-21',true)[0];
 assert.equal(auto.projected,9);
 const played=matchupRows([scored([official('2026-10-20',8)])],lineup([player({games:[{...game('2026-10-20',8),projected:6}]})]),period,false,'2026-10-20',false)[0];
 assert.equal(played.projected,0);
});

test('full starter slots precede bench, which never adds projected team points', () => {
 const starter=player({slot:'PG',games:[game('2026-10-20',0,{status:'scheduled',stats:{},projected:8})]});
 const reserve=player({player_id:'bench',full_name:'Bench player',slot:'',counts_last:true,games:[game('2026-10-20',0,{status:'scheduled',stats:{},projected:99})]});
 const rows=matchupRows([],lineup([reserve,starter],{slots:[{name:'PG',positions:['PG'],count:2}]}),period,false,'2026-10-20',true);
 assert.deepEqual(rows.map(r=>r.id),['curry','empty-PG-1','bench']);
 assert.equal(rows[1].empty,true);
 assert.equal(rows[2].bench,true);
 assert.equal(rows[2].slot,'BN');
 assert.equal(rows[2].games[0].label,'Bench · Not counted');
 assert.equal(rows[2].games[0].counted,false);
 assert.equal(rows.reduce((sum,r)=>sum+r.projected,0),8);
 assert.equal(rows.reduce((sum,r)=>sum+r.points,0),0);
});
test('benched former starters retain official contributions without losing bench placement', () => {
 const p=player({slot:'',games:[game('2026-10-20',12)]});
 const row=matchupRows([scored([official('2026-10-20',12)])],lineup([p]),period,false,'2026-10-20',false)[0];
 assert.equal(row.bench,true);
 assert.equal(row.earlier,true);
 assert.equal(row.points,12);
 assert.equal(row.games.length,1);
 assert.equal(row.games[0].counted,true);
});

test('lock-in offers stay visible and a full lock adds no later projection', () => {
 const offer=game('2026-10-20',30,{counts:false,eligible:true,can_lock_in:true});
 const later=game('2026-10-23',0,{status:'scheduled',counts:false,projected:99});
 const open=matchupRows([],lineup([player({counts_last:true,locks_used:0,games:[offer,later]})],{lock_in:true,lock_in_limit:1}),period,false,'2026-10-21',true)[0];
 assert.equal(open.games[0].label,'Lock in available');
 assert.equal(open.projected,99);
 const locked=matchupRows([scored([official('2026-10-20',30)])],lineup([player({counts_last:true,locks_used:1,games:[{...offer,can_lock_in:false,locked_in:true,counts:true},later]})],{lock_in:true,lock_in_limit:1}),period,false,'2026-10-21',true)[0];
 assert.equal(locked.games[0].label,'Locked in');
 assert.equal(locked.projected,0);
 assert.equal(locked.selection,'1/1 locked in');
});

test('doubled lock-in projections fill only the remaining choices', () => {
 const ahead=(day,projected)=>game(day,0,{status:'scheduled',counts:false,projected});
 const p=player({counts_last:true,locks_used:1,games:[ahead('2026-10-20',40),ahead('2026-10-23',10)]});
 const row=matchupRows([],lineup([p],{lock_in:true,lock_in_limit:2}),period,false,'2026-10-20',true)[0];
 assert.equal(row.projected,10);
 assert.equal(row.selection,'1/2 locked in');
 const empty=matchupRows([],lineup([{...p,locks_used:0}],{lock_in:true,lock_in_limit:2}),period,false,'2026-10-20',true)[0];
 assert.equal(empty.projected,50);
});


test('basketball bench rows show the best game in the viewed week, not accumulated stats', () => {
 for (const sport of ['nba','cbb','wnba']) {
  const p=player({slot:'',games:[
   game('2026-10-19',18,{stats:{min:20,pts:30,reb:1}}),
   game('2026-10-23',25,{stats:{min:30,pts:15,reb:12}}),
   game('2026-10-27',99,{stats:{min:35,pts:60,reb:30}})
  ]});
  const row=matchupRows([],lineup([p]),period,false,'2026-10-23',true,sport)[0];
  assert.equal(row.points,25);
  assert.deepEqual(rowStats(row).line,{min:30,pts:15,reb:12});
  assert.equal(row.projected,0);
  assert.equal(visibleGames(row).length,1);
  assert.equal(visibleGames(row)[0].day,'2026-10-23');
  assert.equal(visibleGames(row,true).length,2);
  assert.match(gameSummary(row,{}),/Best game/);
 }
});

test('non-basketball bench rows retain accumulated production', () => {
 const p=player({slot:'',games:[game('2026-10-19',18,{stats:{goals:1}}),game('2026-10-23',25,{stats:{goals:2}})]});
 const row=matchupRows([],lineup([p]),period,false,'2026-10-23',true,'nhl')[0];
 assert.equal(row.points,43);
 assert.deepEqual(rowStats(row).line,{goals:3});
 assert.equal(visibleGames(row).length,2);
});
