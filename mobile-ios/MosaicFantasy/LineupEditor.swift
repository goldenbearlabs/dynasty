// Edits daily and weekly starters using server slots, locks, counted games, and stats.
import SwiftUI

struct LineupDraft {
    let source: LineupData
    var seats: [String?]
    var picks: [String: String]

    init(_ source: LineupData) {
        self.source = source
        let slots = source.slots.flatMap { Array(repeating: $0, count: max(0, $0.count)) }
        var occupied: [String?] = Array(repeating: nil, count: slots.count)
        for player in source.players where !player.slot.isEmpty {
            if let index = slots.indices.first(where: { slots[$0].name == player.slot && occupied[$0] == nil }) {
                occupied[index] = player.playerId
            }
        }
        seats = occupied
        picks = Dictionary(uniqueKeysWithValues: source.players.map {
            ($0.playerId, $0.countsLast ? "auto" : $0.countsFrom)
        })
    }

    var slots: [LineupSlot] { source.slots.flatMap { Array(repeating: $0, count: max(0, $0.count)) } }
    var weekly: Bool { source.day != source.lastDay }
    var dirty: Bool {
        source.players.contains { player in
            slot(of: player.playerId) != player.slot ||
                picks[player.playerId] != (player.countsLast ? "auto" : player.countsFrom)
        }
    }

    func slot(of id: String) -> String {
        guard let index = seats.firstIndex(where: { $0 == id }) else { return "" }
        return slots[index].name
    }

    func frozen(_ player: LineupPlayer, canEdit: Bool) -> Bool {
        !canEdit || !source.locked.isEmpty || player.locked
    }

    func fits(_ player: LineupPlayer, in slot: LineupSlot) -> Bool {
        player.starterEligible &&
            (slot.positions.contains("*") || player.positions.contains(where: slot.positions.contains))
    }

    func canPlace(_ id: String, in target: Int?, canEdit: Bool) -> Bool {
        guard let player = source.players.first(where: { $0.playerId == id }),
              !frozen(player, canEdit: canEdit) else { return false }
        guard let target else { return seats.contains(id) }
        guard slots.indices.contains(target), fits(player, in: slots[target]),
              seats[target] != id else { return false }
        if let otherID = seats[target], let other = source.players.first(where: { $0.playerId == otherID }) {
            return !frozen(other, canEdit: canEdit)
        }
        return true
    }

    mutating func place(_ id: String, in target: Int?, canEdit: Bool) {
        guard canPlace(id, in: target, canEdit: canEdit) else { return }
        let from = seats.firstIndex(where: { $0 == id })
        guard let target else { if let from { seats[from] = nil }; return }
        let previous = seats[target]
        seats[target] = id
        if let from {
            if let previous, let other = source.players.first(where: { $0.playerId == previous }),
               fits(other, in: slots[from]) {
                seats[from] = previous
            } else { seats[from] = nil }
        }
        if slots[target].gamesPerWeek > 0,
           source.players.first(where: { $0.playerId == id })?.slot != slots[target].name {
            picks[id] = "auto"
        }
        if let from, let previous, seats[from] == previous, slots[from].gamesPerWeek > 0,
           source.players.first(where: { $0.playerId == previous })?.slot != slots[from].name {
            picks[previous] = "auto"
        }
    }

    func entries() -> [LineupEntry] {
        seats.enumerated().compactMap { index, id in
            guard let id else { return nil }
            let choice = picks[id] ?? ""
            return LineupEntry(slot: slots[index].name, playerId: id,
                               countsFrom: choice == "auto" ? "" : choice, auto: choice == "auto")
        }
    }

    func countedGames(_ player: LineupPlayer) -> [LineupGame] {
        guard let slot = slots.first(where: { $0.name == self.slot(of: player.playerId) }) else { return [] }
        let limit = slot.gamesPerWeek * (source.doubled == nil ? 1 : 2)
        if limit == 0 { return player.games }
        let pick = picks[player.playerId] ?? ""
        if self.slot(of: player.playerId) == player.slot &&
            pick == (player.countsLast ? "auto" : player.countsFrom) {
            return player.games.filter(\.counts)
        }
        if pick == "auto" { return [] }
        let from = pick.isEmpty ? player.games.first(where: { $0.status == "scheduled" })?.day ?? "" : pick
        return Array(player.games.filter { $0.day >= from }.prefix(limit))
    }
}

