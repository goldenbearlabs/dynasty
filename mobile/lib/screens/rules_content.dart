// Mirrors the website's general rule-book topics for offline-readable mobile sections.

typedef RuleItem = ({String lead, String detail});
typedef RuleTopic = ({String title, List<RuleItem> items});

const generalRules = <RuleTopic>[
  (
    title: 'The basics',
    items: [
      (
        lead: 'One franchise, a team in every sport.',
        detail: 'Each manager runs one franchise with a separate roster in every league. Each league has its own standings and champion.',
      ),
      (
        lead: 'Every league has its own rules.',
        detail: 'Roster sizes, lineups, scoring, waivers, and other settings are set per league by the commissioner. The sport sections below show the current values.',
      ),
      (
        lead: 'Players are kept for good.',
        detail: 'This is a dynasty: a player stays on your roster from season to season until you drop or trade him.',
      ),
      (
        lead: 'Rule changes recalculate points.',
        detail: 'Season totals are worked out from the current rules, so a scoring change updates them at once.',
      ),
      (
        lead: 'The day rolls over at 5 a.m. Eastern.',
        detail: 'A late West Coast game counts for the day it started on.',
      ),
    ],
  ),
  (
    title: 'Rosters',
    items: [
      (
        lead: 'Two lists.',
        detail: 'The main roster holds starters and bench players. The reserve list holds players for later. Only main-roster players can start.',
      ),
      (
        lead: 'One owner per league.',
        detail: 'Nobody else can add, claim, or draft a player you hold on either list or as an unsigned pick.',
      ),
      (
        lead: 'Reserve eligibility varies.',
        detail: 'A league may allow anyone, prospects only, or prospects and players outside starting conferences. Newly signed rookie picks may use reserve until moved up.',
      ),
      (
        lead: 'Reserve locks may apply.',
        detail: 'A non-prospect sent to reserve may have to stay there for the league’s lock period. Commissioner placements and graduations are exempt.',
      ),
      (
        lead: 'Fix an over-limit roster first.',
        detail: 'If a roster exceeds a limit, only moves that bring it closer to legal are allowed.',
      ),
      (
        lead: 'Leaving main benches a player.',
        detail: 'Dropping, trading, or reserving a player removes him from future lineups; points already scored stand.',
      ),
    ],
  ),
  (
    title: 'Lineups',
    items: [
      (
        lead: 'A lineup stays in force.',
        detail: 'The lineup carries on day after day or week after week until changed.',
      ),
      (
        lead: 'Daily or weekly by league.',
        detail: 'A daily league sets a lineup each day; a weekly league sets one for the week.',
      ),
      (
        lead: 'Locks depend on league rules.',
        detail: 'A starter locks at his game start or period start. Commissioners can override a lock.',
      ),
      (
        lead: 'Slots require eligible positions.',
        detail:
            'Flex and utility slots can accept several positions or anyone.',
      ),
      (
        lead: 'Some slots limit games.',
        detail: 'Where set, only the chosen number of games each week score. A past game cannot be selected retroactively.',
      ),
      (
        lead: 'Baseball may cap pitcher starts.',
        detail: 'Starts beyond the weekly cap score nothing. A reliever opening a game uses a start only if he pitches more than an inning.',
      ),
      (
        lead: 'College leagues may limit conferences.',
        detail: 'Players outside starting conferences can be held, but cannot start.',
      ),
    ],
  ),
  (
    title: 'Scoring',
    items: [
      (
        lead: 'Only starters score.',
        detail: 'A player scores for your franchise only while in its starting lineup.',
      ),
      (
        lead: 'Stats use league values.',
        detail: 'Game points are the player’s stats multiplied by this sport league’s scoring values.',
      ),
      (
        lead: 'Live updates and corrections.',
        detail: 'Games in progress refresh regularly; official stat corrections are picked up afterward.',
      ),
      (
        lead: 'Bonuses count per game.',
        detail: 'A 50-point game also qualifies for a 40-point-game bonus. Double-doubles and triple-doubles count qualifying stat categories.',
      ),
      (
        lead: 'Sport details matter.',
        detail: 'A hockey shutout is credited after the game; a baseball inning pitched is three outs.',
      ),
    ],
  ),
  (
    title: 'Seasons and matchups',
    items: [
      (
        lead: 'The commissioner starts a season.',
        detail: 'Only games inside its dates count; each league has one season at a time.',
      ),
      (
        lead: 'Two formats.',
        detail: 'Head-to-head leagues rank by matchup record; total-points leagues rank by season points.',
      ),
      (
        lead: 'Schedules build themselves.',
        detail: 'A head-to-head season gets regular and playoff matchups. Begun or commissioner-set matchups are preserved when a schedule is rebuilt.',
      ),
      (
        lead: 'Weeks line up.',
        detail: 'Weekly lineup and matchup periods use the same days; short opening or closing stretches join a neighboring week.',
      ),
      (
        lead: 'Playoffs re-seed.',
        detail: 'The best remaining seed plays the worst. Higher seeds may receive byes, and the better seed wins a tied playoff matchup.',
      ),
      (
        lead: 'Standings account for byes.',
        detail: 'Head-to-head leagues rank by the share of matchups won, counting a tie as half a win, then by points.',
      ),
      (
        lead: 'Commissioners can set matchups by hand.',
        detail: 'An unfinished matchup can be corrected, given a bye, or returned to the automatic schedule.',
      ),
      (
        lead: 'Champions are named at close.',
        detail: 'The playoff winner, or total-points leader, is champion when the commissioner closes the season.',
      ),
    ],
  ),
  (
    title: 'Adding and dropping',
    items: [
      (
        lead: 'Free agents can be added.',
        detail: 'An unowned player not on waivers may join main or reserve if the league permits it and there is room.',
      ),
      (
        lead: 'Some new entrants wait for a draft.',
        detail: 'A league may hold players who joined after its last draft until the next draft.',
      ),
      (
        lead: 'Free agency pauses during a live draft.',
        detail:
            'No regular add can be made while that league’s draft is running.',
      ),
      (
        lead: 'A weekly limit may apply.',
        detail: 'Adds and won waiver claims count; drafts, trades, and commissioner placements do not.',
      ),
      (
        lead: 'You can always drop.',
        detail: 'Dropping a player is never refused.',
      ),
    ],
  ),
  (
    title: 'Waivers',
    items: [
      (
        lead: 'Released players go on waivers.',
        detail: 'Players dropped from a list or released as unsigned picks cannot be added during the waiver period.',
      ),
      (
        lead: 'Claims are private.',
        detail: 'A claim names the destination list and can name a player to drop. It can be changed or withdrawn before settlement.',
      ),
      (
        lead: 'Order or blind bids decide.',
        detail: 'The highest-priority valid claim wins, or the highest blind bid where the league uses a budget.',
      ),
      (
        lead: 'Invalid claims are passed over.',
        detail: 'No room, a weekly limit, or an unaffordable bid lets the next claim try.',
      ),
      (
        lead: 'Winners move back.',
        detail: 'A winning franchise goes to the end of the order. New seasons reset order from previous standings, worst first.',
      ),
      (
        lead: 'Before the first season, order is alphabetical.',
        detail: 'The commissioner may rearrange the waiver order at any time.',
      ),
      (
        lead: 'Unclaimed players become free agents.',
        detail: 'Once waivers expire, they can be added normally.',
      ),
    ],
  ),
  (
    title: 'Drafts',
    items: [
      (
        lead: 'One startup draft per league.',
        detail: 'It fills main and reserve lists. Prospects can be drafted; managers can adjust reserve afterward.',
      ),
      (
        lead: 'Rookie drafts follow each year.',
        detail: 'They include new players and free agents. Rounds follow the league setting or half its reserve size, rounded up.',
      ),
      (
        lead: 'Future picks can be traded.',
        detail: 'Upcoming rookie-draft picks exist before the draft begins.',
      ),
      (
        lead: 'Queues protect your clock.',
        detail: 'When time expires, the first available queued player is taken; an empty queue may skip the pick.',
      ),
      (
        lead: 'Auto pick is optional.',
        detail: 'When enabled, your draft-room queue is used automatically when your turn arrives.',
      ),
      (
        lead: 'Rookie picks can be passed.',
        detail: 'Passing forfeits that pick; an expired clock with no queued player may instead leave a skipped pick you can make up while the draft runs.',
      ),
      (
        lead: 'Private rankings feed queues.',
        detail: 'Managers can maintain ranked lists and import one into a draft queue; players already taken are omitted.',
      ),
      (
        lead: 'Season injury swaps are limited.',
        detail: 'A main-roster player marked IR, IL, or Out can swap with a reserve player once per injured player per season; the injured player stays on reserve for that season.',
      ),
      (
        lead: 'Rookie picks must be signed.',
        detail: 'A drafted rookie is held as rights until signed to a list during the league’s signing window.',
      ),
      (
        lead: 'Unsigned picks are released.',
        detail: 'A released or expired unsigned pick goes on waivers.',
      ),
      (
        lead: 'Commissioners run the room.',
        detail: 'They set order, pause, undo the last pick, and can finish a draft early.',
      ),
    ],
  ),
  (
    title: 'Trades',
    items: [
      (
        lead: 'Trade across sports.',
        detail: 'One deal can include players and future picks from different leagues.',
      ),
      (
        lead: 'Future picks lock at draft start.',
        detail: 'A pick can trade until its draft begins; unsigned rookie rights can trade too.',
      ),
      (
        lead: 'Both sides accept.',
        detail: 'A trade completes after both franchises accept unless commissioner approval is required.',
      ),
      (
        lead: 'Rosters must remain legal.',
        detail:
            'A player arrives on the list he left, with reserve locks intact.',
      ),
      (
        lead: 'Deadlines apply by league.',
        detail: 'Nothing involving a league can trade after its deadline until that deadline is moved.',
      ),
      (
        lead: 'Commissioners can reverse a trade.',
        detail: 'A reversed trade sends its assets back.',
      ),
    ],
  ),
  (
    title: 'Players and moving up',
    items: [
      (
        lead: 'One career record per player.',
        detail: 'College seasons stay with a player when he moves into a professional league.',
      ),
      (
        lead: 'Graduation can preserve ownership.',
        detail: 'Where a college league links to a pro league, your player moves to your pro roster under that league’s landing rule.',
      ),
      (
        lead: 'Inactive players need a decision.',
        detail: 'The commissioner decides what happens to college players who leave without turning pro.',
      ),
      (
        lead: 'Football uses skill positions.',
        detail: 'Quarterbacks, backs, receivers, and tight ends are in the pool; linemen, defenders, and kickers are not.',
      ),
      (
        lead: 'Prospects can be held early.',
        detail: 'Recruiting classes, draft classes, minor leaguers, and prospect lists enter the pool.',
      ),
      (
        lead: 'Real-world data refreshes.',
        detail: 'Scores update live, rosters refresh nightly, and prospect lists refresh weekly.',
      ),
    ],
  ),
  (
    title: 'The commissioner',
    items: [
      (
        lead: 'Sets the rules.',
        detail: 'Commissioners control league rules, seasons, and drafts.',
      ),
      (
        lead: 'Can override roster and lineup rules.',
        detail: 'They can make corrections past limits or locks; affected franchises must still resolve overages.',
      ),
      (
        lead: 'Settles exceptional cases.',
        detail: 'They approve or reverse trades, change waiver order, name a tied champion, and handle players who leave a league.',
      ),
      (
        lead: 'Manages invitations.',
        detail: 'They add franchises and issue one-use links for account creation or password resets.',
      ),
    ],
  ),
];
