// Presents one sport roster with manager actions and read-only rival browsing.
import SwiftUI

struct SportTeamView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let franchiseID: String
    let initialRoster: LeagueRoster
    let onChanged: () async -> Void
    var preview = false

    @State private var roster: LeagueRoster?
    @State private var currentDynasty: Dynasty?
    @State private var standings: [StandingsTable] = []
    @State private var loading = true
    @State private var error: String?
    @State private var unavailable = false
    @State private var actionError: String?
    @State private var busy = false
    @State private var pending: TeamMutation?
    @State private var editingIdentity = false
    @State private var editingSeason = false
    @State private var editingSplit = false
    @State private var searching = false
    @State private var nicknamePlayer: RosterPlayer?
    @State private var injuredPlayer: RosterPlayer?
    @State private var watched: Set<String> = []
    @State private var blocked: Set<String> = []
    @State private var targeted: Set<String> = []

    private var shown: LeagueRoster { roster ?? initialRoster }
    private var mine: Bool { franchiseID == selected.membership.franchiseId }
    private var league: SportLeague? { selected.dynasty.leagues.first { $0.id == shown.leagueId } }
    private var main: [RosterPlayer] { shown.players.filter { $0.list == "main" } }
    private var reserve: [RosterPlayer] { shown.players.filter { $0.list == "reserve" } }
    private var rights: [RosterPlayer] { shown.players.filter { $0.list == "rights" } }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 26) {
                if let error {
                    StatePanel(symbol: "wifi.exclamationmark", title: "Could not refresh team",
                               detail: error, actionTitle: "Try again", action: { Task { await load() } })
                }
                if !unavailable {
                header
                if loading && roster == nil && !preview { ProgressView("Loading team") }
                if let actionError {
                    Label(actionError, systemImage: "exclamationmark.circle")
                        .font(.subheadline).foregroundStyle(MosaicPalette.bad)
                }
                if let season = shown.season, season.needsSetting && mine {
                    Button { editingSeason = true } label: {
                        Label("Set \(String(season.year)) season roster", systemImage: "checklist")
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .buttonStyle(.borderedProminent)
                    if let deadline = season.deadline {
                        Text("Deadline \(SeasonRosterEditor.deadlineLabel(deadline))")
                            .font(.caption).foregroundStyle(MosaicPalette.gold)
                    }
                }
                standingSummary
                if !rights.isEmpty { unsignedRights }
                LineupEditor(session: session, selected: selected, franchiseID: franchiseID,
                             roster: shown, onChanged: { await load() }, preview: preview)
                rosterSection("Main roster", players: main, limit: shown.limits.main)
                rosterSection("Reserve list", players: reserve, limit: shown.limits.reserve)
                if mine {
                    HStack {
                        Button("Find players") { searching = true }.buttonStyle(.borderedProminent)
                        Button("Set lists") { editingSplit = true }.buttonStyle(.bordered)
                            .disabled(shown.season?.locked == true)
                    }
                    if shown.season?.locked == true {
                        Text("Season lists are locked. Drops, free agents, trades, and eligible injury swaps still follow league rules.")
                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                }
                if let league {
                    if league.settings?.format?.type == "head_to_head" {
                        MatchupsSection(session: session, selected: selected, league: league, preview: preview)
                    }
                    TeamScheduleSection(session: session, selected: selected, league: league,
                                        franchiseID: franchiseID, preview: preview)
                }
                }
            }
            .foregroundStyle(MosaicPalette.ink)
            .padding(.horizontal, 20)
            .padding(.top, 18)
            .padding(.bottom, 32)
        }
        .background(MosaicPalette.ground)
        .navigationTitle(shown.name)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .task { if !preview { await load() } }
        .sheet(isPresented: $editingIdentity) {
            TeamIdentityEditor(session: session, selected: selected, leagueID: shown.leagueId,
                               name: ownIdentityName, imageURL: ownIdentityImage, onSaved: { await load() })
        }
        .sheet(isPresented: $editingSeason) {
            SeasonRosterEditor(session: session, selected: selected, roster: shown, onSaved: { await load() })
        }
        .sheet(isPresented: $editingSplit) {
            RosterSplitEditor(session: session, selected: selected, roster: shown, onSaved: { await load() })
        }
        .sheet(isPresented: $searching) {
            TeamPlayerSearch(session: session, selected: selected, roster: shown, onSaved: { await load() })
        }
        .sheet(item: $nicknamePlayer) { player in
            TeamNicknameEditor(session: session, selected: selected, player: player, onSaved: { await load() })
        }
        .sheet(item: $injuredPlayer) { player in
            InjurySwapEditor(session: session, selected: selected, roster: shown,
                             injured: player, onSaved: { await load() })
        }
        .confirmationDialog(pending?.title ?? "Confirm roster change",
                            isPresented: Binding(get: { pending != nil }, set: { if !$0 { pending = nil } }),
                            titleVisibility: .visible) {
            if let pending {
                Button(pending.button, role: pending.action == "drop" ? .destructive : nil) {
                    Task { await execute(pending) }
                }
                Button("Cancel", role: .cancel) { self.pending = nil }
            }
        } message: {
            Text(pending?.detail ?? "")
        }
    }

    private var ownIdentityName: String {
        (currentDynasty ?? selected.dynasty).teamIdentities.first {
            $0.franchiseId == franchiseID && $0.leagueId == shown.leagueId
        }?.name ?? ""
    }

    private var ownIdentityImage: String {
        (currentDynasty ?? selected.dynasty).teamIdentities.first {
            $0.franchiseId == franchiseID && $0.leagueId == shown.leagueId
        }?.imageUrl ?? ""
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                TeamCrest(name: shown.teamName, imageURL: shown.imageUrl, baseURL: session.api.baseURL)
                VStack(alignment: .leading, spacing: 4) {
                    SportTag(sport: shown.competition)
                    Text(shown.teamName).font(.title3.bold())
                }
                Spacer(minLength: 4)
                if mine {
                    Button { editingIdentity = true } label: { Image(systemName: "pencil") }
                        .buttonStyle(.bordered).accessibilityLabel("Edit team identity")
                }
            }
            if !mine {
                Label("Viewing another franchise · read only", systemImage: "eye")
                    .font(.subheadline).foregroundStyle(MosaicPalette.soft)
            }
            if (shown.overage ?? 0) > 0 {
                Text("Over roster limit by \(shown.overage ?? 0)")
                    .font(.subheadline.weight(.semibold)).foregroundStyle(MosaicPalette.bad)
            }
        }
    }

    private var standingSummary: some View {
        let table = standings.first { $0.leagueId == shown.leagueId }
        let rank = table?.rows.firstIndex { $0.franchiseId == franchiseID }
        let row = rank.flatMap { table?.rows[$0] }
        return HStack(spacing: 20) {
            if let rank { metric("Place", "#\(rank + 1)") }
            if let wins = row?.wins, let losses = row?.losses {
                metric("Record", StandingsOrder.record(wins, losses, row?.ties))
            }
            if let points = row?.points ?? row?.pointsFor {
                metric("Points", StandingsOrder.points(points))
            }
            if rank == nil { Text("No standings yet").font(.subheadline).foregroundStyle(MosaicPalette.soft) }
            Spacer(minLength: 0)
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
    }

    private func metric(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label).font(.caption).foregroundStyle(MosaicPalette.soft)
            Text(value).font(.headline.monospacedDigit())
        }
    }

    private var unsignedRights: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Unsigned draft rights").font(.headline)
            Text("Sign each pick to main or reserve before its deadline. Unsigned rights can be released.")
                .font(.subheadline).foregroundStyle(MosaicPalette.soft)
            ForEach(rights) { player in
                playerRow(player)
                if let deadline = player.rightsUntil {
                    Text("Sign by \(SeasonRosterEditor.deadlineLabel(deadline))")
                        .font(.caption).foregroundStyle(MosaicPalette.gold)
                }
            }
        }
    }

    private func rosterSection(_ title: String, players: [RosterPlayer], limit: Int) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(title).font(.headline)
                Spacer()
                Text("\(players.count)/\(limit)").font(.subheadline.monospacedDigit())
                    .foregroundStyle(players.count > limit ? MosaicPalette.bad : MosaicPalette.soft)
            }
            if players.isEmpty {
                Text("No players on this list.").font(.subheadline).foregroundStyle(MosaicPalette.soft)
            }
            ForEach(players.sorted { $0.fullName < $1.fullName }) { player in
                playerRow(player)
                if player.id != players.last?.id { Divider().overlay(MosaicPalette.rule) }
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
    }

    private func playerRow(_ player: RosterPlayer) -> some View {
        HStack(alignment: .top, spacing: 8) {
            NavigationLink {
                TeamPlayerProfileView(session: session, selected: selected,
                                      leagueID: shown.leagueId, playerID: player.playerId)
            } label: {
                VStack(alignment: .leading, spacing: 3) {
                    Text(player.nickname?.isEmpty == false ? player.nickname! : player.fullName)
                        .font(.subheadline.weight(.semibold))
                    Text("\((player.positions ?? []).joined(separator: "/")) · \(player.teamAbbrev ?? "")")
                        .font(.caption).foregroundStyle(MosaicPalette.soft)
                    if let status = player.status, status == "prospect" {
                        Text("Prospect").font(.caption).foregroundStyle(MosaicPalette.gold)
                    } else if player.status == "inactive" {
                        Text("Inactive").font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                    if player.injuryReserveLocked == true {
                        Text("Injury swap · reserve for season")
                            .font(.caption).foregroundStyle(MosaicPalette.gold)
                    }
                    if let year = player.draftYear, let round = player.draftRound {
                        Text("\(String(year)) draft · Round \(round)")
                            .font(.caption).foregroundStyle(MosaicPalette.gold)
                    }
                    if let note = player.note, !note.isEmpty {
                        Text(note).font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                    if let injury = player.injuryDesignation, !injury.isEmpty {
                        Text(injury).font(.caption).foregroundStyle(MosaicPalette.bad)
                    }
                    if let locked = player.lockedUntil {
                        Text("Reserve lock until \(SeasonRosterEditor.deadlineLabel(locked))")
                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.plain)
            Menu {
                Button(watched.contains(player.playerId) ? "Remove from watchlist" : "Watch player",
                       systemImage: watched.contains(player.playerId) ? "star.slash" : "star") {
                    Task { await toggleWatch(player.playerId) }
                }
                if mine {
                    Button(blocked.contains(player.playerId) ? "Remove from trade block" : "Put on trade block",
                           systemImage: "arrow.left.arrow.right") {
                        Task { await toggleTrade(player.playerId, own: true) }
                    }
                    Button("Edit nickname", systemImage: "pencil") { nicknamePlayer = player }
                    if player.list == "rights" {
                        Button("Sign to main") { pending = .init(action: "move", player: player, list: "main",
                                                                  detail: "Sign this draft pick to your main roster?") }
                            .disabled(main.count >= shown.limits.main)
                        Button("Sign to reserve") { pending = .init(action: "move", player: player, list: "reserve",
                                                                     detail: "Sign this draft pick to reserve?") }
                            .disabled(reserve.count >= shown.limits.reserve)
                    } else if shown.season?.locked != true &&
                                player.injuryReserveLocked != true && player.lockedUntil == nil {
                        Button(player.list == "main" ? "Move to reserve" : "Move to main") {
                            pending = .init(action: "move", player: player,
                                            list: player.list == "main" ? "reserve" : "main",
                                            detail: "Move \(player.fullName) between roster lists?")
                        }
                    }
                    if player.list == "main", ["IR", "OUT", "IL7", "IL10", "IL15", "IL60"].contains(player.injuryDesignation ?? "") {
                        Button("Injury swap") { injuredPlayer = player }
                    }
                    Button(player.list == "rights" ? "Release rights" : "Drop player",
                           systemImage: "person.crop.circle.badge.minus", role: .destructive) {
                        pending = .init(action: "drop", player: player, list: nil,
                                        detail: dropExplanation(player))
                    }
                } else {
                    Button(targeted.contains(player.playerId) ? "Remove target" : "Mark as trade target",
                           systemImage: "scope") {
                        Task { await toggleTrade(player.playerId, own: false) }
                    }
                }
            } label: { Image(systemName: "ellipsis.circle").frame(width: 44, height: 44) }
                .accessibilityLabel("Actions for \(player.fullName)")
        }
        .frame(minHeight: 50)
    }

    private func dropExplanation(_ player: RosterPlayer) -> String {
        if player.list == "rights" { return "Release your unsigned rights to \(player.fullName)?" }
        let waiver = league?.settings?.waivers
        if waiver?.mode == "none" { return "\(player.fullName) becomes a free agent." }
        return "\(player.fullName) goes on waivers for \(waiver?.hours ?? 0) hours before becoming a free agent."
    }

    @MainActor
    private func execute(_ command: TeamMutation) async {
        guard mine, !busy, let token = session.token else { return }
        pending = nil
        busy = true
        actionError = nil
        defer { busy = false }
        do {
            try await session.api.changeRoster(for: selected, leagueID: shown.leagueId,
                                               action: command.action, playerID: command.player.playerId,
                                               list: command.list, token: token)
            await load()
            await onChanged()
        } catch {
            actionError = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    @MainActor
    private func toggleWatch(_ id: String) async {
        guard let token = session.token else { return }
        let on = !watched.contains(id)
        do {
            try await session.api.setWatched(for: selected, playerID: id, on: on, token: token)
            if on { watched.insert(id) } else { watched.remove(id) }
        } catch { actionError = error.localizedDescription }
    }

    @MainActor
    private func toggleTrade(_ id: String, own: Bool) async {
        guard let token = session.token else { return }
        let on = !(own ? blocked : targeted).contains(id)
        do {
            try await session.api.setTradeFlag(for: selected, leagueID: shown.leagueId,
                                               playerID: id, on: on, ownPlayer: own, token: token)
            if own {
                if on { blocked.insert(id) } else { blocked.remove(id) }
            } else {
                if on { targeted.insert(id) } else { targeted.remove(id) }
            }
        } catch { actionError = error.localizedDescription }
    }

    @MainActor
    private func load() async {
        guard let token = session.token else { return }
        loading = true
        error = nil
        defer { loading = false }
        do {
            let freshDynasty = try await session.api.dynasty(token: token)
            guard freshDynasty.id == selected.id,
                  freshDynasty.franchises.contains(where: { $0.id == franchiseID }),
                  freshDynasty.leagues.contains(where: { $0.id == initialRoster.leagueId }) else {
                unavailable = true
                throw APIError(message: "This team is no longer in the selected league.", statusCode: nil)
            }
            async let detail = session.api.franchise(for: selected, franchiseID: franchiseID, token: token)
            async let tables = session.api.standings(for: selected, token: token)
            async let watch = session.api.watchlist(for: selected, token: token)
            async let market = session.api.tradeMarket(for: selected, token: token)
            let (fetched, newTables) = try await (detail, tables)
            guard let fresh = fetched.rosters.first(where: { $0.leagueId == initialRoster.leagueId }) else {
                throw APIError(message: "This sport team is no longer available.", statusCode: nil)
            }
            roster = fresh
            unavailable = false
            currentDynasty = freshDynasty
            standings = newTables
            watched = Set((try? await watch)?.map(\.playerId) ?? [])
            if let flags = try? await market {
                blocked = Set(flags.block.filter { $0.leagueId == fresh.leagueId }.map(\.playerId))
                targeted = Set(flags.targets.filter { $0.leagueId == fresh.leagueId }.map(\.playerId))
            }
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

private struct TeamMutation {
    let action: String
    let player: RosterPlayer
    let list: String?
    let detail: String
    var title: String { action == "drop" ? "Remove \(player.fullName)?" : "Change roster?" }
    var button: String { action == "drop" ? "Remove player" : "Confirm change" }
}

private struct TeamScheduleSection: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let league: SportLeague
    let franchiseID: String
    let preview: Bool
    @State private var periods: [SchedulePeriod] = []
    @State private var error: String?
    @State private var selectedMatchup: TeamScheduleMatchup?

    var body: some View {
        DisclosureGroup("Schedule") {
            VStack(alignment: .leading, spacing: 12) {
                if let error {
                    Text(error).font(.subheadline).foregroundStyle(MosaicPalette.bad)
                    Button("Try again") { Task { await load() } }
                } else if periods.isEmpty {
                    Text("No schedule has been published.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                } else {
                    ForEach(periods) { period in
                        let games = period.matchups.filter {
                            $0.homeFranchiseId == franchiseID || $0.awayFranchiseId == franchiseID
                        }
                        ForEach(games) { game in
                            let otherID = game.homeFranchiseId == franchiseID ? game.awayFranchiseId : game.homeFranchiseId
                            let opponent = selected.dynasty.franchises.first { $0.id == otherID }?.name ?? "Bye"
                            Button {
                                selectedMatchup = TeamScheduleMatchup(id: game.id)
                            } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 3) {
                                        Text("Round \(period.seq) · \(opponent)").font(.subheadline.weight(.semibold))
                                        Text("\(period.startsOn) – \(period.endsOn) · \(game.final ? "Final" : "Scheduled")")
                                            .font(.caption).foregroundStyle(MosaicPalette.soft)
                                    }
                                    Spacer()
                                    Image(systemName: "chevron.right").font(.caption)
                                }
                                .frame(minHeight: 44)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
            .padding(.top, 10)
        }
        .font(.headline)
        .task { if !preview { await load() } }
        .sheet(item: $selectedMatchup) { choice in
            MatchupDetailSheet(session: session, selected: selected, league: league,
                               matchupID: choice.id, preview: preview)
        }
    }

    private func load() async {
        guard let token = session.token else { return }
        do { periods = try await session.api.schedule(for: selected, leagueID: league.id, token: token) }
        catch { self.error = error.localizedDescription }
    }
}

private struct TeamScheduleMatchup: Identifiable { let id: String }

#Preview("Sport team") {
    NavigationStack {
        SportTeamView(session: PreviewFixtures.signedInStore,
                      selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                      franchiseID: "franchise-1", initialRoster: PreviewFixtures.activeRoster,
                      onChanged: {}, preview: true)
    }
}
