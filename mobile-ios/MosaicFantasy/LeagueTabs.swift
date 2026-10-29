// Holds the selected league's five native destinations and keeps league switching in More.
import SwiftUI

private enum LeagueTab: Hashable {
    case teams, standings, matchups, players, more

    var title: String {
        switch self {
        case .teams: "Teams"
        case .standings: "Standings"
        case .matchups: "Matchups"
        case .players: "Players"
        case .more: "More"
        }
    }
}

struct LeagueTabs: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let onChooseLeagues: () -> Void
    var preview = false

    @State private var tab: LeagueTab = .standings

    var body: some View {
        TabView(selection: $tab) {
            TeamsView(session: session, selected: selected, preview: preview)
                .tabItem { Label("Teams", systemImage: "shield.lefthalf.filled") }
                .tag(LeagueTab.teams)
            LeagueView(session: session, selected: selected,
                       previewState: preview ? .loaded : nil)
                .tabItem { Label("Standings", systemImage: "list.number") }
                .tag(LeagueTab.standings)
            MatchupsView(session: session, selected: selected, active: tab == .matchups,
                         preview: preview)
                .tabItem { Label("Matchups", systemImage: "sportscourt") }
                .tag(LeagueTab.matchups)
            comingSoon("Players", symbol: "person.3", detail: "Player search and profiles are coming soon.")
                .tabItem { Label("Players", systemImage: "person.3") }
                .tag(LeagueTab.players)
            MoreView(session: session, onLeagues: onChooseLeagues,
                     currentLeagueName: selected.membership.name)
                .tabItem { Label("More", systemImage: "line.3.horizontal") }
                .tag(LeagueTab.more)
        }
        .navigationTitle(tab == .more ? tab.title : selected.membership.name)
        .navigationBarTitleDisplayMode(.inline)
        .tint(MosaicPalette.brand)
    }

    private func comingSoon(_ title: String, symbol: String, detail: String) -> some View {
        ScrollView {
            StatePanel(symbol: symbol, title: "\(title) coming soon", detail: detail)
                .padding(20)
        }
        .background(MosaicPalette.ground)
    }
}

#Preview("League tabs") {
    NavigationStack {
        LeagueTabs(session: PreviewFixtures.signedInStore,
                   selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                   onChooseLeagues: {}, preview: true)
    }
    .preferredColorScheme(.dark)
}
