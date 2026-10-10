<script lang="ts">
	// The rule book. Rules that hold everywhere are sorted into topics, one
	// on screen at a time. Rules that differ by league are on that league's
	// tab, read from its own settings so they are right whatever the
	// commissioner has changed.
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { rookieRounds, type Competition, type LeagueSettings } from '#lib/api.ts';
	import SportBadge from '#lib/ui/SportBadge.svelte';
	import Tabs from '#lib/ui/Tabs.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	type Guide = { key: string; name: string; rules: LeagueSettings; sport: Competition };
	// The dynasty's leagues with their rules, or every sport's recommended rules before there is one.
	const guides = $derived.by<Guide[]>(() => {
		const sportOf = (key: string) => data.competitions.find((c) => c.key === key)!;
		if (data.dynasty) return data.dynasty.leagues.map((l) => ({ key: l.competition, name: l.name, rules: l.settings, sport: sportOf(l.competition) }));
		return data.competitions.map((c) => ({ key: c.key, name: c.name, rules: c.defaults, sport: c }));
	});

	// ---- which tab, and which topic within the site-wide rules ----
	let tab = $state(untrack(() => page.url.searchParams.get('tab') ?? ''));
	let topicId = $state(untrack(() => page.url.searchParams.get('topic') ?? 'basics'));
	const tabs = $derived([
		{ value: '', label: 'Everywhere' },
		...guides.map((g) => ({ value: g.key, label: g.name, sport: g.key })),
		{ value: 'balance', label: 'How it was balanced' }
	]);
	const guide = $derived(guides.find((g) => g.key === tab));
	function go(change: { tab?: string; topic?: string }) {
		if (change.tab !== undefined) tab = change.tab;
		if (change.topic !== undefined) topicId = change.topic;
		const query = new URLSearchParams();
		if (tab) query.set('tab', tab);
		if (!tab && topicId !== 'basics') query.set('topic', topicId);
		goto(`?${query}`, { replaceState: true });
	}

	// ---- helpers for a league's own rules ----
	const starters = (rules: LeagueSettings) => rules.lineup.slots.reduce((n, s) => n + s.count, 0);
	const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
	const signed = (n: number) => (n > 0 ? `+${n}` : `${n}`);
	const positions = (list: string[]) => (list.includes('*') ? 'anyone' : list.join(', '));
	const scoring = (g: Guide) => g.sport.stats.filter((s) => g.rules.scoring[s.key]).map((s) => ({ label: s.label, points: g.rules.scoring[s.key] }));
	const signature = (points: Record<string, number>) =>
		JSON.stringify(Object.entries(points).filter(([, v]) => v !== 0).sort(([a], [b]) => a.localeCompare(b)));
	const sameScoring = (g: Guide) => guides.filter((o) => o.key !== g.key && signature(o.rules.scoring) === signature(g.rules.scoring));
	const listed = (items: string[]) => items.join(items.length === 2 ? ' and ' : ', ').replace(/, ([^,]*)$/, ' and $1');
	const recommended = (g: Guide) => signature(g.rules.scoring) === signature(g.sport.defaults.scoring);
	const limited = (rules: LeagueSettings) => rules.lineup.slots.filter((s) => s.games_per_week > 0);
	const capital = (word: string) => word[0].toUpperCase() + word.slice(1);
	const conferences = (g: Guide) =>
		(g.rules.lineup.conferences ?? []).map((id) => g.sport.conferences?.find((c) => c.id === id)?.name ?? id);
	const reserveFor = {
		anyone: 'anyone',
		prospects_only: 'prospects only',
		prospects_or_ineligible: 'prospects, and players outside the starting conferences'
	};
	const overall = $derived(data.dynasty?.settings.overall_title);

	// ---- the rules that hold in every league ----
	// Each is a short lead and the detail behind it.
	type Rule = [lead: string, detail: string];
	const topics: { id: string; title: string; about: string; rules: Rule[] }[] = [
		{
			id: 'basics',
			title: 'The basics',
			about: 'How the whole thing fits together.',
			rules: [
				['One franchise, a team in every sport.', 'Each manager runs one franchise. It has a separate roster in every league, and each league is its own competition with its own standings and its own champion.'],
				['Every league has its own rules.', 'Roster sizes, lineups, scoring, waivers and the rest are set per league by the commissioner. The league tabs on this page show what each one is playing under right now.'],
				['Players are kept for good.', 'This is a dynasty: a player stays on your roster from season to season until you drop or trade him.'],
				['Nothing is hidden in a rule change.', 'Points are worked out from the rules whenever they are shown, so if the commissioner changes a scoring value, every total for the season changes with it at once.'],
				['The day rolls over at 5 a.m. Eastern.', 'A late West Coast game counts for the day it started on.']
			]
		},
		{
			id: 'rosters',
			title: 'Rosters',
			about: 'The main roster, the reserve list, and their limits.',
			rules: [
				['Two lists.', 'The main roster holds your starters and bench. The reserve list is for players you are holding for later. Only main-roster players can start. Sizes differ by league.'],
				['A player is on one roster per league.', 'Nobody else can add, claim or draft a player while you hold him, on either list or as an unsigned draft pick.'],
				['Who may sit on reserve is a league rule.', 'It is one of: anyone; prospects only; or prospects and players outside the starting conferences (college). A rookie you signed from a draft may sit there in any league until you move him up.'],
				['A reserve lock, where a league has one.', 'If you send a player who is not a prospect to the reserve list, he cannot come back to the main roster for the number of days the league sets. Players put there by the commissioner or by graduating are not locked.'],
				['Over the limit means fix it first.', 'If a roster ends up over a limit (after a rule change, or a college player turning pro), the only moves allowed are ones that bring it closer to legal.'],
				['Leaving the main roster benches him.', 'A player you drop, trade or move to reserve comes out of your lineup from then on. What he already scored for you stands; if his game today has started, the change takes effect tomorrow.']
			]
		},
		{
			id: 'lineups',
			title: 'Lineups',
			about: 'Who starts, when it locks, and which games count.',
			rules: [
				['Set once, in force until changed.', 'A lineup carries on day after day (or week after week) until you change it. You do not have to set it every time.'],
				['Daily or weekly, by league.', 'A daily league sets a lineup for each day. A weekly league sets one for the week, and a matchup week matches the lineup week.'],
				['Locks.', 'By league, a starter locks either when his own game starts or when the day or week starts. A locked player cannot be moved in or out. The commissioner can override a lock.'],
				['Slots take positions.', 'Each starting slot lists the positions that fit it; a utility or flex slot lists several, or anyone.'],
				['One game a week, where a league says so.', 'Some slots count only a set number of a player’s games each week (one, in basketball). You pick the game on the lineup grid. Left alone it is his next game. A game already played cannot be picked afterwards, and he locks when the picked game starts, not before.'],
				['Pitcher starts, in baseball.', 'A league can cap how many pitcher starts score for a team in a week. A start past the cap, in the order played, scores nothing. A reliever who opens a game uses a start only if he pitches more than an inning.'],
				['Starting conferences, in college.', 'A college league names the conferences whose players may start. Players elsewhere can be drafted and held, but not started.']
			]
		},
		{
			id: 'scoring',
			title: 'Scoring',
			about: 'How points are counted.',
			rules: [
				['Only starters score.', 'A player scores for you only in games played while he is in your starting lineup.'],
				['Stats times the league’s values.', 'Each league lists what each stat is worth. A player’s points for a game are his stats multiplied by those values.'],
				['Live.', 'Games in progress are checked about every thirty seconds and pages update on their own. Official stat corrections are picked up the day after.'],
				['Bonuses are counted per game.', 'A double-double is ten or more in two of points, rebounds, assists, steals and blocks; a triple-double, three. A 50-point game also counts as a 40-point game, so it earns both bonuses.'],
				['Hockey and baseball details.', 'A shutout is credited when the game is over, to a goalie who played all of it without conceding. Power-play points count goals and assists. An inning pitched is three outs.']
			]
		},
		{
			id: 'seasons',
			title: 'Seasons and matchups',
			about: 'Schedules, standings, playoffs and titles.',
			rules: [
				['The commissioner starts each season.', 'A season has a first and last day. Only games inside it count. One season at a time per league.'],
				['Two formats.', 'Head to head: you play one franchise each matchup and are ranked by your record. Total points: most points over the season wins.'],
				['The schedule builds itself.', 'Starting a head-to-head season creates a round-robin schedule and the playoff rounds. It is rebuilt if the dates, the rules or the franchises change, keeping matchups that have begun, and is frozen once the playoffs start.'],
				['Weeks line up.', 'In a weekly league a matchup covers the same days as the lineup. A stretch of fewer than four days at the start or end of a season joins the week next to it.'],
				['Standings.', 'Head-to-head leagues rank by the share of matchups won, a tie counting half, then by points. Using the share keeps a team that has had a bye comparable.'],
				['Playoffs.', 'The top teams qualify. They are re-seeded every round, the best playing the worst left. If the number is not a power of two, the top seeds sit out the first round. A tied playoff matchup goes to the better seed.'],
				['The commissioner can step in.', 'On the Matchups page the commissioner can set any matchup that is not over by hand, in the regular season or the playoffs, and choose who has a bye. Matchups set this way are marked, survive a rebuilt schedule, and can be handed back to the automatic ones.'],
				['Champions.', 'The winner of the playoff final, or first place in a total-points league, is champion when the commissioner closes the season. A tie for first has to be settled by the commissioner naming one.']
			]
		},
		{
			id: 'moves',
			title: 'Adding and dropping',
			about: 'Free agents and the weekly limit.',
			rules: [
				['Free agents can be added at any time.', 'Anyone not on a roster and not on waivers can be added, to the main roster or the reserve list, if there is room and the league allows him on that list.'],
				['Unless the league holds new players for the draft.', 'Where that rule is on, a player who joined the pool after the league’s last draft cannot be added until the next draft has had its chance at him.'],
				['Paused during a draft.', 'A league’s free agency stops while its draft is live.'],
				['A weekly limit, where a league has one.', 'Free agent adds and waiver claims you win count toward it. The week starts on the day the league’s lineup week starts. Draft picks, trades and the commissioner’s placements do not count.'],
				['You can always drop.', 'Dropping a player is never refused.']
			]
		},
		{
			id: 'waivers',
			title: 'Waivers',
			about: 'What happens to a released player.',
			rules: [
				['Everyone released goes on waivers.', 'From the main roster, the reserve list, or as an unsigned draft pick. He stays for the time the league sets (a day by default) and cannot be added in that time, only claimed.'],
				['Claims.', 'A claim says which list he lands on and may name a player of yours to drop if it wins. You can change or withdraw it until it is settled. Nobody sees anyone else’s claims.'],
				['The waiver order decides.', 'When his time is up, the claim from the franchise highest in the waiver order gets him. A league may use blind bids instead: the highest bid wins and the order breaks ties.'],
				['A claim that cannot be carried out loses.', 'No room, over the weekly limit, or a bid bigger than the budget left: the claim is passed over, with the reason shown to you, and the next one gets its turn.'],
				['Winning sends you to the back.', 'A franchise that wins a claim moves to the end of the order.'],
				['The order is reset each season.', 'A new season sets it to last season’s standings reversed, worst first. The commissioner can rearrange it at any time. Before any season has been played it is alphabetical.'],
				['Unclaimed, he is a free agent.', 'Anyone can then add him at any time, to either list.']
			]
		},
		{
			id: 'drafts',
			title: 'Drafts',
			about: 'The startup draft, rookie drafts, and signing your picks.',
			rules: [
				['One startup draft per league.', 'It fills every roster, the main roster and the reserve list, and can cover several leagues at once. Prospects can be drafted too. A pick lands on the main roster until it is full and then on the reserve list, a prospect the other way round, and afterwards you set your reserve list yourself on your team page, any time before the season starts. A startup pick can sit on reserve whatever the league\'s rule for it. Picks are slow, four hours each unless the commissioner sets another clock. A league never has a second.'],
				['Your queue and auto pick.', 'Queue the players you want, in order. If your clock runs out, you get the first one still available; with nobody queued the pick is skipped and owed to you. Turn auto pick on in the draft room and your picks are made about three seconds after they come up: the top of your queue, or with nobody queued a random one of the best players left that you have a roster or reserve spot for. Your slots on the board change color so everyone can see it is on.'],
				['Then a rookie draft every year.', 'Open to anyone not on a roster: players new to the pool and free agents alike. The number of rounds is half the league’s reserve list, rounded up, unless the commissioner sets it.'],
				['Picks exist years ahead.', 'Each league’s next few rookie drafts are on the books from the day the league is created, so their picks can be traded.'],
				['The clock.', 'If a league uses a pick clock and yours runs out, the first available player in your queue is drafted for you. With an empty queue the pick is skipped, and you can make it up later while the draft is still running.'],
				['Passing.', 'In a rookie draft you may pass the pick you are on. A passed pick is gone for good.'],
				['Rankings and queues.', 'You can keep private ranked lists ahead of a draft and load one into your queue in the draft room. Players already taken are left out.'],
				['Season injury swaps.', 'A main-roster player tagged IR, IL or Out can swap with one reserve player, once per injured player per season. Both moves happen together. The injured player remains on reserve for the rest of that season, even after recovering, being traded or being dropped and re-added.'],
				['Rookie picks are held, then signed.', 'A rookie-draft pick is yours but on neither list, and counts against no limit. When the draft ends you have the league’s signing window to sign each one to the reserve list or the main roster, making room if you must.'],
				['Unsigned picks are released.', 'A pick you release, or have not signed when the window closes, goes on waivers like anyone else.'],
				['The commissioner runs the room.', 'They set the pick order before it starts, and can pause, undo the last pick, or finish the draft early, which forfeits picks not yet made.']
			]
		},
		{
			id: 'trades',
			title: 'Trades',
			about: 'What can be traded, and when a trade goes through.',
			rules: [
				['Players and picks, across leagues.', 'One trade between two franchises can include players from any leagues and draft picks from any drafts: a hockey player for a basketball pick is fine.'],
				['Picks trade until their draft starts.', 'Once a draft has started its picks stay where they are. Unsigned rookie picks can be traded too, and keep their signing deadline.'],
				['Both sides accept.', 'A trade goes through when both franchises accept, unless a league involved requires the commissioner’s approval, in which case it waits for that.'],
				['Rosters must be legal afterwards.', 'Every roster the trade touches is checked against its league’s limits when the trade is carried out. A player arrives on the same list he left, and a reserve lock travels with him.'],
				['Deadlines.', 'A league can set a trade deadline; after it, nothing involving that league can be traded until the commissioner moves it.'],
				['Reversal.', 'The commissioner can reverse a completed trade, sending everything back.']
			]
		},
		{
			id: 'pool',
			title: 'Players and moving up',
			about: 'Who is in the pool, and what happens when a player changes level.',
			rules: [
				['One record per person.', 'A player keeps the same record for his whole career, so his college seasons stay with him in the pros.'],
				['College players who turn pro stay yours.', 'Where a college league is linked to the pro league, a player you hold who reaches the pros moves to your roster there, landing on the list that league sets. If that puts you over a limit, the over-the-limit rule applies.'],
				['College players who leave without turning pro.', 'They are marked inactive and the commissioner decides what happens to them.'],
				['Football is skill positions only.', 'Quarterbacks, running backs, fullbacks, receivers and tight ends. Linemen, defenders and kickers are not in the pool.'],
				['Prospects.', 'Recruiting classes, draft classes, minor leaguers and club prospect lists are loaded so they can be drafted and held before they arrive.'],
				['When things update.', 'Scores are live. Rosters refresh every night, so a real-world signing or call-up shows the next morning. Prospect lists refresh weekly.']
			]
		},
		{
			id: 'commissioner',
			title: 'The commissioner',
			about: 'What the commissioner can do that managers cannot.',
			rules: [
				['Sets the rules.', 'Every league rule on this page, the seasons, and the drafts.'],
				['Can override the roster rules.', 'Placing, moving or dropping a player on any franchise, past limits, locks and free agency rules. A franchise left over a limit has to fix it before doing anything else.'],
				['Can override lineup locks.', 'To correct a lineup after a lock.'],
				['Settles what the rules cannot.', 'Approves trades where a league requires it, reverses trades, sets the waiver order, names a champion in a tie, and decides on players who have left their league.'],
				['Manages managers.', 'Adds franchises and issues invite links. A link works once; a new one resets a manager’s login.']
			]
		}
	];
	const topic = $derived(topics.find((t) => t.id === topicId) ?? topics[0]);

	// A first-round player's effect on a weekly matchup under the recommended
	// rules, from last season's games.
	const balance = [
		{ key: 'nba', top: 29.2, first: 18.9, third: 11.3 },
		{ key: 'nfl', top: 21.3, first: 15.7, third: 8.3 },
		{ key: 'cbb', top: 18.9, first: 13.8, third: 7.0 },
		{ key: 'nhl', top: 16.5, first: 11.6, third: 7.4 },
		{ key: 'mlb', top: 12.0, first: 8.0, third: 5.3 },
		{ key: 'wnba', top: 41.3, first: 30.0, third: 15.2 }
	];
	const nameOf = (key: string) => data.competitions.find((c) => c.key === key)?.name ?? key.toUpperCase();
