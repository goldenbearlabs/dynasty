// The one place the frontend talks to the Go API.

// ---- settings: mirrors backend/internal/settings ----

/** games_per_week: how many of a player's games count each week in this slot; 0 for all of them. */
export type Slot = { name: string; positions: string[]; count: number; games_per_week: number };

export type LeagueSettings = {
	roster: {
		main: number;
		reserve: number;
		reserve_eligibility: 'prospects_only' | 'anyone';
		/** Days a non-prospect sent to reserve must stay there; 0 for none. */
		reserve_lock_days: number;
	};
	lineup: { period: 'day' | 'week'; week_start: string; lock: 'game_start' | 'period_start'; slots: Slot[] };
	scoring: Record<string, number>;
	format: { type: 'total_points' | 'head_to_head'; matchup_days: number; playoff_teams: number };
	/** weekly_limit: acquisitions a franchise may make in a week; 0 for no limit. */
	free_agency: { mode: 'open' | 'closed'; new_entrants_draft_only: boolean; weekly_limit: number };
	/** What happens to a dropped player: days on waivers, and how claims are ordered. */
	waivers: { mode: 'none' | 'rolling' | 'faab'; days: number; budget: number };
	draft: { rounds: number; order: 'linear' | 'snake'; pick_clock_seconds: number; future_years: number };
	trades: { deadline: string; approval: 'none' | 'commissioner' };
	continuity: { into: string; land_on: List } | null;
};

export type DynastySettings = {
	overall_title: { enabled: boolean; points_by_finish: number[] };
};

// ---- data ----

export type List = 'main' | 'reserve';

