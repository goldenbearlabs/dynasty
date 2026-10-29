// Supplies synthetic account and league data for fast SwiftUI previews without API calls.
import Foundation

enum PreviewFixtures {
    static let api = APIClient(baseURL: URL(string: "https://preview.example/")!)

    @MainActor static var sessionStore: SessionStore { SessionStore(api: api) }

    @MainActor static var signedInStore: SessionStore {
        SessionStore(api: api, previewSession: SessionInfo(
            id: "franchise-1", name: "Northern Lights", managerName: "Ethan Carter",
            imageUrl: nil, email: "ethan@example.com", verified: true
        ))
    }

    static var leagues: [MyLeague] {
        [MyLeague(id: "dynasty-1", name: "Mosaic Dynasty", franchiseId: "franchise-1",
                  franchiseName: "Northern Lights", sports: ["nba", "nhl", "nfl", "mlb"], ready: true)]
    }

    static var dynasty: Dynasty {
        let data = """
        {"id":"dynasty-1","name":"Mosaic Dynasty",
         "settings":{"overall_title":{"enabled":true,"points_by_finish":[14,12,10]}},
         "leagues":[
          {"id":"nba-1","name":"NBA","competition":"nba","settings":{"format":{"type":"head_to_head","matchup_days":7,"playoff_teams":1,"standings_by":"record","tiebreakers":["points_for","head_to_head","points_against"]}}},
          {"id":"nhl-1","name":"NHL","competition":"nhl","settings":{"format":{"type":"total_points","playoff_teams":0}}},
          {"id":"cbb-1","name":"CBB","competition":"cbb","settings":{"format":{"type":"head_to_head","matchup_days":7,"playoff_teams":1}}}],
         "franchises":[{"id":"franchise-1","name":"Northern Lights","slug":"northern-lights","image_url":""},
                       {"id":"franchise-2","name":"Rival Club","slug":"rival-club","image_url":""}],
         "team_identities":[]}
        """
        return try! JSONDecoder.mosaic.decode(Dynasty.self, from: Data(data.utf8))
    }

    static var dashboard: DashboardData {
        let standings = """
        [{"league_id":"nba-1","competition":"nba","format":"head_to_head",
          "season":{"year":2026,"status":"active","starts_on":"2026-10-01","ends_on":"2027-04-01"},"rows":[
            {"franchise_id":"franchise-1","points_for":1182,"points_against":1064,"wins":8,"losses":3,"ties":0},
            {"franchise_id":"franchise-2","points_for":1107,"points_against":1120,"wins":6,"losses":5,"ties":0}]},
         {"league_id":"nhl-1","competition":"nhl","format":"total_points",
          "season":{"year":2026,"status":"complete","champion_franchise_id":"franchise-2","starts_on":"2026-01-01","ends_on":"2026-06-01"},"rows":[
            {"franchise_id":"franchise-2","points":900},{"franchise_id":"franchise-1","points":850}]},
         {"league_id":"cbb-1","competition":"cbb","format":"head_to_head","season":null,"rows":[]}]
        """
        let overall = """
        [{"year":2026,"final":false,"rows":[
          {"franchise_id":"franchise-1","points":42,"adjusted_points":41.5,"finishes":{"nba":1,"nhl":2},"wins":8,"losses":3,"ties":0},
          {"franchise_id":"franchise-2","points":36,"adjusted_points":36,"finishes":{"nba":2,"nhl":1},"wins":6,"losses":5,"ties":0}]}]
        """
        let activity = """
        [{"id":1,"kind":"add","competition":"nba","franchise_name":"Northern Lights",
          "player_name":"Sample Player","created_at":"2026-10-24T12:00:00Z"}]
        """
        let decoder = JSONDecoder.mosaic
        return DashboardData(
            standings: try! decoder.decode([StandingsTable].self, from: Data(standings.utf8)),
            overall: try! decoder.decode([OverallYear].self, from: Data(overall.utf8)),
            activity: try! decoder.decode([ActivityItem].self, from: Data(activity.utf8)),
            odds: try! decoder.decode(LeagueOdds.self, from: Data("""
            {"leagues":[{"league_id":"nba-1","competition":"nba","rows":[
              {"franchise_id":"franchise-1","playoffs":0.78,"wins":10.2,"losses":4.8,"clinched":false,"eliminated":false},
              {"franchise_id":"franchise-2","playoffs":0.22,"wins":8.4,"losses":6.6,"clinched":false,"eliminated":false}]}],
             "overall":[{"franchise_id":"franchise-1","title":0.64},{"franchise_id":"franchise-2","title":0.36}]}
            """.utf8))
        )
    }

