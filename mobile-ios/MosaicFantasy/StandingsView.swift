// Presents the website's sortable sport and overall standings in readable iPhone rows.
import SwiftUI
import UIKit

struct StandingsView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let data: DashboardData
    @Binding var shown: String
    @Binding var activitySport: String
    var refreshKey = 0
    var preview = false

    @State private var sortKey = "place"
    @State private var descending = false
    @State private var selectedFranchise: StandingFranchiseSelection?

    private var tables: [StandingsTable] { StandingsOrder.sports(data.standings) }
    private var tabs: [String] { StandingsOrder.tabs(data.standings) }
    private var current: String {
        if tabs.contains(shown) { return shown }
        return StandingsOrder.initialTab(tables: data.standings, overall: data.overall)
    }
    private var table: StandingsTable? { tables.first(where: { $0.competition == current }) }
    private var sportLeague: SportLeague? { selected.dynasty.leagues.first(where: { $0.id == table?.leagueId }) }

    var body: some View {
        VStack(alignment: .leading, spacing: 15) {
            if tabs.isEmpty {
                StatePanel(symbol: "list.number", title: "No standings yet",
                           detail: "Standings appear when a sport season begins.")
            } else {
                HStack(spacing: 8) {
                    pill("overall").padding(.leading, 4)
                    if tabs.count > 1 {
                        ScrollView(.horizontal) {
                            HStack(spacing: 8) {
                                ForEach(tabs.filter { $0 != "overall" }, id: \.self) { tab in
                                    pill(tab).frame(minWidth: 88)
                                }
                            }
                            .padding(.leading, 4)
                            .padding(.trailing, 28)
                            .padding(.vertical, 3)
                        }
                        .scrollIndicators(.hidden)
                        .accessibilityLabel("Sport standings tabs")
                    }
                }
                if current == "overall" {
                    if let year = data.overall.first {
                        overallContent(year)
                    } else {
                        StatePanel(symbol: "trophy", title: "No overall standings yet",
                                   detail: selected.dynasty.settings?.overallTitle?.enabled == false
                                   ? "This league does not currently have an overall title."
                                   : "Overall standings will appear when sport seasons begin.")
                    }
                } else if let table {
                    sportContent(table)
                }
            }
        }
        .onChange(of: current) { _, _ in sortKey = "place"; descending = false }
        .sheet(item: $selectedFranchise) { choice in
            FranchiseStandingSheet(selected: selected, data: data, franchiseID: choice.id,
                                   baseURL: session.api.baseURL)
                .presentationDetents([.medium, .large])
        }
    }

    private func pill(_ tab: String) -> some View {
        let active = current == tab
        return Button {
            shown = tab
            activitySport = tab == "overall" ? "all" : tab
        } label: {
            HStack(spacing: 8) {
                if tab != "overall" { sportDot(tab) }
                Text(tab == "overall" ? "Overall" : tab.uppercased())
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(active ? MosaicPalette.ink : MosaicPalette.soft)
            }
            .padding(.horizontal, 15)
            .frame(minHeight: 44)
            .background(active ? MosaicPalette.raised : MosaicPalette.surface, in: Capsule())
            .overlay(Capsule().stroke(active ? MosaicPalette.brand.opacity(0.75) : .clear, lineWidth: 1))
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(active ? .isSelected : [])
        .accessibilityLabel(tab == "overall" ? "Overall standings" : "\(tab.uppercased()) standings")
    }

    private func sportDot(_ sport: String) -> some View {
        let color = MosaicPalette.sport(sport)
        return Circle().fill(color).frame(width: 8, height: 8)
            .shadow(color: color.opacity(0.55), radius: 6)
            .accessibilityHidden(true)
    }

    private func overallContent(_ year: OverallYear) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Text("\(String(year.year)) overall")
                if year.finalYear { Text("Final").foregroundStyle(MosaicPalette.brand) }
            }
            .font(.subheadline.weight(.medium))
            .foregroundStyle(MosaicPalette.soft)
            if year.rows.isEmpty {
                StatePanel(symbol: "chart.bar", title: "No overall points yet",
                           detail: "Results will appear as seasons progress.")
            } else {
                sortControl(overall: true, hasOdds: data.odds != nil && !year.finalYear)
                let ranked = StandingsOrder.sorted(year.rows, key: sortKey, descending: descending) { row, place, key in
                    switch key {
                    case "name": return .text(identity(row.franchiseId).name)
                    case "record": return .number(StandingsOrder.winShare(row.wins, row.losses, row.ties))
                    case "adjusted": return .number(row.adjustedPoints)
                    case "points": return .number(row.points)
                    case "title": return .number(titleOdds(row.franchiseId)?.title ?? 0)
                    default:
                        if key.hasPrefix("finish:") { return .number(Double(row.finishes?[String(key.dropFirst(7))] ?? 99)) }
                        return .number(Double(place))
                    }
                }
                VStack(spacing: 0) {
                    ForEach(ranked, id: \.row.franchiseId) { entry in
                        overallRow(entry.row, place: entry.place, year: year)
                        if entry.row.franchiseId != ranked.last?.row.franchiseId { Divider().overlay(MosaicPalette.rule) }
                    }
                }
                .padding(.horizontal, 16)
                .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
            }
            overallExplanation(year)
        }
    }

    private func overallRow(_ row: OverallRow, place: Int, year: OverallYear) -> some View {
        Button { selectedFranchise = StandingFranchiseSelection(id: row.franchiseId) } label: {
            VStack(alignment: .leading, spacing: 12) {
                HStack(spacing: 10) {
                    rank(place)
                    TeamCrest(name: identity(row.franchiseId).name,
                              imageURL: identity(row.franchiseId).imageURL,
                              baseURL: session.api.baseURL)
                    Text(identity(row.franchiseId).name)
                        .font(.body.weight(.semibold))
                        .multilineTextAlignment(.leading)
                    Spacer(minLength: 4)
                    Text("\(StandingsOrder.points(row.points)) pts")
                        .font(.body.weight(.bold)).monospacedDigit()
                }
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 120), alignment: .leading)],
                          alignment: .leading, spacing: 10) {
                    stat("Record", StandingsOrder.record(row.wins, row.losses, row.ties))
                    stat("Adj. PF", StandingsOrder.signedPoints(row.adjustedPoints))
                    if !year.finalYear, data.odds != nil {
                        stat("Title chance", StandingsOrder.percent(titleOdds(row.franchiseId)?.title ?? 0))
                    }
                }
                if !data.standings.isEmpty {
                    ScrollView(.horizontal) {
                        HStack(spacing: 12) {
                            ForEach(data.standings, id: \.leagueId) { table in
                                HStack(spacing: 5) {
                                    sportDot(table.competition)
                                    Text("\(table.competition.uppercased()) \(row.finishes?[table.competition].map(String.init) ?? "—")")
                                        .font(.caption.weight(.medium))
                                        .foregroundStyle(MosaicPalette.soft)
                                }
                            }
                        }
                    }
                    .scrollIndicators(.hidden)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(.vertical, 14)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityHint("Show franchise standings")
    }

    private func sportContent(_ table: StandingsTable) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            if table.season == nil {
                StatePanel(symbol: "trophy", title: "No season yet",
                           detail: "The commissioner starts this season. Points count from its first day.")
            } else {
                if let champion = table.season?.championFranchiseId,
                   selected.dynasty.franchises.contains(where: { $0.id == champion }) {
                    Label("\(identity(champion, leagueID: table.leagueId).name) won the \(String(table.season?.year ?? 0)) title.",
                          systemImage: "trophy.fill")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(MosaicPalette.ink)
                        .padding(15)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(MosaicPalette.raised, in: RoundedRectangle(cornerRadius: 14))
                }
                if let consolation = table.consolationChampionId,
                   selected.dynasty.franchises.contains(where: { $0.id == consolation }) {
                    Text("\(identity(consolation, leagueID: table.leagueId).name) won the consolation bracket.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                }
                if table.rows.isEmpty {
                    StatePanel(symbol: "chart.bar", title: "No standings yet",
                               detail: "Results will appear after games are scored.")
                } else {
                    let odds = playoffOdds(for: table)
                    sortControl(overall: false, hasOdds: odds != nil)
                    let ranked = StandingsOrder.sorted(table.rows, key: sortKey, descending: descending) { row, place, key in
                        switch key {
                        case "name": return .text(identity(row.franchiseId, leagueID: table.leagueId).name)
                        case "record": return .number(StandingsOrder.winShare(row.wins, row.losses, row.ties))
                        case "points": return .number(table.format == "head_to_head" ? row.pointsFor ?? 0 : row.points ?? 0)
                        case "against": return .number(row.pointsAgainst ?? 0)
                        case "playoffs": return .number(odds?.rows.first(where: { $0.franchiseId == row.franchiseId })?.playoffs ?? 0)
                        default: return .number(Double(place))
                        }
                    }
                    let cutoff = table.format == "head_to_head" ? sportLeague?.settings?.format?.playoffTeams ?? 0 : 0
                    VStack(spacing: 0) {
                        ForEach(ranked, id: \.row.franchiseId) { entry in
                            sportRow(entry.row, place: entry.place, table: table, odds: odds)
                            if sortKey == "place", !descending, entry.place == cutoff, cutoff < table.rows.count {
                                Rectangle().fill(MosaicPalette.sport(table.competition)).frame(height: 2)
                                Text("Playoff cutoff").font(.caption2).foregroundStyle(MosaicPalette.soft)
                                    .frame(maxWidth: .infinity, alignment: .leading).padding(.vertical, 5)
                            } else if entry.row.franchiseId != ranked.last?.row.franchiseId {
                                Divider().overlay(MosaicPalette.rule)
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
                }
                sportExplanation(table)
                if table.format == "head_to_head", let league = sportLeague {
                    MatchupsSection(session: session, selected: selected, league: league,
                                    refreshKey: refreshKey, preview: preview)
                }
            }
        }
    }

    private func sportRow(_ row: StandingsRow, place: Int, table: StandingsTable, odds: SportOdds?) -> some View {
        let chance = odds?.rows.first(where: { $0.franchiseId == row.franchiseId })
        return Button { selectedFranchise = StandingFranchiseSelection(id: row.franchiseId) } label: {
            VStack(alignment: .leading, spacing: 11) {
                HStack(spacing: 10) {
                    rank(place)
                    TeamCrest(name: identity(row.franchiseId, leagueID: table.leagueId).name,
                              imageURL: identity(row.franchiseId, leagueID: table.leagueId).imageURL,
                              baseURL: session.api.baseURL)
                    Text(identity(row.franchiseId, leagueID: table.leagueId).name)
                        .font(.body.weight(.semibold)).multilineTextAlignment(.leading)
                    Spacer(minLength: 4)
                    Text("\(StandingsOrder.points(table.format == "head_to_head" ? row.pointsFor ?? 0 : row.points ?? 0)) pts")
                        .font(.body.weight(.bold)).monospacedDigit()
                }
                if table.format == "head_to_head" {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 112), alignment: .leading)],
                              alignment: .leading, spacing: 10) {
                        stat("Record", StandingsOrder.record(row.wins, row.losses, row.ties))
                        stat("Points against", StandingsOrder.points(row.pointsAgainst ?? 0))
                        if odds != nil { stat("Playoffs", chance.map { StandingsOrder.percent($0.playoffs) } ?? "—") }
                    }
                    if let chance {
                        let status = chance.clinched == true ? "Clinched" : chance.eliminated == true ? "Eliminated" : "Projected"
                        Text("\(status): \(StandingsOrder.points(chance.wins ?? 0))–\(StandingsOrder.points(chance.losses ?? 0)) projected record")
                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                }
            }
            .padding(.vertical, 14)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityHint("Show franchise standings")
    }

    private func sortControl(overall: Bool, hasOdds: Bool) -> some View {
        let options = sortOptions(overall: overall, hasOdds: hasOdds)
        return HStack(spacing: 8) {
            Menu {
                ForEach(options, id: \.self) { key in
                    Button(sortLabel(key)) { chooseSort(key) }
                }
            } label: {
                Label("Sort: \(sortLabel(sortKey))", systemImage: "arrow.up.arrow.down")
                    .lineLimit(1)
            }
            .accessibilityLabel("Sort standings by \(sortLabel(sortKey))")
            Button { descending.toggle() } label: {
                Image(systemName: descending ? "arrow.down" : "arrow.up")
            }
            .accessibilityLabel(descending ? "Sort descending" : "Sort ascending")
        }
        .font(.subheadline)
        .buttonStyle(.bordered)
    }

    private func sortOptions(overall: Bool, hasOdds: Bool) -> [String] {
        var options = ["place", "name"]
        if overall {
            options.append(contentsOf: data.standings.map { "finish:\($0.competition)" })
            options.append(contentsOf: ["record", "adjusted", "points"])
            if hasOdds { options.append("title") }
        } else {
            if table?.format == "head_to_head" { options.append("record") }
            options.append("points")
            if table?.format == "head_to_head" { options.append("against") }
            if hasOdds { options.append("playoffs") }
        }
        return options
    }

    private func chooseSort(_ key: String) {
        if key == sortKey { descending.toggle() }
        else {
            sortKey = key
            descending = !(["place", "name"].contains(key) || key.hasPrefix("finish:"))
        }
    }

    private func sortLabel(_ key: String) -> String {
        switch key {
        case "place": return "Rank"
        case "name": return "Franchise"
        case "record": return "Record"
        case "adjusted": return "Adjusted PF"
        case "points": return "Points"
        case "against": return "Points against"
        case "playoffs": return "Playoff chance"
        case "title": return "Title chance"
        default: return key.hasPrefix("finish:") ? "\(String(key.dropFirst(7)).uppercased()) finish" : key
        }
    }

    private func stat(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(MosaicPalette.soft)
            Text(value).font(.subheadline.weight(.semibold)).monospacedDigit().foregroundStyle(MosaicPalette.ink)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func rank(_ place: Int) -> some View {
        Text("\(place)").font(.caption.monospacedDigit().weight(.bold))
            .foregroundStyle(MosaicPalette.brand).frame(width: 22, alignment: .leading)
    }

    private func titleOdds(_ id: String) -> TitleOdds? {
        data.odds?.overall.first(where: { $0.franchiseId == id })
    }

    private func playoffOdds(for table: StandingsTable) -> SportOdds? {
        guard table.format == "head_to_head", table.season?.status == "active" else { return nil }
        return data.odds?.leagues.first(where: { $0.leagueId == table.leagueId })
    }

    private func identity(_ franchiseID: String, leagueID: String? = nil) -> (name: String, imageURL: String?) {
        let franchise = selected.dynasty.franchises.first(where: { $0.id == franchiseID })
        let team = selected.dynasty.teamIdentities.first(where: { $0.franchiseId == franchiseID && $0.leagueId == leagueID })
        let teamName = team?.name.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return (teamName.isEmpty ? franchise?.name ?? "Franchise" : teamName,
                team?.imageUrl?.isEmpty == false ? team?.imageUrl : franchise?.imageUrl)
    }

    private func overallExplanation(_ year: OverallYear) -> some View {
        let finishPoints = selected.dynasty.settings?.overallTitle?.pointsByFinish ?? []
        let rules = finishPoints.enumerated().map { "\($0.element) for \(StandingsOrder.ordinal($0.offset + 1))" }
        return Text("\(String(year.year)) overall title, \(year.finalYear ? "final" : "in progress"). Points come from each sport's finish\(rules.isEmpty ? "" : " (" + rules.joined(separator: ", ") + ")"). Ties are broken by adjusted points for: points above each sport's median, added up.\(data.odds != nil && !year.finalYear ? " Title chances combine simulated seasons." : "")")
            .font(.caption).foregroundStyle(MosaicPalette.soft)
    }

    private func sportExplanation(_ table: StandingsTable) -> some View {
        let format = sportLeague?.settings?.format
        let ranking: String
        switch format?.standingsBy {
        case "win_percentage": ranking = "win percentage"
        case "points": ranking = "points for"
        default: ranking = "wins, then fewer losses"
        }
        let tie = (format?.tiebreakers ?? ["points_for", "head_to_head", "points_against"]).map {
            switch $0 {
            case "points_for": "points for"
            case "head_to_head": "head-to-head"
            default: "points against (higher first)"
            }
        }.joined(separator: ", ")
        let year = table.season?.year.map(String.init) ?? "Current"
        let status = table.season?.status == "complete" ? "final" : "in progress"
        let cutoff = table.format == "head_to_head" ? format?.playoffTeams ?? 0 : 0
        let oddsText = playoffOdds(for: table) == nil ? "" : " Playoff chances come from playing out the rest of the season thousands of times. Results so far are fixed; later matchups are less certain. Only clinched or eliminated teams show 100% or 0%."
        let recordText = table.format == "head_to_head" ? " Ranked by \(ranking); tiebreakers: \(tie)." : ""
        return Text("\(year) season, \(status). Only starters score.\(cutoff > 0 ? " The line marks the playoff cutoff." : "")\(oddsText)\(recordText)")
            .font(.caption).foregroundStyle(MosaicPalette.soft)
    }
}

private struct StandingFranchiseSelection: Identifiable { let id: String }

struct TeamCrest: View {
    let name: String
    let imageURL: String?
    let baseURL: URL

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 9).fill(MosaicPalette.raised)
            Text(String(name.split(separator: " ").prefix(2).compactMap(\.first)).uppercased())
                .font(.caption2.weight(.bold)).foregroundStyle(MosaicPalette.ink)
            if let imageURL, imageURL.hasPrefix("data:image/"),
               let encoded = imageURL.split(separator: ",", maxSplits: 1).last,
               let data = Data(base64Encoded: String(encoded)),
               let image = UIImage(data: data) {
                Image(uiImage: image).resizable().scaledToFill()
            } else if let imageURL, !imageURL.isEmpty,
               let url = URL(string: imageURL, relativeTo: baseURL)?.absoluteURL {
                AsyncImage(url: url) { image in image.resizable().scaledToFill() } placeholder: { Color.clear }
            }
        }
        .frame(width: 30, height: 30)
        .clipShape(RoundedRectangle(cornerRadius: 9))
        .accessibilityHidden(true)
    }
}

