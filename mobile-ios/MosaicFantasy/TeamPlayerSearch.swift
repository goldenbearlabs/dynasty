// Searches a sport's players and lets a manager add free agents or place waiver claims.
import SwiftUI

struct TeamPlayerSearch: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let roster: LeagueRoster
    let onSaved: () async -> Void

    @State private var query = ""
    @State private var freshRoster: LeagueRoster?
    @State private var page: PlayerPage?
    @State private var loading = false
    @State private var error: String?
    @State private var picked: TeamPlayer?

    private var activeRoster: LeagueRoster { freshRoster ?? roster }

    var body: some View {
        NavigationStack {
            List {
                if let error {
                    Section {
                        Text(error).foregroundStyle(MosaicPalette.bad)
                        Button("Try again") { Task { await search() } }
                    }
                }
                if loading { ProgressView("Searching players") }
                if let page {
                    Section("Players") {
                        ForEach(page.players) { player in
                            HStack(spacing: 10) {
                                NavigationLink {
                                    TeamPlayerProfileView(session: session, selected: selected,
                                                          leagueID: roster.leagueId, playerID: player.id)
                                } label: {
                                    VStack(alignment: .leading, spacing: 3) {
                                        Text(player.fullName).font(.subheadline.weight(.semibold))
                                        Text("\(player.positions.joined(separator: "/")) · \(player.teamAbbrev ?? "")")
                                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                                        if let owner = player.ownerName, !owner.isEmpty {
                                            Text(owner).font(.caption).foregroundStyle(MosaicPalette.soft)
                                        } else if player.waiverUntil != nil {
                                            Text("On waivers").font(.caption).foregroundStyle(MosaicPalette.gold)
                                        }
                                    }
                                }
                                if player.ownerName?.isEmpty != false {
                                    Button(player.waiverUntil == nil ? "Add" : "Claim") { picked = player }
                                        .buttonStyle(.bordered)
                                }
                            }
                            .frame(minHeight: 50)
                        }
                    }
                    if page.players.isEmpty && !loading {
                        Text("No players match this search. Try a name or position.")
                            .foregroundStyle(MosaicPalette.soft)
                    }
                    if page.page * page.perPage < page.total {
                        Button("Load more") { Task { await search(next: true) } }
                    }
                } else if !loading {
                    Text("Search the \(roster.name) player pool by name.")
                        .foregroundStyle(MosaicPalette.soft)
                }
            }
            .scrollContentBackground(.hidden).background(MosaicPalette.ground)
            .searchable(text: $query, prompt: "Search players")
            .navigationTitle("Find players")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .topBarTrailing) { Button("Done") { dismiss() } } }
            .task(id: query) {
                try? await Task.sleep(for: .milliseconds(300))
                if !Task.isCancelled { await search() }
            }
            .sheet(item: $picked) { player in
                TeamPlayerAdd(session: session, selected: selected, roster: activeRoster,
                              player: player, onSaved: {
                                  await onSaved()
                                  await refreshRoster()
                                  await search()
                              })
            }
        }
        .preferredColorScheme(.dark)
    }

    @MainActor private func refreshRoster() async {
        guard let token = session.token else { return }
        if let detail = try? await session.api.myFranchise(for: selected, token: token) {
            freshRoster = detail.rosters.first { $0.leagueId == roster.leagueId }
        }
    }

    @MainActor private func search(next: Bool = false) async {
        guard let token = session.token else { return }
        loading = true
        error = nil
        defer { loading = false }
        do {
            let target = next ? (page?.page ?? 0) + 1 : 1
            let found = try await session.api.players(for: selected, leagueID: roster.leagueId,
                                                      query: query, page: target, token: token)
            if next, let old = page {
                page = PlayerPage(players: old.players + found.players, total: found.total,
                                  page: found.page, perPage: found.perPage)
            } else { page = found }
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

private struct TeamPlayerAdd: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let roster: LeagueRoster
    let player: TeamPlayer
    let onSaved: () async -> Void

    @State private var list = "main"
    @State private var dropID = ""
    @State private var bid = 0
    @State private var waiverState: WaiverState?
    @State private var confirming = false
    @State private var saving = false
    @State private var error: String?

    private var onWaivers: Bool { player.waiverUntil != nil }
    private var waiverMode: String {
        waiverState?.rules?.mode ??
            selected.dynasty.leagues.first { $0.id == roster.leagueId }?.settings?.waivers?.mode ?? "none"
    }
    private var waiverBudget: Int {
        waiverState?.standings.first {
            $0.franchiseId == selected.membership.franchiseId
        }?.budgetLeft ?? selected.dynasty.leagues.first {
            $0.id == roster.leagueId
        }?.settings?.waivers?.budget ?? 0
    }
    private var currentCount: Int { roster.players.filter { $0.list == list }.count }
    private var limit: Int { list == "main" ? roster.limits.main : roster.limits.reserve }
    private var valid: Bool {
        (currentCount < limit || !dropID.isEmpty) && (!onWaivers || waiverState != nil)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(player.fullName).font(.headline)
                    Text(onWaivers ? "This player is on waivers. Submit a claim to acquire him."
                         : "Add this free agent to your roster.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    if onWaivers && waiverState == nil {
                        ProgressView("Loading waiver rules")
                    }
                    Picker("Roster list", selection: $list) {
                        Text("Main").tag("main")
                        Text("Reserve").tag("reserve")
                    }
                    Picker("Drop to make room", selection: $dropID) {
                        Text("Nobody").tag("")
                        ForEach(roster.players.filter { $0.list == list }) { held in
                            Text(held.fullName).tag(held.playerId)
                        }
                    }
                    if onWaivers && waiverMode == "faab" {
                        Stepper("Bid \(bid) · \(waiverBudget) available", value: $bid, in: 0...max(0, waiverBudget))
                    }
                    if onWaivers, let standing = waiverState?.standings.first(where: {
                        $0.franchiseId == selected.membership.franchiseId
                    }), let priority = standing.priority {
                        Text("Waiver priority #\(priority)").font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    }
                    if onWaivers, let limit = waiverState?.weeklyLimit, limit > 0 {
                        Text("Weekly acquisitions: \(waiverState?.acquisitions ?? 0)/\(limit)")
                            .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    }
                } footer: {
                    Text(currentCount >= limit && dropID.isEmpty
                         ? "This list is full. Choose someone to drop."
                         : "Position eligibility, roster limits, and acquisition rules apply.")
                }
                if let error {
                    Section {
                        Text(error).foregroundStyle(MosaicPalette.bad)
                        if onWaivers && waiverState == nil {
                            Button("Try again") { Task { await loadWaivers() } }
                        }
                    }
                }
            }
            .scrollContentBackground(.hidden).background(MosaicPalette.ground)
            .navigationTitle(onWaivers ? "Claim player" : "Add player")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { Button("Cancel") { dismiss() }.disabled(saving) }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Review") { confirming = true }.disabled(!valid || saving)
                }
            }
            .confirmationDialog(onWaivers ? "Submit waiver claim?" : "Add player?",
                                isPresented: $confirming, titleVisibility: .visible) {
                Button(onWaivers ? "Submit claim" : "Add player") { Task { await save() } }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(dropID.isEmpty ? "Add \(player.fullName) to \(list)."
                     : "Add \(player.fullName) to \(list) and drop a player in the same move.")
            }
            .task { if onWaivers { await loadWaivers() } }
        }
    }

    @MainActor private func loadWaivers() async {
        guard let token = session.token else { return }
        do {
            waiverState = try await session.api.waiverState(for: selected,
                                                           leagueID: roster.leagueId, token: token)
            error = nil
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    @MainActor private func save() async {
        guard valid, !saving, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            if onWaivers {
                try await session.api.claimWaiver(for: selected, leagueID: roster.leagueId,
                                                  playerID: player.id, dropPlayerID: dropID.isEmpty ? nil : dropID,
                                                  list: list, bid: bid, token: token)
            } else {
                try await session.api.changeRoster(for: selected, leagueID: roster.leagueId,
                                                   action: "add", playerID: player.id, list: list,
                                                   dropPlayerID: dropID.isEmpty ? nil : dropID, token: token)
            }
            await onSaved()
            dismiss()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

struct TeamPlayerProfileView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let leagueID: String
    let playerID: String

    @State private var profile: TeamPlayerProfile?
    @State private var error: String?
    @State private var watched = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                if let profile {
                    SportTag(sport: profile.player.competition)
                    Text(profile.player.fullName).font(.title2.bold())
                    Text("\(profile.player.positions.joined(separator: "/")) · \(profile.player.teamAbbrev ?? "")")
                        .foregroundStyle(MosaicPalette.soft)
                    if let injury = profile.player.injuryDesignation, !injury.isEmpty {
                        Label(injury, systemImage: "cross.case")
                            .foregroundStyle(MosaicPalette.bad)
                    }
                    Text(profile.player.status.capitalized).foregroundStyle(MosaicPalette.soft)
                    if let note = profile.player.note, !note.isEmpty { Text(note) }
                    if let eligibility = profile.eligibilityNote, !eligibility.isEmpty {
                        Text(eligibility).font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    }
                    if let ownership = profile.ownership?.first(where: { $0.leagueId == leagueID }) {
                        Label("\(ownership.franchiseName) · \(ownership.list.capitalized)",
                              systemImage: "shield")
                            .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    }
                    if let waiver = profile.waivers?.first(where: { $0.leagueId == leagueID }) {
                        Label("On waivers until \(SeasonRosterEditor.deadlineLabel(waiver.clearsAt))",
                              systemImage: "clock")
                            .font(.subheadline).foregroundStyle(MosaicPalette.gold)
                    }
                    Button(watched ? "Remove from watchlist" : "Add to watchlist",
                           systemImage: watched ? "star.slash" : "star") {
                        Task { await toggleWatch() }
                    }
                    .buttonStyle(.bordered)
                    if let seasons = profile.seasons, !seasons.isEmpty {
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Seasons").font(.headline)
                            ForEach(seasons) { season in
                                HStack {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text("\(season.season) · \(season.competition.uppercased())")
                                            .font(.subheadline.weight(.semibold))
                                        Text(season.team).font(.caption).foregroundStyle(MosaicPalette.soft)
                                    }
                                    Spacer()
                                    Text("\(season.games) GP · \(StandingsOrder.points(season.pointsPerGame)) FP/G")
                                        .font(.caption.monospacedDigit())
                                }
                            }
                        }
                        .padding(16)
                        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
                    }
                    if let games = profile.gameLog?.games, !games.isEmpty {
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Recent games").font(.headline)
                            ForEach(games.prefix(10)) { game in
                                HStack {
                                    Text("\(game.day) · \(game.awayAbbrev) @ \(game.homeAbbrev)")
                                        .font(.subheadline)
                                    Spacer(minLength: 5)
                                    Text("\(StandingsOrder.points(game.points)) FP")
                                        .font(.subheadline.monospacedDigit())
                                }
                            }
                        }
                        .padding(16)
                        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
                    }
                } else if let error {
                    StatePanel(symbol: "wifi.exclamationmark", title: "Could not load player",
                               detail: error, actionTitle: "Try again", action: { Task { await load() } })
                } else {
                    ProgressView("Loading player")
                }
            }
            .foregroundStyle(MosaicPalette.ink)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(20)
        }
        .background(MosaicPalette.ground)
        .navigationTitle("Player")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .task { await load() }
    }

    @MainActor private func load() async {
        guard let token = session.token else { return }
        do {
            let found = try await session.api.playerProfile(for: selected, playerID: playerID, token: token)
            guard selected.dynasty.leagues.contains(where: { $0.id == leagueID && $0.competition == found.player.competition }) else {
                throw APIError(message: "This player is not in the selected sport.", statusCode: nil)
            }
            profile = found
            watched = (try? await session.api.watchlist(for: selected, token: token))?.contains {
                $0.playerId == playerID
            } ?? false
            error = nil
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    @MainActor private func toggleWatch() async {
        guard let token = session.token else { return }
        do {
            try await session.api.setWatched(for: selected, playerID: playerID, on: !watched, token: token)
            watched.toggle()
        } catch { self.error = error.localizedDescription }
    }
}
