// Combines complete standings and recent activity for the selected, verified dynasty.
import SwiftUI

enum LeaguePreviewState { case loading, loaded, empty, error }

struct LeagueView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    var previewState: LeaguePreviewState? = nil

    @State private var data: DashboardData?
    @State private var loading = true
    @State private var error: String?
    @State private var standing = ""
    @State private var sport = "all"
    @State private var activityType = "all"
    @State private var refreshKey = 0

    private var shownData: DashboardData? { previewState == .loaded ? PreviewFixtures.dashboard : data }
    private var currentSelected: SelectedLeague {
        SelectedLeague(membership: selected.membership, dynasty: shownData?.dynasty ?? selected.dynasty)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 32) {
                if previewState == .loading || previewState == nil && loading && data == nil {
                    StatePanel(symbol: "hourglass", title: "Loading League", detail: "Getting the latest standings and activity…")
                    ProgressView().frame(maxWidth: .infinity)
                } else if previewState == .error || error != nil && data == nil {
                    StatePanel(symbol: "wifi.exclamationmark", title: "Could not load League",
                               detail: error ?? "Check your connection and try again.",
                               actionTitle: "Try again", action: { Task { await load() } })
                } else if let shownData {
                    if let error {
                        Label("Could not refresh: \(error)", systemImage: "exclamationmark.circle")
                            .font(.subheadline).foregroundStyle(MosaicPalette.bad)
                    }
                    StandingsView(session: session, selected: currentSelected, data: shownData,
                                  shown: $standing, activitySport: $sport,
                                  refreshKey: refreshKey, preview: previewState != nil)
                    activitySection(shownData)
                } else {
                    StatePanel(symbol: "chart.bar", title: "Nothing to show yet",
                               detail: "League data will appear after the commissioner starts a season.")
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 12)
            .padding(.bottom, 40)
        }
        .background(MosaicPalette.ground)
        .refreshable { await load() }
        .task { if previewState == nil { await load() } }
    }

    private func activitySection(_ data: DashboardData) -> some View {
        let items = data.activity.filter { item in
            (sport == "all" || item.competition == sport) && matchesType(item)
        }
        return VStack(alignment: .leading, spacing: 14) {
            Text("Recent activity").font(.title2.bold()).foregroundStyle(MosaicPalette.ink)
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 10) { sportFilter; activityFilter }
                VStack(alignment: .leading, spacing: 10) { sportFilter; activityFilter }
            }
            .font(.subheadline.weight(.medium))
            .buttonStyle(.bordered)
            if sport != "all" || activityType != "all" {
                Button("Clear filters") { sport = "all"; activityType = "all" }
                    .font(.subheadline).buttonStyle(.plain)
            }
            if items.isEmpty {
                StatePanel(symbol: "clock.arrow.circlepath", title: data.activity.isEmpty ? "No activity yet" : "No matching activity",
                           detail: data.activity.isEmpty ? "League updates will appear here." : "Try a different sport or activity filter.")
            } else {
                VStack(spacing: 0) {
                    ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                        HStack(alignment: .top, spacing: 12) {
                            SportTag(sport: item.competition.isEmpty ? "MF" : item.competition)
                                .frame(minWidth: 52, alignment: .leading)
                            VStack(alignment: .leading, spacing: 4) {
                                Text(describe(item)).font(.subheadline.weight(.medium))
                                    .foregroundStyle(MosaicPalette.ink)
                                if let day = item.createdAt?.prefix(10) {
                                    Text(String(day)).font(.caption).foregroundStyle(MosaicPalette.soft)
                                }
                            }
                            Spacer(minLength: 0)
                        }
                        .padding(.vertical, 13)
                        if index < items.count - 1 { Divider().overlay(MosaicPalette.rule) }
                    }
                }
                .padding(.horizontal, 16)
                .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
            }
        }
    }

    private var sportFilter: some View {
        Menu {
            Button("All sports") { sport = "all" }
            ForEach(currentSelected.dynasty.leagues, id: \.competition) { league in
                Button(league.competition.uppercased()) { sport = league.competition }
            }
        } label: {
            Label(sport == "all" ? "All sports" : sport.uppercased(), systemImage: "line.3.horizontal.decrease")
        }
    }

    private var activityFilter: some View {
        Menu {
            Button("All activity") { activityType = "all" }
            Button("Trades") { activityType = "trade" }
            Button("Roster moves") { activityType = "roster" }
            Button("Rule changes") { activityType = "settings" }
        } label: {
            Text(activityType == "all" ? "All activity" : activityType == "trade" ? "Trades"
                 : activityType == "roster" ? "Roster moves" : "Rule changes")
        }
    }

    private func matchesType(_ item: ActivityItem) -> Bool {
        if activityType == "all" { return true }
        if activityType == "roster" {
            return Set(["add", "drop", "claim", "sign", "unsigned", "move", "graduated", "released"]).contains(item.kind)
        }
        return item.kind == activityType
    }

    private func describe(_ item: ActivityItem) -> String {
        if item.kind == "settings" { return "League rules changed" }
        if item.kind == "trade" {
            return "\(item.franchiseName) acquired \(item.playerName.isEmpty ? "a draft pick" : item.playerName)"
        }
        let verbs = ["add": "added", "drop": "dropped", "claim": "claimed", "pick": "drafted",
                     "sign": "signed", "move": "moved", "graduated": "kept", "released": "lost"]
        return "\(item.franchiseName) \(verbs[item.kind] ?? item.kind) \(item.playerName)"
            .trimmingCharacters(in: .whitespaces)
    }

    @MainActor
    private func load() async {
        guard previewState == nil, let token = session.token else { return }
        if loading && data != nil { return }
        loading = true
        error = nil
        defer { loading = false }
        do {
            data = try await session.api.dashboard(for: selected, token: token)
            refreshKey += 1
        } catch {
            if (error as? APIError)?.statusCode == 401 || error.localizedDescription.contains("not available yet") {
                data = nil
            }
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

#Preview("League") {
    NavigationStack {
        LeagueView(session: PreviewFixtures.signedInStore,
                   selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty), previewState: .loaded)
    }
    .preferredColorScheme(.dark)
}

#Preview("League loading") {
    NavigationStack {
        LeagueView(session: PreviewFixtures.signedInStore,
                   selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty), previewState: .loading)
    }
    .preferredColorScheme(.dark)
}

#Preview("League empty") {
    NavigationStack {
        LeagueView(session: PreviewFixtures.signedInStore,
                   selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty), previewState: .empty)
    }
    .preferredColorScheme(.dark)
}

#Preview("League error") {
    NavigationStack {
        LeagueView(session: PreviewFixtures.signedInStore,
                   selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty), previewState: .error)
    }
    .preferredColorScheme(.dark)
}