private struct FranchiseStandingSheet: View {
    @Environment(\.dismiss) private var dismiss
    let selected: SelectedLeague
    let data: DashboardData
    let franchiseID: String
    let baseURL: URL

    private var franchise: DynastyFranchise? { selected.dynasty.franchises.first(where: { $0.id == franchiseID }) }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    HStack(spacing: 12) {
                        TeamCrest(name: franchise?.name ?? "Franchise", imageURL: franchise?.imageUrl, baseURL: baseURL)
                        Text(franchise?.name ?? "Franchise").font(.title3.bold())
                    }
                    .padding(.vertical, 8)
                }
                if let year = data.overall.first,
                   let row = year.rows.first(where: { $0.franchiseId == franchiseID }) {
                    Section("\(String(year.year)) overall") {
                        LabeledContent("Rank", value: "\((year.rows.firstIndex(where: { $0.franchiseId == franchiseID }) ?? 0) + 1)")
                        LabeledContent("Points", value: StandingsOrder.points(row.points))
                        LabeledContent("Adjusted PF", value: StandingsOrder.signedPoints(row.adjustedPoints))
                    }
                }
                Section("Sports") {
                    ForEach(data.standings, id: \.leagueId) { table in
                        if let row = table.rows.first(where: { $0.franchiseId == franchiseID }) {
                            HStack {
                                SportTag(sport: table.competition)
                                Spacer()
                                Text("#\((table.rows.firstIndex(where: { $0.franchiseId == franchiseID }) ?? 0) + 1) · \(StandingsOrder.points(table.format == "head_to_head" ? row.pointsFor ?? 0 : row.points ?? 0)) pts")
                                    .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                            }
                        }
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .background(MosaicPalette.ground)
            .navigationTitle("Franchise")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .topBarTrailing) { Button("Done") { dismiss() } } }
        }
        .preferredColorScheme(.dark)
    }
}

