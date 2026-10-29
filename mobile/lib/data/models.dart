// Converts public Go API JSON into the types needed by the first mobile screens.

typedef Json = Map<String, dynamic>;

Json asJson(Object? value) => value as Json;
List<T> readList<T>(Object? value, T Function(Json) convert) =>
    (value as List<dynamic>? ?? [])
        .map((item) => convert(asJson(item)))
        .toList();

class Dynasty {
  const Dynasty({
    this.id = '',
    required this.name,
    required this.leagues,
    required this.franchises,
    this.teamIdentities = const [],
    this.settings = const {},
  });
  final String id;
  final String name;
  final List<League> leagues;
  final List<Franchise> franchises;
  final List<TeamIdentity> teamIdentities;
  final Json settings;

  factory Dynasty.fromJson(Json json) => Dynasty(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    leagues: readList(json['leagues'], League.fromJson),
    franchises: readList(json['franchises'], Franchise.fromJson),
    teamIdentities: readList(json['team_identities'], TeamIdentity.fromJson),
    settings: json['settings'] is Json ? asJson(json['settings']) : {},
  );
}

class TeamIdentity {
  const TeamIdentity({
    required this.franchiseId,
    required this.leagueId,
    required this.name,
    this.imageUrl = '',
  });
  final String franchiseId;
  final String leagueId;
  final String name;
  final String imageUrl;

  factory TeamIdentity.fromJson(Json json) => TeamIdentity(
    franchiseId: json['franchise_id'] as String? ?? '',
    leagueId: json['league_id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    imageUrl: json['image_url'] as String? ?? '',
  );
}

class MyLeague {
  const MyLeague({
    required this.id,
    required this.name,
    required this.franchiseId,
    required this.franchiseName,
    required this.sports,
    required this.ready,
  });
  final String id;
  final String name;
  final String franchiseId;
  final String franchiseName;
  final List<String> sports;
  final bool ready;

  factory MyLeague.fromJson(Json json) => MyLeague(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    franchiseId: json['franchise_id'] as String? ?? '',
    franchiseName: json['franchise_name'] as String? ?? '',
    sports: (json['sports'] as List<dynamic>? ?? []).cast<String>(),
    ready: json['ready'] as bool? ?? false,
  );
}

class StandingsTable {
  const StandingsTable({
    required this.leagueId,
    required this.competition,
    required this.format,
    required this.season,
    required this.rows,
  });
  final String leagueId;
  final String competition;
  final String format;
  final Json? season;
  final List<StandingsRow> rows;

  factory StandingsTable.fromJson(Json json) => StandingsTable(
    leagueId: json['league_id'] as String? ?? '',
    competition: json['competition'] as String? ?? '',
    format: json['format'] as String? ?? '',
    season: json['season'] is Json ? asJson(json['season']) : null,
    rows: readList(json['rows'], StandingsRow.fromJson),
  );
}

class StandingsRow {
  const StandingsRow({
    required this.franchiseId,
    required this.points,
    required this.pointsFor,
    required this.pointsAgainst,
    required this.wins,
    required this.losses,
    required this.ties,
  });
  final String franchiseId;
  final double points;
  final double pointsFor;
  final double pointsAgainst;
  final int wins;
  final int losses;
  final int ties;

  factory StandingsRow.fromJson(Json json) => StandingsRow(
    franchiseId: json['franchise_id'] as String? ?? '',
    points: (json['points'] as num? ?? 0).toDouble(),
    pointsFor: (json['points_for'] as num? ?? 0).toDouble(),
    pointsAgainst: (json['points_against'] as num? ?? 0).toDouble(),
    wins: (json['wins'] as num? ?? 0).toInt(),
    losses: (json['losses'] as num? ?? 0).toInt(),
    ties: (json['ties'] as num? ?? 0).toInt(),
  );
}

class OverallYear {
  const OverallYear({
    required this.year,
    required this.finalYear,
    required this.rows,
  });
  final int year;
  final bool finalYear;
  final List<OverallRow> rows;

