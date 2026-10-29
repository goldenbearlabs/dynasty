// Covers the native client's API decoding, league safety check, and token lifecycle without a live server.
import Foundation
import XCTest
@testable import MosaicFantasy

private final class MockURLProtocol: URLProtocol {
    static var handler: ((URLRequest) throws -> (Int, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func stopLoading() {}
    override func startLoading() {
        do {
            let (status, data) = try Self.handler!(request)
            let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil,
                                           headerFields: ["Content-Type": "application/json"])!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch { client?.urlProtocol(self, didFailWithError: error) }
    }
}

private final class MemoryTokenStore: TokenStore {
    var value: String?
    init(_ value: String? = nil) { self.value = value }
    func read() throws -> String? { value }
    func write(_ token: String) throws { value = token }
    func delete() throws { value = nil }
}

final class MosaicFantasyTests: XCTestCase {
    private func api() -> APIClient {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return APIClient(baseURL: URL(string: "https://example.test/")!,
                         session: URLSession(configuration: configuration))
    }

    private func bodyData(_ request: URLRequest) -> Data? {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let count = stream.read(&buffer, maxLength: buffer.count)
            if count <= 0 { break }
            data.append(contentsOf: buffer[..<count])
        }
        return data
    }

    func testScoreFeedUsesBearerHeaderForWebSocketHandshake() throws {
        let url = try XCTUnwrap(URL(string: "ws://localhost:8090/api/scores/nhl/ws"))
        let request = ScoreFeed.request(url: url, token: "mobile-session")
        XCTAssertEqual(request.url, url)
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer mobile-session")
        XCTAssertNil(request.url?.query)
    }

    func testModelsDecodeServerKeys() throws {
        let league = try JSONDecoder.mosaic.decode(MyLeague.self, from: Data("""
        {"id":"one","name":"Mosaic","franchise_id":"mine","franchise_name":"My Team", "sports":["nba","nhl"],"ready":true}
        """.utf8))
        XCTAssertEqual(league.franchiseName, "My Team")
        XCTAssertEqual(league.sports, ["nba", "nhl"])
        let year = try JSONDecoder.mosaic.decode(OverallYear.self, from: Data("""
        {"year":2026,"final":false,"rows":[{"franchise_id":"mine","points":12,"adjusted_points":11.5}]}
        """.utf8))
        XCTAssertFalse(year.finalYear)
        XCTAssertEqual(year.rows.first?.adjustedPoints, 11.5)
    }

    func testStandingsFixturesDecodeWebsiteFieldsAndSort() throws {
        let dynasty = PreviewFixtures.dynasty
        let dashboard = PreviewFixtures.dashboard
        XCTAssertEqual(dynasty.settings?.overallTitle?.pointsByFinish, [14, 12, 10])
        XCTAssertEqual(dashboard.standings.first?.season?.year, 2026)
        XCTAssertEqual(dashboard.standings[1].season?.championFranchiseId, "franchise-2")
        XCTAssertEqual(dashboard.overall.first?.rows.first?.finishes?["nba"], 1)
        XCTAssertEqual(dashboard.odds?.leagues.first?.rows.first?.playoffs, 0.78)
        XCTAssertEqual(PreviewFixtures.matchups.period?.seq, 2)
        XCTAssertEqual(PreviewFixtures.matchupDetail.homePlayers.first?.points, 24.3)
        XCTAssertEqual(StandingsOrder.sports(dashboard.standings).map(\.competition), ["nba", "nhl", "cbb"])
        XCTAssertEqual(StandingsOrder.tabs([]), ["overall"])
        XCTAssertEqual(StandingsOrder.initialTab(tables: dashboard.standings, overall: []), "nba")
        let points = [10.0, 25.0, 15.0]
        let ranked = StandingsOrder.sorted(points, key: "points", descending: true) { value, _, _ in .number(value) }
        XCTAssertEqual(ranked.map(\.place), [2, 3, 1])
        XCTAssertEqual(StandingsOrder.record(8, 3, 1), "8-3-1")
        XCTAssertEqual(StandingsOrder.sorted(dashboard.standings[0].rows, key: "place", descending: false) { _, place, _ in .number(Double(place)) }.map(\.place), [1, 2])
        XCTAssertEqual(StandingsOrder.percent(0.64), "64.0%")
    }

    func testSeasonRosterEligibilityAndLimits() throws {
        let detail = PreviewFixtures.franchiseDetail
        let nba = try XCTUnwrap(detail.rosters.first)
        XCTAssertTrue(try XCTUnwrap(nba.season).needsSetting)
        XCTAssertEqual(SeasonRosterRules.signed(nba).count, 3)
        XCTAssertTrue(SeasonRosterRules.valid(nba, reserve: ["three"]))
        XCTAssertFalse(SeasonRosterRules.valid(nba, reserve: []))
        XCTAssertFalse(SeasonRosterRules.valid(nba, reserve: ["one", "two", "three"]))
        XCTAssertFalse(SeasonRosterRules.valid(nba, reserve: ["three", "unsigned"]))
        XCTAssertEqual(SeasonRosterRules.counts(nba, reserve: ["one", "three"]).main, 1)
        XCTAssertEqual(SeasonRosterRules.due(detail.rosters).map(\.competition), ["nba"])

        let seasons = try JSONDecoder.mosaic.decode([SeasonRosterState].self, from: Data("""
        [{"id":"one","year":2026,"starts_on":"2026-11-01","set_at":null,"started":false,"deadline":null,"expired":false},
         {"id":"two","year":2026,"starts_on":"2026-11-01","set_at":null,"started":true,"deadline":null,"expired":false},
         {"id":"three","year":2026,"starts_on":"2026-11-01","set_at":"2026-10-25T12:00:00Z","started":false,"deadline":null,"expired":false},
         {"id":"four","year":2026,"starts_on":"2026-11-01","set_at":null,"started":false,"deadline":null,"expired":true}]
        """.utf8))
        XCTAssertEqual(seasons.filter(\.needsSetting).map(\.id), ["one", "two"])
        let secondDue = try JSONDecoder.mosaic.decode(LeagueRoster.self, from: Data("""
        {"league_id":"cbb-1","competition":"cbb","name":"CBB","team_name":"College Lights",
         "limits":{"main":2,"reserve":1},"players":[],
         "season":{"id":"another","year":2026,"starts_on":"2026-11-01","set_at":null,
                   "started":false,"deadline":null,"expired":false}}
        """.utf8))
        XCTAssertEqual(SeasonRosterRules.due(detail.rosters + [secondDue]).map(\.competition), ["nba", "cbb"])
    }

    func testFranchiseFetchUsesVerifiedMembershipAndCurrentDynasty() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            if request.url?.path == "/api/dynasty" {
                return (200, Data("""
                {"id":"dynasty-1","name":"Renamed league","leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"Northern Lights","slug":"northern-lights"}]}
                """.utf8))
            }
            return (200, Data("""
            {"franchise":{"id":"franchise-1","name":"Northern Lights","slug":"northern-lights"},"rosters":[]}
            """.utf8))
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        let detail = try await api().myFranchise(for: selected, token: "token")
        XCTAssertEqual(detail.franchise.id, selected.membership.franchiseId)
        XCTAssertEqual(paths, ["/api/dynasty", "/api/franchises/northern-lights"])
    }