</script>

<svelte:head><title>Rules</title></svelte:head>

<div class="stack">
	<header class="stack tight">
		<h1>Rules</h1>
		<Tabs {tabs} bind:value={() => tab, (v) => go({ tab: v })} label="Rules for" />
	</header>

	{#if tab === ''}
		<!-- ---- rules that hold in every league ---- -->
		<p class="muted">
			These hold in every league. Where a number or a choice differs by league, it is on that league's tab.
		</p>
		<div class="book">
			<nav class="topics" aria-label="Topics">
				{#each topics as t (t.id)}
					<button class:on={t.id === topic.id} aria-current={t.id === topic.id ? 'true' : undefined} onclick={() => go({ topic: t.id })}>
						<strong>{t.title}</strong>
						<span class="muted small-text">{t.about}</span>
					</button>
				{/each}
			</nav>
			<section class="panel topic" aria-live="polite">
				<h2>{topic.title}</h2>
				<dl>
					{#each topic.rules as [lead, detail] (lead)}
						<div class="rule">
							<dt>{lead}</dt>
							<dd>{detail}</dd>
						</div>
					{/each}
				</dl>
				{#if topic.id === 'basics' && overall?.enabled}
					<div class="rule note">
						<dt>The overall title.</dt>
						<dd>
							This dynasty also crowns an overall champion each year. A league finish earns points
							({overall.points_by_finish.map((p, i) => `${p} for ${i + 1}${['st', 'nd', 'rd'][i] ?? 'th'}`).join(', ')}), and the
							points from every league whose season started that year are added up.
						</dd>
					</div>
				{/if}
			</section>
		</div>
	{:else if guide}
		<!-- ---- one league's own rules ---- -->
		{@const g = guide}
		{@const count = starters(g.rules)}
		{@const some = limited(g.rules)}
		<p class="muted">
			What is specific to {g.name}{data.dynasty ? ', as it is set right now' : ', as recommended'}. Everything on the
			<button class="link" onclick={() => go({ tab: '' })}>Everywhere</button> tab applies too.
		</p>
		<div class="cards" data-sport={g.key}>
			<section class="card">
				<h3><SportBadge sport={g.key} solid /> Roster</h3>
				<ul>
					<li><strong>{g.rules.roster.main}</strong> on the main roster: {plural(count, 'starter')} and {plural(Math.max(g.rules.roster.main - count, 0), 'bench spot')}</li>
					<li><strong>{g.rules.roster.reserve}</strong> on the reserve list, open to {reserveFor[g.rules.roster.reserve_eligibility]}</li>
					{#if g.rules.roster.reserve_lock_season}
						<li>Once the season starts, nobody on the reserve list can be called up until it ends. A player can still be sent down, and stays there</li>
					{/if}
					<li>
						{#if g.rules.roster.reserve_lock_days > 0}
							A player sent to reserve is locked there for {plural(g.rules.roster.reserve_lock_days, 'day')}
						{:else if g.rules.roster.reserve_lock_season}
							Between seasons a player can move between the lists at any time
						{:else}
							No reserve lock: a player can come back up at any time
						{/if}
					</li>
				</ul>
			</section>

			<section class="card">
				<h3>Lineup</h3>
				<ul class="slots">
					{#each g.rules.lineup.slots as slot (slot.name)}
						<li><strong>{slot.count} × {slot.name}</strong> <span class="muted">{positions(slot.positions)}</span></li>
					{/each}
				</ul>
				<ul>
					<li>Set by the {g.rules.lineup.period}{g.rules.lineup.period === 'week' ? `, starting ${capital(g.rules.lineup.week_start)}` : ''}</li>
					<li>A starter locks when {g.rules.lineup.lock === 'game_start' ? 'his game starts' : `the ${g.rules.lineup.period} starts`}</li>
					{#if some.length > 0}
						<li>
							<strong>{some.length === g.rules.lineup.slots.length ? 'Every starter' : `Each ${listed(some.map((s) => s.name))}`}
								scores in {some[0].games_per_week === 1 ? 'one game' : `${some[0].games_per_week} games`} a week</strong>, which you pick
						</li>
					{:else}
						<li>Every game a starter plays counts</li>
					{/if}
					{#if g.rules.lineup.pitcher_starts_per_week > 0}
						<li><strong>{plural(g.rules.lineup.pitcher_starts_per_week, 'pitcher start')} a week</strong> score for a team</li>
					{/if}
					{#if conferences(g).length > 0}
						<li>Only players in these conferences can start: {listed(conferences(g))}</li>
					{/if}
				</ul>
			</section>

			<section class="card scoring">
				<h3>Scoring</h3>
				{#if sameScoring(g).length > 0}<p class="small-text">Scored the same as {listed(sameScoring(g).map((o) => o.name))}.</p>{/if}
				{#if data.dynasty && !recommended(g)}
					<p class="small-text muted">
						Differs from the recommended scoring for this sport.
						{#if data.me?.is_commissioner}<a href="/commissioner?section=rules">Change it in Rules</a>.{/if}
					</p>
				{/if}
				<table>
					<tbody>
						{#each scoring(g) as row (row.label)}
							<tr><td>{row.label}</td><td class="num" class:minus={row.points < 0}>{signed(row.points)}</td></tr>
						{/each}
					</tbody>
				</table>
			</section>

			<section class="card">
				<h3>Season</h3>
				<ul>
					{#if g.rules.format.type === 'head_to_head'}
						<li><strong>Head to head</strong>, {g.rules.format.matchup_days === 7 ? 'one-week' : `${g.rules.format.matchup_days}-day`} matchups</li>
						<li>{g.rules.format.playoff_teams} teams make the playoffs</li>
					{:else}
						<li><strong>Total points</strong>: most over the season wins</li>
					{/if}
					{#if g.rules.continuity}
						<li>
							A player who moves up to the {nameOf(g.rules.continuity.into)} stays with his franchise, on its
							{g.rules.continuity.land_on === 'reserve' ? 'reserve list' : 'main roster'} there
						</li>
					{/if}
				</ul>
			</section>

			<section class="card">
				<h3>Free agents and waivers</h3>
				<ul>
					{#if g.rules.free_agency.mode === 'closed'}
						<li><strong>No free agent adds</strong>: drafts and trades only</li>
					{:else}
						<li>Free agents can be added{g.rules.free_agency.weekly_limit > 0 ? `, up to ${g.rules.free_agency.weekly_limit} a week` : ', with no weekly limit'}</li>
						<li>
							{g.rules.free_agency.new_entrants_draft_only
								? 'Players new to the pool wait for the next rookie draft'
								: 'Players new to the pool can be added straight away'}
						</li>
					{/if}
					{#if g.rules.waivers.mode === 'none'}
						<li>No waivers: a released player is a free agent at once</li>
					{:else}
						<li><strong>{plural(g.rules.waivers.hours, 'hour')}</strong> on waivers for anyone released</li>
						<li>
							{g.rules.waivers.mode === 'faab'
								? `Claims go to the highest blind bid; each franchise has ${g.rules.waivers.budget} to bid a season`
								: 'Claims go by the waiver order'}
						</li>
					{/if}
				</ul>
			</section>

			<section class="card">
				<h3>Rookie draft</h3>
				<ul>
					<li><strong>{plural(rookieRounds(g.rules), 'round')}</strong> a year{g.rules.draft.rounds ? '' : ' (half the reserve list, rounded up)'}</li>
					<li>{g.rules.draft.order === 'snake' ? 'Snake order' : 'Same order every round'}</li>
					<li>{g.rules.draft.pick_clock_seconds > 0 ? `${g.rules.draft.pick_clock_seconds} seconds a pick` : 'No pick clock'}</li>
					<li><strong>{plural(g.rules.draft.signing_days, 'day')}</strong> to sign picks afterwards</li>
					<li>Picks can be traded {plural(g.rules.draft.future_years, 'year')} ahead</li>
				</ul>
			</section>

			<section class="card">
				<h3>Trades</h3>
				<ul>
					<li>{g.rules.trades.approval === 'commissioner' ? 'The commissioner must approve each trade' : 'Final when both sides accept'}</li>
					<li>{g.rules.trades.deadline ? `Deadline: ${g.rules.trades.deadline}` : 'No trade deadline'}</li>
				</ul>
			</section>
		</div>
	{:else}
		<!-- ---- how the recommended rules were arrived at ---- -->
		<section class="panel prose">
			<div class="text stack tight">
				<p>
					The aim is that a star is worth about the same in every sport, so a trade of a top hockey player for a top
					football player is a fair one. The recommended rules were tested against every game of last season: for each
					sport a 16-team league was filled, and each player was measured by how much he raises an average team's chance
					of winning a one-week matchup compared with the best player left on waivers.
				</p>
				<div class="scroll">
					<table class="balance">
						<thead>
							<tr><th>League</th><th class="num">Best player</th><th class="num">First round</th><th class="num">Third round</th></tr>
						</thead>
						<tbody>
							{#each balance as b (b.key)}
								<tr data-sport={b.key}>
									<td><SportBadge sport={b.key} /> {nameOf(b.key)}</td>
									<td class="num">+{b.top.toFixed(1)}</td><td class="num">+{b.first.toFixed(1)}</td><td class="num">+{b.third.toFixed(1)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
				<p class="muted small-text">
					Points of win probability added per matchup. +16 turns a .500 team into a .660 team. These describe the
					recommended rules; a league whose rules have been changed will differ.
				</p>
				<h3>What that led to</h3>
				<ul>
					<li><strong>Basketball counts one game a week per player.</strong> Stars separate from the field so much that counting every game changes little, and one game keeps lineups a decision. Three-pointers score so that guards are worth as much as big men.</li>
					<li><strong>Hockey scores shots at half a point.</strong> Shot volume is the steadiest sign of who the good players are, and it was the one change that lifted every tier of player. Lineup size made almost no difference.</li>
					<li><strong>One goalie, scored generously.</strong> A second goalie slot mostly adds luck. With wins, shutouts and saves weighted up, the best goalies are worth an early pick without swamping the skaters.</li>
					<li><strong>Baseball has a utility slot and four starts a week.</strong> Without the utility slot a designated hitter could not play anywhere. The starts cap stops a team winning on volume.</li>
					<li><strong>Football is full-point PPR with a superflex.</strong> Sixteen teams starting up to two quarterbacks makes the position scarce enough to matter.</li>
					<li><strong>The WNBA keeps rosters short.</strong> The whole league is 180 players, so 16 franchises can hold little more than their starters before nobody is left on waivers.</li>
				</ul>
				<h3>What is still uneven</h3>
				<p>
					Baseball is the noisiest sport week to week, so its stars move a matchup about half as much as football's. The
					WNBA runs the other way: with so few players to go round, its stars are worth close to double. No setting
					tested at one-week matchups closed either gap.
				</p>
			</div>
		</section>
	{/if}
</div>

<style>
	/* ---- site-wide rules: topics beside the one being read ---- */
	.book {
		display: grid;
		grid-template-columns: 17rem minmax(0, 1fr);
		gap: 1rem;
		align-items: start;
	}
	.topics {
		display: grid;
		gap: 0.25rem;
	}
	.topics button {
		display: grid;
		width: 100%;
		gap: 0.1rem;
		justify-content: start;
		justify-items: start;
		text-align: left;
		white-space: normal;
		font-weight: 400;
		padding: 0.55rem 0.75rem;
		border-color: transparent;
		background: transparent;
	}
	.topics button:hover {
		background: var(--surface);
	}
	.topics button.on {
		background: var(--surface);
		border-color: var(--rule-strong);
		box-shadow: inset 3px 0 0 var(--brand);
	}
	.topic {
		padding: 1rem 1.25rem 0.5rem;
	}
	dl {
		margin: 0.75rem 0 0;
	}
	.rule {
		padding: 0.7rem 0;
		border-top: 1px solid var(--rule);
		max-width: 44rem;
	}
	dt {
		font-weight: 650;
	}
	dd {
		margin: 0.15rem 0 0;
		color: var(--ink-soft);
	}
	.note dt {
		color: var(--gold);
	}
	@media (max-width: 820px) {
		.book {
			grid-template-columns: 1fr;
		}
		/* Topics become a strip to swipe along, so the rules stay near the top. */
		.topics {
			grid-auto-flow: column;
			grid-auto-columns: max-content;
			overflow-x: auto;
			padding-bottom: 0.3rem;
		}
		.topics button span {
			display: none;
		}
		.topics button.on {
			box-shadow: inset 0 -3px 0 var(--brand);
		}
	}

	/* ---- one league ---- */
	.link {
		display: inline;
		padding: 0;
		border: 0;
		background: none;
		color: var(--brand);
		font-weight: 600;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17.5rem, 1fr));
		gap: 0.8rem;
		align-items: start;
	}
	.cards h3 {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
	}
	.cards .card {
		border-top: 3px solid var(--sport);
	}
	ul {
		margin: 0;
		padding-left: 1.1rem;
	}
	ul + ul {
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--rule);
	}
	.slots {
		list-style: none;
		padding-left: 0;
	}
	li {
		margin: 0.2rem 0;
	}
	.scoring {
		grid-row: span 2;
	}
	.scoring td {
		padding: 0.28rem 0;
	}
	.scoring td.num {
		font-weight: 650;
		color: var(--good);
	}
	.scoring td.minus {
		color: var(--bad);
	}

	/* ---- balance ---- */
	.text {
		padding: 0.9rem 1rem 1.1rem;
		max-width: 46rem;
	}
	.text h3 {
		margin-top: 0.5rem;
	}
	.balance td:first-child {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
</style>
