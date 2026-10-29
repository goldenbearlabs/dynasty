// Compares matchup starters and official scoring with a native score card and box scores.
import SwiftUI

struct MatchupGameRow: Identifiable {
    let id: String
    let gameId: String?
    let atHome: Bool?
    let day: String
    let opponent: String
    let status: String
    let points: Double
    let projected: Double
    let counted: Bool
    let stats: [String: Double]
    let label: String
}

struct MatchupPlayerRow: Identifiable {
    let id: String
    let name: String
    let slot: String
    let injury: String
    let points: Double
    let projected: Double
    let games: [MatchupGameRow]
    let bench: Bool
    let empty: Bool
}

enum MatchupRows {
    static func make(lineup: LineupData?, scorers: [MatchupScorer], period: MatchupPeriod,
                     day: String, wholePeriod: Bool, weekly: Bool) -> [MatchupPlayerRow] {
        let first = wholePeriod ? period.startsOn : weekly ? lineup?.day ?? day : day
        let last = wholePeriod ? period.endsOn : weekly ? lineup?.lastDay ?? day : day
        let players = lineup?.players ?? []
        let scorerIDs = scorers.filter { ($0.gameLines ?? []).contains { $0.day >= first && $0.day <= last } }
            .map(\.playerId)
        let ids = Array(NSOrderedSet(array: players.map(\.playerId) + scorerIDs)) as? [String] ?? []
        let rows: [MatchupPlayerRow] = ids.compactMap { id in
            let player = players.first { $0.playerId == id }
            let scorer = scorers.first { $0.playerId == id }
            guard player != nil || scorer != nil else { return nil }
            let official = (scorer?.gameLines ?? []).filter { $0.day >= first && $0.day <= last }
            let games = (player?.games ?? []).filter { $0.day >= first && $0.day <= last }
            let bench = player?.slot.isEmpty ?? false
            let auto = player?.countsLast == true
            let autoGame = auto ? games.last(where: { $0.status == "scheduled" })?.gameId : nil
            var gameRows = games.map { game -> MatchupGameRow in
                let scored = official.first { $0.gameId == game.gameId }
                let picked = game.counts || (auto && game.gameId == autoGame)
                let projected = scored == nil && game.status == "scheduled" && !bench && picked
                    ? game.projected : 0
                return MatchupGameRow(
                    id: game.gameId ?? "\(id)-\(game.day)-\(game.opponent)",
                    gameId: game.gameId,
                    atHome: game.atHome,
                    day: game.day,
                    opponent: "\(game.atHome ? "vs" : "@") \(game.opponent)",
                    status: scored?.status ?? game.status,
                    points: scored?.points ?? (bench ? game.points : 0),
                    projected: projected,
                    counted: scored != nil,
                    stats: scored?.stats ?? game.stats ?? [:],
                    label: scored != nil ? "Counted" : bench ? "Bench · Not counted" : picked ? "Selected" : "Not selected"
                )
            }
            for scored in official where !gameRows.contains(where: { $0.id == scored.gameId }) {
                gameRows.append(MatchupGameRow(id: scored.gameId, gameId: scored.gameId,
                                               atHome: nil, day: scored.day,
                                               opponent: "\(scored.away) @ \(scored.home)",
                                               status: scored.status, points: scored.points,
                                               projected: 0, counted: true, stats: scored.stats,
                                               label: "Counted"))
            }
            gameRows.sort { $0.day < $1.day }
            let displayName: String
            if let nickname = player?.nickname, !nickname.isEmpty { displayName = nickname }
            else if let fullName = player?.fullName { displayName = fullName }
            else if let nickname = scorer?.nickname, !nickname.isEmpty { displayName = nickname }
            else { displayName = scorer?.fullName ?? "Player" }
            return MatchupPlayerRow(
                id: id, name: displayName,
                slot: bench ? "BN" : player?.slot ?? official.first?.slot ?? "",
                injury: player?.injury ?? "",
                points: bench ? gameRows.reduce(0) { $0 + $1.points } : official.reduce(0) { $0 + $1.points },
                projected: gameRows.reduce(0) { $0 + $1.projected }, games: gameRows,
                bench: bench, empty: false
            )
        }
        var output: [MatchupPlayerRow] = []
        var used = Set<String>()
        for slot in lineup?.slots ?? [] {
            for index in 0..<slot.count {
                if let row = rows.first(where: { $0.slot == slot.name && !$0.bench && !used.contains($0.id) }) {
                    output.append(row)
                    used.insert(row.id)
                } else {
                    output.append(MatchupPlayerRow(id: "empty-\(slot.name)-\(index)", name: "Empty",
                                                   slot: slot.name, injury: "", points: 0,
                                                   projected: 0, games: [], bench: false, empty: true))
                }
            }
        }
        output.append(contentsOf: rows.filter { !$0.bench && !used.contains($0.id) })
        output.append(contentsOf: rows.filter(\.bench))
        return output
    }