enum StandingsOrder {
    enum Value {
        case text(String)
        case number(Double)
    }

    struct Ranked<Row> {
        let row: Row
        let place: Int
    }

    static func sports(_ tables: [StandingsTable], today: Date = Date()) -> [StandingsTable] {
        tables.sorted { progress($0, today: today) > progress($1, today: today) }
    }

    static func tabs(_ tables: [StandingsTable]) -> [String] {
        ["overall"] + sports(tables).map(\.competition)
    }

    static func initialTab(tables: [StandingsTable], overall: [OverallYear]) -> String {
        if !overall.isEmpty { return "overall" }
        return sports(tables).first?.competition ?? "overall"
    }

    private static func progress(_ table: StandingsTable, today: Date) -> Double {
        guard let season = table.season else { return -3 }
        if season.status == "complete" { return -2 }
        let format = DateFormatter()
        format.dateFormat = "yyyy-MM-dd"
        format.locale = Locale(identifier: "en_CA")
        guard let start = season.startsOn.flatMap(format.date(from:)),
              let end = season.endsOn.flatMap(format.date(from:)) else { return 0 }
        if today < start { return -1 - start.timeIntervalSince(today) / 1e9 }
        return min(1, max(0, today.timeIntervalSince(start) / max(1, end.timeIntervalSince(start))))
    }

