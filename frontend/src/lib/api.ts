// The one place the frontend talks to the Go API.

// ---- settings: mirrors backend/internal/settings ----

/** games_per_week: how many of a player's games count each week in this slot; 0 for all of them. */
export type Slot = { name: string; positions: string[]; count: number; games_per_week: number };

export type LeagueSettings = {
	roster: {
		main: number;
		reserve: number;
		reserve_eligibility: 'prospects_only' | 'anyone' | 'prospects_or_ineligible';
		/** Days a non-prospect sent to reserve must stay there; 0 for none. */
		reserve_lock_days: number;
		/** Nobody on reserve can be called up while the season is being played. */
		reserve_lock_season: boolean;
	};
	lineup: {
		conferences?: string[] | null;
		period: 'day' | 'week';
		week_start: string;
		lock: 'game_start' | 'period_start';
		slots: Slot[];
		/** Baseball: how many pitcher starts score for a team in a week; 0 for all of them. */
		pitcher_starts_per_week: number;
	};
	scoring: Record<string, number>;
	format: {
 type: 'total_points' | 'head_to_head'; matchup_days: number; playoff_teams: number;
 standings_by?: 'record' | 'win_percentage' | 'points';
 tiebreakers?: ('points_for' | 'head_to_head' | 'points_against')[];
 reseed?: boolean; consolation?: boolean | null; consolation_seeding?: 'bottom_first' | 'top_first';
 };
	/** weekly_limit: acquisitions a franchise may make in a week; 0 for no limit. */
	free_agency: { mode: 'open' | 'closed'; new_entrants_draft_only: boolean; weekly_limit: number };
	/** What happens to a dropped player: days on waivers, and how claims are ordered. */
	waivers: { mode: 'none' | 'rolling' | 'faab'; hours: number; budget: number };
	/** rounds 0 means half the reserve list, rounded up. signing_days: how long picks can be signed after a rookie draft. */
	draft: { rounds: number; order: 'linear' | 'snake'; pick_clock_seconds: number; future_years: number; signing_days: number };
	trades: { deadline: string; approval: 'none' | 'commissioner' };
	continuity: { into: string; land_on: List } | null;
};

export type DynastySettings = {
	overall_title: { enabled: boolean; points_by_finish: number[] };
};

// ---- data ----

/** "rights" is a rookie-draft pick not yet signed: owned, but on neither list. */
export type List = 'main' | 'reserve' | 'rights';

export type Competition = {
	conferences: { id: string; name: string }[] | null;
	key: string;
	name: string;
	positions: string[];
	stats: { key: string; label: string }[];
	defaults: LeagueSettings;
	/** When the real regular season usually runs: ["10-20", "04-12"]. */
	season: [string, string];
	players: number;
	has_prospects: boolean;
};

export type League = {
	id: string;
	competition: string;
	name: string;
	settings: LeagueSettings;
	last_draft_at: string | null;
};

export type Franchise = {
	image_url: string;
	id: string;
	name: string;
	manager_name: string;
	slug: string;
	is_commissioner: boolean;
};

/** The signed-in manager: their franchise and the account they sign in with. */
export type Session = Franchise & { email: string };

/** A franchise as the commissioner manages it. */
export type Invite = Franchise & {
	email: string; // empty until the manager creates an account
	invite_token: string; // empty when no invite link is outstanding
};

export type Credentials = { email: string; password: string };

export type TeamIdentity = { franchise_id: string; league_id: string; competition: string; name: string; image_url: string };
export type Dynasty = {
 team_identities: TeamIdentity[];
	id: string;
	name: string;
	settings: DynastySettings;
	leagues: League[];
	franchises: Franchise[];
};

export type ProspectRankingFields = {
 prospect_rank: number | null; prospect_rank_year: number | null;
 prospect_rank_source: string; prospect_rank_url: string; prospect_eta: string;
};

export type Player = ProspectRankingFields & {
	id: string;
	competition: string;
	status: 'prospect' | 'active' | 'inactive';
	injury_designation: string;
	full_name: string;
	positions: string[];
	birth_date: string | null;
	class: string;
	note: string;
	headshot_url: string;
	team_abbrev: string;
	team_name: string;
	owner_name: string;
	owner_slug: string;
	/** Set while he is on waivers: he can be claimed, not added. */
	waiver_until: string | null;
	/** The season the two figures below are for, e.g. "2025-26"; empty when he has none on record. */
	season: string;
	/** Fantasy points that season under his league's scoring. */
	season_points: number;
	/**
	 * Those points on a scale shared by every sport: 100 is the average player a league
	 * that size would roster, and 15 is one standard deviation among them. 0 when unknown.
	 */
	season_index: number;
};

/** A season there are stats for in one sport. */
export type StatSeason = { competition: string; year: number; label: string };

export type PlayerPage = { players: Player[]; total: number; page: number; per_page: number };