    static var matchups: MatchupsData {
        let json = """
        {"periods":[
          {"id":"period-1","seq":1,"starts_on":"2026-10-12","ends_on":"2026-10-18","is_playoff":false,"by_hand":false},
          {"id":"period-2","seq":2,"starts_on":"2026-10-26","ends_on":"2026-11-01","is_playoff":false,"by_hand":false}],
         "period":{"id":"period-2","seq":2,"starts_on":"2026-10-26","ends_on":"2026-11-01","is_playoff":false,"by_hand":false},
         "matchups":[{"id":"matchup-1","bracket":"championship","bracket_slot":0,
           "home_franchise_id":"franchise-1","away_franchise_id":"franchise-2",
           "home_points":123.5,"away_points":118.2,"final":false}]}
        """
        return try! JSONDecoder.mosaic.decode(MatchupsData.self, from: Data(json.utf8))
    }

    static var franchiseDetail: FranchiseDetail {
        let json = """
        {"franchise":{"id":"franchise-1","name":"Northern Lights","slug":"northern-lights"},
         "rosters":[{"league_id":"nba-1","competition":"nba","name":"NBA","team_name":"Northern Lights",
          "limits":{"main":2,"reserve":2},"season":{"id":"season-1","year":2026,"starts_on":"2026-11-01",
          "set_at":null,"started":false,"deadline":"2026-10-31T18:00:00Z","expired":false},
          "players":[{"player_id":"one","full_name":"Alex North","list":"main","positions":["G"]},
                     {"player_id":"two","full_name":"Blake West","list":"main","positions":["F"]},
                     {"player_id":"three","full_name":"Casey Rivers","list":"reserve","positions":["C"]},
                     {"player_id":"unsigned","full_name":"Drew Future","list":"rights","positions":["G"]}]},
         {"league_id":"nhl-1","competition":"nhl","name":"NHL","team_name":"Northern Lights",
          "limits":{"main":18,"reserve":8},"season":null,"players":[]}]}
        """
        return try! JSONDecoder.mosaic.decode(FranchiseDetail.self, from: Data(json.utf8))
    }

    static var activeRoster: LeagueRoster {
        let json = """
        {"league_id":"nba-1","competition":"nba","name":"NBA","team_name":"Northern Lights",
         "image_url":"","limits":{"main":3,"reserve":2},"overage":0,
         "season":{"id":"season-1","year":2026,"starts_on":"2026-10-01","ends_on":"2027-04-01",
           "set_at":"2026-09-30T12:00:00Z","started":true,"locked":false,"deadline":null,"expired":false},
         "players":[
           {"player_id":"one","full_name":"Alex North","list":"main","positions":["G"],
            "nickname":"Ace","status":"active","team_abbrev":"NY"},
           {"player_id":"two","full_name":"Blake West","list":"main","positions":["F"],
            "status":"active","injury_designation":"OUT","team_abbrev":"LA"},
           {"player_id":"three","full_name":"Casey Rivers","list":"reserve","positions":["C"],
            "status":"prospect","team_abbrev":"CHI"}]}
        """
        return try! JSONDecoder.mosaic.decode(LeagueRoster.self, from: Data(json.utf8))
    }