    static func sorted<Row>(_ rows: [Row], key: String, descending: Bool,
                            value: (Row, Int, String) -> Value) -> [Ranked<Row>] {
        let ranked = rows.enumerated().map { Ranked(row: $0.element, place: $0.offset + 1) }
        if key == "place" && !descending { return ranked }
        return ranked.sorted { left, right in
            let a = value(left.row, left.place, key)
            let b = value(right.row, right.place, key)
            let result: ComparisonResult
            switch (a, b) {
            case let (.text(x), .text(y)):
                result = x.localizedCaseInsensitiveCompare(y)
            case let (.number(x), .number(y)):
                result = x == y ? .orderedSame : (x < y ? .orderedAscending : .orderedDescending)
            default: result = .orderedSame
            }
            if result == .orderedSame { return left.place < right.place }
            return descending ? result == .orderedDescending : result == .orderedAscending
        }
    }

    static func winShare(_ wins: Int?, _ losses: Int?, _ ties: Int?) -> Double {
        let games = (wins ?? 0) + (losses ?? 0) + (ties ?? 0)
        return games == 0 ? 0 : (Double(wins ?? 0) + Double(ties ?? 0) / 2) / Double(games)
    }

    static func record(_ wins: Int?, _ losses: Int?, _ ties: Int?) -> String {
        "\(wins ?? 0)-\(losses ?? 0)" + ((ties ?? 0) == 0 ? "" : "-\(ties ?? 0)")
    }