export type ResearchPlayer = Pick<Player, 'injury_designation' | 'id' | 'competition' | 'full_name' | 'positions' | 'status' | 'headshot_url' | 'birth_date' | 'owner_name' | 'owner_slug'> & {
	team: string;
	season: string;
	games: number;
	stats: Record<string, number>;
	points: number;
	points_per_game: number;
	row_key: string;
	scoring_source: 'league' | 'defaults';
	normalization_group: string;
	qualification_note: string;
	benchmark_minimum_games: number;
	benchmark_minimum_innings: number;
	has_scoring_stats: boolean;
	qualified: boolean;
	metric_position: string;
	league_index: number | null;
	position_index: number | null;
	percentile: number | null;
	replacement_rate: number | null;
	replacement_rank: number | null;
	points_above_replacement: number | null;
	par_per_game: number | null;
	win_share_added: number | null;
	availability: number | null;
	production_share: number | null;
};
export type ResearchBenchmark = {
	mean_index: number | null;
	competition: string; season: string; position: string; players: number; mean: number; sd: number;
	starter_slots: number; replacement_rank: number | null; replacement_rate: number | null;
};
export type ResearchPool = { competition: string; season: string };
export type ResearchChartPoint = { key: string; label: string; group: string; x: number | null; y: number | null };
export type ResearchAnalysis = {
	competition: string; season: string; scoring_source: 'league' | 'defaults'; players: number; qualified: number;
	scored: number; games: number; points: number; median: number; p90: number; top_ten_share: number;
	contributions: Record<string, number>;
};
export type ResearchFilter = Omit<PlayerFilter, 'available_in' | 'draft_id'> & {
raw_per_game?: string; pools?: string; max_games?: number; max_rate?: number; max_index?: number; min_par?: number;
	qualified_only?: string; missing_only?: string; stat_key?: string; min_stat?: number; max_stat?: number;
	include_chart?: string; chart_x?: string; chart_y?: string; chart_group?: string;
	season?: string; sort?: string; position?: string; team?: string; owner?: string;
	min_games?: number; min_rate?: number; min_index?: number; benchmark_games?: number; pitcher_workload_percent?: number;
	replacement_rank?: number; above_replacement?: string; ascending?: string;
};
export type ResearchPage = {
	pitcher_workload_percent: number;
	starting_conferences: Record<string, string[]>;
	players: ResearchPlayer[];
	total: number;
	page: number;
	per_page: number;
	seasons: { label: string; year: number }[];
	teams: string[];
	benchmarks: ResearchBenchmark[];
	benchmark_games: number;
	raw_stat_keys: string[];
	catalog: { competition: string; label: string; year: number; players: number; synced_at: string }[];
	analysis: ResearchAnalysis[];
	chart: ResearchChartPoint[];
	chart_total: number;
	warnings: string[];
};

export type PlayerResearch = {
	scoring_source: 'league' | 'defaults';
	player: Player;
	games: { id: string; day: string; away_abbrev: string; home_abbrev: string; stats: Record<string, number>; points: number }[];
};

/** One season of a player's, valued under his league's current scoring. */
export type PlayerSeason = {
	competition: string; // where it was played: an NBA player's college seasons are "cbb"
	season: string; // "2025-26"
	team: string;
	league: string; // named when it is not the player's current league: "College Basketball", "OHL"
	games: number;
	stats: Record<string, number>;
	points: number;
	points_per_game: number;
};

export type ProfileSeason = PlayerSeason & {
 key: string; year: number; synced_at: string; scoring_source: string;
 research: ResearchPlayer | null; research_note: string; eligible_splits: number; splits: number;
};
export type PlayerEvent = {
 id: number; kind: string; detail: Record<string, unknown>; created_at: string;
 competition: string; franchise_name: string; franchise_slug: string;
 trade_id: string | null; draft_id: string | null; draft_name: string; pick_round: number; pick_position: number;
};
export type PlayerTimeline = { events: PlayerEvent[]; total: number; page: number; per_page: number };
export type ProfileGame = { id: string; competition: string; day: string; starts_at: string;
 stats: Record<string, number>; away_abbrev: string; home_abbrev: string; points: number; scoring_source: string };
export type PlayerGameLog = { games: ProfileGame[]; total: number; page: number; per_page: number };
export type PlayerProfile = {
 fantasy_production: { season_id: string; year: number; competition: string; franchise_name: string; franchise_slug: string; games: number; points: number }[];
 player: ProspectRankingFields & Pick<Player, 'injury_designation' | 'id' | 'competition' | 'status' | 'full_name' | 'positions' | 'birth_date' | 'class' | 'note' | 'headshot_url'> & { team_abbrev: string };
 seasons: ProfileSeason[]; game_log: PlayerGameLog; timeline: PlayerTimeline;
 ownership: { league_id: string; competition: string; league_name: string; franchise_name: string; franchise_slug: string;
 franchise_id: string; list: string; acquired_via: string; acquired_at: string; reserved_at: string | null; slot: string }[];
 drafts: { id: string; round: number; position: number; auto_picked: boolean; picked_at: string | null;
 draft_id: string; draft_name: string; year: number; kind: string; franchise_name: string; franchise_slug: string; competition: string }[];
 trades: { id: string; status: string; note: string; created_at: string; resolved_at: string | null;
 from_name: string; from_slug: string; to_name: string; to_slug: string; competition: string }[];
 waivers: { league_id: string; competition: string; clears_at: string }[];
 starter_eligible: boolean; eligibility_note: string; conference: string;
 scoring_rules: Record<string, Record<string, number>>; rules_sources: Record<string, string>;
 reserve_locked_until: Record<string, string>;
};