    func testSeasonRosterSaveSendsOnlyReserveAndPropagatesRejection() async throws {
        var paths: [String] = []
        var reject = false
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            if request.url?.path == "/api/dynasty" {
                return (200, Data("""
                {"id":"dynasty-1","name":"Mosaic","leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"Northern Lights","slug":"northern-lights"}]}
                """.utf8))
            }
            XCTAssertEqual(request.url?.path, "/api/leagues/nba-1/roster/season")
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer token")
            let body = try XCTUnwrap(self.bodyData(request).flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] })
            XCTAssertEqual(body["reserve"] as? [String], ["three"])
            XCTAssertNil(body["force"])
            XCTAssertNil(body["franchise_id"])
            return reject ? (422, Data("{\"error\":\"Roster already set.\"}".utf8)) : (204, Data())
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        try await api().setSeasonRoster(for: selected, leagueID: "nba-1", reserve: ["three"], token: "token")
        reject = true
        do {
            try await api().setSeasonRoster(for: selected, leagueID: "nba-1", reserve: ["three"], token: "token")
            XCTFail("Expected server rejection")
        } catch let error as APIError {
            XCTAssertEqual(error.statusCode, 422)
        }
        XCTAssertEqual(paths, ["/api/dynasty", "/api/leagues/nba-1/roster/season",
                               "/api/dynasty", "/api/leagues/nba-1/roster/season"])
    }

    func testSeasonRosterWrongLeagueNeverPosts() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            return (200, Data("""
            {"id":"other","name":"Other","leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],"franchises":[]}
            """.utf8))
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        do {
            try await api().setSeasonRoster(for: selected, leagueID: "nba-1", reserve: [], token: "token")
            XCTFail("Expected wrong-league protection")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not available"))
        }
        XCTAssertEqual(paths, ["/api/dynasty"])
    }

    func testPendingSetupIsHiddenFromPlayableMemberships() throws {
        let memberships = try JSONDecoder.mosaic.decode([MyLeague].self, from: Data("""
        [{"id":"live","name":"Live","franchise_id":"mine","franchise_name":"Mine","sports":["nba"],"ready":true},
         {"id":"setup","name":"Saved","franchise_id":"mine","franchise_name":"Mine","sports":[],"ready":false}]
        """.utf8))
        XCTAssertEqual(MyLeague.playable(in: memberships).map(\.id), ["live"])
    }

    func testWrongLeagueNeverReachesDashboard() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            return (200, Data("{\"id\":\"legacy\",\"name\":\"Original\",\"leagues\":[],\"franchises\":[]}".utf8))
        }
        let membership = MyLeague(id: "other", name: "Other", franchiseId: "mine",
                                  franchiseName: "Mine", sports: ["nba"], ready: true)
        do {
            _ = try await api().verifiedSelection(membership, token: "token")
            XCTFail("Expected a mismatch")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not available"))
        }
        XCTAssertEqual(paths, ["/api/dynasty"])
    }

    func testDashboardRefreshChecksDynastyBeforeLoadingResults() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            return (200, Data("{\"id\":\"changed\",\"name\":\"Changed\",\"leagues\":[],\"franchises\":[]}".utf8))
        }
        let membership = MyLeague(id: "original", name: "Original", franchiseId: "mine",
                                  franchiseName: "Mine", sports: ["nba"], ready: true)
        let selected = SelectedLeague(membership: membership, dynasty: PreviewFixtures.dynasty)
        do {
            _ = try await api().dashboard(for: selected, token: "token")
            XCTFail("Expected a mismatch")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not available"))
        }
        XCTAssertEqual(paths, ["/api/dynasty"])
    }

    func testDashboardFailureCanRetryWithoutOdds() async throws {
        var failStandings = true
        MockURLProtocol.handler = { request in
            switch request.url!.path {
            case "/api/dynasty":
                return (200, Data("{\"id\":\"one\",\"name\":\"League\",\"leagues\":[],\"franchises\":[]}".utf8))
            case "/api/standings":
                if failStandings {
                    failStandings = false
                    return (500, Data("{\"error\":\"temporary failure\"}".utf8))
                }
                return (200, Data("[]".utf8))
            case "/api/overall", "/api/activity": return (200, Data("[]".utf8))
            case "/api/odds": return (503, Data("{\"error\":\"not ready\"}".utf8))
            default: XCTFail("Unexpected request"); return (404, Data())
            }
        }
        let membership = MyLeague(id: "one", name: "League", franchiseId: "mine",
                                  franchiseName: "Mine", sports: ["nba"], ready: true)
        let selected = SelectedLeague(membership: membership, dynasty: PreviewFixtures.dynasty)
        let client = api()
        do {
            _ = try await client.dashboard(for: selected, token: "token")
            XCTFail("Expected a dashboard failure")
        } catch let error as APIError {
            XCTAssertEqual(error.message, "temporary failure")
        }
        let dashboard = try await client.dashboard(for: selected, token: "token")
        XCTAssertTrue(dashboard.standings.isEmpty)
        XCTAssertTrue(dashboard.activity.isEmpty)
        XCTAssertNil(dashboard.odds)
        XCTAssertEqual(dashboard.dynasty?.name, "League")
    }

    func testMatchupsRejectWrongDynastyBeforeGameplayRequest() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            return (200, Data("{\"id\":\"different\",\"name\":\"Different\",\"leagues\":[],\"franchises\":[]}".utf8))
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        do {
            _ = try await api().matchups(for: selected, leagueID: "nba-1", period: nil, token: "token")
            XCTFail("Expected mismatch")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not available"))
        }
        XCTAssertEqual(paths, ["/api/dynasty"])
    }

    func testMatchupDetailRejectsDifferentSport() async throws {
        MockURLProtocol.handler = { request in
            if request.url?.path == "/api/dynasty" {
                return (200, Data("""
                {"id":"dynasty-1","name":"Mosaic Dynasty","leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"Northern Lights"}]}
                """.utf8))
            }
            XCTAssertEqual(request.url?.path, "/api/matchups/matchup-1")
            let wrong = """
            {"id":"matchup-1","competition":"nhl",
             "period":{"id":"period-1","seq":1,"starts_on":"2026-10-01","ends_on":"2026-10-07","is_playoff":false,"by_hand":false},
             "home_franchise_id":"one","away_franchise_id":"two","home_points":0,"away_points":0,"final":false,
             "home_players":[],"away_players":[]}
            """
            return (200, Data(wrong.utf8))
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        do {
            _ = try await api().matchupDetail(for: selected, leagueID: "nba-1", matchupID: "matchup-1", token: "token")
            XCTFail("Expected sport mismatch")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not in the selected league"))
        }
    }

    func testMatchupModelsAndPresentation() throws {
        let detail = try JSONDecoder.mosaic.decode(MatchupDetail.self, from: Data("""
        {"id":"m1","competition":"nba","bracket":"championship","bracket_slot":0,
         "period":{"id":"p1","seq":3,"starts_on":"2026-10-26","ends_on":"2026-11-01",
                   "is_playoff":false,"by_hand":false,"double_games":true},
         "home_franchise_id":"franchise-1","away_franchise_id":"franchise-2",
         "home_points":25,"away_points":19,"final":false,"home_projected":63.2,"away_projected":59.1,
         "home_players":[{"player_id":"one","full_name":"Alex North","nickname":"Ace","points":25,
           "game_lines":[{"game_id":"g1","day":"2026-10-27","slot":"G","points":25,
             "stats":{"points":20},"starts_at":"2026-10-27T20:00:00Z","status":"final",
             "home":"NY","away":"BOS","positions":["G"]}]}],"away_players":[]}
        """.utf8))
        XCTAssertEqual(detail.homeProjected, 63.2)
        XCTAssertTrue(detail.period.doubleGames == true)
        XCTAssertEqual(detail.homePlayers[0].gameLines?.first?.stats["points"], 20)
        let daily = MatchupRows.make(lineup: PreviewFixtures.lineup,
                                     scorers: detail.homePlayers, period: detail.period,
                                     day: "2026-10-27", wholePeriod: false, weekly: false)
        XCTAssertEqual(daily.first?.name, "Ace")
        XCTAssertEqual(daily.first?.points, 25)
        XCTAssertEqual(daily.first?.games.first?.label, "Counted")
        XCTAssertEqual(daily.last?.slot, "F")
        let weekly = MatchupRows.make(lineup: PreviewFixtures.lineup,
                                      scorers: detail.homePlayers, period: detail.period,
                                      day: "2026-10-27", wholePeriod: false, weekly: true)
        XCTAssertEqual(weekly.first?.projected, 20.1)
        XCTAssertEqual(MatchupRows.visibleGames(weekly[0], includeUncounted: false).count, 2)
        XCTAssertEqual(MatchupsLogic.label(detail.period, periods: [detail.period], days: 7), "Week 3")
        let period = PreviewFixtures.matchups.period!
        XCTAssertEqual(MatchupsLogic.label(period, periods: PreviewFixtures.matchups.periods, days: 7), "Week 2")
    }

    func testMatchupSportAndScheduleSelection() throws {
        let league = PreviewFixtures.dynasty.leagues[0]
        let active = PreviewFixtures.matchups
        let chosen = MatchupsLogic.preferredSport([(league, active)], today: "2026-10-22")
        XCTAssertEqual(chosen, league.id)
        XCTAssertEqual(MatchupsLogic.ordered(active.matchups, mine: "franchise-1").first?.id, "matchup-1")
        let schedule = try JSONDecoder.mosaic.decode([SchedulePeriod].self, from: Data("""
        [{"id":"p1","seq":1,"starts_on":"2026-10-19","ends_on":"2026-10-25",
          "is_playoff":false,"by_hand":false,"matchups":[
           {"id":"m1","bracket":"championship","bracket_slot":0,"home_franchise_id":"franchise-1",
            "away_franchise_id":"franchise-2","home_points":10,"away_points":8,"final":true},
           {"id":"m2","bracket":"championship","bracket_slot":1,"home_franchise_id":"franchise-3",
            "away_franchise_id":null,"home_points":0,"away_points":0,"final":false}]}]
        """.utf8))
        XCTAssertEqual(MatchupsLogic.schedule(schedule, team: nil)[0].matchups.count, 2)
        XCTAssertEqual(MatchupsLogic.schedule(schedule, team: "franchise-3")[0].matchups.map(\.id), ["m2"])
        XCTAssertTrue(MatchupsLogic.schedule(schedule, team: "unknown")[0].matchups.isEmpty)
    }

    func testBenchAndUncountedGamesStayOutOfStarterPoints() throws {
        let lineup = try JSONDecoder.mosaic.decode(LineupData.self, from: Data("""
        {"day":"2026-10-26","last_day":"2026-11-01","locked":"",
         "slots":[{"name":"G","positions":["G"],"count":2,"games_per_week":1}],
         "players":[{"player_id":"one","full_name":"Starter","nickname":"","positions":["G"],
           "team_abbrev":"NY","injury":"","slot":"G","counts_from":"","counts_last":false,
           "locked":false,"starter_eligible":true,"eligibility_note":"","points":10,
           "games":[{"game_id":"g1","day":"2026-10-27","starts_at":"2026-10-27T20:00:00Z",
             "status":"final","opponent":"BOS","at_home":true,"points":10,"counts":true,
             "starter":"","projected":0,"stats":{"points":10}},
            {"game_id":"g2","day":"2026-10-28","starts_at":"2026-10-28T20:00:00Z",
             "status":"final","opponent":"CHI","at_home":false,"points":20,"counts":false,
             "starter":"","projected":0,"stats":{"points":20}}]},
          {"player_id":"two","full_name":"Bench","nickname":"","positions":["G"],
           "team_abbrev":"LA","injury":"","slot":"","counts_from":"","counts_last":false,
           "locked":false,"starter_eligible":true,"eligibility_note":"","points":9,
           "games":[{"game_id":"g3","day":"2026-10-27","starts_at":"2026-10-27T21:00:00Z",
             "status":"final","opponent":"PHI","at_home":true,"points":9,"counts":false,
             "starter":"","projected":0,"stats":{"points":9}}]}],"starts":null,"doubled":null}
        """.utf8))
        let scorer = try JSONDecoder.mosaic.decode(MatchupScorer.self, from: Data("""
        {"player_id":"one","full_name":"Starter","points":10,"game_lines":[
         {"game_id":"g1","day":"2026-10-27","slot":"G","points":10,"stats":{"points":10},
          "starts_at":"2026-10-27T20:00:00Z","status":"final","home":"NY","away":"BOS","positions":["G"]}]}
        """.utf8))
        let period = try JSONDecoder.mosaic.decode(MatchupPeriod.self, from: Data("""
        {"id":"p1","seq":1,"starts_on":"2026-10-26","ends_on":"2026-11-01",
         "is_playoff":false,"by_hand":false}
        """.utf8))
        let rows = MatchupRows.make(lineup: lineup, scorers: [scorer], period: period,
                                    day: "2026-10-27", wholePeriod: true, weekly: true)
        XCTAssertEqual(rows[0].points, 10)
        XCTAssertEqual(rows[1].name, "Empty")
        XCTAssertEqual(rows[2].points, 9)
        XCTAssertTrue(rows[2].bench)
        XCTAssertEqual(MatchupRows.visibleGames(rows[0], includeUncounted: false).map(\.id), ["g1"])
        XCTAssertEqual(MatchupRows.visibleGames(rows[0], includeUncounted: true).map(\.id), ["g1", "g2"])
    }

    func testGameBoxScoreDecodesAndRejectsWrongSport() async throws {
        let box = try JSONDecoder.mosaic.decode(GameBoxScore.self, from: Data("""
        {"id":"g1","competition":"nba","status":"final","detail":"Final",
         "home_score":100,"away_score":98,"home_name":"New York","away_name":"Boston",
         "lines":[{"player_id":"p1","full_name":"Alex","positions":["G"],
          "stats":{"points":24},"points":32.5,"owner_name":"Northern Lights"}]}
        """.utf8))
        XCTAssertEqual(box.lines[0].stats["points"], 24)
        MockURLProtocol.handler = { request in
            if request.url?.path == "/api/dynasty" {
                return (200, Data("""
                {"id":"dynasty-1","name":"Mosaic Dynasty","leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"Northern Lights"}]}
                """.utf8))
            }
            return (200, Data("""
            {"id":"g1","competition":"nhl","status":"final","detail":"Final",
             "home_score":1,"away_score":2,"home_name":"A","away_name":"B","lines":[]}
            """.utf8))
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        do {
            _ = try await api().gameDetail(for: selected, leagueID: "nba-1", gameID: "g1", token: "token")
            XCTFail("Expected sport mismatch")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not in the selected sport"))
        }
    }

    func testMatchupDetailRejectsSameSportFromOtherLeague() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            let path = request.url!.path
            paths.append(path)
            switch path {
            case "/api/dynasty":
                return (200, Data("""
                {"id":"dynasty-1","name":"Mosaic Dynasty","leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"Northern Lights"}]}
                """.utf8))
            case "/api/matchups/m1":
                return (200, Data("""
                {"id":"m1","competition":"nba","period":{"id":"other-period","seq":1,
                 "starts_on":"2026-10-01","ends_on":"2026-10-07","is_playoff":false,"by_hand":false},
                 "home_franchise_id":"franchise-1","away_franchise_id":"franchise-2",
                 "home_points":0,"away_points":0,"final":false,"home_players":[],"away_players":[]}
                """.utf8))
            case "/api/leagues/nba1/schedule", "/api/leagues/nba-1/schedule":
                return (200, Data("[]".utf8))
            default: XCTFail("Unexpected request \(path)"); return (500, Data())
            }
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        do {
            _ = try await api().matchupDetail(for: selected, leagueID: "nba-1", matchupID: "m1", token: "token")
            XCTFail("Expected rejection")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not in the selected league"))
        }
        XCTAssertEqual(paths.last, "/api/leagues/nba-1/schedule")
    }

    func testPendingLeagueDoesNotRequestDynasty() async throws {
        MockURLProtocol.handler = { _ in XCTFail("No request expected"); return (500, Data()) }
        let membership = MyLeague(id: "pending", name: "Future", franchiseId: "mine",
                                  franchiseName: "Mine", sports: [], ready: false)
        do {
            _ = try await api().verifiedSelection(membership, token: "token")
            XCTFail("Expected pending protection")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not playable"))
        }
    }

    @MainActor
    func testRestoredSessionRotatesTokenAndLogsOut() async throws {
        let storage = MemoryTokenStore("old")
        MockURLProtocol.handler = { request in
            switch request.url!.path {
            case "/api/mobile/auth/refresh":
                XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer old")
                return (200, Data("""
                {"token":"fresh","expires_at":"2030-01-01T00:00:00Z","session":{"email":"manager@example.com","verified":true}}
                """.utf8))
            case "/api/mobile/auth/logout":
                XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer fresh")
                return (204, Data())
            default: XCTFail("Unexpected request"); return (404, Data())
            }
        }
        let store = SessionStore(api: api(), tokenStore: storage)
        await store.restore()
        XCTAssertEqual(store.session?.email, "manager@example.com")
        XCTAssertEqual(storage.value, "fresh")
        try await store.signOut()
        XCTAssertNil(store.session)
        XCTAssertNil(storage.value)
    }

    @MainActor
    func testExpiredSessionClearsKeychain() async throws {
        let storage = MemoryTokenStore("expired")
        MockURLProtocol.handler = { _ in (401, Data("{\"error\":\"expired\"}".utf8)) }
        let store = SessionStore(api: api(), tokenStore: storage)
        await store.restore()
        XCTAssertTrue(store.ready)
        XCTAssertNil(store.session)
        XCTAssertNil(storage.value)
    }

    @MainActor
    func testRegistrationAndInviteUseExistingEndpoints() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            switch request.url!.path {
            case "/api/accounts": return (201, Data("{\"email_sent\":true}".utf8))
            case "/api/mobile/auth/login", "/api/mobile/invites/abc/accept":
                return (200, Data("""
                {"token":"fresh","expires_at":"2030-01-01T00:00:00Z","session":{"email":"new@example.com","verified":true}}
                """.utf8))
            default: XCTFail("Unexpected request"); return (404, Data())
            }
        }
        let store = SessionStore(api: api(), tokenStore: MemoryTokenStore())
        let sent = try await store.register(email: "new@example.com", password: "password")
        XCTAssertTrue(sent)
        try await store.acceptInvite("https://example.test/join/abc")
        XCTAssertEqual(paths, ["/api/accounts", "/api/mobile/auth/login", "/api/mobile/invites/abc/accept"])
    }

    @MainActor
    func testInviteClaimCreatesMobileSession() async throws {
        MockURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.path, "/api/mobile/invites/new/claim")
            return (200, Data("""
            {"token":"joined","expires_at":"2030-01-01T00:00:00Z","session":{"email":"new@example.com","verified":true}}
            """.utf8))
        }
        let storage = MemoryTokenStore()
        let store = SessionStore(api: api(), tokenStore: storage)
        try await store.claim(inviteURL: "https://example.test/join/new", email: "new@example.com", password: "password")
        XCTAssertEqual(storage.value, "joined")
        XCTAssertEqual(store.session?.email, "new@example.com")
    }

    func testTeamModelsDecodePicksRosterStatusAndLineup() throws {
        let detail = try JSONDecoder.mosaic.decode(FranchiseDetail.self, from: Data("""
        {"franchise":{"id":"mine","name":"North","slug":"north","manager_name":"Ethan"},
         "rosters":[{"league_id":"nba-1","competition":"nba","name":"NBA","team_name":"Hoops",
           "image_url":"","limits":{"main":2,"reserve":1,"reserve_lock_season":true},"overage":0,
           "players":[{"player_id":"p1","full_name":"First Player","list":"main","positions":["G"],
             "nickname":"Fast","status":"active","injury_designation":"OUT","locked_until":null,
             "rights_until":null,"injury_reserve_locked":false},
             {"player_id":"p2","full_name":"Rookie","list":"rights","positions":["F"],
              "rights_until":"2026-11-01T12:00:00Z","draft_year":2026,"draft_round":1}],
           "season":null}],
         "picks":[{"id":"pick","round":1,"position":3,"original_franchise_id":"mine",
           "draft_id":"draft","draft_name":"Rookie Draft","year":2027,"draft_status":"scheduled",
           "competitions":["nba"]}]}
        """.utf8))
        XCTAssertEqual(detail.franchise.managerName, "Ethan")
        XCTAssertEqual(detail.rosters[0].players[0].nickname, "Fast")
        XCTAssertEqual(detail.rosters[0].players[1].rightsUntil, "2026-11-01T12:00:00Z")
        XCTAssertEqual(detail.picks[0].round, 1)
        let waivers = try JSONDecoder.mosaic.decode(WaiverState.self, from: Data("""
        {"rules":{"mode":"faab","hours":48,"budget":100},"weekly_limit":3,"acquisitions":1,
         "standings":[{"franchise_id":"mine","name":"North","slug":"north","priority":4,"budget_left":72}],
         "players":[],"claims":[]}
        """.utf8))
        XCTAssertEqual(waivers.standings.first?.budgetLeft, 72)
        XCTAssertEqual(waivers.weeklyLimit, 3)

        let lineup = try JSONDecoder.mosaic.decode(LineupData.self, from: Data("""
        {"day":"2026-10-26","last_day":"2026-11-01","locked":"",
         "slots":[{"name":"G","positions":["G"],"count":1,"games_per_week":1}],
         "players":[
           {"player_id":"p1","full_name":"First Player","nickname":"","positions":["G"],
            "team_abbrev":"NY","injury":"","slot":"G","counts_from":"","counts_last":true,
            "locked":false,"starter_eligible":true,"eligibility_note":"","points":8,
            "games":[{"game_id":"g1","day":"2026-10-27","starts_at":"2026-10-27T20:00:00Z",
              "status":"scheduled","opponent":"BOS","at_home":true,"points":0,"counts":false,
              "starter":"","projected":12,"expected":12,"stats":{}},
             {"game_id":"g2","day":"2026-10-29","starts_at":"2026-10-29T20:00:00Z",
              "status":"scheduled","opponent":"CHI","at_home":false,"points":0,"counts":false,
              "starter":"","projected":14,"expected":14,"stats":{}}]},
           {"player_id":"p2","full_name":"Bench Guard","nickname":"","positions":["G"],
            "team_abbrev":"LA","injury":"","slot":"","counts_from":"","counts_last":false,
            "locked":false,"starter_eligible":true,"eligibility_note":"","points":0,"games":[]}],
         "starts":{"limit":2,"made":[]},"doubled":null}
        """.utf8))
        XCTAssertEqual(lineup.slots[0].gamesPerWeek, 1)
        XCTAssertEqual(lineup.starts?.limit, 2)
        var draft = LineupDraft(lineup)
        XCTAssertTrue(draft.weekly)
        XCTAssertFalse(draft.dirty)
        draft.picks["p1"] = "2026-10-29"
        XCTAssertTrue(draft.dirty)
        XCTAssertEqual(draft.countedGames(lineup.players[0]).map(\.gameId), ["g2"])
        draft.place("p2", in: 0, canEdit: true)
        XCTAssertEqual(draft.slot(of: "p2"), "G")
        XCTAssertEqual(draft.slot(of: "p1"), "")
        XCTAssertEqual(draft.entries().first?.playerId, "p2")
        XCTAssertEqual(draft.entries().first?.auto, true)
    }

    func testTeamWritesCheckDynastyAndNeverSendCommissionerFields() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            if request.url?.path == "/api/dynasty" {
                return (200, Data("""
                {"id":"dynasty-1","name":"Mosaic",
                 "leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"North","slug":"north"}]}
                """.utf8))
            }
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer token")
            let body = try XCTUnwrap(self.bodyData(request).flatMap {
                try? JSONSerialization.jsonObject(with: $0) as? [String: Any]
            })
            XCTAssertNil(body["force"])
            XCTAssertNil(body["franchise_id"])
            if request.url?.path == "/api/leagues/nba-1/roster/move" {
                XCTAssertEqual(body["player_id"] as? String, "player")
                XCTAssertEqual(body["list"] as? String, "reserve")
            } else if request.url?.path == "/api/leagues/nba-1/lineup" {
                XCTAssertEqual(body["day"] as? String, "2026-10-26")
                XCTAssertEqual((body["entries"] as? [[String: Any]])?.first?["player_id"] as? String, "player")
            } else { XCTFail("Unexpected path") }
            return (204, Data())
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        try await api().changeRoster(for: selected, leagueID: "nba-1", action: "move",
                                     playerID: "player", list: "reserve", token: "token")
        try await api().saveLineup(for: selected, leagueID: "nba-1", day: "2026-10-26",
                                  entries: [LineupEntry(slot: "G", playerId: "player",
                                                        countsFrom: "", auto: true)], token: "token")
        XCTAssertEqual(paths, ["/api/dynasty", "/api/leagues/nba-1/roster/move",
                               "/api/dynasty", "/api/leagues/nba-1/lineup"])

        paths = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            return (200, Data("{\"id\":\"other\",\"name\":\"Other\",\"leagues\":[],\"franchises\":[]}".utf8))
        }
        do {
            try await api().changeRoster(for: selected, leagueID: "nba-1", action: "drop",
                                         playerID: "player", token: "token")
            XCTFail("Expected dynasty check")
        } catch let error as APIError {
            XCTAssertTrue(error.message.contains("not available"))
        }
        XCTAssertEqual(paths, ["/api/dynasty"])
    }

    func testLineupLocksAndPositionEligibility() throws {
        let lineup = try JSONDecoder.mosaic.decode(LineupData.self, from: Data("""
        {"day":"2026-10-26","last_day":"2026-10-26","locked":"",
         "slots":[{"name":"G","positions":["G"],"count":1,"games_per_week":0}],
         "players":[
           {"player_id":"a","full_name":"Locked","nickname":"","positions":["G"],"team_abbrev":"",
            "injury":"","slot":"G","counts_from":"","counts_last":false,"locked":true,
            "starter_eligible":true,"eligibility_note":"","points":0,"games":[]},
           {"player_id":"b","full_name":"Forward","nickname":"","positions":["F"],"team_abbrev":"",
            "injury":"","slot":"","counts_from":"","counts_last":false,"locked":false,
            "starter_eligible":true,"eligibility_note":"","points":0,"games":[]},
           {"player_id":"c","full_name":"Guard","nickname":"","positions":["G"],"team_abbrev":"",
            "injury":"","slot":"","counts_from":"","counts_last":false,"locked":false,
            "starter_eligible":true,"eligibility_note":"","points":0,"games":[]}],
         "starts":null,"doubled":null}
        """.utf8))
        var draft = LineupDraft(lineup)
        XCTAssertFalse(draft.weekly)
        XCTAssertFalse(draft.canPlace("b", in: 0, canEdit: true))
        XCTAssertFalse(draft.canPlace("c", in: 0, canEdit: true))
        draft.place("c", in: 0, canEdit: true)
        XCTAssertEqual(draft.slot(of: "a"), "G")
        XCTAssertFalse(draft.canPlace("a", in: nil, canEdit: true))
    }

    func testRivalFranchiseReadAndManagerActionsUseExistingRoutes() async throws {
        var paths: [String] = []
        MockURLProtocol.handler = { request in
            paths.append(request.url!.path)
            if request.url?.path == "/api/dynasty" {
                return (200, Data("""
                {"id":"dynasty-1","name":"Mosaic",
                 "leagues":[{"id":"nba-1","name":"NBA","competition":"nba"}],
                 "franchises":[{"id":"franchise-1","name":"Mine","slug":"mine"},
                               {"id":"franchise-2","name":"Rival","slug":"rival"}]}
                """.utf8))
            }
            if request.url?.path == "/api/franchises/rival" {
                XCTAssertEqual(request.httpMethod, "GET")
                return (200, Data("""
                {"franchise":{"id":"franchise-2","name":"Rival","slug":"rival"},
                 "rosters":[{"league_id":"nba-1","competition":"nba","name":"NBA",
                 "team_name":"Rival","limits":{"main":2,"reserve":1},"players":[],"season":null}],
                 "picks":[]}
                """.utf8))
            }
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer token")
            let body = try XCTUnwrap(self.bodyData(request).flatMap {
                try? JSONSerialization.jsonObject(with: $0) as? [String: Any]
            })
            XCTAssertNil(body["force"])
            XCTAssertNil(body["franchise_id"])
            switch request.url?.path {
            case "/api/franchises/franchise-1/leagues/nba-1/identity":
                XCTAssertEqual(body["name"] as? String, "")
                XCTAssertEqual(body["image_url"] as? String, "")
            case "/api/leagues/nba-1/waivers/claims":
                XCTAssertEqual(body["player_id"] as? String, "free-agent")
                XCTAssertEqual(body["bid"] as? Int, 3)
            case "/api/trade-market/targets":
                XCTAssertEqual(body["player_id"] as? String, "rival-player")
                XCTAssertEqual(body["on"] as? Bool, true)
            default: XCTFail("Unexpected request")
            }
            return (204, Data())
        }
        let selected = SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty)
        let rival = try await api().franchise(for: selected, franchiseID: "franchise-2", token: "token")
        XCTAssertEqual(rival.franchise.name, "Rival")
        try await api().updateIdentity(for: selected, leagueID: "nba-1", name: "", imageURL: "", token: "token")
        try await api().claimWaiver(for: selected, leagueID: "nba-1", playerID: "free-agent",
                                    dropPlayerID: nil, list: "reserve", bid: 3, token: "token")
        try await api().setTradeFlag(for: selected, leagueID: "nba-1", playerID: "rival-player",
                                     on: true, ownPlayer: false, token: "token")
        XCTAssertEqual(paths.filter { $0 != "/api/dynasty" },
                       ["/api/franchises/rival",
                        "/api/franchises/franchise-1/leagues/nba-1/identity",
                        "/api/leagues/nba-1/waivers/claims",
                        "/api/trade-market/targets"])
    }
}