struct LineupEditor: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let franchiseID: String
    let roster: LeagueRoster
    let onChanged: () async -> Void
    var preview = false

    @State private var requestedDay: String?
    @State private var draft: LineupDraft?
    @State private var performance: RosterPerformance?
    @State private var scoring: [String: Double] = [:]
    @State private var window = "today"
    @State private var loading = true
    @State private var saving = false
    @State private var error: String?
    @State private var autoConfirm = false
    @State private var pendingDay: String?

    private var mine: Bool { franchiseID == selected.membership.franchiseId }
    private var shown: LineupDraft? { draft ?? (preview ? LineupDraft(PreviewFixtures.lineup) : nil) }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Text("Lineup").font(.title3.bold())
                Spacer()
                if mine, let shown, shown.source.locked.isEmpty {
                    Button("Auto-set") { autoConfirm = true }
                        .font(.subheadline).disabled(saving)
                }
            }
            if let error {
                Label(error, systemImage: "exclamationmark.circle")
                    .font(.subheadline).foregroundStyle(MosaicPalette.bad)
                Button("Try again") { Task { await load() } }.font(.subheadline)
            }
            if loading && shown == nil { ProgressView("Loading lineup") }
            if let shown {
                periodControls(shown)
                if !shown.source.locked.isEmpty {
                    Label(shown.source.locked, systemImage: "lock")
                        .font(.subheadline).foregroundStyle(MosaicPalette.gold)
                }
                if let doubled = shown.source.doubled {
                    Text("Double-games matchup · \(doubled.startsOn) – \(doubled.endsOn). Counted games and pitcher start limits double across this matchup.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                }
                statsPicker
                ForEach(shown.slots.indices, id: \.self) { index in
                    seat(index, draft: shown)
                }
                if shown.slots.isEmpty {
                    Text("No starting slots are configured for this sport.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                }
                Text("Bench").font(.headline).padding(.top, 6)
                let bench = shown.source.players.filter { shown.slot(of: $0.playerId).isEmpty }
                if bench.isEmpty {
                    Text("Everyone on the main roster is starting.")
                        .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                }
                ForEach(bench) { player in playerRow(player, draft: shown, seatIndex: nil) }
                if let starts = shown.source.starts { pitcherStarts(starts) }
                if !scoring.isEmpty {
                    DisclosureGroup("How fantasy points are scored") {
                        VStack(spacing: 7) {
                            ForEach(scoring.keys.sorted(), id: \.self) { key in
                                HStack {
                                    Text(key.replacingOccurrences(of: "_", with: " ").capitalized)
                                    Spacer()
                                    Text("\(StandingsOrder.points(scoring[key] ?? 0)) pts")
                                        .monospacedDigit()
                                }
                                .font(.caption).foregroundStyle(MosaicPalette.soft)
                            }
                        }
                        .padding(.top, 8)
                    }
                    .font(.subheadline.weight(.semibold))
                }
                if mine && shown.dirty {
                    Button(saving ? "Saving…" : "Save lineup") { Task { await save() } }
                        .buttonStyle(.borderedProminent).disabled(saving || !shown.source.locked.isEmpty)
                }
                Text(shown.weekly
                     ? "Only starters score. A limited-game slot counts the selected games; Auto counts the last eligible games played. Locked games cannot be changed."
                     : "Only starters score. Each player locks when the league's lineup rules say his game begins.")
                    .font(.caption).foregroundStyle(MosaicPalette.soft)
            }
        }
        .task(id: requestedDay) { if !preview { await load() } }
        .task(id: window) { if !preview { await loadStats() } }
        .confirmationDialog("Auto-set your lineup?", isPresented: $autoConfirm,
                            titleVisibility: .visible) {
            Button("Auto-set") { Task { await autoSet() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text((shown?.weekly == true ? "Set the best lineup for this week." : "Set the best lineup for every day left this week.") +
                 (shown?.dirty == true ? " Unsaved changes on this screen will be replaced." : ""))
        }
        .confirmationDialog("Discard unsaved lineup changes?",
                            isPresented: Binding(get: { pendingDay != nil },
                                                 set: { if !$0 { pendingDay = nil } }),
                            titleVisibility: .visible) {
            Button("Discard changes", role: .destructive) {
                requestedDay = pendingDay
                pendingDay = nil
            }
            Button("Keep editing", role: .cancel) { pendingDay = nil }
        } message: {
            Text("Save this lineup before moving to another \(shown?.weekly == true ? "week" : "day") to keep your changes.")
        }
    }

    private func periodControls(_ value: LineupDraft) -> some View {
        VStack(spacing: 8) {
            HStack {
                Button { moveDay(by: value.weekly ? -7 : -1) } label: { Image(systemName: "chevron.left") }
                    .accessibilityLabel(value.weekly ? "Previous week" : "Previous day")
                Spacer(minLength: 8)
                Text(periodLabel(value))
                    .font(.subheadline.weight(.semibold))
                    .multilineTextAlignment(.center)
                Spacer(minLength: 8)
                Button { moveDay(by: value.weekly ? 7 : 1) } label: { Image(systemName: "chevron.right") }
                    .accessibilityLabel(value.weekly ? "Next week" : "Next day")
            }
            HStack {
                Text(value.weekly ? "Week" : "Day")
                    .font(.caption).foregroundStyle(MosaicPalette.soft)
                Spacer()
                Button("Jump to today") {
                    selectDay(Date.now.formatted(.iso8601.year().month().day().dateSeparator(.dash)))
                }
                .font(.caption)
            }
        }
        .buttonStyle(.bordered)
    }

    private func periodLabel(_ draft: LineupDraft) -> String {
        let parser = DateFormatter()
        parser.dateFormat = "yyyy-MM-dd"
        parser.locale = Locale(identifier: "en_US_POSIX")
        guard let start = parser.date(from: draft.source.day) else { return draft.source.day }
        if draft.weekly, let end = parser.date(from: draft.source.lastDay) {
            return "\(start.formatted(.dateTime.month(.abbreviated).day())) – \(end.formatted(.dateTime.month(.abbreviated).day()))"
        }
        return start.formatted(.dateTime.month(.abbreviated).day().year())
    }

    private var statsPicker: some View {
        Picker("Statistics", selection: $window) {
            Text("Today").tag("today")
            Text("7 days").tag("7")
            Text("14 days").tag("14")
            Text("30 days").tag("30")
            Text("Current season").tag("current")
            ForEach(performance?.seasons.filter { $0.label != performance?.currentSeason } ?? []) { season in
                Text(season.label).tag("season:\(season.label)")
            }
        }
        .pickerStyle(.menu)
    }

    private func seat(_ index: Int, draft: LineupDraft) -> some View {
        let slot = draft.slots[index]
        let player = draft.seats[index].flatMap { id in draft.source.players.first { $0.playerId == id } }
        return VStack(alignment: .leading, spacing: 7) {
            HStack {
                Text(slot.name).font(.caption.weight(.bold)).foregroundStyle(MosaicPalette.sport(roster.competition))
                Spacer()
                if slot.gamesPerWeek > 0 {
                    let count = slot.gamesPerWeek * (draft.source.doubled == nil ? 1 : 2)
                    Text("\(count) \(count == 1 ? "game" : "games") count")
                        .font(.caption).foregroundStyle(MosaicPalette.soft)
                }
            }
            if let player {
                playerRow(player, draft: draft, seatIndex: index)
            } else if mine && draft.source.locked.isEmpty {
                Menu {
                    ForEach(draft.source.players.filter { draft.canPlace($0.playerId, in: index, canEdit: mine) }) { candidate in
                        Button(candidate.fullName) { self.draft?.place(candidate.playerId, in: index, canEdit: mine) }
                    }
                } label: {
                    Label("Choose starter", systemImage: "plus.circle")
                        .frame(minHeight: 44)
                }
            } else {
                Text("Empty starter slot").font(.subheadline).foregroundStyle(MosaicPalette.soft)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 14))
    }

    private func playerRow(_ player: LineupPlayer, draft: LineupDraft, seatIndex: Int?) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(spacing: 10) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(player.nickname.isEmpty ? player.fullName : player.nickname)
                        .font(.subheadline.weight(.semibold))
                    Text("\(player.positions.joined(separator: "/")) · \(player.teamAbbrev)")
                        .font(.caption).foregroundStyle(MosaicPalette.soft)
                    if !player.injury.isEmpty {
                        Text(player.injury).font(.caption).foregroundStyle(MosaicPalette.bad)
                    }
                }
                Spacer(minLength: 4)
                Text("\(StandingsOrder.points(player.points)) FP")
                    .font(.subheadline.weight(.semibold)).monospacedDigit()
                if mine && !draft.frozen(player, canEdit: mine) {
                    Menu {
                        ForEach(draft.slots.indices.filter { draft.canPlace(player.playerId, in: $0, canEdit: mine) }, id: \.self) { index in
                            Button("Start at \(draft.slots[index].name)") {
                                self.draft?.place(player.playerId, in: index, canEdit: mine)
                            }
                        }
                        if draft.canPlace(player.playerId, in: nil, canEdit: mine) {
                            Button("Move to bench") { self.draft?.place(player.playerId, in: nil, canEdit: mine) }
                        }
                    } label: { Image(systemName: "arrow.up.arrow.down.circle").frame(width: 44, height: 44) }
                        .accessibilityLabel("Move \(player.fullName)")
                } else if player.locked {
                    Image(systemName: "lock.fill").foregroundStyle(MosaicPalette.soft)
                        .accessibilityLabel("Player locked")
                }
            }
            if !player.starterEligible, !player.eligibilityNote.isEmpty {
                Text(player.eligibilityNote).font(.caption).foregroundStyle(MosaicPalette.gold)
            }
            if let seatIndex, draft.slots[seatIndex].gamesPerWeek > 0 {
                gamePicker(player, draft: draft)
            } else if !player.games.isEmpty {
                ForEach(player.games) { game in gameLine(game) }
            }
            if let stats = performance?.players.first(where: { $0.playerId == player.playerId }) {
                DisclosureGroup("Stats · \(stats.games) GP · \(StandingsOrder.points(stats.pointsPerGame)) FP/G") {
                    ForEach(stats.stats.keys.sorted(), id: \.self) { key in
                        HStack {
                            Text(key.replacingOccurrences(of: "_", with: " ").capitalized)
                            Spacer()
                            Text(StandingsOrder.points(stats.stats[key] ?? 0)).monospacedDigit()
                        }
                        .font(.caption).foregroundStyle(MosaicPalette.soft)
                    }
                }
                .font(.caption)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func gamePicker(_ player: LineupPlayer, draft: LineupDraft) -> some View {
        let choice = draft.picks[player.playerId] ?? ""
        return VStack(alignment: .leading, spacing: 5) {
            Text("Counted games").font(.caption.weight(.semibold)).foregroundStyle(MosaicPalette.soft)
            if mine && !draft.frozen(player, canEdit: mine) {
                Menu {
                    Button("Auto · last games played") { self.draft?.picks[player.playerId] = "auto" }
                    Button("Next game") { self.draft?.picks[player.playerId] = "" }
                    ForEach(Array(Set(player.games.filter { $0.status == "scheduled" }.map(\.day))).sorted(), id: \.self) { day in
                        Button("Start counting \(day)") { self.draft?.picks[player.playerId] = day }
                    }
                } label: {
                    HStack(spacing: 8) {
                        Text(choice == "auto" ? "Auto · last games played" :
                             choice.isEmpty ? "Next game" : "Start \(choice)")
                            .font(.subheadline.weight(.medium))
                            .multilineTextAlignment(.leading)
                        Spacer(minLength: 4)
                        Image(systemName: "chevron.up.chevron.down").font(.caption)
                    }
                    .frame(minHeight: 44)
                }
                .accessibilityLabel("Count games for \(player.fullName)")
            } else {
                Text(choice == "auto" ? "Auto" : choice.isEmpty ? "Next game" : choice)
                    .font(.caption)
            }
            ForEach(player.games) { game in
                HStack {
                    Image(systemName: draft.countedGames(player).contains(where: { $0.id == game.id })
                          ? "checkmark.circle.fill" : "circle")
                        .foregroundStyle(MosaicPalette.sport(roster.competition))
                    gameLine(game)
                }
            }
        }
    }

    private func gameLine(_ game: LineupGame) -> some View {
        HStack(spacing: 7) {
            Text("\(game.day) \(game.atHome ? "vs" : "@") \(game.opponent)")
            if !game.starter.isEmpty { Text(game.starter.capitalized) }
            Spacer()
            Text(game.status == "scheduled"
                 ? "Proj \(StandingsOrder.points(game.projected))"
                 : "\(StandingsOrder.points(game.points)) FP")
                .monospacedDigit()
        }
        .font(.caption).foregroundStyle(MosaicPalette.soft)
    }

    private func pitcherStarts(_ starts: PitcherStarts) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Pitcher starts · \(starts.made.filter(\.counts).count)/\(starts.limit)")
                .font(.headline)
            Text("Starts beyond the cap do not score. The server applies the cap in game order.")
                .font(.caption).foregroundStyle(MosaicPalette.soft)
            ForEach(Array(starts.made.enumerated()), id: \.offset) { _, start in
                Text("\(start.day) · \(start.fullName) · \(start.counts ? "Counts" : "Over cap")")
                    .font(.caption).foregroundStyle(start.counts ? MosaicPalette.ink : MosaicPalette.bad)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 14))
    }

    private func moveDay(by offset: Int) {
        guard let day = draft?.source.day else { return }
        let parser = DateFormatter()
        parser.dateFormat = "yyyy-MM-dd"
        parser.locale = Locale(identifier: "en_US_POSIX")
        guard let date = parser.date(from: day), let next = Calendar.current.date(byAdding: .day, value: offset, to: date) else { return }
        selectDay(parser.string(from: next))
    }

    private func selectDay(_ day: String) {
        if shown?.dirty == true { pendingDay = day }
        else { requestedDay = day }
    }

    @MainActor private func load() async {
        guard let token = session.token else { return }
        loading = true
        error = nil
        defer { loading = false }
        do {
            let found = try await session.api.lineup(for: selected, leagueID: roster.leagueId,
                                                     franchiseID: franchiseID, day: requestedDay, token: token)
            draft = LineupDraft(found)
            if let current = try? await session.api.dynasty(token: token), current.id == selected.id {
                scoring = current.leagues.first(where: { $0.id == roster.leagueId })?.settings?.scoring ?? [:]
            }
            await loadStats()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    @MainActor private func loadStats() async {
        guard let token = session.token, let draft else { return }
        do {
            let historical = window.hasPrefix("season:")
            performance = try await session.api.rosterPerformance(
                for: selected, leagueID: roster.leagueId, franchiseID: franchiseID,
                window: historical ? "season" : window,
                season: historical ? String(window.dropFirst(7)) : nil,
                day: window == "today" ? draft.source.day : nil, token: token)
        } catch { performance = nil }
    }

    @MainActor private func save() async {
        guard mine, !saving, let draft, draft.dirty, draft.source.locked.isEmpty,
              let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.saveLineup(for: selected, leagueID: roster.leagueId,
                                             day: draft.source.day, entries: draft.entries(), token: token)
            await load()
            await onChanged()
        } catch {
            let message = error.localizedDescription
            await load()
            self.error = message
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    @MainActor private func autoSet() async {
        guard mine, !saving, let day = draft?.source.day, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.autoSetLineup(for: selected, leagueID: roster.leagueId,
                                                day: day, token: token)
            await load()
            await onChanged()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}