export type PlayerFilter = {
	competition?: string;
	status?: string;
	q?: string;
	/** One of the sport's positions. */
	position?: string;
	/** Ages in whole years. Either one leaves out players whose birth date is not known. */
	min_age?: number;
	max_age?: number;
	available_in?: string;
	draft_id?: string;
	/** "points" or "index" puts the season's highest scorers first; otherwise by name. */
	sort?: string;
	/** Which season to score: 0 is each sport's latest, 1 the one before, and so on. */
	season_back?: number;
	page?: number;
};

export type RosterPlayer = {
 injury_reserve_locked: boolean;
 nickname: string;
	player_id: string;
	list: List;
	full_name: string;
	positions: string[];
	status: Player['status'];
	injury_designation: string;
	class: string;
	note: string;
	headshot_url: string;
	team_abbrev: string;
	/** Set while a reserve lock keeps him off the main roster. */
	locked_until: string | null;
	/** For a rookie-draft pick held as rights: when he is released if not signed. Null until the draft ends. */
	rights_until: string | null;
};

/** How many rounds a league's rookie draft has: its own number, or half the reserve list, rounded up. */
export const rookieRounds = (rules: LeagueSettings) => rules.draft.rounds || Math.max(1, Math.ceil(rules.roster.reserve / 2));

export type LeagueRoster = {
 team_name: string; image_url: string;
	league_id: string;
	competition: string;
	name: string;
	limits: LeagueSettings['roster'];
	overage: number;
	players: RosterPlayer[];
	/** The season the commissioner has opened, and where this franchise stands with its roster for it; null when none is open. */
	season: SeasonRoster | null;
};

export type RosterPerformancePlayer = {
 player_id: string; games: number; stats: Record<string,number>; points: number; points_per_game: number;
 overall_rank: number | null; position_ranks: Record<string,number>;
};
export type RosterPerformance = {
 current_season: string; window: string; season: string; from_day: string; to_day: string; source: 'games' | 'season';
 ranked_players: number; seasons: {label:string;year:number}[]; players: RosterPerformancePlayer[];
};

export type SeasonRoster = {
	id: string;
	year: number;
	starts_on: string;
	ends_on: string;
	/** When the franchise set its roster for the season; null until it has. */
	set_at: string | null;
	/** The season's first day has come. */
	started: boolean;
	/** Players stay on the list they are on until the season ends. */
	locked: boolean;
	/** When rosters not yet set lock as they stand; null for no deadline. */
	deadline: string | null;
	/** The deadline has passed. */
	expired: boolean;
};

/** An unused draft pick a franchise holds. */
export type HeldPick = {
	id: string;
	round: number;
	position: number;
	original_franchise_id: string;
	draft_id: string;
	draft_name: string;
	year: number;
	draft_status: DraftStatus; // only picks in a scheduled draft can be traded
	competitions: string[];
};

export type FranchiseDetail = { franchise: Franchise; rosters: LeagueRoster[]; picks: HeldPick[] };

export type Activity = {
	id: number;
	kind: string;
	detail: { list?: List; forced?: boolean; from?: string; reversed?: boolean; bid?: number };
	created_at: string;
	competition: string;
	franchise_name: string;
	franchise_slug: string;
	player_name: string;
	draft_name: string; // for a traded pick
	pick_round: number;
};

export type NewPlayer = {
	competition: string;
	full_name: string;
	positions: string[];
	birth_date: string;
	note: string;
	status: string;
};

export type MergeSuggestion = {
	prospect_id: string;
	player_id: string;
	full_name: string;
	competition: string;
	prospect_note: string;
	player_status: string;
	player_class: string;
	player_team: string;
};

export type IngestRun = {
	id: number;
	competition: string;
	job: string;
	started_at: string;
	finished_at: string | null;
	status: 'running' | 'ok' | 'error';
	rows_upserted: number;
	error: string;
};

export type Setup = {
	name: string;
	settings: DynastySettings;
	leagues: { competition: string; settings: LeagueSettings }[];
	franchises: { name: string; manager_name: string }[];
	account: Credentials;
};

// ---- seasons, lineups and standings ----

export type Season = {
	id: string;
	league_id: string;
	year: number;
	starts_on: string;
	ends_on: string;
	status: 'active' | 'complete';
	champion_franchise_id: string | null;
	/** When rosters not yet set for the season lock as they stand; null for no deadline. */
	roster_deadline: string | null;
};

export type StandingsRow = {
	points_for: number;
	points_against: number;
	franchise_id: string;
	points: number;
	/** A head-to-head league's record over its finished regular-season matchups. */
	wins: number;
	losses: number;
	ties: number;
	players: { player_id: string; full_name: string; headshot_url: string; nickname?: string; games: number; points: number }[];
};

/** One league's table for its latest season, best first. */
export type Standings = {
	consolation_champion_id: string | null;
	league_id: string;
	competition: string;
	format: LeagueSettings['format']['type'];
	season: Season | null;
	rows: StandingsRow[];
};

// ---- head-to-head ----

/** by_hand: the commissioner set its matchups, so the schedule leaves them alone.
 * double_games: a long matchup in which slots count twice the games and the pitcher start cap doubles. */
export type Period = { id: string; seq: number; starts_on: string; ends_on: string; is_playoff: boolean; by_hand: boolean; double_games: boolean };

export type Matchup = {
	id: string;
	bracket: 'championship' | 'consolation';
	bracket_slot: number;
	home_franchise_id: string;
	away_franchise_id: string | null; // null for a bye
	home_points: number;
	away_points: number;
	final: boolean;
};