export type Competition = {
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

export type Dynasty = {
	id: string;
	name: string;
	settings: DynastySettings;
	leagues: League[];
	franchises: Franchise[];
};

export type Player = {
	id: string;
	competition: string;
	status: 'prospect' | 'active' | 'inactive';
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
};

export type PlayerPage = { players: Player[]; total: number; page: number; per_page: number };

export type PlayerResearch = {
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

export type PlayerFilter = {
	competition?: string;
	status?: string;
	q?: string;
	available_in?: string;
	draft_id?: string;
	page?: number;
};

export type RosterPlayer = {
	player_id: string;
	list: List;
	full_name: string;
	positions: string[];
	status: Player['status'];
	class: string;
	note: string;
	headshot_url: string;
	team_abbrev: string;
	/** Set while a reserve lock keeps him off the main roster. */
	locked_until: string | null;
};

export type LeagueRoster = {
	league_id: string;
	competition: string;
	name: string;
	limits: LeagueSettings['roster'];
	overage: number;
	players: RosterPlayer[];
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
};

export type StandingsRow = {
	franchise_id: string;
	points: number;
	/** A head-to-head league's record over its finished regular-season matchups. */
	wins: number;
	losses: number;
	ties: number;
	players: { player_id: string; full_name: string; headshot_url: string; games: number; points: number }[];
};

/** One league's table for its latest season, best first. */
export type Standings = {
	league_id: string;
	competition: string;
	format: LeagueSettings['format']['type'];
	season: Season | null;
	rows: StandingsRow[];
};

// ---- head-to-head ----

export type Period = { id: string; seq: number; starts_on: string; ends_on: string; is_playoff: boolean };

export type Matchup = {
	id: string;
	home_franchise_id: string;
	away_franchise_id: string | null; // null for a bye
	home_points: number;
	away_points: number;
	final: boolean;
};

/** One period of a head-to-head season. */
export type Matchups = { periods: Period[]; period: Period | null; matchups: Matchup[] };

export type MatchupDetail = Matchup & {
	competition: string;
	period: Period;
	home_players: StandingsRow['players'];
	away_players: StandingsRow['players'];
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
	day: string;
	starts_at: string;
	status: string;
	opponent: string;
	at_home: boolean;
	points: number;
	/** False for a game left out by a slot that counts only some games a week. */
	counts: boolean;
};

export type LineupPlayer = {
	player_id: string;
	full_name: string;
	positions: string[];
	headshot_url: string;
	team_abbrev: string;
	slot: string; // empty on the bench
	/** The day his counted games begin, in a slot that counts only some; empty for his first game. */
	counts_from: string;
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
};

export type LineupChange = {
	day: string;
	entries: { slot: string; player_id: string; counts_from?: string }[];
	franchise_id?: string;
	force?: boolean;
};

// ---- trades ----

export type TradeStatus = 'proposed' | 'accepted' | 'executed' | 'rejected' | 'cancelled' | 'reversed';

/** One asset changing hands, as shown in a trade. */
export type TradeItem = {
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

export type TradeAnswer = 'accept' | 'reject' | 'cancel';
export type TradeRuling = 'approve' | 'veto' | 'reverse';

// ---- drafts ----

export type DraftStatus = 'scheduled' | 'live' | 'paused' | 'complete';

export type Draft = {
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
	player_name: string;
	player_positions: string[];
	player_headshot: string;
	competition: string;
};

export type DraftState = {
	draft: Draft;
	league_ids: string[];
	picks: DraftPick[];
	on_clock_pick_id: string | null;
};

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
		note: string;
		headshot_url: string;
		team_abbrev: string;
	}[];
	standings: { franchise_id: string; name: string; slug: string; priority: number; budget_left: number }[];
	claims: {
		id: string;
		player_id: string;
		bid: number;
		status: 'pending' | 'won' | 'lost';
		reason: string;
		player_name: string;
		drop_player_name: string;
		created_at: string;
		resolved_at: string | null;
	}[];
};
export type WaiverClaim = { player_id: string; drop_player_id?: string; bid: number };

export type RosterChange = { player_id: string; list?: List; franchise_id?: string; force?: boolean };

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
		if (value !== undefined && value !== '') search.set(key, String(value));
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
export const getPlayerResearch = (id: string) => request<PlayerResearch>('GET', `/players/${id}`);
export const getPlayerSeasons = (id: string) => request<PlayerSeason[]>('GET', `/players/${id}/seasons`);

export const getDynasty = () => orNull(request<Dynasty>('GET', '/dynasty'));
export const createDynasty = (setup: Setup) => request<void>('POST', '/dynasty', setup);
export const updateDynasty = (name: string, settings: DynastySettings) =>
	request<void>('PUT', '/dynasty', { name, settings });
export const updateLeagueSettings = (leagueId: string, settings: LeagueSettings) =>
	request<void>('PUT', `/leagues/${leagueId}/settings`, settings);

export const addLeague = (competition: string) => request<League>('POST', '/admin/leagues', { competition });

export const getSession = () => request<Session | null>('GET', '/session');
export const logIn = (credentials: Credentials) => request<void>('POST', '/auth/login', credentials);
export const logOut = () => request<void>('POST', '/auth/logout');
export const getInvite = (token: string) => request<{ franchise: Franchise; reset: boolean }>('GET', `/invites/${token}`);
export const claimInvite = (token: string, credentials: Credentials) =>
	request<void>('POST', `/invites/${token}/claim`, credentials);

export const getFranchise = (slug: string) => request<FranchiseDetail>('GET', `/franchises/${slug}`);
export const changeRoster = (leagueId: string, action: 'add' | 'drop' | 'move', change: RosterChange) =>
	request<void>('POST', `/leagues/${leagueId}/roster/${action}`, change);
export const getWaivers = (leagueId: string) => request<Waivers>('GET', `/leagues/${leagueId}/waivers`);
export const claimWaiver = (leagueId: string, claim: WaiverClaim) =>
	request<void>('POST', `/leagues/${leagueId}/waivers/claims`, claim);
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

export type SyncJob = 'rosters' | 'prospects' | 'games' | 'seasons';
export const startSync = (competition: string, job: SyncJob) =>
	request<void>('POST', `/admin/sync/${competition}/${job}`);
export const getIngestRuns = () => request<IngestRun[]>('GET', '/admin/ingest-runs');

export const getDrafts = () => orNull(request<DraftSummary[]>('GET', '/drafts')).then((d) => d ?? []);
export const getDraft = (id: string) => request<DraftState>('GET', `/drafts/${id}`);
export const makePick = (id: string, pick: { player_id: string; list: List; pick_id?: string }) =>
	request<void>('POST', `/drafts/${id}/pick`, pick);
export const getQueue = (id: string) => request<QueuedPlayer[]>('GET', `/drafts/${id}/queue`);
export const setQueue = (id: string, player_ids: string[]) => request<void>('PUT', `/drafts/${id}/queue`, { player_ids });

export const createDraft = (draft: NewDraft) => request<Draft>('POST', '/admin/drafts', draft);
export const setDraftPicks = (id: string, slots: PickSlot[]) => request<void>('PUT', `/admin/drafts/${id}/picks`, slots);
export const setDraftClock = (id: string, pick_clock_seconds: number) =>
	request<void>('PUT', `/admin/drafts/${id}`, { pick_clock_seconds });
export const deleteDraft = (id: string) => request<void>('DELETE', `/admin/drafts/${id}`);
export const controlDraft = (id: string, action: DraftAction) => request<void>('POST', `/admin/drafts/${id}/${action}`);
export const createFutureDrafts = () => request<{ created: number }>('POST', '/admin/drafts/future');

export const getTrades = () => orNull(request<Trade[]>('GET', '/trades')).then((t) => t ?? []);
export const proposeTrade = (note: string, items: OfferItem[]) => request<Trade>('POST', '/trades', { note, items });
export const answerTrade = (id: string, answer: TradeAnswer) => request<void>('POST', `/trades/${id}/${answer}`);
export const ruleOnTrade = (id: string, ruling: TradeRuling) => request<void>('POST', `/admin/trades/${id}/${ruling}`);

export const getStandings = () => request<Standings[]>('GET', '/standings');
export const getOverall = () => request<OverallYear[]>('GET', '/overall');
export const getSeasons = () => request<Season[]>('GET', '/seasons');
export const createSeason = (season: { league_id: string; year: number; starts_on: string; ends_on: string }) =>
	request<Season>('POST', '/admin/seasons', season);
export const setSeasonDates = (id: string, starts_on: string, ends_on: string) =>
	request<void>('PUT', `/admin/seasons/${id}`, { starts_on, ends_on });
export const closeSeason = (id: string, champion_franchise_id?: string) =>
	request<void>('POST', `/admin/seasons/${id}/close`, { champion_franchise_id });
export const reopenSeason = (id: string) => request<void>('POST', `/admin/seasons/${id}/reopen`);

export const getLineup = (leagueId: string, franchiseId: string, day?: string) =>
	request<Lineup>('GET', `/leagues/${leagueId}/lineup${query({ franchise: franchiseId, day })}`);
export const setLineup = (leagueId: string, change: LineupChange) => request<void>('PUT', `/leagues/${leagueId}/lineup`, change);

export const getScores = (competition: string, day?: string) =>
	request<Scoreboard>('GET', `/scores/${competition}${query({ day })}`);
export const getGame = (id: string) => request<GameDetail>('GET', `/games/${id}`);

export const getMatchups = (leagueId: string, period?: number) =>
	request<Matchups>('GET', `/leagues/${leagueId}/matchups${query({ period })}`);
export const getMatchup = (id: string) => request<MatchupDetail>('GET', `/matchups/${id}`);
export const generateSchedule = (seasonId: string) => request<void>('POST', `/admin/seasons/${seasonId}/schedule`);
export const advancePlayoffs = () => request<void>('POST', '/admin/playoffs/advance');

export const getReviewQueue = () => request<ReviewItem[]>('GET', '/admin/review-queue');
export const carryOver = (leagueId: string, player_id: string) =>
	request<void>('POST', `/admin/leagues/${leagueId}/carry-over`, { player_id });
