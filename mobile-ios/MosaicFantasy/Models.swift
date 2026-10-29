// Defines the account, league, standings, odds, and activity data returned by the Go API.
import Foundation

struct SessionInfo: Decodable {
    let id: String?
    let name: String?
    let managerName: String?
    let imageUrl: String?
    let email: String
    let verified: Bool

    var displayName: String {
        let manager = managerName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return manager.isEmpty ? email : manager
    }

    var initials: String {
        let parts = displayName.split(separator: " ")
        return String(parts.prefix(2).compactMap(\.first)).uppercased()
    }
}

struct MobileAuth: Decodable {
    let token: String
    let expiresAt: String
    let session: SessionInfo
}

struct MyLeague: Decodable, Identifiable {
    let id: String
    let name: String
    let franchiseId: String
    let franchiseName: String
    let sports: [String]
    let ready: Bool

    static func playable(in memberships: [MyLeague]) -> [MyLeague] {
        memberships.filter(\.ready)
    }
}

struct Dynasty: Decodable {
    let id: String
    let name: String
    let settings: DynastySettings?
    let leagues: [SportLeague]
    let franchises: [DynastyFranchise]
    let teamIdentities: [TeamIdentity]

    enum CodingKeys: String, CodingKey { case id, name, settings, leagues, franchises, teamIdentities }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        id = try values.decode(String.self, forKey: .id)
        name = try values.decode(String.self, forKey: .name)
        settings = try values.decodeIfPresent(DynastySettings.self, forKey: .settings)
        leagues = try values.decodeIfPresent([SportLeague].self, forKey: .leagues) ?? []
        franchises = try values.decodeIfPresent([DynastyFranchise].self, forKey: .franchises) ?? []
        teamIdentities = try values.decodeIfPresent([TeamIdentity].self, forKey: .teamIdentities) ?? []
    }
}

struct DynastySettings: Decodable {
    let overallTitle: OverallTitleSettings?
}

struct OverallTitleSettings: Decodable {
    let enabled: Bool?
    let pointsByFinish: [Int]?
}

struct SportLeague: Decodable, Identifiable {
    let id: String
    let name: String
    let competition: String
    let settings: SportLeagueSettings?
}

struct SportLeagueSettings: Decodable {
    let format: SportFormatSettings?
    let lineup: LineupSettings?
    let waivers: WaiverSettings?
    let scoring: [String: Double]?
}

struct WaiverSettings: Decodable {
    let mode: String?
    let hours: Int?
    let budget: Int?
}

struct WaiverState: Decodable {
    let rules: WaiverSettings?
    let weeklyLimit: Int?
    let acquisitions: Int?
    let standings: [WaiverStanding]
}

struct WaiverStanding: Decodable {
    let franchiseId: String
    let priority: Int?
    let budgetLeft: Int?
}

struct LineupSettings: Decodable {
    let period: String
    let weekStart: String?
    let lock: String?
    let slots: [LineupSlot]
    let pitcherStartsPerWeek: Int?
}

struct LineupSlot: Decodable, Identifiable {
    let name: String
    let positions: [String]
    let count: Int
    let gamesPerWeek: Int
    var id: String { name }
}

struct SportFormatSettings: Decodable {
    let type: String?
    let matchupDays: Int?
    let playoffTeams: Int?
    let standingsBy: String?
    let tiebreakers: [String]?
}

struct DynastyFranchise: Decodable, Identifiable {
    let id: String
    let name: String
    let imageUrl: String?
    let slug: String?
    let managerName: String?
}

struct TeamIdentity: Decodable {
    let franchiseId: String
    let leagueId: String
    let name: String
    let imageUrl: String?
}

struct SelectedLeague: Identifiable, Hashable {
    let membership: MyLeague
    let dynasty: Dynasty
    var id: String { membership.id }

    static func == (lhs: SelectedLeague, rhs: SelectedLeague) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}

struct StandingsTable: Decodable {
    let consolationChampionId: String?
    let leagueId: String
    let competition: String
    let format: String
    let season: SeasonInfo?
    let rows: [StandingsRow]
}

struct SeasonInfo: Decodable {
    let year: Int?
    let status: String?
    let startsOn: String?
    let endsOn: String?
    let championFranchiseId: String?
}

struct StandingsRow: Decodable, Identifiable {
    let franchiseId: String
    let points: Double?
    let pointsFor: Double?
    let pointsAgainst: Double?
    let wins: Int?
    let losses: Int?
    let ties: Int?
    var id: String { franchiseId }
}

struct OverallYear: Decodable {
    let year: Int
    let finalYear: Bool
    let rows: [OverallRow]
    enum CodingKeys: String, CodingKey { case year, rows, finalYear = "final" }
}

struct OverallRow: Decodable, Identifiable {
    let franchiseId: String
    let points: Double
    let adjustedPoints: Double
    let finishes: [String: Int]?
    let wins: Int?
    let losses: Int?
    let ties: Int?
    var id: String { franchiseId }
}

