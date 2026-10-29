// Provides focused manager sheets for roster splits, injury swaps, and player nicknames.
import SwiftUI

struct RosterSplitEditor: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let roster: LeagueRoster
    let onSaved: () async -> Void

    @State private var reserve: Set<String> = []
    @State private var confirming = false
    @State private var saving = false
    @State private var error: String?

    private var signed: [RosterPlayer] { roster.players.filter { $0.list != "rights" } }
    private var valid: Bool {
        let main = signed.count - reserve.count
        return main <= roster.limits.main && reserve.count <= roster.limits.reserve
            && reserve.isSubset(of: Set(signed.map(\.playerId))) && roster.season?.locked != true
            && signed.allSatisfy {
                let moved = reserve.contains($0.playerId) != ($0.list == "reserve")
                return !moved || ($0.injuryReserveLocked != true && $0.lockedUntil == nil)
            }
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text("Choose which signed players are on reserve. Everyone else stays on the main roster.")
                        .foregroundStyle(MosaicPalette.soft)
                    Text("Main \(signed.count - reserve.count)/\(roster.limits.main) · Reserve \(reserve.count)/\(roster.limits.reserve)")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(valid ? MosaicPalette.ink : MosaicPalette.bad)
                }
                Section("Players") {
                    ForEach(signed.sorted { $0.fullName < $1.fullName }) { player in
                        Button {
                            if reserve.contains(player.playerId) { reserve.remove(player.playerId) }
                            else { reserve.insert(player.playerId) }
                        } label: {
                            HStack {
                                Text(player.fullName)
                                Spacer()
                                Text(reserve.contains(player.playerId) ? "Reserve" : "Main")
                                    .foregroundStyle(MosaicPalette.soft)
                            }
                            .frame(minHeight: 44)
                        }
                        .buttonStyle(.plain)
                        .disabled(player.injuryReserveLocked == true || player.lockedUntil != nil)
                    }
                }
                if let error { Section { Text(error).foregroundStyle(MosaicPalette.bad) } }
            }
            .scrollContentBackground(.hidden).background(MosaicPalette.ground)
            .navigationTitle("Set roster lists")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { Button("Cancel") { dismiss() }.disabled(saving) }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Review") { confirming = true }.disabled(!valid || saving)
                }
            }
            .confirmationDialog("Save main and reserve lists?", isPresented: $confirming,
                                titleVisibility: .visible) {
                Button("Save lists") { Task { await save() } }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("League eligibility and reserve-lock rules still apply.")
            }
            .onAppear { reserve = Set(roster.players.filter { $0.list == "reserve" }.map(\.playerId)) }
        }
    }

    @MainActor private func save() async {
        guard valid, !saving, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.changeRoster(for: selected, leagueID: roster.leagueId, action: "set",
                                               reserve: Array(reserve).sorted(), token: token)
            await onSaved()
            dismiss()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

struct InjurySwapEditor: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let roster: LeagueRoster
    let injured: RosterPlayer
    let onSaved: () async -> Void

    @State private var replacementID = ""
    @State private var confirming = false
    @State private var saving = false
    @State private var error: String?

    private var replacements: [RosterPlayer] {
        roster.players.filter { $0.list == "reserve" && $0.injuryReserveLocked != true }
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Move \(injured.fullName) to reserve and call up an eligible replacement.")
                    Picker("Call up", selection: $replacementID) {
                        Text("Choose a player").tag("")
                        ForEach(replacements) { player in Text(player.fullName).tag(player.playerId) }
                    }
                } footer: {
                    Text("The injured player must be eligible, and the reserve replacement must be unlocked.")
                }
                if let error { Section { Text(error).foregroundStyle(MosaicPalette.bad) } }
            }
            .scrollContentBackground(.hidden).background(MosaicPalette.ground)
            .navigationTitle("Injury swap")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { Button("Cancel") { dismiss() }.disabled(saving) }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Review") { confirming = true }.disabled(replacementID.isEmpty || saving)
                }
            }
            .confirmationDialog("Confirm injury swap?", isPresented: $confirming) {
                Button("Swap players") { Task { await save() } }
                Button("Cancel", role: .cancel) {}
            }
        }
    }

    @MainActor private func save() async {
        guard !saving, !replacementID.isEmpty, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.changeRoster(for: selected, leagueID: roster.leagueId,
                                               action: "injury-swap", playerID: injured.playerId,
                                               replacementID: replacementID, token: token)
            await onSaved()
            dismiss()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

struct TeamNicknameEditor: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let player: RosterPlayer
    let onSaved: () async -> Void

    @State private var nickname = ""
    @State private var saving = false
    @State private var error: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(player.fullName).foregroundStyle(MosaicPalette.soft)
                    TextField("Nickname", text: $nickname)
                        .textInputAutocapitalization(.words)
                    Text("\(nickname.count)/40 characters")
                        .font(.caption).foregroundStyle(nickname.count > 40 ? MosaicPalette.bad : MosaicPalette.soft)
                } footer: { Text("Leave blank to show the player's full name.") }
                if let error { Section { Text(error).foregroundStyle(MosaicPalette.bad) } }
            }
            .scrollContentBackground(.hidden).background(MosaicPalette.ground)
            .navigationTitle("Player nickname")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { Button("Cancel") { dismiss() }.disabled(saving) }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Save") { Task { await save() } }.disabled(nickname.count > 40 || saving)
                }
            }
            .onAppear { nickname = player.nickname ?? "" }
        }
    }

    @MainActor private func save() async {
        guard nickname.count <= 40, !saving, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.setNickname(for: selected, playerID: player.playerId,
                                              nickname: nickname, token: token)
            await onSaved()
            dismiss()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}
