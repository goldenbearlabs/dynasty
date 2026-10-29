// Browses the selected dynasty's head-to-head pairings and season schedule on iPhone.
import SwiftUI

enum MatchupsLogic {
    static func today() -> String {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: Date())
    }

    static func preferredSport(_ views: [(SportLeague, MatchupsData)], today: String) -> String? {
        views.first(where: { item in
            guard let period = item.1.period else { return false }
            return period.startsOn <= today && period.endsOn >= today && !item.1.matchups.isEmpty
        })?.0.id ?? views.first(where: { $0.1.period != nil && !$0.1.matchups.isEmpty })?.0.id
            ?? views.first(where: { $0.1.period != nil })?.0.id ?? views.first?.0.id
    }

    static func ordered(_ matchups: [MatchupCard], mine: String) -> [MatchupCard] {
        matchups.enumerated().sorted { a, b in
            let aMine = a.element.homeFranchiseId == mine || a.element.awayFranchiseId == mine
            let bMine = b.element.homeFranchiseId == mine || b.element.awayFranchiseId == mine
            return aMine == bMine ? a.offset < b.offset : aMine
        }.map(\.element)
    }

    static func schedule(_ periods: [SchedulePeriod], team: String?) -> [SchedulePeriod] {
        guard let team else { return periods }
        return periods.map { period in
            SchedulePeriod(id: period.id, seq: period.seq, startsOn: period.startsOn,
                           endsOn: period.endsOn, isPlayoff: period.isPlayoff,
                           matchups: period.matchups.filter {
                               $0.homeFranchiseId == team || $0.awayFranchiseId == team
                           })
        }
    }

    static func label(_ period: MatchupPeriod, periods: [MatchupPeriod], days: Int?) -> String {
        let regular = periods.filter { !$0.isPlayoff }.count
        if period.isPlayoff {
            return period.seq == periods.last?.seq ? "Finals" : "Playoffs · Round \(period.seq - regular)"
        }
        return "\(days == 7 ? "Week" : "Matchup") \(period.seq)"
    }
}

