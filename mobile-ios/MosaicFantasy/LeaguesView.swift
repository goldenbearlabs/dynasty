// Shows only playable memberships, with native invite guidance and dynasty-ID protection.
import SwiftUI

enum LeaguesPreviewState { case loading, empty, loaded, error }

struct LeaguesView: View {
    @ObservedObject var session: SessionStore
    var previewState: LeaguesPreviewState? = nil

    @State private var leagues: [MyLeague] = []
    @State private var loading = true
    @State private var error: String?
    @State private var notice: String?
    @State private var busyLeague: String?
    @State private var resendBusy = false
    @State private var showAccount = false
    @State private var showInvite = false
    @State private var selection: SelectedLeague?

    private var playable: [MyLeague] {
        MyLeague.playable(in: previewState == .loaded ? PreviewFixtures.leagues : leagues)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                if let notice {
                    Label(notice, systemImage: "info.circle")
                        .font(.subheadline)
                        .foregroundStyle(MosaicPalette.soft)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(14)
                        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 12))
                }
                if previewState == .loading || previewState == nil && loading {
                    StatePanel(symbol: "hourglass", title: "Loading your leagues", detail: "Checking your memberships…")
                    ProgressView().frame(maxWidth: .infinity)
                } else if previewState == .error || error != nil {
                    StatePanel(symbol: "wifi.exclamationmark", title: "Could not load your leagues",
                               detail: error ?? "Check your connection and try again.",
                               actionTitle: "Try again", action: { Task { await load() } })
                } else if playable.isEmpty {
                    emptyState
                } else {
                    LazyVStack(spacing: 14) {
                        ForEach(playable) { league in leagueCard(league) }
                    }
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 12)
            .padding(.bottom, 40)
        }
        .background(MosaicPalette.ground)
        .navigationTitle("Leagues")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button { showAccount = true } label: {
                    ProfileAvatar(session: session.session, size: 34)
                        .frame(width: 44, height: 44)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Open account")
            }
        }
        .navigationDestination(isPresented: $showAccount) {
            AccountView(session: session)
        }
        .sheet(isPresented: $showInvite) {
            InviteSheet(session: session, onJoined: { await load() })
        }
        .navigationDestination(item: $selection) { selected in
            LeagueTabs(session: session, selected: selected, onChooseLeagues: { selection = nil })
        }
        .refreshable { await load() }
        .task { if previewState == nil { await load() } }
        .onChange(of: session.session?.id) { _, _ in selection = nil }
    }

    private var emptyState: some View {
        VStack(spacing: 14) {
            if session.session?.verified == false {
                StatePanel(symbol: "envelope.badge", title: "Verify your email",
                           detail: "Verify your email, then ask a commissioner for an invite link to join a league.",
                           actionTitle: resendBusy ? "Sending…" : "Resend verification email",
                           action: { Task { await resend() } })
            } else {
                StatePanel(symbol: "person.2", title: "No leagues yet",
                           detail: "Ask a commissioner for an invite link to join a playable league.",
                           actionTitle: "Join a league", action: { showInvite = true })
            }
        }
    }

    private func leagueCard(_ league: MyLeague) -> some View {
        Button {
            Task { await open(league) }
        } label: {
            VStack(alignment: .leading, spacing: 15) {
                HStack(alignment: .top, spacing: 12) {
                    Text(league.name)
                        .font(.title3.weight(.bold))
                        .foregroundStyle(MosaicPalette.ink)
                        .multilineTextAlignment(.leading)
                    Spacer(minLength: 0)
                    if busyLeague == league.id { ProgressView() }
                }
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 78), spacing: 12, alignment: .leading)],
                          alignment: .leading, spacing: 10) {
                    ForEach(league.sports, id: \.self) { sport in SportTag(sport: sport) }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .padding(18)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MosaicPalette.raised, in: RoundedRectangle(cornerRadius: 16))
        }
        .buttonStyle(.plain)
        .disabled(busyLeague != nil)
        .accessibilityHint("Opens this league")
    }

    private func load() async {
        guard previewState == nil, let token = session.token else { return }
        loading = true
        error = nil
        defer { loading = false }
        do { leagues = try await session.api.myLeagues(token: token) }
        catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    private func open(_ league: MyLeague) async {
        guard let token = session.token, busyLeague == nil else { return }
        busyLeague = league.id
        notice = nil
        defer { busyLeague = nil }
        do { selection = try await session.api.verifiedSelection(league, token: token) }
        catch { notice = error.localizedDescription }
    }

    private func resend() async {
        guard !resendBusy, let email = session.session?.email else { return }
        resendBusy = true
        defer { resendBusy = false }
        do {
            try await session.api.resendVerification(email: email)
            notice = "If verification is needed, a new link has been sent."
        } catch { notice = error.localizedDescription }
    }
}

private struct InviteSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let onJoined: () async -> Void
    @State private var inviteURL = ""
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Commissioner invite URL", text: $inviteURL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        .textContentType(.URL)
                } footer: {
                    Text("Paste the full invite link sent by your commissioner.")
                }
                if let error { Text(error).foregroundStyle(MosaicPalette.bad) }
            }
            .scrollContentBackground(.hidden)
            .background(MosaicPalette.ground)
            .navigationTitle("Join a league")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(busy ? "Joining…" : "Join") { Task { await join() } }
                        .disabled(busy || inviteURL.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func join() async {
        busy = true
        defer { busy = false }
        do {
            try await session.acceptInvite(inviteURL)
            await onJoined()
            dismiss()
        } catch { self.error = error.localizedDescription }
    }
}

#Preview("Leagues") {
    NavigationStack { LeaguesView(session: PreviewFixtures.signedInStore, previewState: .loaded) }
        .preferredColorScheme(.dark)
}

#Preview("No league") {
    NavigationStack { LeaguesView(session: PreviewFixtures.signedInStore, previewState: .empty) }
        .preferredColorScheme(.dark)
}

#Preview("Loading") {
    NavigationStack { LeaguesView(session: PreviewFixtures.signedInStore, previewState: .loading) }
        .preferredColorScheme(.dark)
}

#Preview("Error") {
    NavigationStack { LeaguesView(session: PreviewFixtures.signedInStore, previewState: .error) }
        .preferredColorScheme(.dark)
}