    static func visibleGames(_ row: MatchupPlayerRow, includeUncounted: Bool) -> [MatchupGameRow] {
        row.games.filter { row.bench || includeUncounted || $0.counted || $0.label == "Selected" }
    }
}

private struct MatchupSelection: Identifiable { let id: String }

struct MatchupDetailView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let league: SportLeague
    let matchup: MatchupCard
    let dynasty: Dynasty
    let standings: [StandingsTable]
    let revision: Int
    var preview = false

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var detail: MatchupDetail?
    @State private var lineups: [LineupData?] = [nil, nil]
    @State private var scoreboards: [String: GameSummary] = [:]
    @State private var day = ""
    @State private var wholePeriod = false
    @State private var boxScores = false
    @State private var showBench = false
    @State private var showUncounted = false
    @State private var loading = true
    @State private var loadingLineups = false
    @State private var error: String?
    @State private var selectedPlayer: MatchupSelection?
    @State private var selectedGame: MatchupSelection?

    private var weekly: Bool { league.settings?.lineup?.period == "week" }
    private var shown: MatchupDetail? { preview ? PreviewFixtures.matchupDetail : detail }
    private var ownAway: Bool { matchup.awayFranchiseId == selected.membership.franchiseId }
    private var ids: [String?] { ownAway ? [matchup.awayFranchiseId, matchup.homeFranchiseId] :
        [matchup.homeFranchiseId, matchup.awayFranchiseId] }
    private var players: [[MatchupScorer]] {
        guard let shown else { return [[], []] }
        return ownAway ? [shown.awayPlayers, shown.homePlayers] : [shown.homePlayers, shown.awayPlayers]
    }
    private var rows: [[MatchupPlayerRow]] {
        guard let shown else { return [[], []] }
        return (0..<2).map { index in
            MatchupRows.make(lineup: lineups[index], scorers: players[index], period: shown.period,
                             day: day, wholePeriod: wholePeriod, weekly: weekly)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            if loading && shown == nil {
                ProgressView("Loading matchup").frame(maxWidth: .infinity, alignment: .leading)
            } else if let error {
                StatePanel(symbol: "wifi.exclamationmark", title: "Could not load matchup",
                           detail: error, actionTitle: "Try again", action: { Task { await load() } })
            } else if let shown {
                scoreboard(shown)
                HStack {
                    Picker("Stats", selection: $wholePeriod) {
                        Text(weekly ? "Lineup week" : "Scoring day").tag(false)
                        Text("Matchup totals").tag(true)
                    }
                    .pickerStyle(.segmented)
                }
                if !wholePeriod { dateControls(shown.period) }
                HStack {
                    Text(boxScores ? "Box scores" : "Starters").font(.headline)
                    Spacer()
                    Button(boxScores ? "Side by side" : "Box scores") { boxScores.toggle() }
                        .font(.subheadline.weight(.semibold))
                }
                if loadingLineups { ProgressView("Updating lineups") }
                if boxScores { boxScoreContent } else { faceoffContent }
            }
        }
        .task(id: revision) { if preview {
                                  day = "2026-10-27"
                                  lineups = [PreviewFixtures.lineup, PreviewFixtures.matchupAwayLineup]
                                  loading = false
                              }
                              else { await load() } }
        .sheet(item: $selectedPlayer) { picked in
            NavigationStack {
                TeamPlayerProfileView(session: session, selected: selected,
                                      leagueID: league.id, playerID: picked.id)
            }
        }
        .sheet(item: $selectedGame) { picked in
            GameBoxScoreView(session: session, selected: selected,
                             league: league, gameID: picked.id)
        }
    }

    private func scoreboard(_ detail: MatchupDetail) -> some View {
        let totals = ownAway ? [detail.awayPoints, detail.homePoints] : [detail.homePoints, detail.awayPoints]
        let projected = ownAway ? [detail.awayProjected, detail.homeProjected] :
            [detail.homeProjected, detail.awayProjected]
        let status = detail.final ? "Final" : MatchupsLogic.today() < detail.period.startsOn
            ? "Upcoming" : "In progress"
        return VStack(spacing: 12) {
            HStack {
                Text(status).font(.caption.weight(.bold))
                    .foregroundStyle(detail.final ? MosaicPalette.soft : MosaicPalette.sport(league.competition))
                if detail.period.doubleGames == true {
                    Text("Double games").font(.caption).foregroundStyle(MosaicPalette.gold)
                }
                Spacer()
                SportTag(sport: league.competition)
            }
            HStack(alignment: .top, spacing: 8) {
                sideHeader(ids[0], points: totals[0], projected: projected[0], alignment: .leading)
                Text("vs").font(.caption.weight(.semibold)).foregroundStyle(MosaicPalette.faint)
                    .padding(.top, 24)
                sideHeader(ids[1], points: totals[1], projected: projected[1], alignment: .trailing)
            }
        }
        .padding(16)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
    }

    private func sideHeader(_ id: String?, points: Double, projected: Double?,
                            alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 5) {
            if let id {
                TeamCrest(name: name(id), imageURL: imageURL(id), baseURL: session.api.baseURL)
            }
            Text(id.map(name) ?? "Bye").font(.subheadline.weight(.bold))
                .lineLimit(2).minimumScaleFactor(0.8)
                .multilineTextAlignment(alignment == .leading ? .leading : .trailing)
            if let record = record(id) { Text(record).font(.caption).foregroundStyle(MosaicPalette.soft) }
            Text(id == nil ? "—" : StandingsOrder.points(points))
                .font(.title3.bold()).monospacedDigit()
            if let projected { Text("Proj \(StandingsOrder.points(projected))")
                    .font(.caption).foregroundStyle(MosaicPalette.soft).monospacedDigit() }
        }
        .foregroundStyle(MosaicPalette.ink)
        .frame(maxWidth: .infinity, alignment: alignment == .leading ? .leading : .trailing)
    }

    private func dateControls(_ period: MatchupPeriod) -> some View {
        HStack(spacing: 8) {
            Button { step(-1, period: period) } label: { Image(systemName: "chevron.left") }
                .disabled(day <= period.startsOn)
                .accessibilityLabel(weekly ? "Previous lineup week" : "Previous scoring day")
            Spacer(minLength: 0)
            if weekly {
                Text("\(lineups.compactMap { $0 }.first?.day ?? day) – \(lineups.compactMap { $0 }.first?.lastDay ?? day)")
                    .font(.subheadline.weight(.semibold)).lineLimit(2)
            } else {
                Menu {
                    ForEach(days(in: period), id: \.self) { date in
                        Button(date) { day = date; Task { await loadLineups() } }
                    }
                } label: { Text(day).font(.subheadline.weight(.semibold)) }
            }
            Spacer(minLength: 0)
            Button { step(1, period: period) } label: { Image(systemName: "chevron.right") }
                .disabled(day >= period.endsOn)
                .accessibilityLabel(weekly ? "Next lineup week" : "Next scoring day")
        }
        .buttonStyle(.bordered)
    }

    @ViewBuilder private var faceoffContent: some View {
        let left = rows[0].filter { !$0.bench }
        let right = rows[1].filter { !$0.bench }
        if left.isEmpty && right.isEmpty {
            Text("No starters or scored contributions in this view.")
                .font(.subheadline).foregroundStyle(MosaicPalette.soft)
        } else if dynamicTypeSize.isAccessibilitySize {
            ForEach(0..<2, id: \.self) { index in
                VStack(alignment: .leading, spacing: 8) {
                    Text(ids[index].map(name) ?? "Bye").font(.headline)
                    ForEach(rows[index].filter { !$0.bench }) { row in playerRow(row) }
                }
                .padding(14).background(MosaicPalette.surface,
                                        in: RoundedRectangle(cornerRadius: 15))
            }
        } else {
            VStack(spacing: 0) {
                HStack {
                    Text(ids[0].map(name) ?? "Bye").frame(maxWidth: .infinity, alignment: .leading)
                    Text("SLOT").font(.caption2).foregroundStyle(MosaicPalette.soft)
                    Text(ids[1].map(name) ?? "Bye").frame(maxWidth: .infinity, alignment: .trailing)
                }
                .font(.caption.weight(.semibold)).padding(12)
                ForEach(0..<max(left.count, right.count), id: \.self) { index in
                    let a: MatchupPlayerRow? = index < left.count ? left[index] : nil
                    let b: MatchupPlayerRow? = index < right.count ? right[index] : nil
                    HStack(alignment: .top, spacing: 4) {
                        faceCell(a, right: false)
                        Text(a?.slot ?? b?.slot ?? "")
                            .font(.caption2.weight(.bold))
                            .foregroundStyle(MosaicPalette.sport(league.competition))
                            .frame(width: 34).padding(.top, 3)
                        faceCell(b, right: true)
                    }
                    .padding(.horizontal, 10).padding(.vertical, 11)
                    if index < max(left.count, right.count) - 1 {
                        Divider().overlay(MosaicPalette.rule)
                    }
                }
                HStack {
                    Text("Starters \(StandingsOrder.points(left.reduce(0) { $0 + $1.points }))")
                    Spacer()
                    Text("\(StandingsOrder.points(right.reduce(0) { $0 + $1.points })) Starters")
                }
                .font(.caption.weight(.semibold)).monospacedDigit()
                .padding(12).background(MosaicPalette.raised)
            }
            .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
        }
        if rows.contains(where: { $0.contains(where: \.bench) }) {
            DisclosureGroup("Bench", isExpanded: $showBench) {
                ForEach(0..<2, id: \.self) { index in
                    VStack(alignment: .leading) {
                        Text(ids[index].map(name) ?? "Bye").font(.caption.weight(.semibold))
                        ForEach(rows[index].filter(\.bench)) { row in playerRow(row) }
                    }.padding(.top, 8)
                }
            }
            .padding(14).background(MosaicPalette.surface,
                                    in: RoundedRectangle(cornerRadius: 15))
        }
    }

    private func faceCell(_ row: MatchupPlayerRow?, right: Bool) -> some View {
        VStack(alignment: right ? .trailing : .leading, spacing: 3) {
            if let row {
                if row.empty { Text("Empty").foregroundStyle(MosaicPalette.soft) }
                else {
                    Button(row.name) { selectedPlayer = MatchupSelection(id: row.id) }
                        .font(.subheadline.weight(.semibold)).lineLimit(2)
                    if !row.injury.isEmpty {
                        Text(row.injury).font(.caption2).foregroundStyle(MosaicPalette.bad)
                    }
                    Text(row.points == 0 && row.projected > 0
                         ? "Proj \(StandingsOrder.points(row.projected))"
                         : StandingsOrder.points(row.points) +
                           (row.projected > 0 ? " +\(StandingsOrder.points(row.projected))" : ""))
                        .font(.caption.weight(.semibold)).monospacedDigit()
                    if let game = MatchupRows.visibleGames(row, includeUncounted: false).first {
                        Text("\(game.opponent) · \(gameState(game))")
                            .font(.caption2).foregroundStyle(MosaicPalette.soft).lineLimit(2)
                    }
                }
            } else { Text("—").foregroundStyle(MosaicPalette.faint) }
        }
        .frame(maxWidth: .infinity, alignment: right ? .trailing : .leading)
        .multilineTextAlignment(right ? .trailing : .leading)
    }

    private var boxScoreContent: some View {
        VStack(alignment: .leading, spacing: 12) {
            Toggle("Show uncounted games", isOn: $showUncounted).font(.subheadline)
            ForEach(0..<2, id: \.self) { index in
                VStack(alignment: .leading, spacing: 10) {
                    Text(ids[index].map(name) ?? "Bye").font(.headline)
                    if let starts = lineups[index]?.starts {
                        Text("Pitcher starts: \(starts.made.filter(\.counts).count) / \(starts.limit)")
                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                    if rows[index].isEmpty {
                        Text("No players in this view.").foregroundStyle(MosaicPalette.soft)
                    }
                    ForEach(rows[index]) { row in
                        if !row.empty {
                            DisclosureGroup {
                                let games = MatchupRows.visibleGames(row, includeUncounted: showUncounted)
                                if games.isEmpty { Text("No games in this view.").foregroundStyle(MosaicPalette.soft) }
                                ForEach(games) { game in
                                    VStack(alignment: .leading, spacing: 4) {
                                        Button("\(game.day) · \(game.opponent)") {
                                            if let gameID = game.gameId {
                                                selectedGame = MatchupSelection(id: gameID)
                                            }
                                        }
                                        .disabled(game.gameId == nil)
                                        Text("\(game.label) · \(gameState(game))")
                                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                                        if !game.stats.isEmpty {
                                            Text(game.stats.sorted { $0.key < $1.key }
                                                .map { "\($0.key.replacingOccurrences(of: "_", with: " ")): \(StandingsOrder.points($0.value))" }
                                                .joined(separator: " · "))
                                                .font(.caption2).foregroundStyle(MosaicPalette.soft)
                                        }
                                        Text("\(StandingsOrder.points(game.points)) FP" +
                                             (game.projected > 0 ? " · Proj \(StandingsOrder.points(game.projected))" : ""))
                                            .font(.caption.weight(.semibold)).monospacedDigit()
                                    }
                                    .padding(.vertical, 5)
                                }
                            } label: {
                                HStack {
                                    Text("\(row.slot) · \(row.name)")
                                    Spacer()
                                    Text("\(StandingsOrder.points(row.points)) FP")
                                        .monospacedDigit()
                                }
                                .font(.subheadline)
                            }
                        }
                    }
                }
                .padding(14)
                .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 15))
            }
            Text("FP counts only games credited to the starting lineup. Bench and uncounted games do not change the matchup score.")
                .font(.caption).foregroundStyle(MosaicPalette.soft)
        }
    }

    private func playerRow(_ row: MatchupPlayerRow) -> some View {
        HStack {
            Text(row.slot).font(.caption.weight(.bold)).foregroundStyle(MosaicPalette.sport(league.competition))
            if row.empty { Text("Empty").foregroundStyle(MosaicPalette.soft) }
            else { Button(row.name) { selectedPlayer = MatchupSelection(id: row.id) } }
            Spacer()
            Text(StandingsOrder.points(row.points)).monospacedDigit()
        }
        .font(.subheadline).foregroundStyle(MosaicPalette.ink)
    }

    private func gameState(_ game: MatchupGameRow) -> String {
        guard let current = scoreboards[game.id], current.status != "scheduled" else {
            return game.status == "final" ? "Final" : game.status == "live" ? "Live" : "Scheduled"
        }
        let score = game.atHome == true ? "\(current.homeScore)–\(current.awayScore)" :
            "\(current.awayScore)–\(current.homeScore)"
        return "\(score) · \(current.detail.isEmpty ? current.status.capitalized : current.detail)"
    }

    private func name(_ id: String) -> String {
        let team = dynasty.teamIdentities.first { $0.franchiseId == id && $0.leagueId == league.id }
        return team?.name.isEmpty == false ? team!.name :
            dynasty.franchises.first { $0.id == id }?.name ?? "Franchise"
    }

    private func imageURL(_ id: String) -> String? {
        let team = dynasty.teamIdentities.first { $0.franchiseId == id && $0.leagueId == league.id }
        return team?.imageUrl?.isEmpty == false ? team?.imageUrl :
            dynasty.franchises.first { $0.id == id }?.imageUrl
    }

    private func record(_ id: String?) -> String? {
        guard let id, let row = standings.first(where: { $0.leagueId == league.id })?.rows.first(where: { $0.franchiseId == id }),
              let wins = row.wins, let losses = row.losses else { return nil }
        return StandingsOrder.record(wins, losses, row.ties ?? 0)
    }

    private func days(in period: MatchupPeriod) -> [String] {
        var result: [String] = []
        var date = period.startsOn
        while date <= period.endsOn && result.count < 40 {
            result.append(date)
            date = shifted(date, days: 1)
        }
        return result
    }

    private func shifted(_ day: String, days: Int) -> String {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd"
        guard let value = formatter.date(from: day),
              let next = Calendar.current.date(byAdding: .day, value: days, to: value) else { return day }
        return formatter.string(from: next)
    }

    private func step(_ direction: Int, period: MatchupPeriod) {
        let current = lineups.compactMap { $0 }.first
        let pivot = weekly ? (direction < 0 ? current?.day ?? day : current?.lastDay ?? day) : day
        let next = shifted(pivot, days: direction)
        day = min(max(next, period.startsOn), period.endsOn)
        Task { await loadLineups() }
    }

    @MainActor private func load() async {
        guard let token = session.token else { return }
        loading = true
        error = nil
        do {
            let fetched = try await session.api.matchupDetail(for: selected, leagueID: league.id,
                                                               matchupID: matchup.id, token: token)
            detail = fetched
            if day.isEmpty {
                day = min(max(MatchupsLogic.today(), fetched.period.startsOn), fetched.period.endsOn)
            }
            await loadLineups()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
        loading = false
    }

    @MainActor private func loadLineups() async {
        guard let token = session.token, let detail else { return }
        loadingLineups = true
        defer { loadingLineups = false }
        do {
            let home = try await session.api.lineup(for: selected, leagueID: league.id,
                                                    franchiseID: detail.homeFranchiseId,
                                                    day: day, token: token)
            var away: LineupData?
            if let awayID = detail.awayFranchiseId {
                away = try await session.api.lineup(for: selected, leagueID: league.id,
                                                    franchiseID: awayID, day: day, token: token)
            }
            lineups = ownAway ? [away, home] : [home, away]
            var scores: [String: GameSummary] = [:]
            let today = MatchupsLogic.today()
            if today >= detail.period.startsOn && today <= detail.period.endsOn,
               let board = try? await session.api.scores(for: selected, leagueID: league.id,
                                                          day: today, token: token) {
                for game in board.games { scores[game.id] = game }
            }
            scoreboards = scores
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

struct GameBoxScoreView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let league: SportLeague
    let gameID: String

    @State private var game: GameBoxScore?
    @State private var error: String?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    if let game {
                        HStack {
                            Text(game.awayName)
                            Spacer()
                            Text("\(game.awayScore) – \(game.homeScore)").font(.headline.monospacedDigit())
                            Spacer()
                            Text(game.homeName)
                        }
                        .font(.subheadline.weight(.semibold))
                        Text(game.detail.isEmpty ? game.status.capitalized : game.detail)
                            .foregroundStyle(MosaicPalette.soft)
                        ForEach(game.lines) { line in
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(line.fullName).font(.subheadline.weight(.semibold))
                                    Spacer()
                                    Text("\(StandingsOrder.points(line.points)) FP").monospacedDigit()
                                }
                                Text(line.ownerName).font(.caption).foregroundStyle(MosaicPalette.soft)
                                Text(line.stats.sorted { $0.key < $1.key }
                                    .map { "\($0.key.replacingOccurrences(of: "_", with: " ")): \(StandingsOrder.points($0.value))" }
                                    .joined(separator: " · "))
                                    .font(.caption).foregroundStyle(MosaicPalette.soft)
                            }
                            .padding(12)
                            .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 12))
                        }
                    } else if let error {
                        StatePanel(symbol: "wifi.exclamationmark", title: "Could not load game",
                                   detail: error, actionTitle: "Try again",
                                   action: { Task { await load() } })
                    } else { ProgressView("Loading game") }
                }
                .padding(20)
            }
            .background(MosaicPalette.ground)
            .navigationTitle("Game box score")
            .navigationBarTitleDisplayMode(.inline)
        }
        .preferredColorScheme(.dark)
        .task { await load() }
    }

    @MainActor private func load() async {
        guard let token = session.token else { return }
        error = nil
        do {
            game = try await session.api.gameDetail(for: selected, leagueID: league.id,
                                                     gameID: gameID, token: token)
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

#Preview("Matchup detail") {
    ScrollView {
        MatchupDetailView(session: PreviewFixtures.signedInStore,
                          selected: SelectedLeague(membership: PreviewFixtures.leagues[0],
                                                   dynasty: PreviewFixtures.dynasty),
                          league: PreviewFixtures.dynasty.leagues[0],
                          matchup: PreviewFixtures.matchups.matchups[0],
                          dynasty: PreviewFixtures.dynasty,
                          standings: PreviewFixtures.dashboard.standings,
                          revision: 0, preview: true)
            .padding(20)
    }
    .background(MosaicPalette.ground)
    .preferredColorScheme(.dark)
}