/** A period of a season's schedule; its matchups have no points until it starts. */
export type SchedulePeriod = Period & { matchups: Matchup[] };
export type GameDayCount = { day: string; games: number };
export type MatchupPairing = Pick<Matchup, 'home_franchise_id' | 'away_franchise_id'> & Partial<Pick<Matchup, 'bracket' | 'bracket_slot'>>;
export type CustomPeriod = Pick<Period, 'starts_on' | 'ends_on' | 'is_playoff' | 'double_games'> & {
 matchups: MatchupPairing[] | null;
};

/** One period of a head-to-head season. */
export type Matchups = { periods: Period[]; period: Period | null; matchups: Matchup[] };

export type ScoredGame = {
 game_id: string; day: string; slot: string; points: number; stats: Record<string, number>;
 starts_at: string; status: string; home: string; away: string; positions: string[];
};
export type MatchupScorer = StandingsRow['players'][number] & { game_lines?: ScoredGame[] };

export type MatchupDetail = Matchup & {
	competition: string;
	period: Period;
	home_players: MatchupScorer[];
	away_players: MatchupScorer[];
	/** Each side's points so far plus what its starters are projected to add; null once final, or for a bye. */
	home_projected: number | null;
	away_projected: number | null;
};

// ---- scores ----

/** A real game: who is playing, the score, and where it stands. */
export type GameSummary = {
	id: string;
	competition: string;
	day: string;
	starts_at: string;
	status: 'scheduled' | 'live' | 'final';
	detail: string; // "7:32 - 3rd", "Final/OT"; empty before the start
	home_score: number;
	away_score: number;
	home_abbrev: string;
	home_name: string;
	home_logo: string;
	away_abbrev: string;
	away_name: string;
	away_logo: string;
};

export type Scoreboard = { competition: string; day: string; games: GameSummary[] };

/** One player's line in a game, with what it is worth in this league. */
export type GameLine = {
	player_id: string;
	full_name: string;
	positions: string[];
	headshot_url: string;
	at_home: boolean;
	stats: Record<string, number>;
	points: number;
	owner_name: string;
	owner_slug: string;
};

export type GameDetail = GameSummary & { lines: GameLine[] };

/** A rostered player who has dropped out of his competition's feed. */
export type ReviewItem = {
	player_id: string;
	full_name: string;
	positions: string[];
	competition: string;
	league_id: string;
	list: List;
	franchise_id: string;
	franchise_name: string;
	franchise_slug: string;
};

/** The cross-sport table for one year's seasons. */
export type OverallYear = {
	year: number;
	final: boolean;
	rows: { franchise_id: string; points: number; finishes: Record<string, number> }[];
};

export type LineupGame = {
 game_id?: string;
	day: string;
	starts_at: string;
	status: string;
	opponent: string;
	at_home: boolean;
	points: number;
	/** False for a game left out by a slot that counts only some games a week. */
	counts: boolean;
	/** Whether a feed has him starting in goal or on the mound; other: a teammate at his position is. */
	starter: '' | 'confirmed' | 'expected' | 'other';
	/** Projected fantasy points in a game still to be played, from his recent seasons and the betting line. */
	projected: number;
	/** His stat line by the competition's stat keys; empty until he plays. */
	stats: Record<string, number>;
};

export type LineupPlayer = {
 nickname: string;
	starter_eligible: boolean;
	eligibility_note: string;
	player_id: string;
	full_name: string;
	positions: string[];
	headshot_url: string;
	team_abbrev: string;
	/** An injured-list designation (IR, IL10, OUT…), or empty. */
	injury: string;
	slot: string; // empty on the bench
	/** The day his counted games begin, in a slot that counts only some; empty for his first game. */
	counts_from: string;
	/** In a slot that counts only some games, on auto: his last games of the week with a stat count. */
	counts_last: boolean;
	locked: boolean;
	points: number;
	games: LineupGame[];
};

/** A franchise's lineup for one day, or for a week in a weekly league. */
export type Lineup = {
	day: string;
	last_day: string;
	slots: Slot[];
	players: LineupPlayer[];
	locked: string; // why the whole lineup cannot change, or empty
	/** The week's pitcher starts against the league's cap; null in a league without one. */
	starts: { limit: number; made: { player_id: string; full_name: string; day: string; innings: number; counts: boolean }[] } | null;
	/** Set in a matchup the commissioner doubled: twice the games count, and the start cap doubles, over the whole matchup. */
	doubled: { starts_on: string; ends_on: string } | null;
};

export type LineupChange = {
	day: string;
	/** auto: in a slot that counts only some games, count his last games with a stat, not a picked one. */
	entries: { slot: string; player_id: string; counts_from?: string; auto?: boolean }[];
	franchise_id?: string;
	force?: boolean;
};

// ---- trades ----

export type TradeStatus = 'proposed' | 'accepted' | 'executed' | 'rejected' | 'cancelled' | 'reversed';

/** One asset changing hands, as shown in a trade. */
export type TradeDrop = { franchise_id: string; league_id: string; player_id: string; list: string; name: string };

export type TradeItem = {
 league_id: string | null;
	id: string;
	from_franchise: string;
	to_franchise: string;
	player_id: string | null;
	draft_pick_id: string | null;
	player_name: string;
	player_positions: string[];
	player_headshot: string;
	competition: string;
	draft_name: string;
	pick_round: number;
	pick_original_franchise_id: string | null;
};