    static var lineup: LineupData {
        let json = """
        {"day":"2026-10-26","last_day":"2026-11-01","locked":"",
         "slots":[{"name":"G","positions":["G"],"count":1,"games_per_week":1},
                  {"name":"F","positions":["F"],"count":1,"games_per_week":0}],
         "players":[
           {"player_id":"one","full_name":"Alex North","nickname":"Ace","positions":["G"],
            "team_abbrev":"NY","injury":"","slot":"G","counts_from":"","counts_last":true,
            "locked":false,"starter_eligible":true,"eligibility_note":"","points":21.5,
            "games":[{"game_id":"g1","day":"2026-10-27","starts_at":"2026-10-27T20:00:00Z",
              "status":"scheduled","opponent":"BOS","at_home":true,"points":0,"counts":false,
              "starter":"","projected":18.2,"expected":18.2,"stats":{}},
             {"game_id":"g2","day":"2026-10-30","starts_at":"2026-10-30T20:00:00Z",
              "status":"scheduled","opponent":"CHI","at_home":false,"points":0,"counts":false,
              "starter":"","projected":20.1,"expected":20.1,"stats":{}}]},
           {"player_id":"two","full_name":"Blake West","nickname":"","positions":["F"],
            "team_abbrev":"LA","injury":"OUT","slot":"F","counts_from":"","counts_last":false,
            "locked":false,"starter_eligible":true,"eligibility_note":"","points":0,"games":[]}],
         "starts":null,"doubled":null}
        """
        return try! JSONDecoder.mosaic.decode(LineupData.self, from: Data(json.utf8))
    }

    static var matchupDetail: MatchupDetail {
        let json = """
        {"id":"matchup-1","competition":"nba",
         "period":{"id":"period-2","seq":2,"starts_on":"2026-10-26","ends_on":"2026-11-01","is_playoff":false,"by_hand":false},
         "home_franchise_id":"franchise-1","away_franchise_id":"franchise-2",
         "home_points":123.5,"away_points":118.2,"final":false,
         "home_projected":178.2,"away_projected":170.8,
         "home_players":[{"player_id":"one","full_name":"Alex North","nickname":"Ace","points":24.3,
           "game_lines":[{"game_id":"g1","day":"2026-10-27","slot":"G","points":24.3,
             "stats":{"points":23,"assists":6},"starts_at":"2026-10-27T20:00:00Z",
             "status":"final","home":"NY","away":"BOS","positions":["G"]}]}],
         "away_players":[{"player_id":"rival-one","full_name":"Jordan Vale","points":22.1,
           "game_lines":[{"game_id":"g3","day":"2026-10-27","slot":"G","points":22.1,
             "stats":{"points":18,"assists":7},"starts_at":"2026-10-27T22:00:00Z",
             "status":"final","home":"LA","away":"CHI","positions":["G"]}]}]}
        """
        return try! JSONDecoder.mosaic.decode(MatchupDetail.self, from: Data(json.utf8))
    }

    static var matchupAwayLineup: LineupData {
        let json = """
        {"day":"2026-10-26","last_day":"2026-11-01","locked":"",
         "slots":[{"name":"G","positions":["G"],"count":1,"games_per_week":1},
                  {"name":"F","positions":["F"],"count":1,"games_per_week":0}],
         "players":[{"player_id":"rival-one","full_name":"Jordan Vale","nickname":"",
           "positions":["G"],"team_abbrev":"LA","injury":"","slot":"G",
           "counts_from":"","counts_last":false,"locked":false,"starter_eligible":true,
           "eligibility_note":"","points":22.1,
           "games":[{"game_id":"g3","day":"2026-10-27","starts_at":"2026-10-27T22:00:00Z",
             "status":"final","opponent":"CHI","at_home":true,"points":22.1,"counts":true,
             "starter":"","projected":0,"stats":{"points":18,"assists":7}}]},
          {"player_id":"rival-two","full_name":"Taylor Reed","nickname":"",
           "positions":["F"],"team_abbrev":"PHI","injury":"","slot":"F",
           "counts_from":"","counts_last":false,"locked":false,"starter_eligible":true,
           "eligibility_note":"","points":0,"games":[]}],"starts":null,"doubled":null}
        """
        return try! JSONDecoder.mosaic.decode(LineupData.self, from: Data(json.utf8))
    }
}
