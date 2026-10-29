// Starts the native iPhone app and switches between account entry and league navigation.
import SwiftUI

@main
struct MosaicFantasyApp: App {
    @StateObject private var session = SessionStore(api: APIClient(baseURL: AppConfig.baseURL))
    @Environment(\.scenePhase) private var scenePhase

    #if DEBUG
    private let showcase = ProcessInfo.processInfo.environment["MOSAIC_SHOWCASE"]
    #endif

    var body: some Scene {
        WindowGroup {
            Group {
                #if DEBUG
                if showcase == "leagues" {
                    NavigationStack { LeaguesView(session: PreviewFixtures.signedInStore, previewState: .loaded) }
                } else if showcase == "league" {
                    NavigationStack {
                        LeagueTabs(session: PreviewFixtures.signedInStore,
                                   selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                                   onChooseLeagues: {}, preview: true)
                    }
                } else if showcase == "teams" {
                    NavigationStack {
                        TeamsView(session: PreviewFixtures.signedInStore,
                                  selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                                  preview: true)
                    }
                } else if showcase == "matchups" {
                    NavigationStack {
                        MatchupsView(session: PreviewFixtures.signedInStore,
                                     selected: SelectedLeague(membership: PreviewFixtures.leagues[0],
                                                              dynasty: PreviewFixtures.dynasty),
                                     active: true, preview: true)
                    }
                } else if showcase == "sport-team" {
                    NavigationStack {
                        SportTeamView(session: PreviewFixtures.signedInStore,
                                      selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                                      franchiseID: "franchise-1", initialRoster: PreviewFixtures.activeRoster,
                                      onChanged: {}, preview: true)
                    }
                } else {
                    appContent
                }
                #else
                appContent
                #endif
            }
            .tint(MosaicPalette.brand)
            .preferredColorScheme(.dark)
            .task {
                #if DEBUG
                if showcase != nil { return }
                #endif
                await session.restore()
            }
            .onChange(of: scenePhase) { _, phase in
                if phase == .active && session.ready { Task { await session.refresh() } }
            }
        }
    }

    @ViewBuilder private var appContent: some View {
        if !session.ready {
            ProgressView("Restoring your session")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(MosaicPalette.ground)
        } else if session.session == nil {
            AuthView(session: session)
        } else {
            NavigationStack { LeaguesView(session: session) }
        }
    }
}