    static func points(_ value: Double) -> String {
        value.rounded() == value ? String(format: "%.0f", value) : String(format: "%.1f", value)
    }

    static func signedPoints(_ value: Double) -> String {
        (value > 0 ? "+" : "") + points(value)
    }

    static func ordinal(_ place: Int) -> String {
        let suffix: String
        if (11...13).contains(place % 100) { suffix = "th" }
        else {
            switch place % 10 {
            case 1: suffix = "st"
            case 2: suffix = "nd"
            case 3: suffix = "rd"
            default: suffix = "th"
            }
        }
        return "\(place)\(suffix)"
    }

    static func percent(_ value: Double) -> String { String(format: "%.1f%%", value * 100) }
}

#Preview("NBA standings") {
    NavigationStack {
        ScrollView {
            StandingsView(session: PreviewFixtures.signedInStore,
                          selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                          data: PreviewFixtures.dashboard, shown: .constant("nba"),
                          activitySport: .constant("nba"), preview: true)
                .padding(20)
        }
        .background(MosaicPalette.ground)
    }
    .preferredColorScheme(.dark)
}

#Preview("Overall before results") {
    let empty = DashboardData(standings: PreviewFixtures.dashboard.standings,
                              overall: [], activity: [], odds: nil)
    NavigationStack {
        ScrollView {
            StandingsView(session: PreviewFixtures.signedInStore,
                          selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                          data: empty, shown: .constant("overall"), activitySport: .constant("all"), preview: true)
                .padding(20)
        }
        .background(MosaicPalette.ground)
    }
    .preferredColorScheme(.dark)
}