export type Trade = {
 drops: TradeDrop[];
	id: string;
	status: TradeStatus;
	proposed_by: string;
	note: string;
	created_at: string;
	resolved_at: string | null;
	parties: { franchise_id: string; accepted_at: string | null }[];
	items: TradeItem[];
};

/** One asset in an offer being built. */
export type OfferItem = {
	from_franchise_id: string;
	to_franchise_id: string;
	league_id?: string;
	player_id?: string;
	draft_pick_id?: string;
};

export type TradeAnswer = 'accept' | 'reject' | 'cancel' | 'drops';
export type TradeRuling = 'approve' | 'veto' | 'reverse';

// ---- drafts ----

export type DraftStatus = 'scheduled' | 'live' | 'paused' | 'complete';

export type Draft = {
 is_placeholder: boolean;
	id: string;
	name: string;
	kind: 'startup' | 'seasonal';
	year: number;
	status: DraftStatus;
	pick_clock_seconds: number;
	clock_expires_at: string | null;
	completed_at: string | null;
};

/** A draft as the list shows it. */
export type DraftSummary = Draft & { competitions: string[]; picks: number; picks_made: number };

export type DraftPick = {
	id: string;
	round: number;
	position: number;
	original_franchise_id: string;
	current_franchise_id: string;
	player_id: string | null;
	picked_at: string | null;
	auto_picked: boolean;
	skipped_at: string | null;
	/** Set when the owner gave the pick up in a rookie draft: it cannot be made later. */
	passed_at: string | null;
	player_name: string;
	player_positions: string[];
	player_headshot: string;
	competition: string;
	/** For the draft's stats: the player's birth date, his year in college and his team. */
	player_birth_date: string | null;
	player_class: string;
	player_team: string;
};

export type DraftState = {
	draft: Draft;
	league_ids: string[];
	picks: DraftPick[];
	on_clock_pick_id: string | null;
	/** Franchises with auto pick on: their picks are made for them a few seconds after they come up. */
	auto_pick_franchise_ids: string[];
	/** Managers' emoji on made picks, oldest first. */
	reactions: PickReaction[];
};

export type PickReaction = { pick_id: string; franchise_id: string; emoji: string };

/** What the draft room's live connection sends. */
export type DraftMessage = DraftState & { type: 'state' | 'update' };

export type QueuedPlayer = {
	player_id: string;
	full_name: string;
	positions: string[];
	competition: string;
	headshot_url: string;
	team_abbrev: string;
};

/** One of the signed-in manager's pre-draft lists, as the list of them shows it. */
export type RankingSummary = {
	id: string;
	league_id: string | null;
	draft_id: string | null;
	name: string;
	competition: string;
	draft_name: string; // empty when it is not attached to a draft
	players: number;
};

export type RankedPlayer = QueuedPlayer & { note: string; owner_name: string };

/** A pre-draft list with its players in order. */
export type Ranking = { id: string; league_id: string | null; draft_id: string | null; name: string; players: RankedPlayer[] };

/** player_ids, when given, replaces the list. */
export type RankingChange = { name: string; league_id?: string; draft_id?: string | null; player_ids?: string[] };

/**
 * The next draft each league will hold: the earliest that is not finished.
 * A draft covering several leagues appears once.
 */
export function nextDrafts(drafts: DraftSummary[]): DraftSummary[] {
	const open = drafts.filter((d) => d.status !== 'complete').toSorted((a, b) => a.year - b.year);
	const next = new Map<string, DraftSummary>();
	for (const draft of open) {
		for (const competition of draft.competitions) if (!next.has(competition)) next.set(competition, draft);
	}
	return open.filter((d) => [...next.values()].includes(d));
}

export type NewDraft = {
	name: string;
	kind: Draft['kind'];
	year: number;
	league_ids: string[];
	rounds: number;
	order: 'linear' | 'snake';
	franchise_order: string[];
	pick_clock_seconds: number;
};

export type PickSlot = {
	id?: string;
	round: number;
	original_franchise_id: string;
	current_franchise_id: string;
};

export type DraftAction = 'start' | 'pause' | 'resume' | 'undo' | 'finish';

/** A league's waivers as the signed-in manager sees them; claims are their own. */
export type Waivers = {
	rules: LeagueSettings['waivers'];
	weekly_limit: number;
	acquisitions: number;
	players: {
		player_id: string;
		clears_at: string;
		full_name: string;
		positions: string[];
		status: Player['status'];
	injury_designation: string;
		note: string;
		headshot_url: string;
		team_abbrev: string;
	}[];
	standings: { franchise_id: string; name: string; slug: string; priority: number; budget_left: number }[];
	claims: {
		id: string;
		player_id: string;
		bid: number;
		list: 'main' | 'reserve';
		status: 'pending' | 'won' | 'lost';
		reason: string;
		player_name: string;
		drop_player_name: string;
		created_at: string;
		resolved_at: string | null;
	}[];
};
export type WaiverClaim = { player_id: string; drop_player_id?: string; bid: number; list?: 'main' | 'reserve' };

/** drop_player_id, for an add: released to make room, in the same move. */
export type RosterChange = { player_id: string; replacement_id?: string; drop_player_id?: string; list?: List; franchise_id?: string; force?: boolean };
/** A franchise's whole reserve list; everyone else it has signed goes to the main roster. */
export type RosterSplit = { reserve: string[]; franchise_id?: string; force?: boolean };