struct LeagueOdds: Decodable {
    let leagues: [SportOdds]
    let overall: [TitleOdds]
}

struct SportOdds: Decodable {
    let leagueId: String?
    let competition: String
    let rows: [PlayoffOdds]
}

struct PlayoffOdds: Decodable {
    let franchiseId: String
    let playoffs: Double
    let clinched: Bool?
    let eliminated: Bool?
    let wins: Double?
    let losses: Double?
}

struct TitleOdds: Decodable {
    let franchiseId: String
    let title: Double
}

struct ActivityItem: Decodable, Identifiable {
    let id: Int
    let kind: String
    let competition: String
    let franchiseName: String
    let playerName: String
    let createdAt: String?
}

struct DashboardData {
    let standings: [StandingsTable]
    let overall: [OverallYear]
    let activity: [ActivityItem]
    let odds: LeagueOdds?
    let dynasty: Dynasty?

    init(standings: [StandingsTable], overall: [OverallYear], activity: [ActivityItem],
         odds: LeagueOdds?, dynasty: Dynasty? = nil) {
        self.standings = standings
        self.overall = overall
        self.activity = activity
        self.odds = odds
        self.dynasty = dynasty
    }
}

struct FranchiseDetail: Decodable {
    let franchise: DynastyFranchise
    let rosters: [LeagueRoster]
    let picks: [HeldPick]

    enum CodingKeys: String, CodingKey { case franchise, rosters, picks }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        franchise = try values.decode(DynastyFranchise.self, forKey: .franchise)
        rosters = try values.decode([LeagueRoster].self, forKey: .rosters)
        picks = try values.decodeIfPresent([HeldPick].self, forKey: .picks) ?? []
    }
}

struct HeldPick: Decodable, Identifiable {
    let id: String
    let round: Int
    let position: Int
    let originalFranchiseId: String
    let draftId: String
    let draftName: String
    let year: Int
    let draftStatus: String
    let competitions: [String]
}

struct LeagueRoster: Decodable, Identifiable {
    let leagueId: String
    let competition: String
    let name: String
    let teamName: String
    let imageUrl: String?
    let limits: RosterLimits
    let players: [RosterPlayer]
    let season: SeasonRosterState?
    let overage: Int?
    var id: String { leagueId }
}

struct RosterLimits: Decodable {
    let main: Int
    let reserve: Int
    let reserveLockSeason: Bool?
}

struct RosterPlayer: Decodable, Identifiable {
    let playerId: String
    let fullName: String
    let list: String
    let positions: [String]?
    let nickname: String?
    let status: String?
    let injuryDesignation: String?
    let injuryReserveLocked: Bool?
    let note: String?
    let headshotUrl: String?
    let teamAbbrev: String?
    let lockedUntil: String?
    let rightsUntil: String?
    let draftYear: Int?
    let draftRound: Int?
    let draftPick: Int?
    let draftOverall: Int?
    var id: String { playerId }
}

struct SeasonRosterState: Decodable {
    let id: String
    let year: Int
    let startsOn: String
    let setAt: String?
    let started: Bool
    let deadline: String?
    let expired: Bool
    let locked: Bool?

    var needsSetting: Bool { setAt == nil && !expired }
}

struct LineupData: Decodable {
    let day: String
    let lastDay: String
    let slots: [LineupSlot]
    let players: [LineupPlayer]
    let locked: String
    let starts: PitcherStarts?
    let doubled: DoubledMatchup?
}

struct LineupPlayer: Decodable, Identifiable {
    let playerId: String
    let fullName: String
    let nickname: String
    let positions: [String]
    let teamAbbrev: String
    let injury: String
    let slot: String
    let countsFrom: String
    let countsLast: Bool
    let locked: Bool
    let starterEligible: Bool
    let eligibilityNote: String
    let points: Double
    let games: [LineupGame]
    var id: String { playerId }
}

struct LineupGame: Decodable, Identifiable {
    let gameId: String?
    let day: String
    let startsAt: String
    let status: String
    let opponent: String
    let atHome: Bool
    let points: Double
    let counts: Bool
    let starter: String
    let projected: Double
    let expected: Double?
    let stats: [String: Double]?
    var id: String { gameId ?? "\(day)-\(opponent)" }
}

struct PitcherStarts: Decodable {
    let limit: Int
    let made: [PitcherStart]
}

struct PitcherStart: Decodable {
    let playerId: String
    let fullName: String
    let day: String
    let innings: Double
    let counts: Bool
}

struct DoubledMatchup: Decodable {
    let startsOn: String
    let endsOn: String
}

struct LineupEntry: Encodable, Equatable {
    let slot: String
    let playerId: String
    let countsFrom: String
    let auto: Bool
}

struct RosterPerformance: Decodable {
    let currentSeason: String
    let window: String
    let season: String
    let seasons: [PerformanceSeason]
    let players: [PerformancePlayer]
}

struct PerformanceSeason: Decodable, Identifiable {
    let label: String
    let year: Int
    var id: String { label }
}