  factory OverallYear.fromJson(Json json) => OverallYear(
    year: (json['year'] as num? ?? 0).toInt(),
    finalYear: json['final'] as bool? ?? false,
    rows: readList(json['rows'], OverallRow.fromJson),
  );
}

class OverallRow {
  const OverallRow({
    required this.franchiseId,
    required this.points,
    required this.adjustedPoints,
  });
  final String franchiseId;
  final double points;
  final double adjustedPoints;

  factory OverallRow.fromJson(Json json) => OverallRow(
    franchiseId: json['franchise_id'] as String? ?? '',
    points: (json['points'] as num? ?? 0).toDouble(),
    adjustedPoints: (json['adjusted_points'] as num? ?? 0).toDouble(),
  );
}

class LeagueOdds {
  const LeagueOdds({required this.leagues, required this.overall});
  final List<SportOdds> leagues;
  final List<TitleOdds> overall;

  factory LeagueOdds.fromJson(Json json) => LeagueOdds(
    leagues: readList(json['leagues'], SportOdds.fromJson),
    overall: readList(json['overall'], TitleOdds.fromJson),
  );
}

class SportOdds {
  const SportOdds({required this.competition, required this.rows});
  final String competition;
  final List<PlayoffOdds> rows;

  factory SportOdds.fromJson(Json json) => SportOdds(
    competition: json['competition'] as String? ?? '',
    rows: readList(json['rows'], PlayoffOdds.fromJson),
  );
}

class PlayoffOdds {
  const PlayoffOdds({required this.franchiseId, required this.playoffs});
  final String franchiseId;
  final double playoffs;

  factory PlayoffOdds.fromJson(Json json) => PlayoffOdds(
    franchiseId: json['franchise_id'] as String? ?? '',
    playoffs: (json['playoffs'] as num? ?? 0).toDouble(),
  );
}

class TitleOdds {
  const TitleOdds({required this.franchiseId, required this.title});
  final String franchiseId;
  final double title;

  factory TitleOdds.fromJson(Json json) => TitleOdds(
    franchiseId: json['franchise_id'] as String? ?? '',
    title: (json['title'] as num? ?? 0).toDouble(),
  );
}

class ActivityItem {
  const ActivityItem({
    required this.id,
    required this.kind,
    required this.competition,
    required this.franchiseName,
    required this.playerName,
    required this.createdAt,
    required this.detail,
  });
  final int id;
  final String kind;
  final String competition;
  final String franchiseName;
  final String playerName;
  final DateTime? createdAt;
  final Json detail;

  factory ActivityItem.fromJson(Json json) => ActivityItem(
    id: (json['id'] as num? ?? 0).toInt(),
    kind: json['kind'] as String? ?? '',
    competition: json['competition'] as String? ?? '',
    franchiseName: json['franchise_name'] as String? ?? '',
    playerName: json['player_name'] as String? ?? '',
    createdAt: DateTime.tryParse(json['created_at'] as String? ?? ''),
    detail: json['detail'] is Json ? asJson(json['detail']) : {},
  );
}

class League {
  const League({
    required this.name,
    required this.competition,
    this.id = '',
    this.settings = const {},
  });
  final String id;
  final String name;
  final String competition;
  final Json settings;

  factory League.fromJson(Json json) => League(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    competition: json['competition'] as String? ?? '',
    settings: json['settings'] is Json ? asJson(json['settings']) : {},
  );
}

class Franchise {
  const Franchise({
    required this.name,
    required this.managerName,
    required this.slug,
    this.id = '',
    this.isCommissioner = false,
    this.imageUrl = '',
  });
  final String id;
  final String name;
  final String managerName;
  final String slug;
  final bool isCommissioner;
  final String imageUrl;

  factory Franchise.fromJson(Json json) => Franchise(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    managerName: json['manager_name'] as String? ?? '',
    slug: json['slug'] as String? ?? '',
    isCommissioner: json['is_commissioner'] as bool? ?? false,
    imageUrl: json['image_url'] as String? ?? '',
  );
}

class FranchiseDetail {
  const FranchiseDetail({required this.franchise, required this.rosters});
  final Franchise franchise;
  final List<LeagueRoster> rosters;