struct MatchupsView: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let active: Bool
    var preview = false

    @State private var sportID: String?
    @State private var matchups: MatchupsData?
    @State private var schedule: [SchedulePeriod] = []
    @State private var standings: [StandingsTable] = []
    @State private var dynasty: Dynasty?
    @State private var matchupID: String?
    @State private var periodSeq: Int?
    @State private var teamFilter: String?
    @State private var showingSchedule = false
    @State private var loading = true
    @State private var error: String?
    @State private var revision = 0

    private var leagues: [SportLeague] {
        (dynasty ?? selected.dynasty).leagues.filter { $0.settings?.format?.type == "head_to_head" }
    }
    private var league: SportLeague? { leagues.first { $0.id == sportID } }
    private var ordered: [MatchupCard] {
        MatchupsLogic.ordered(matchups?.matchups ?? [], mine: selected.membership.franchiseId)
    }
    private var currentMatchup: MatchupCard? { ordered.first { $0.id == matchupID } }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                if leagues.isEmpty {
                    StatePanel(symbol: "sportscourt", title: "No head-to-head sports",
                               detail: "This league has no matchup schedule.")
                        .padding(.horizontal, 20)
                } else {
                    Picker("View", selection: $showingSchedule) {
                        Text("Matchups").tag(false)
                        Text("Schedule").tag(true)
                    }
                    .pickerStyle(.segmented).padding(.horizontal, 20)
                    sportPicker
                    if loading && matchups == nil {
                        ProgressView("Loading matchups").padding(20)
                    } else if let error {
                        StatePanel(symbol: "wifi.exclamationmark", title: "Could not load matchups",
                                   detail: error, actionTitle: "Try again",
                                   action: { Task { await loadCurrent() } })
                            .padding(.horizontal, 20)
                    } else if showingSchedule {
                        scheduleContent.padding(.horizontal, 20)
                    } else {
                        matchupsContent
                    }
                }
            }
            .padding(.top, 12).padding(.bottom, 28)
        }
        .background(MosaicPalette.ground)
        .refreshable { await loadCurrent() }
        .task { if preview { loadPreview() } else { await initialize() } }
        .task(id: active ? sportID : nil) {
            guard active, !preview, let league, let token = session.token,
                  let url = session.api.scoresSocketURL(sport: league.competition) else { return }
            await ScoreFeed.run(urls: [url], token: token) {
                await refreshLive()
            }
        }
    }

    private var sportPicker: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 8) {
                ForEach(leagues) { sport in
                    Button {
                        guard sportID != sport.id else { return }
                        sportID = sport.id
                        matchups = nil
                        periodSeq = nil
                        matchupID = nil
                        teamFilter = nil
                        Task { await loadCurrent() }
                    } label: {
                        SportTag(sport: sport.competition)
                            .padding(.horizontal, 14).frame(minHeight: 44)
                            .background(sportID == sport.id ? MosaicPalette.raised : MosaicPalette.surface,
                                        in: Capsule())
                            .overlay(Capsule().stroke(sportID == sport.id ? MosaicPalette.brand : .clear))
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(sportID == sport.id ? .isSelected : [])
                }
            }
            .padding(.horizontal, 20).padding(.vertical, 2)
        }
        .scrollIndicators(.hidden)
        .accessibilityLabel("Matchup sports")
    }

    @ViewBuilder private var matchupsContent: some View {
        if let league, let data = matchups, let period = data.period {
            VStack(alignment: .leading, spacing: 15) {
                periodControls(period, periods: data.periods, league: league)
                if ordered.isEmpty {
                    StatePanel(symbol: "calendar.badge.clock", title: "Pairings not set yet",
                               detail: period.isPlayoff
                               ? "This playoff round will be paired when the previous round finishes."
                               : "The commissioner has not set this round's matchups.")
                        .padding(.horizontal, 20)
                } else {
                    ScrollView(.horizontal) {
                        HStack(spacing: 10) {
                            ForEach(ordered) { matchup in
                                pairingButton(matchup, period: period, league: league)
                            }
                        }
                        .padding(.horizontal, 20).padding(.vertical, 2)
                    }
                    .scrollIndicators(.hidden)
                    if let currentMatchup {
                        MatchupDetailView(session: session, selected: selected, league: league,
                                          matchup: currentMatchup, dynasty: dynasty ?? selected.dynasty,
                                          standings: standings, revision: revision, preview: preview)
                            .id(currentMatchup.id)
                            .padding(.horizontal, 20)
                    }
                }
            }
        } else {
            StatePanel(symbol: "calendar", title: "No matchups scheduled",
                       detail: "Matchups appear after the commissioner starts this sport's season.")
                .padding(.horizontal, 20)
        }
    }

    private func periodControls(_ period: MatchupPeriod, periods: [MatchupPeriod], league: SportLeague) -> some View {
        HStack(spacing: 10) {
            Button { changePeriod(period.seq - 1) } label: { Image(systemName: "chevron.left") }
                .disabled(period.seq <= (periods.first?.seq ?? 1))
                .accessibilityLabel("Previous matchup period")
            Spacer(minLength: 0)
            Menu {
                ForEach(periods) { item in
                    Button(MatchupsLogic.label(item, periods: periods,
                                               days: league.settings?.format?.matchupDays)) {
                        changePeriod(item.seq)
                    }
                }
            } label: {
                VStack(spacing: 3) {
                    Text(MatchupsLogic.label(period, periods: periods,
                                             days: league.settings?.format?.matchupDays))
                        .font(.headline)
                    Text("\(period.startsOn) – \(period.endsOn)")
                        .font(.caption).foregroundStyle(MosaicPalette.soft)
                    if period.doubleGames == true {
                        Text("Double games").font(.caption2).foregroundStyle(MosaicPalette.gold)
                    }
                }
                .frame(maxWidth: .infinity)
            }
            Spacer(minLength: 0)
            Button { changePeriod(period.seq + 1) } label: { Image(systemName: "chevron.right") }
                .disabled(period.seq >= (periods.last?.seq ?? period.seq))
                .accessibilityLabel("Next matchup period")
        }
        .buttonStyle(.bordered)
        .padding(.horizontal, 20)
    }

    private func pairingButton(_ matchup: MatchupCard, period: MatchupPeriod,
                               league: SportLeague) -> some View {
        let mine = matchup.homeFranchiseId == selected.membership.franchiseId ||
                   matchup.awayFranchiseId == selected.membership.franchiseId
        return Button { matchupID = matchup.id } label: {
            VStack(alignment: .leading, spacing: 5) {
                Text(mine ? "Your matchup" : period.isPlayoff
                     ? matchup.bracket == "consolation" ? "Consolation" : "Championship"
                     : "Matchup")
                    .font(.caption2.weight(.semibold)).foregroundStyle(MosaicPalette.soft)
                HStack {
                    Text(name(matchup.homeFranchiseId, league: league)).lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                    Spacer()
                    Text(StandingsOrder.points(matchup.homePoints)).monospacedDigit()
                }
                HStack {
                    Text(name(matchup.awayFranchiseId, league: league)).lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                    Spacer()
                    Text(matchup.awayFranchiseId == nil ? "—" : StandingsOrder.points(matchup.awayPoints))
                        .monospacedDigit()
                }
            }
            .font(.subheadline)
            .foregroundStyle(MosaicPalette.ink)
            .padding(12).frame(width: dynamicTypeSize.isAccessibilitySize ? 300 : 220,
                               alignment: .leading)
            .background(matchupID == matchup.id ? MosaicPalette.raised : MosaicPalette.surface,
                        in: RoundedRectangle(cornerRadius: 14))
            .overlay(RoundedRectangle(cornerRadius: 14)
                .stroke(matchupID == matchup.id ? MosaicPalette.sport(league.competition) : .clear))
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(matchupID == matchup.id ? .isSelected : [])
    }

    private var scheduleContent: some View {
        VStack(alignment: .leading, spacing: 14) {
            Menu {
                Button("All teams") { teamFilter = nil }
                ForEach((dynasty ?? selected.dynasty).franchises) { franchise in
                    Button(name(franchise.id, league: league)) { teamFilter = franchise.id }
                }
            } label: {
                Label(teamFilter.map { name($0, league: league) } ?? "All teams", systemImage: "line.3.horizontal.decrease")
                    .font(.subheadline.weight(.semibold))
            }
            if schedule.isEmpty {
                StatePanel(symbol: "calendar", title: "No schedule yet",
                           detail: "The commissioner has not published this sport's schedule.")
            } else {
                ForEach(MatchupsLogic.schedule(schedule, team: teamFilter)) { period in
                    VStack(alignment: .leading, spacing: 10) {
                        HStack {
                            Text(period.isPlayoff ? "Playoffs · Round \(period.seq - schedule.filter { !$0.isPlayoff }.count)"
                                 : "\(league?.settings?.format?.matchupDays == 7 ? "Week" : "Matchup") \(period.seq)")
                                .font(.headline)
                            Spacer()
                            Text(period.startsOn == period.endsOn ? period.startsOn :
                                 "\(period.startsOn) – \(period.endsOn)")
                                .font(.caption).foregroundStyle(MosaicPalette.soft)
                        }
                        if period.matchups.isEmpty {
                            Text(teamFilter == nil ? "Pairings to be decided" : "No matchup this round")
                                .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                        }
                        ForEach(period.matchups) { matchup in
                            Button {
                                showingSchedule = false
                                periodSeq = period.seq
                                matchupID = matchup.id
                                Task { await loadCurrent() }
                            } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 4) {
                                        if period.isPlayoff {
                                            Text(matchup.bracket == "consolation" ? "Consolation" : "Championship")
                                                .font(.caption2.weight(.semibold))
                                                .foregroundStyle(MosaicPalette.sport(league?.competition ?? ""))
                                        }
                                        Text(name(matchup.homeFranchiseId, league: league))
                                        Text(matchup.awayFranchiseId.map { name($0, league: league) } ?? "Bye")
                                    }
                                    Spacer()
                                    if period.startsOn <= MatchupsLogic.today() && matchup.awayFranchiseId != nil {
                                        VStack(alignment: .trailing, spacing: 4) {
                                            if let teamFilter, matchup.final {
                                                let ours = matchup.homeFranchiseId == teamFilter
                                                    ? matchup.homePoints : matchup.awayPoints
                                                let theirs = matchup.homeFranchiseId == teamFilter
                                                    ? matchup.awayPoints : matchup.homePoints
                                                Text(ours > theirs ? "W" : ours < theirs ? "L" : "T")
                                                    .font(.caption.weight(.bold))
                                                    .foregroundStyle(ours > theirs ? MosaicPalette.sport(league?.competition ?? "") :
                                                        ours < theirs ? MosaicPalette.bad : MosaicPalette.soft)
                                            } else if !matchup.final {
                                                Text("Live").font(.caption2.weight(.semibold))
                                                    .foregroundStyle(MosaicPalette.sport(league?.competition ?? ""))
                                            }
                                            Text("\(StandingsOrder.points(matchup.homePoints)) – \(StandingsOrder.points(matchup.awayPoints))")
                                                .monospacedDigit()
                                        }
                                    } else {
                                        Image(systemName: "chevron.right").foregroundStyle(MosaicPalette.soft)
                                    }
                                }
                                .font(.subheadline.weight(.medium)).foregroundStyle(MosaicPalette.ink)
                                .padding(12).background(MosaicPalette.raised,
                                                        in: RoundedRectangle(cornerRadius: 12))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(14)
                    .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
                }
            }
        }
    }

    private func name(_ id: String?, league: SportLeague?) -> String {
        guard let id else { return "Bye" }
        let current = dynasty ?? selected.dynasty
        let identity = current.teamIdentities.first { $0.franchiseId == id && $0.leagueId == league?.id }
        return identity?.name.isEmpty == false ? identity!.name
            : current.franchises.first { $0.id == id }?.name ?? "Franchise"
    }

    private func changePeriod(_ seq: Int) {
        periodSeq = seq
        matchupID = nil
        Task { await loadCurrent() }
    }

    @MainActor private func initialize() async {
        guard let token = session.token, !leagues.isEmpty else { loading = false; return }
        var views: [(SportLeague, MatchupsData)] = []
        for league in leagues {
            if let data = try? await session.api.matchups(for: selected, leagueID: league.id,
                                                           period: nil, token: token) {
                views.append((league, data))
            }
        }
        sportID = MatchupsLogic.preferredSport(views, today: MatchupsLogic.today())
        await loadCurrent()
    }

    @MainActor private func loadCurrent() async {
        guard let league, let token = session.token else { loading = false; return }
        let requestedID = league.id
        loading = true
        error = nil
        do {
            let fetched = try await session.api.matchups(for: selected, leagueID: requestedID,
                                                         period: periodSeq, token: token)
            let season = try await session.api.schedule(for: selected, leagueID: requestedID, token: token)
            let tables = try await session.api.standings(for: selected, token: token)
            let current = try await session.api.dynasty(token: token)
            guard sportID == requestedID else { return }
            dynasty = current
            matchups = fetched
            schedule = season
            standings = tables
            if !fetched.matchups.contains(where: { $0.id == matchupID }) {
                matchupID = MatchupsLogic.ordered(fetched.matchups,
                                                 mine: selected.membership.franchiseId).first?.id
            }
            revision += 1
        } catch {
            guard sportID == requestedID else { return }
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
        loading = false
    }

    @MainActor private func refreshLive() async {
        guard active, !showingSchedule, let league, let token = session.token else { return }
        do {
            let fresh = try await session.api.matchups(for: selected, leagueID: league.id,
                                                       period: periodSeq, token: token)
            guard sportID == league.id else { return }
            matchups = fresh
            revision += 1
        } catch {
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }

    private func loadPreview() {
        sportID = "nba-1"
        matchups = PreviewFixtures.matchups
        matchupID = PreviewFixtures.matchups.matchups.first?.id
        standings = PreviewFixtures.dashboard.standings
        loading = false
    }
}

#Preview("Matchups") {
    NavigationStack {
        MatchupsView(session: PreviewFixtures.signedInStore,
                     selected: SelectedLeague(membership: PreviewFixtures.leagues[0],
                                              dynasty: PreviewFixtures.dynasty),
                     active: true, preview: true)
    }
    .preferredColorScheme(.dark)
}