struct PerformancePlayer: Decodable, Identifiable {
    let playerId: String
    let games: Int
    let stats: [String: Double]
    let points: Double
    let pointsPerGame: Double
    var id: String { playerId }
}

struct PlayerPage: Decodable { let players: [TeamPlayer]; let total: Int; let page: Int; let perPage: Int }

struct TeamPlayer: Decodable, Identifiable {
    let id: String
    let fullName: String
    let competition: String
    let positions: [String]
    let status: String
    let injuryDesignation: String?
    let teamAbbrev: String?
    let ownerName: String?
    let waiverUntil: String?
    let seasonPoints: Double?
}

struct TeamPlayerProfile: Decodable {
    let player: ProfilePlayer
    let eligibilityNote: String?
    let starterEligible: Bool?
    let seasons: [ProfileSeason]?
    let gameLog: ProfileGameLog?
    let ownership: [ProfileOwnership]?
    let waivers: [ProfileWaiver]?
}

struct ProfilePlayer: Decodable {
    let id: String
    let fullName: String
    let competition: String
    let positions: [String]
    let status: String
    let injuryDesignation: String?
    let teamAbbrev: String?
    let note: String?
}

struct ProfileSeason: Decodable, Identifiable {
    let key: String
    let season: String
    let competition: String
    let team: String
    let games: Int
    let points: Double
    let pointsPerGame: Double
    let stats: [String: Double]
    var id: String { key }
}

struct ProfileGameLog: Decodable { let games: [ProfileGame] }

struct ProfileGame: Decodable, Identifiable {
    let id: String
    let day: String
    let awayAbbrev: String
    let homeAbbrev: String
    let points: Double
    let stats: [String: Double]
}

struct ProfileOwnership: Decodable {
    let leagueId: String
    let franchiseName: String
    let list: String
}

struct ProfileWaiver: Decodable {
    let leagueId: String
    let clearsAt: String
}

struct WatchlistEntry: Decodable { let playerId: String }

struct TradeMarket: Decodable {
    let block: [TradeMarketPlayer]
    let targets: [TradeMarketPlayer]
    let interest: [TradeMarketPlayer]
}

struct TradeMarketPlayer: Decodable {
    let leagueId: String
    let playerId: String
    let franchiseId: String?
    let byFranchiseId: String?
}

struct MatchupPeriod: Decodable, Identifiable {
    let id: String
    let seq: Int
    let startsOn: String
    let endsOn: String
    let isPlayoff: Bool
    let byHand: Bool
    let doubleGames: Bool?
}

struct MatchupCard: Decodable, Identifiable {
    let id: String
    let bracket: String
    let bracketSlot: Int
    let homeFranchiseId: String
    let awayFranchiseId: String?
    let homePoints: Double
    let awayPoints: Double
    let final: Bool
}

struct MatchupsData: Decodable {
    let periods: [MatchupPeriod]
    let period: MatchupPeriod?
    let matchups: [MatchupCard]
}

struct SchedulePeriod: Decodable, Identifiable {
    let id: String
    let seq: Int
    let startsOn: String
    let endsOn: String
    let isPlayoff: Bool
    let matchups: [MatchupCard]
}

struct MatchupScorer: Decodable, Identifiable {
    let playerId: String
    let fullName: String
    let nickname: String?
    let points: Double
    let headshotUrl: String?
    let gameLines: [ScoredMatchupGame]?
    var id: String { playerId }
}

struct ScoredMatchupGame: Decodable, Identifiable {
    let gameId: String
    let day: String
    let slot: String
    let points: Double
    let stats: [String: Double]
    let startsAt: String
    let status: String
    let home: String
    let away: String
    let positions: [String]
    var id: String { gameId }
}

struct MatchupDetail: Decodable {
    let id: String
    let competition: String
    let period: MatchupPeriod
    let homeFranchiseId: String
    let awayFranchiseId: String?
    let homePoints: Double
    let awayPoints: Double
    let final: Bool
    let homePlayers: [MatchupScorer]
    let awayPlayers: [MatchupScorer]
    let homeProjected: Double?
    let awayProjected: Double?
}

struct GameScoreboard: Decodable {
    let competition: String
    let day: String
    let games: [GameSummary]
}

struct GameSummary: Decodable, Identifiable {
    let id: String
    let competition: String
    let day: String
    let startsAt: String
    let status: String
    let detail: String
    let homeScore: Int
    let awayScore: Int
    let homeAbbrev: String
    let homeName: String
    let awayAbbrev: String
    let awayName: String
}

struct GameBoxScore: Decodable {
    let id: String
    let competition: String
    let status: String
    let detail: String
    let homeScore: Int
    let awayScore: Int
    let homeName: String
    let awayName: String
    let lines: [GameBoxLine]
}

struct GameBoxLine: Decodable, Identifiable {
    let playerId: String
    let fullName: String
    let positions: [String]
    let stats: [String: Double]
    let points: Double
    let ownerName: String
    var id: String { playerId }
}

extension JSONDecoder {
    static var mosaic: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return decoder
    }
}