  factory FranchiseDetail.fromJson(Json json) => FranchiseDetail(
    franchise: Franchise.fromJson(asJson(json['franchise'])),
    rosters: readList(json['rosters'], LeagueRoster.fromJson),
  );
}

class LeagueRoster {
  const LeagueRoster({
    required this.name,
    required this.players,
    this.leagueId = '',
    this.competition = '',
  });
  final String leagueId;
  final String competition;
  final String name;
  final List<RosterPlayer> players;

  factory LeagueRoster.fromJson(Json json) => LeagueRoster(
    leagueId: json['league_id'] as String? ?? '',
    competition: json['competition'] as String? ?? '',
    name: (json['team_name'] as String?)?.isNotEmpty == true
        ? json['team_name'] as String
        : json['name'] as String? ?? '',
    players: readList(json['players'], RosterPlayer.fromJson),
  );
}

class SessionInfo {
  const SessionInfo({
    required this.franchise,
    required this.email,
    this.verified = true,
  });
  final Franchise? franchise;
  final String email;
  final bool verified;

  factory SessionInfo.fromJson(Json json) => SessionInfo(
    franchise: json['id'] == null ? null : Franchise.fromJson(json),
    email: json['email'] as String? ?? '',
    verified: json['verified'] as bool? ?? true,
  );
}

class MobileAuth {
  const MobileAuth({
    required this.token,
    required this.expiresAt,
    required this.session,
  });
  final String token;
  final DateTime expiresAt;
  final SessionInfo session;

  factory MobileAuth.fromJson(Json json) => MobileAuth(
    token: json['token'] as String,
    expiresAt: DateTime.parse(json['expires_at'] as String),
    session: SessionInfo.fromJson(asJson(json['session'])),
  );
}

class RosterPlayer {
  const RosterPlayer({
    required this.name,
    required this.list,
    required this.positions,
  });
  final String name;
  final String list;
  final List<String> positions;

  factory RosterPlayer.fromJson(Json json) => RosterPlayer(
    name: json['full_name'] as String? ?? '',
    list: json['list'] as String? ?? '',
    positions: (json['positions'] as List<dynamic>? ?? []).cast<String>(),
  );
}

class PlayerPage {
  const PlayerPage({
    required this.players,
    required this.total,
    required this.page,
    required this.perPage,
  });
  final List<Player> players;
  final int total;
  final int page;
  final int perPage;

  factory PlayerPage.fromJson(Json json) => PlayerPage(
    players: readList(json['players'], Player.fromJson),
    total: json['total'] as int? ?? 0,
    page: json['page'] as int? ?? 1,
    perPage: json['per_page'] as int? ?? 50,
  );
}

class Player {
  const Player({
    required this.name,
    required this.competition,
    required this.positions,
    required this.team,
    required this.owner,
  });
  final String name;
  final String competition;
  final List<String> positions;
  final String team;
  final String owner;

  factory Player.fromJson(Json json) => Player(
    name: json['full_name'] as String? ?? '',
    competition: json['competition'] as String? ?? '',
    positions: (json['positions'] as List<dynamic>? ?? []).cast<String>(),
    team: json['team_abbrev'] as String? ?? '',
    owner: json['owner_name'] as String? ?? '',
  );
}

class Scoreboard {
  const Scoreboard({required this.day, required this.games});
  final String day;
  final List<GameSummary> games;

  factory Scoreboard.fromJson(Json json) => Scoreboard(
    day: json['day'] as String? ?? '',
    games: readList(json['games'], GameSummary.fromJson),
  );
}

class GameSummary {
  const GameSummary({
    required this.home,
    required this.away,
    required this.homeScore,
    required this.awayScore,
    required this.status,
    required this.detail,
  });
  final String home;
  final String away;
  final int homeScore;
  final int awayScore;
  final String status;
  final String detail;

  factory GameSummary.fromJson(Json json) => GameSummary(
    home: json['home_abbrev'] as String? ?? json['home_name'] as String? ?? '',
    away: json['away_abbrev'] as String? ?? json['away_name'] as String? ?? '',
    homeScore: json['home_score'] as int? ?? 0,
    awayScore: json['away_score'] as int? ?? 0,
    status: json['status'] as String? ?? '',
    detail: json['detail'] as String? ?? '',
  );
}
