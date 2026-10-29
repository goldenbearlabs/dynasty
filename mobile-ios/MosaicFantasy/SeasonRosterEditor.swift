// Lets a manager make the season's one-time main and reserve roster choice.
import SwiftUI

enum SeasonRosterRules {
    static func due(_ rosters: [LeagueRoster]) -> [LeagueRoster] {
        rosters.filter { $0.season?.needsSetting == true }
    }

    static func signed(_ roster: LeagueRoster) -> [RosterPlayer] {
        roster.players.filter { $0.list != "rights" }
    }

    static func counts(_ roster: LeagueRoster, reserve: Set<String>) -> (main: Int, reserve: Int) {
        let signed = signed(roster)
        return (signed.count - signed.filter { reserve.contains($0.playerId) }.count,
                signed.filter { reserve.contains($0.playerId) }.count)
    }

    static func valid(_ roster: LeagueRoster, reserve: Set<String>) -> Bool {
        let counts = counts(roster, reserve: reserve)
        return counts.main <= roster.limits.main && counts.reserve <= roster.limits.reserve
            && reserve.isSubset(of: Set(signed(roster).map(\.playerId)))
    }
}

struct SeasonRosterEditor: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let roster: LeagueRoster
    let onSaved: () async -> Void

    @State private var reserve: Set<String> = []
    @State private var confirming = false
    @State private var saving = false
    @State private var error: String?

    private var season: SeasonRosterState? { roster.season }
    private var signed: [RosterPlayer] { SeasonRosterRules.signed(roster) }
    private var mainPlayers: [RosterPlayer] { signed.filter { !reserve.contains($0.playerId) }.sorted(by: byName) }
    private var reservePlayers: [RosterPlayer] { signed.filter { reserve.contains($0.playerId) }.sorted(by: byName) }
    private var valid: Bool { SeasonRosterRules.valid(roster, reserve: reserve) }
    private var limitMessage: String? {
        if mainPlayers.count > roster.limits.main {
            return "Move \(mainPlayers.count - roster.limits.main) more to reserve."
        }
        if reservePlayers.count > roster.limits.reserve {
            return "Move \(reservePlayers.count - roster.limits.reserve) more to the main roster."
        }
        return nil
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    if let season {
                        VStack(alignment: .leading, spacing: 8) {
                            SportTag(sport: roster.competition)
                            Text("Set your \(roster.name) season roster")
                                .font(.title2.bold()).foregroundStyle(MosaicPalette.ink)
                            Text("\(String(season.year)) season · \(season.started ? "Started" : "Starts") \(Self.dayLabel(season.startsOn))")
                                .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                            Text("Choose who is on your main roster and reserve list. Only main-roster players can start. This choice is final for the season.")
                                .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                            if let deadline = season.deadline {
                                Label("Lists lock \(Self.deadlineLabel(deadline)) if you do not set them.", systemImage: "clock")
                                    .font(.subheadline).foregroundStyle(MosaicPalette.gold)
                            }
                        }
                        rosterList("Main roster", players: mainPlayers, limit: roster.limits.main, toReserve: true)
                        rosterList("Reserve list", players: reservePlayers, limit: roster.limits.reserve, toReserve: false)
                        if let limitMessage {
                            Label(limitMessage, systemImage: "exclamationmark.circle")
                                .font(.subheadline).foregroundStyle(MosaicPalette.bad)
                        }
                        if let error {
                            Label(error, systemImage: "exclamationmark.circle")
                                .font(.subheadline).foregroundStyle(MosaicPalette.bad)
                        }
                    }
                }
                .padding(20)
                .padding(.bottom, 20)
            }
            .background(MosaicPalette.ground)
            .safeAreaInset(edge: .bottom) {
                Button("Review and save") { confirming = true }
                    .buttonStyle(.borderedProminent)
                    .disabled(!valid || saving || season?.needsSetting != true)
                    .frame(maxWidth: .infinity)
                    .padding(.horizontal, 20)
                    .padding(.vertical, 10)
                    .background(MosaicPalette.surface)
            }
            .navigationTitle("Season roster")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }.disabled(saving)
                }
            }
            .confirmationDialog("Lock in your \(String(season?.year ?? 0)) \(roster.name) roster?",
                                isPresented: $confirming, titleVisibility: .visible) {
                Button(saving ? "Saving…" : "Confirm and lock in") { Task { await save() } }
                    .disabled(saving)
                Button("Go back", role: .cancel) {}
            } message: {
                Text("\(mainPlayers.count) main, \(reservePlayers.count) reserve. This is final for the season. Drops, free agents, trades, and injury swaps still follow league rules.")
            }
        }
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(saving)
        .onAppear {
            reserve = Set(roster.players.filter { $0.list == "reserve" }.map(\.playerId))
        }
    }

    private func rosterList(_ title: String, players: [RosterPlayer], limit: Int, toReserve: Bool) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(title).font(.headline)
                Spacer()
                Text("\(players.count)/\(limit)")
                    .font(.subheadline.monospacedDigit().weight(.semibold))
                    .foregroundStyle(players.count > limit ? MosaicPalette.bad : MosaicPalette.soft)
            }
            if players.isEmpty {
                Text("Nobody here.").font(.subheadline).foregroundStyle(MosaicPalette.soft)
            } else {
                ForEach(players) { player in
                    HStack(spacing: 10) {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(player.fullName).font(.subheadline.weight(.medium))
                            Text(player.positions?.joined(separator: "/") ?? "")
                                .font(.caption).foregroundStyle(MosaicPalette.soft)
                        }
                        Spacer(minLength: 0)
                        Button(toReserve ? "Reserve" : "Main") {
                            if toReserve { reserve.insert(player.playerId) }
                            else { reserve.remove(player.playerId) }
                        }
                        .font(.subheadline)
                        .buttonStyle(.bordered)
                        .accessibilityLabel("Move \(player.fullName) to \(toReserve ? "reserve" : "main roster")")
                    }
                    .frame(minHeight: 44)
                    if player.id != players.last?.id { Divider().overlay(MosaicPalette.rule) }
                }
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
    }

    private func byName(_ a: RosterPlayer, _ b: RosterPlayer) -> Bool {
        a.fullName.localizedCaseInsensitiveCompare(b.fullName) == .orderedAscending
    }

    @MainActor
    private func save() async {
        guard !saving, valid, season?.needsSetting == true, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.setSeasonRoster(for: selected, leagueID: roster.leagueId,
                                                   reserve: Array(reserve).sorted(), token: token)
            await onSaved()
            dismiss()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    static func dayLabel(_ value: String) -> String {
        let parser = DateFormatter()
        parser.dateFormat = "yyyy-MM-dd"
        parser.locale = Locale(identifier: "en_US_POSIX")
        guard let day = parser.date(from: value) else { return value }
        return day.formatted(date: .abbreviated, time: .omitted)
    }

    static func deadlineLabel(_ value: String) -> String {
        let parser = ISO8601DateFormatter()
        parser.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let date = parser.date(from: value) ?? ISO8601DateFormatter().date(from: value)
        return date?.formatted(date: .abbreviated, time: .shortened) ?? value
    }
}

#Preview("Set season roster") {
    SeasonRosterEditor(session: PreviewFixtures.signedInStore,
                       selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                       roster: PreviewFixtures.franchiseDetail.rosters[0], onSaved: {})
        .preferredColorScheme(.dark)
}
