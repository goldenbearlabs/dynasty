// Shows the head-to-head period and matchup scores beneath sport standings.
import SwiftUI

struct MatchupsSection: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let league: SportLeague
    var refreshKey = 0
    var preview = false

    @State private var view: MatchupsData?
    @State private var periodNumber: Int?
    @State private var loading = true
    @State private var error: String?
    @State private var selectedMatchup: MatchupSelection?

    private var shown: MatchupsData? { preview ? PreviewFixtures.matchups : view }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Matchups").font(.title3.bold()).foregroundStyle(MosaicPalette.ink)
            if !preview && loading && view == nil {
                ProgressView("Loading matchups").frame(maxWidth: .infinity, alignment: .leading)
            } else if let error {
                StatePanel(symbol: "wifi.exclamationmark", title: "Could not load matchups", detail: error,
                           actionTitle: "Try again", action: { Task { await load() } })
            } else if let shown, let period = shown.period {
                periodHeader(period, periods: shown.periods)
                if shown.matchups.isEmpty {
                    Text("This round's matchups are set once the round before it has finished.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                } else {
                    ForEach(ordered(shown.matchups)) { matchup in matchupCard(matchup, period: period) }
                }
            } else {
                Text("No matchups are scheduled yet.")
                    .font(.subheadline).foregroundStyle(MosaicPalette.soft)
            }
        }
        .task(id: "\(league.id)-\(refreshKey)") { if !preview { await load() } }
        .sheet(item: $selectedMatchup) { choice in
            MatchupDetailSheet(session: session, selected: selected, league: league,
                               matchupID: choice.id, preview: preview)
                .presentationDetents([.medium, .large])
        }
    }

    private func periodHeader(_ period: MatchupPeriod, periods: [MatchupPeriod]) -> some View {
        let regular = periods.filter { !$0.isPlayoff }.count
        let last = periods.last?.seq ?? period.seq
        let label = period.isPlayoff
            ? (period.seq == last ? "Finals" : "Playoffs, round \(period.seq - regular)")
            : "\(league.settings?.format?.matchupDays == 7 ? "Week" : "Matchup") \(period.seq) of \(regular)"
        return VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(label).font(.headline)
                    Text(period.startsOn == period.endsOn ? period.startsOn : "\(period.startsOn) – \(period.endsOn)")
                        .font(.caption).foregroundStyle(MosaicPalette.soft)
                }
                Spacer()
                HStack(spacing: 5) {
                    Button { changePeriod(period.seq - 1) } label: { Image(systemName: "chevron.left") }
                        .disabled(period.seq <= 1)
                        .accessibilityLabel("Earlier matchup period")
                    Button { changePeriod(period.seq + 1) } label: { Image(systemName: "chevron.right") }
                        .disabled(period.seq >= last)
                        .accessibilityLabel("Later matchup period")
                }
                .buttonStyle(.bordered)
            }
            if period.byHand {
                Label("Set by the commissioner", systemImage: "hand.draw")
                    .font(.caption).foregroundStyle(MosaicPalette.brand)
            }
        }
    }

    private func matchupCard(_ matchup: MatchupCard, period: MatchupPeriod) -> some View {
        Button { selectedMatchup = MatchupSelection(id: matchup.id) } label: {
            VStack(alignment: .leading, spacing: 9) {
                matchupSide(matchup.homeFranchiseId, points: matchup.homePoints,
                            other: matchup.awayPoints, hasOpponent: matchup.awayFranchiseId != nil)
                matchupSide(matchup.awayFranchiseId, points: matchup.awayPoints,
                            other: matchup.homePoints, hasOpponent: matchup.awayFranchiseId != nil)
                if period.isPlayoff {
                    Text("\(matchup.bracket == "consolation" ? "Consolation" : "Championship") · Matchup \(matchup.bracketSlot + 1)")
                        .font(.caption).foregroundStyle(MosaicPalette.brand)
                }
                if matchup.final { Text("Final").font(.caption.weight(.bold)).foregroundStyle(MosaicPalette.soft) }
            }
            .padding(15)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 15))
            .overlay(RoundedRectangle(cornerRadius: 15)
                .stroke(isMine(matchup) ? MosaicPalette.brand : .clear, lineWidth: 1))
        }
        .buttonStyle(.plain)
        .accessibilityHint("Opens matchup scorers")
    }

    private func matchupSide(_ id: String?, points: Double, other: Double, hasOpponent: Bool) -> some View {
        HStack(spacing: 9) {
            if let id {
                TeamCrest(name: name(id), imageURL: imageURL(id), baseURL: session.api.baseURL)
                Text(name(id)).font(.subheadline.weight(points > other && hasOpponent ? .bold : .medium))
                    .foregroundStyle(MosaicPalette.ink).lineLimit(2)
                Spacer(minLength: 4)
                Text(StandingsOrder.points(points))
                    .font(.body.weight(.bold)).monospacedDigit()
                    .foregroundStyle(points > other && hasOpponent ? MosaicPalette.sport(league.competition) : MosaicPalette.ink)
            } else {
                Text("Bye").font(.subheadline).foregroundStyle(MosaicPalette.soft)
            }
        }
    }

    private func ordered(_ matchups: [MatchupCard]) -> [MatchupCard] {
        matchups.sorted { isMine($0) && !isMine($1) }
    }

    private func isMine(_ matchup: MatchupCard) -> Bool {
        let id = selected.membership.franchiseId
        return matchup.homeFranchiseId == id || matchup.awayFranchiseId == id
    }

    private func name(_ id: String) -> String {
        let team = selected.dynasty.teamIdentities.first(where: { $0.franchiseId == id && $0.leagueId == league.id })
        return team?.name.isEmpty == false ? team!.name
            : selected.dynasty.franchises.first(where: { $0.id == id })?.name ?? "Franchise"
    }

    private func imageURL(_ id: String) -> String? {
        let team = selected.dynasty.teamIdentities.first(where: { $0.franchiseId == id && $0.leagueId == league.id })
        return team?.imageUrl?.isEmpty == false ? team?.imageUrl
            : selected.dynasty.franchises.first(where: { $0.id == id })?.imageUrl
    }

    private func changePeriod(_ number: Int) {
        periodNumber = number
        Task { await load() }
    }

    private func load() async {
        guard let token = session.token else { return }
        loading = true
        error = nil
        defer { loading = false }
        do {
            view = try await session.api.matchups(for: selected, leagueID: league.id,
                                                  period: periodNumber, token: token)
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

private struct MatchupSelection: Identifiable { let id: String }

struct MatchupDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let league: SportLeague
    let matchupID: String
    let preview: Bool

    @State private var detail: MatchupDetail?
    @State private var error: String?

    private var shown: MatchupDetail? { preview ? PreviewFixtures.matchupDetail : detail }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    if let shown {
                        Text(shown.final ? "Final" : "In progress")
                            .font(.subheadline.weight(.semibold)).foregroundStyle(MosaicPalette.brand)
                        scorers(shown.homeFranchiseId, points: shown.homePoints, players: shown.homePlayers)
                        if let away = shown.awayFranchiseId {
                            scorers(away, points: shown.awayPoints, players: shown.awayPlayers)
                        } else {
                            Text("Bye").foregroundStyle(MosaicPalette.soft)
                        }
                    } else if let error {
                        StatePanel(symbol: "wifi.exclamationmark", title: "Could not load matchup", detail: error,
                                   actionTitle: "Try again", action: { Task { await load() } })
                    } else {
                        ProgressView("Loading matchup").frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .padding(20)
            }
            .background(MosaicPalette.ground)
            .navigationTitle("Matchup")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .topBarTrailing) { Button("Done") { dismiss() } } }
        }
        .preferredColorScheme(.dark)
        .task { if !preview { await load() } }
    }

    private func scorers(_ id: String, points: Double, players: [MatchupScorer]) -> some View {
        let franchise = selected.dynasty.franchises.first(where: { $0.id == id })
        let team = selected.dynasty.teamIdentities.first(where: { $0.franchiseId == id && $0.leagueId == league.id })
        let name = team?.name.isEmpty == false ? team!.name : franchise?.name ?? "Franchise"
        return VStack(alignment: .leading, spacing: 10) {
            HStack {
                TeamCrest(name: name, imageURL: team?.imageUrl?.isEmpty == false ? team?.imageUrl : franchise?.imageUrl,
                          baseURL: session.api.baseURL)
                Text(name).font(.headline)
                Spacer()
                Text("\(StandingsOrder.points(points)) pts").font(.headline.monospacedDigit())
            }
            if players.isEmpty {
                Text("No starter points yet.").font(.subheadline).foregroundStyle(MosaicPalette.soft)
            } else {
                ForEach(players) { player in
                    HStack {
                        Text(player.nickname?.isEmpty == false ? player.nickname! : player.fullName)
                            .font(.subheadline)
                        Spacer()
                        Text(StandingsOrder.points(player.points))
                            .font(.subheadline.weight(.semibold)).monospacedDigit()
                    }
                    .padding(.vertical, 4)
                }
            }
        }
        .padding(16)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 15))
    }

    private func load() async {
        guard let token = session.token else { return }
        error = nil
        do {
            detail = try await session.api.matchupDetail(for: selected, leagueID: league.id,
                                                           matchupID: matchupID, token: token)
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}