// ---- transport ----

/** An error whose message the server wrote for the user to read. */
export class ApiError extends Error {
	constructor(
		message: string,
		public status: number
	) {
		super(message);
	}
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const response = await fetch(`/api${path}`, {
		method,
		headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
	const text = await response.text();
	const data = text ? JSON.parse(text) : undefined;
	if (!response.ok) {
		throw new ApiError(data?.error ?? `Request failed (${response.status})`, response.status);
	}
	return data;
}

function query(params: Record<string, string | number | undefined>): string {
	const search = new URLSearchParams();
	for (const [key, value] of Object.entries(params)) {
		// A cleared number input is bound as null.
		if (value != null && value !== '') search.set(key, String(value));
	}
	return search.size ? `?${search}` : '';
}

/** Resolves to null when the server answers 404. */
const orNull = <T>(promise: Promise<T>) =>
	promise.catch((e) => {
		if (e instanceof ApiError && e.status === 404) return null;
		throw e;
	});

// ---- endpoints ----

export const getCompetitions = () => request<Competition[]>('GET', '/competitions');
export const getPlayers = (filter: PlayerFilter) => request<PlayerPage>('GET', `/players${query(filter)}`);
export const getResearch = (filter: ResearchFilter) =>
	request<ResearchPage>('GET', `/research${query(filter)}`);
export const getStatSeasons = () => request<StatSeason[]>('GET', '/stat-seasons');
export const getPlayerResearch = (id: string) => request<PlayerResearch>('GET', `/players/${id}`);
export const getPlayerProfile = (id: string) => request<PlayerProfile>('GET', `/players/${id}/profile`);
export const getPlayerTimeline = (id: string, page: number) => request<PlayerTimeline>('GET', `/players/${id}/transactions${query({page})}`);
export const getPlayerGameLog = (id: string, page: number, competition = '') => request<PlayerGameLog>('GET', `/players/${id}/games${query({page, competition})}`);
export const getPlayerSeasons = (id: string) => request<PlayerSeason[]>('GET', `/players/${id}/seasons`);

export const getDynasty = () => orNull(request<Dynasty>('GET', '/dynasty'));
export const createDynasty = (setup: Setup) => request<void>('POST', '/dynasty', setup);
export const updateDynasty = (name: string, settings: DynastySettings) =>
	request<void>('PUT', '/dynasty', { name, settings });
export const updateLeagueSettings = (leagueId: string, settings: LeagueSettings) =>
	request<void>('PUT', `/leagues/${leagueId}/settings`, settings);

export const addLeague = (competition: string) => request<League>('POST', '/admin/leagues', { competition });

/** A real team, for its logo. */
export type ProTeam = { competition: string; abbrev: string; name: string; logo_url: string };
export const getTeams = () => request<ProTeam[]>('GET', '/teams');
/** A player on the trade market: on a manager's block, or someone a manager wants. */
export type MarketPlayer = {
	league_id: string;
	competition: string;
	player_id: string;
	full_name: string;
	positions: string[];
	headshot_url: string;
	team_abbrev: string;
	list: List;
};
export type BlockEntry = MarketPlayer & { franchise_id: string; note: string; added_at: string };
export type TradeWants = { franchise_id: string; league_id: string; competition: string; positions: string[]; note: string; updated_at: string };
export type TradeTarget = MarketPlayer & { by_franchise_id: string; owner_franchise_id: string; created_at: string };
/** The trade market as the signed-in manager sees it. Targets are seen only by whoever set them and the player's owner. */
export type TradeMarket = {
	block: BlockEntry[];
	wants: TradeWants[];
	/** Players this manager is after. */
	targets: TradeTarget[];
	/** This manager's players others are after. */
	interest: TradeTarget[];
};
export const getTradeMarket = () => request<TradeMarket>('GET', '/trade-market');
export const setTradeBlock = (change: { league_id: string; player_id: string; on: boolean; note?: string }) =>
	request<void>('PUT', '/trade-market/block', change);
export const setTradeWants = (change: { league_id: string; positions: string[]; note: string }) =>
	request<void>('PUT', '/trade-market/wants', change);
export const setTradeTarget = (change: { league_id: string; player_id: string; on: boolean }) =>
	request<void>('PUT', '/trade-market/targets', change);

export const getSession = () => request<Session | null>('GET', '/session');
export const logIn = (credentials: Credentials) => request<void>('POST', '/auth/login', credentials);
export const logOut = () => request<void>('POST', '/auth/logout');
export const getInvite = (token: string) => request<{ franchise: Franchise; reset: boolean }>('GET', `/invites/${token}`);
export const claimInvite = (token: string, credentials: Credentials) =>
	request<void>('POST', `/invites/${token}/claim`, credentials);

export const setOrganizationIdentity = (id: string, name: string, image_url: string) => request<void>('PUT', `/franchises/${id}/identity`, {name,image_url});
export const setTeamIdentity = (id: string, league: string, name: string, image_url: string) => request<void>('PUT', `/franchises/${id}/leagues/${league}/identity`, {name,image_url});
export const getPlayerNickname = (franchise: string, player: string) => request<{nickname: string}>('GET', `/franchises/${franchise}/players/${player}/nickname`);
export const setPlayerNickname = (franchise: string, player: string, nickname: string) => request<void>('PUT', `/franchises/${franchise}/players/${player}/nickname`, {nickname});
export const getFranchise = (slug: string) => request<FranchiseDetail>('GET', `/franchises/${slug}`);
export const changeRoster = (leagueId: string, action: 'add' | 'drop' | 'move' | 'injury-swap', change: RosterChange) =>
	request<void>('POST', `/leagues/${leagueId}/roster/${action}`, change);
export const setRoster = (leagueId: string, split: RosterSplit) =>
	request<void>('POST', `/leagues/${leagueId}/roster/set`, split);
/** Sets the roster for the season the commissioner has opened: final, once. */
export const setSeasonRoster = (leagueId: string, split: RosterSplit) =>
	request<void>('POST', `/leagues/${leagueId}/roster/season`, split);
export const getWaivers = (leagueId: string) => request<Waivers>('GET', `/leagues/${leagueId}/waivers`);
export const claimWaiver = (leagueId: string, claim: WaiverClaim) =>
	request<void>('POST', `/leagues/${leagueId}/waivers/claims`, claim);
export const setWaiverOrder = (leagueId: string, franchise_ids: string[]) =>
	request<void>('PUT', `/admin/leagues/${leagueId}/waiver-order`, { franchise_ids });
export const cancelWaiverClaim = (id: string) => request<void>('DELETE', `/waivers/claims/${id}`);
export const getActivity = () => request<Activity[]>('GET', '/activity');

export const getInvites = () => request<Invite[]>('GET', '/admin/franchises');
export const addFranchise = (name: string, manager_name: string) =>
	request<Invite>('POST', '/admin/franchises', { name, manager_name });
export const updateFranchise = (f: Franchise) =>
	request<void>('PUT', `/admin/franchises/${f.id}`, {
		name: f.name,
		manager_name: f.manager_name,
		is_commissioner: f.is_commissioner
	});
export const reissueInvite = (franchiseId: string) =>
	request<{ invite_token: string }>('POST', `/admin/franchises/${franchiseId}/invite`);

export const addPlayers = (players: NewPlayer[]) => request<{ added: number }>('POST', '/admin/players', players);
export const getMergeSuggestions = () => request<MergeSuggestion[]>('GET', '/admin/players/merge-suggestions');
export const mergePlayers = (keep_id: string, duplicate_id: string) =>
	request<void>('POST', '/admin/players/merge', { keep_id, duplicate_id });

export type SyncJob = 'injuries' | 'rosters' | 'prospects' | 'games' | 'schedule' | 'seasons' | 'prospect-rankings';
export const startSync = (competition: string, job: SyncJob) =>
	request<void>('POST', `/admin/sync/${competition}/${job}`);
export const getIngestRuns = () => request<IngestRun[]>('GET', '/admin/ingest-runs');

export const getDrafts = () => orNull(request<DraftSummary[]>('GET', '/drafts')).then((d) => d ?? []);
export const getDraft = (id: string) => request<DraftState>('GET', `/drafts/${id}`);
export const makePick = (id: string, pick: { player_id: string; pick_id?: string }) =>
	request<void>('POST', `/drafts/${id}/pick`, pick);
export const passPick = (id: string) => request<void>('POST', `/drafts/${id}/pass`);
/** Adds the signed-in franchise's emoji to a made pick, or takes it back if it is there. */
export const reactToPick = (id: string, pickId: string, emoji: string) =>
	request<void>('POST', `/drafts/${id}/picks/${pickId}/react`, { emoji });
export const getQueue = (id: string) => request<QueuedPlayer[]>('GET', `/drafts/${id}/queue`);
export const setQueue = (id: string, player_ids: string[]) => request<void>('PUT', `/drafts/${id}/queue`, { player_ids });
/** franchise_id lets a commissioner set it for another franchise; without it, it is the signed-in one. */
export const setAutoPick = (id: string, on: boolean, franchise_id?: string) =>
	request<void>('PUT', `/drafts/${id}/autopick`, { on, franchise_id });

export const getRankings = () => request<RankingSummary[]>('GET', '/rankings');
export const getRanking = (id: string) => request<Ranking>('GET', `/rankings/${id}`);
export const createRanking = (ranking: RankingChange) => request<Ranking>('POST', '/rankings', ranking);
export const updateRanking = (id: string, ranking: RankingChange) => request<Ranking>('PUT', `/rankings/${id}`, ranking);
export const deleteRanking = (id: string) => request<void>('DELETE', `/rankings/${id}`);
export const importRanking = (draftId: string, ranking_id: string) =>
	request<{ added: number }>('POST', `/drafts/${draftId}/queue/import`, { ranking_id });

export const createDraft = (draft: NewDraft) => request<Draft>('POST', '/admin/drafts', draft);
export const setDraftPicks = (id: string, slots: PickSlot[]) => request<void>('PUT', `/admin/drafts/${id}/picks`, slots);
/** Replaces the player taken with a pick already made. */
export const overridePick = (id: string, pickId: string, player_id: string) =>
	request<void>('PUT', `/admin/drafts/${id}/picks/${pickId}`, { player_id });
export const setDraftClock = (id: string, pick_clock_seconds: number) =>
	request<void>('PUT', `/admin/drafts/${id}`, { pick_clock_seconds });
export const deleteDraft = (id: string) => request<void>('DELETE', `/admin/drafts/${id}`);
export const controlDraft = (id: string, action: DraftAction) => request<void>('POST', `/admin/drafts/${id}/${action}`);
export const createFutureDrafts = () => request<{ created: number }>('POST', '/admin/drafts/future');

export const getTrades = () => orNull(request<Trade[]>('GET', '/trades')).then((t) => t ?? []);
export const proposeTrade = (note: string, items: OfferItem[], drops: TradeDrop[] = []) => request<Trade>('POST', '/trades', { note, items, drops });
export const answerTrade = (id: string, answer: TradeAnswer, drops?: TradeDrop[]) => request<void>('POST', `/trades/${id}/${answer}`, drops ? { drops } : undefined);
export const ruleOnTrade = (id: string, ruling: TradeRuling) => request<void>('POST', `/admin/trades/${id}/${ruling}`);

export const getStandings = () => request<Standings[]>('GET', '/standings');
export const getOverall = () => request<OverallYear[]>('GET', '/overall');
export const getSeasons = () => request<Season[]>('GET', '/seasons');
export const createSeason = (season: { league_id: string; year: number; starts_on: string; ends_on: string }) =>
	request<Season>('POST', '/admin/seasons', season);
export const setSeasonDates = (id: string, starts_on: string, ends_on: string, reset_schedule = false) =>
	request<void>('PUT', `/admin/seasons/${id}`, { starts_on, ends_on, reset_schedule });
/** Sets, or with null clears, the time by which every season roster must be set. */
export const setRosterDeadline = (id: string, deadline: string | null) =>
	request<void>('PUT', `/admin/seasons/${id}/roster-deadline`, { deadline });
export const closeSeason = (id: string, champion_franchise_id?: string) =>
	request<void>('POST', `/admin/seasons/${id}/close`, { champion_franchise_id });
export const reopenSeason = (id: string) => request<void>('POST', `/admin/seasons/${id}/reopen`);

export const getLineup = (leagueId: string, franchiseId: string, day?: string) =>
	request<Lineup>('GET', `/leagues/${leagueId}/lineup${query({ franchise: franchiseId, day })}`);
export const setLineup = (leagueId: string, change: LineupChange) => request<void>('PUT', `/leagues/${leagueId}/lineup`, change);
/** Sets the best lineup for every day left in the week of day, or for that week in a weekly league. */
export const autoSetLineup = (leagueId: string, change: { day: string; franchise_id?: string }) =>
	request<void>('POST', `/leagues/${leagueId}/lineup/auto`, change);

export const getScores = (competition: string, day?: string) =>
	request<Scoreboard>('GET', `/scores/${competition}${query({ day })}`);
export const getGame = (id: string) => request<GameDetail>('GET', `/games/${id}`);

export const getMatchups = (leagueId: string, period?: number) =>
	request<Matchups>('GET', `/leagues/${leagueId}/matchups${query({ period })}`);
export const getSchedule = (leagueId: string) => request<SchedulePeriod[]>('GET', `/leagues/${leagueId}/schedule`);
export const getGameCounts = (leagueId: string, starts_on: string, ends_on: string) =>
	request<GameDayCount[]>('GET', `/leagues/${leagueId}/game-counts${query({ starts_on, ends_on })}`);
export const getMatchup = (id: string) => request<MatchupDetail>('GET', `/matchups/${id}`);
export const setMatchups = (periodId: string, matchups: MatchupPairing[]) =>
	request<void>('PUT', `/admin/periods/${periodId}/matchups`, { matchups });
export const resetMatchups = (periodId: string) => request<void>('DELETE', `/admin/periods/${periodId}/matchups`);
export const setSchedule = (seasonId: string, periods: CustomPeriod[]) => request<void>('PUT', `/admin/seasons/${seasonId}/schedule`, { periods });
export const generateSchedule = (seasonId: string) => request<void>('POST', `/admin/seasons/${seasonId}/schedule`);
export const advancePlayoffs = () => request<void>('POST', '/admin/playoffs/advance');

export const getReviewQueue = () => request<ReviewItem[]>('GET', '/admin/review-queue');
export const carryOver = (leagueId: string, player_id: string) =>
	request<void>('POST', `/admin/leagues/${leagueId}/carry-over`, { player_id });

export const scheduleDraft = (id: string) => request<void>('POST', `/admin/drafts/${id}/schedule`);

// Private, account-backed player watchlist.
export type WatchlistEntry = {
 player_id: string; full_name: string; competition: string; positions: string[];
 headshot_url: string; team_name: string; status: string; owner_name: string; owner_slug: string;
 category: string; notes: string;
};
export const getWatchlist = () => request<WatchlistEntry[]>('GET', '/watchlist');
export const addWatchlistPlayer = (id: string) => request<void>('PUT', `/watchlist/players/${id}`);
export const removeWatchlistPlayer = (id: string) => request<void>('DELETE', `/watchlist/players/${id}`);
export const editWatchlistPlayer = (id: string, category: string, notes: string) => request<void>('PATCH', `/watchlist/players/${id}`, { category, notes });
export const orderWatchlist = (player_ids: string[]) => request<void>('PUT', '/watchlist/order', { player_ids });

export const getRosterPerformance = (leagueId: string, franchiseId: string, window: string, season?: string) =>
 request<RosterPerformance>('GET', `/leagues/${leagueId}/roster-stats${query({franchise:franchiseId,window,season})}`);
