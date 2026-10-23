// Converts public Go API JSON into the types needed by the first mobile screens.

typedef Json = Map<String, dynamic>;

Json asJson(Object? value) => value as Json;
List<T> readList<T>(Object? value, T Function(Json) convert) =>
    (value as List<dynamic>? ?? [])
        .map((item) => convert(asJson(item)))
        .toList();

class Dynasty {
  const Dynasty({
    required this.name,
    required this.leagues,
    required this.franchises,
    this.settings = const {},
  });
  final String name;
  final List<League> leagues;
  final List<Franchise> franchises;
  final Json settings;

  factory Dynasty.fromJson(Json json) => Dynasty(
    name: json['name'] as String? ?? '',
    leagues: readList(json['leagues'], League.fromJson),
    franchises: readList(json['franchises'], Franchise.fromJson),
    settings: json['settings'] is Json ? asJson(json['settings']) : {},
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
  });
  final String id;
  final String name;
  final String managerName;
  final String slug;

  factory Franchise.fromJson(Json json) => Franchise(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    managerName: json['manager_name'] as String? ?? '',
    slug: json['slug'] as String? ?? '',
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
