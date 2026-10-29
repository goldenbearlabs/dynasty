// Sends typed requests to the shared Go API and keeps mobile bearer authentication explicit.
import Foundation

struct APIError: LocalizedError {
    let message: String
    let statusCode: Int?
    var errorDescription: String? { message }
}

struct APIClient {
    let baseURL: URL
    let session: URLSession

    init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    private struct ServerError: Decodable { let error: String }
    private struct EmailResponse: Decodable { let emailSent: Bool }

    private func request<Response: Decodable>(
        _ method: String, _ path: String, token: String? = nil,
        body: [String: String]? = nil, bodyData: Data? = nil,
        as type: Response.Type = Response.self
    ) async throws -> Response {
        guard let url = URL(string: path, relativeTo: baseURL)?.absoluteURL else {
            throw APIError(message: "The API address is invalid.", statusCode: nil)
        }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.timeoutInterval = 15
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONEncoder().encode(body)
        } else if let bodyData {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = bodyData
        }
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse else {
                throw APIError(message: "The server sent an invalid response.", statusCode: nil)
            }
            guard (200..<300).contains(http.statusCode) else {
                let detail = try? JSONDecoder().decode(ServerError.self, from: data)
                let message = http.statusCode == 404 && path.hasPrefix("api/mobile/")
                    ? "Mobile sign-in is unavailable on this server. Check the API address and backend version."
                    : (detail?.error ?? "The server returned \(http.statusCode). Please try again.")
                throw APIError(message: message, statusCode: http.statusCode)
            }
            if Response.self == EmptyResponse.self && data.isEmpty {
                return EmptyResponse() as! Response
            }
            return try JSONDecoder.mosaic.decode(Response.self, from: data)
        } catch let error as APIError {
            throw error
        } catch is DecodingError {
            throw APIError(message: "The server sent data this app could not read.", statusCode: nil)
        } catch {
            throw APIError(message: "Could not reach the API. Check your connection and server address.", statusCode: nil)
        }
    }

    private struct EmptyResponse: Decodable {}

    private func safe(_ value: String) -> String {
        value.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? value
    }

    private func payload<T: Encodable>(_ value: T) throws -> Data {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        return try encoder.encode(value)
    }

    private func checkedDynasty(_ selected: SelectedLeague, token: String) async throws -> Dynasty {
        let current = try await dynasty(token: token)
        guard current.id == selected.id, selected.membership.ready,
              current.franchises.contains(where: { $0.id == selected.membership.franchiseId }) else {
            throw APIError(message: "This league's mobile gameplay is not available yet.", statusCode: nil)
        }
        return current
    }

    private func checkedLeague(_ selected: SelectedLeague, leagueID: String, token: String) async throws -> Dynasty {
        let current = try await checkedDynasty(selected, token: token)
        guard current.leagues.contains(where: { $0.id == leagueID }) else {
            throw APIError(message: "This sport is not in the selected league.", statusCode: nil)
        }
        return current
    }

    func login(email: String, password: String) async throws -> MobileAuth {
        try await request("POST", "api/mobile/auth/login", body: ["email": email, "password": password])
    }

    func register(email: String, password: String) async throws -> Bool {
        let response: EmailResponse = try await request("POST", "api/accounts", body: ["email": email, "password": password])
        return response.emailSent
    }

    func resendVerification(email: String) async throws {
        let _: EmptyResponse = try await request("POST", "api/accounts/verification", body: ["email": email])
    }

    func requestPasswordReset(email: String) async throws {
        let _: EmptyResponse = try await request("POST", "api/accounts/password-reset", body: ["email": email])
    }

    func claim(invite: String, email: String, password: String) async throws -> MobileAuth {
        let safe = invite.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? invite
        return try await request("POST", "api/mobile/invites/\(safe)/claim", body: ["email": email, "password": password])
    }

    func accept(invite: String, token: String) async throws -> MobileAuth {
        let safe = invite.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? invite
        return try await request("POST", "api/mobile/invites/\(safe)/accept", token: token)
    }

    func refresh(token: String) async throws -> MobileAuth {
        try await request("POST", "api/mobile/auth/refresh", token: token)
    }

    func logout(token: String) async throws {
        let _: EmptyResponse = try await request("POST", "api/mobile/auth/logout", token: token)
    }

    func myLeagues(token: String) async throws -> [MyLeague] {
        try await request("GET", "api/my-leagues", token: token)
    }

    func dynasty(token: String) async throws -> Dynasty {
        try await request("GET", "api/dynasty", token: token)
    }

    func verifiedSelection(_ league: MyLeague, token: String) async throws -> SelectedLeague {
        guard league.ready else {
            throw APIError(message: "This league is not playable yet.", statusCode: nil)
        }
        let dynasty = try await dynasty(token: token)
        guard dynasty.id == league.id && !dynasty.id.isEmpty else {
            throw APIError(message: "This league's mobile gameplay is not available yet.", statusCode: nil)
        }
        return SelectedLeague(membership: league, dynasty: dynasty)
    }

    func dashboard(for selected: SelectedLeague, token: String) async throws -> DashboardData {
        let current = try await dynasty(token: token)
        guard current.id == selected.id else {
            throw APIError(message: "This league's mobile gameplay is not available yet.", statusCode: nil)
        }
        async let standings: [StandingsTable] = request("GET", "api/standings", token: token)
        async let overall: [OverallYear] = request("GET", "api/overall", token: token)
        async let activity: [ActivityItem] = request("GET", "api/activity", token: token)
        let odds: LeagueOdds? = try? await request("GET", "api/odds", token: token)
        return try await DashboardData(standings: standings, overall: overall, activity: activity,
                                       odds: odds, dynasty: current)
    }

    func myFranchise(for selected: SelectedLeague, token: String) async throws -> FranchiseDetail {
        try await franchise(for: selected, franchiseID: selected.membership.franchiseId, token: token)
    }

    func franchise(for selected: SelectedLeague, franchiseID: String, token: String) async throws -> FranchiseDetail {
        let current = try await checkedDynasty(selected, token: token)
        guard let franchise = current.franchises.first(where: { $0.id == franchiseID }),
              let slug = franchise.slug, !slug.isEmpty else {
            throw APIError(message: "This franchise is not available in this league.", statusCode: nil)
        }
        let safeSlug = slug.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/?#"))) ?? slug
        let detail: FranchiseDetail = try await request("GET", "api/franchises/\(safeSlug)", token: token)
        guard detail.franchise.id == franchiseID,
              detail.rosters.allSatisfy({ roster in current.leagues.contains(where: { $0.id == roster.leagueId }) }) else {
            throw APIError(message: "This franchise is not available in this league.", statusCode: nil)
        }
        return detail
    }

    func lineup(for selected: SelectedLeague, leagueID: String, franchiseID: String,
                day: String?, token: String) async throws -> LineupData {
        let current = try await checkedLeague(selected, leagueID: leagueID, token: token)
        guard current.franchises.contains(where: { $0.id == franchiseID }) else {
            throw APIError(message: "This franchise is not in the selected league.", statusCode: nil)
        }
        var query = "?franchise=\(safe(franchiseID))"
        if let day { query += "&day=\(safe(day))" }
        return try await request("GET", "api/leagues/\(safe(leagueID))/lineup\(query)", token: token)
    }

    func saveLineup(for selected: SelectedLeague, leagueID: String, day: String,
                    entries: [LineupEntry], token: String) async throws {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        let body = try payload(LineupSave(day: day, entries: entries))
        let _: EmptyResponse = try await request("PUT", "api/leagues/\(safe(leagueID))/lineup",
                                                 token: token, bodyData: body)
    }

    func autoSetLineup(for selected: SelectedLeague, leagueID: String, day: String, token: String) async throws {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        let _: EmptyResponse = try await request("POST", "api/leagues/\(safe(leagueID))/lineup/auto",
                                                 token: token, body: ["day": day])
    }

    private struct LineupSave: Encodable { let day: String; let entries: [LineupEntry] }

    func rosterPerformance(for selected: SelectedLeague, leagueID: String, franchiseID: String,
                           window: String, season: String? = nil, day: String? = nil,
                           token: String) async throws -> RosterPerformance {
        let current = try await checkedLeague(selected, leagueID: leagueID, token: token)
        guard current.franchises.contains(where: { $0.id == franchiseID }) else {
            throw APIError(message: "This franchise is not in the selected league.", statusCode: nil)
        }
        var query = "?franchise=\(safe(franchiseID))&window=\(safe(window))"
        if let season { query += "&season=\(safe(season))" }
        if let day { query += "&day=\(safe(day))" }
        return try await request("GET", "api/leagues/\(safe(leagueID))/roster-stats\(query)", token: token)
    }

    func changeRoster(for selected: SelectedLeague, leagueID: String, action: String,
                      playerID: String? = nil, list: String? = nil, replacementID: String? = nil,
                      dropPlayerID: String? = nil, reserve: [String]? = nil, token: String) async throws {
        let allowed = ["add", "drop", "move", "set", "injury-swap"]
        guard allowed.contains(action) else { throw APIError(message: "Unknown roster action.", statusCode: nil) }
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        let body = try payload(RosterMutation(playerId: playerID, list: list,
                                               replacementId: replacementID, dropPlayerId: dropPlayerID,
                                               reserve: reserve))
        let _: EmptyResponse = try await request("POST", "api/leagues/\(safe(leagueID))/roster/\(action)",
                                                 token: token, bodyData: body)
    }

    private struct RosterMutation: Encodable {
        let playerId: String?
        let list: String?
        let replacementId: String?
        let dropPlayerId: String?
        let reserve: [String]?
    }

    func claimWaiver(for selected: SelectedLeague, leagueID: String, playerID: String,
                     dropPlayerID: String?, list: String, bid: Int, token: String) async throws {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        let body = try payload(WaiverRequest(playerId: playerID, dropPlayerId: dropPlayerID, list: list, bid: bid))
        let _: EmptyResponse = try await request("POST", "api/leagues/\(safe(leagueID))/waivers/claims",
                                                 token: token, bodyData: body)
    }

    private struct WaiverRequest: Encodable {
        let playerId: String
        let dropPlayerId: String?
        let list: String
        let bid: Int
    }

    func waiverState(for selected: SelectedLeague, leagueID: String, token: String) async throws -> WaiverState {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        return try await request("GET", "api/leagues/\(safe(leagueID))/waivers", token: token)
    }

    func updateIdentity(for selected: SelectedLeague, leagueID: String?, name: String,
                        imageURL: String, token: String) async throws {
        if let leagueID { _ = try await checkedLeague(selected, leagueID: leagueID, token: token) }
        else { _ = try await checkedDynasty(selected, token: token) }
        let id = safe(selected.membership.franchiseId)
        let path = leagueID.map { "api/franchises/\(id)/leagues/\(safe($0))/identity" }
            ?? "api/franchises/\(id)/identity"
        let body = try payload(IdentityInput(name: name, imageUrl: imageURL))
        let _: EmptyResponse = try await request("PUT", path, token: token, bodyData: body)
    }

    private struct IdentityInput: Encodable { let name: String; let imageUrl: String }

    func setNickname(for selected: SelectedLeague, playerID: String, nickname: String,
                     token: String) async throws {
        _ = try await checkedDynasty(selected, token: token)
        let path = "api/franchises/\(safe(selected.membership.franchiseId))/players/\(safe(playerID))/nickname"
        let _: EmptyResponse = try await request("PUT", path, token: token, body: ["nickname": nickname])
    }

    func players(for selected: SelectedLeague, leagueID: String, query: String,
                 page: Int = 1, token: String) async throws -> PlayerPage {
        let current = try await checkedLeague(selected, leagueID: leagueID, token: token)
        guard let league = current.leagues.first(where: { $0.id == leagueID }) else {
            throw APIError(message: "This sport is not in the selected league.", statusCode: nil)
        }
        var parts = URLComponents()
        parts.queryItems = [URLQueryItem(name: "competition", value: league.competition),
                            URLQueryItem(name: "q", value: query),
                            URLQueryItem(name: "page", value: String(page))]
        return try await request("GET", "api/players\(parts.percentEncodedQuery.map { "?" + $0 } ?? "")", token: token)
    }

    func playerProfile(for selected: SelectedLeague, playerID: String, token: String) async throws -> TeamPlayerProfile {
        _ = try await checkedDynasty(selected, token: token)
        return try await request("GET", "api/players/\(safe(playerID))/profile", token: token)
    }

    func watchlist(for selected: SelectedLeague, token: String) async throws -> [WatchlistEntry] {
        _ = try await checkedDynasty(selected, token: token)
        return try await request("GET", "api/watchlist", token: token)
    }

    func setWatched(for selected: SelectedLeague, playerID: String, on: Bool, token: String) async throws {
        _ = try await checkedDynasty(selected, token: token)
        let _: EmptyResponse = try await request(on ? "PUT" : "DELETE", "api/watchlist/players/\(safe(playerID))", token: token)
    }

    func tradeMarket(for selected: SelectedLeague, token: String) async throws -> TradeMarket {
        _ = try await checkedDynasty(selected, token: token)
        return try await request("GET", "api/trade-market", token: token)
    }

    func setTradeFlag(for selected: SelectedLeague, leagueID: String, playerID: String,
                      on: Bool, ownPlayer: Bool, token: String) async throws {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        let path = ownPlayer ? "api/trade-market/block" : "api/trade-market/targets"
        let body = try payload(TradeFlagInput(leagueId: leagueID, playerId: playerID, on: on))
        let _: EmptyResponse = try await request("PUT", path, token: token, bodyData: body)
    }

    private struct TradeFlagInput: Encodable { let leagueId: String; let playerId: String; let on: Bool }

    func schedule(for selected: SelectedLeague, leagueID: String, token: String) async throws -> [SchedulePeriod] {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        return try await request("GET", "api/leagues/\(safe(leagueID))/schedule", token: token)
    }

    func standings(for selected: SelectedLeague, token: String) async throws -> [StandingsTable] {
        _ = try await checkedDynasty(selected, token: token)
        return try await request("GET", "api/standings", token: token)
    }

    func setSeasonRoster(for selected: SelectedLeague, leagueID: String,
                         reserve: [String], token: String) async throws {
        let current = try await dynasty(token: token)
        guard current.id == selected.id,
              current.franchises.contains(where: { $0.id == selected.membership.franchiseId }),
              current.leagues.contains(where: { $0.id == leagueID }) else {
            throw APIError(message: "This league's mobile gameplay is not available yet.", statusCode: nil)
        }
        let safeID = leagueID.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? leagueID
        let payload = try JSONEncoder().encode(SeasonRosterRequest(reserve: reserve))
        let _: EmptyResponse = try await request("POST", "api/leagues/\(safeID)/roster/season",
                                                 token: token, bodyData: payload)
    }

    private struct SeasonRosterRequest: Encodable { let reserve: [String] }

    func matchups(for selected: SelectedLeague, leagueID: String, period: Int?, token: String) async throws -> MatchupsData {
        _ = try await checkedLeague(selected, leagueID: leagueID, token: token)
        let safeID = leagueID.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? leagueID
        let suffix = period.map { "?period=\($0)" } ?? ""
        return try await request("GET", "api/leagues/\(safeID)/matchups\(suffix)", token: token)
    }

    func matchupDetail(for selected: SelectedLeague, leagueID: String, matchupID: String, token: String) async throws -> MatchupDetail {
        let current = try await checkedLeague(selected, leagueID: leagueID, token: token)
        guard let league = current.leagues.first(where: { $0.id == leagueID }) else {
            throw APIError(message: "This sport is not in the selected league.", statusCode: nil)
        }
        let safeID = matchupID.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? matchupID
        let detail: MatchupDetail = try await request("GET", "api/matchups/\(safeID)", token: token)
        guard detail.competition == league.competition else {
            throw APIError(message: "This matchup is not in the selected league.", statusCode: nil)
        }
        let scheduled = try await schedule(for: selected, leagueID: leagueID, token: token)
        guard scheduled.contains(where: { $0.id == detail.period.id && $0.matchups.contains(where: { $0.id == matchupID }) }) else {
            throw APIError(message: "This matchup is not in the selected league.", statusCode: nil)
        }
        return detail
    }

    func scores(for selected: SelectedLeague, leagueID: String, day: String, token: String) async throws -> GameScoreboard {
        let current = try await checkedLeague(selected, leagueID: leagueID, token: token)
        guard let sport = current.leagues.first(where: { $0.id == leagueID })?.competition else {
            throw APIError(message: "This sport is not in the selected league.", statusCode: nil)
        }
        return try await request("GET", "api/scores/\(safe(sport))?day=\(safe(day))", token: token)
    }

    func gameDetail(for selected: SelectedLeague, leagueID: String, gameID: String, token: String) async throws -> GameBoxScore {
        let current = try await checkedLeague(selected, leagueID: leagueID, token: token)
        guard let sport = current.leagues.first(where: { $0.id == leagueID })?.competition else {
            throw APIError(message: "This sport is not in the selected league.", statusCode: nil)
        }
        let game: GameBoxScore = try await request("GET", "api/games/\(safe(gameID))", token: token)
        guard game.competition == sport else {
            throw APIError(message: "This game is not in the selected sport.", statusCode: nil)
        }
        return game
    }

    func scoresSocketURL(sport: String) -> URL? {
        let safe = sport.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? sport
        var parts = URLComponents(url: baseURL.appendingPathComponent("api/scores/\(safe)/ws"), resolvingAgainstBaseURL: false)
        parts?.scheme = baseURL.scheme == "https" ? "wss" : "ws"
        return parts?.url
    }
}

enum AppConfig {
    static let baseURL: URL = {
        #if DEBUG
        let value = ProcessInfo.processInfo.environment["API_BASE_URL"] ?? "http://localhost:8090"
        #else
        let value = "https://mosaicfantasy.com"
        #endif
        return URL(string: value.hasSuffix("/") ? value : value + "/")!
    }()
}
