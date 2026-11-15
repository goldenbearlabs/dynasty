import test from 'node:test';
import assert from 'node:assert/strict';
import { basketballMatchupPerformance, bestBasketballGame, defaultRosterStatsView } from '../src/lib/basketballStats.ts';

const game = (points, stats, extra = {}) => ({points, stats, status:'final', counts:false, ...extra});
const player = (games, slot = 'G') => ({player_id:'test', games, slot});

test('basketball opens This Matchup while other sports keep Today', () => {
 for (const sport of ['nba','cbb','wnba']) assert.equal(defaultRosterStatsView(sport),'matchup');
 for (const sport of ['nhl','mlb','nfl']) assert.equal(defaultRosterStatsView(sport),'today');
});

test('My Team uses the locked game instead of accumulating other games', () => {
 const shown = basketballMatchupPerformance(player([
  game(45,{pts:50,reb:20}),game(18,{pts:20,reb:4},{counts:true,locked_in:true}),game(30,{pts:35,reb:10})
 ]));
 assert.equal(shown.games,1);
 assert.deepEqual(shown.stats,{pts:20,reb:4});
 assert.equal(shown.points,18);
 assert.equal(shown.points_per_game,18);
});

test('doubled matchups show both counted games and ignore unselected games', () => {
 const shown = basketballMatchupPerformance(player([
  game(18,{pts:20,reb:4},{counts:true,locked_in:true}),game(9,{pts:10,reb:2},{counts:true,locked_in:true}),game(99,{pts:100,reb:25})
 ]));
 assert.equal(shown.games,2);
 assert.deepEqual(shown.stats,{pts:30,reb:6});
 assert.equal(shown.points,27);
 assert.equal(shown.points_per_game,13.5);
});

test('a starter with no counted game has no accumulated matchup stats', () => {
 const shown = basketballMatchupPerformance(player([game(45,{pts:50})]));
 assert.equal(shown.games,0);
 assert.equal(shown.points,0);
 assert.deepEqual(shown.stats,{});
});

test('bench stats stay together from the best fantasy game, not each stat maximum', () => {
 const shown = basketballMatchupPerformance(player([game(18,{min:20,pts:30,reb:1}),game(25,{min:30,pts:15,reb:12})],''));
 assert.equal(shown.games,1);
 assert.equal(shown.points,25);
 assert.deepEqual(shown.stats,{min:30,pts:15,reb:12});
});

test('best game handles negative scores, zero-point appearances, and DNPs', () => {
 const negative = game(-3,{min:10,pts:0,tov:3});
 const dnp = game(0,{min:0,pts:0});
 assert.equal(bestBasketballGame([game(-5,{min:20,tov:5}),negative,dnp]),negative);
 const zero = game(0,{min:5,pts:0});
 assert.equal(bestBasketballGame([negative,zero]),zero);
 assert.equal(bestBasketballGame([dnp,game(99,{pts:99},{status:'scheduled'})]),undefined);
});
