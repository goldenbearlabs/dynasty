// Shows each franchise's sport teams, season tasks, standings context, and draft picks.
import SwiftUI

struct TeamsView: View {
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    var preview = false

    @State private var viewedFranchiseID: String?
    @State private var currentDynasty: Dynasty?
    @State private var detail: FranchiseDetail?
    @State private var loadedFranchiseID: String?
    @State private var standings: [StandingsTable] = []
    @State private var matchups: [String: MatchupsData] = [:]
    @State private var loading = true
    @State private var error: String?
    @State private var editingSeason: LeagueRoster?
    @State private var editingIdentity = false

    private var viewedID: String { viewedFranchiseID ?? selected.membership.franchiseId }
    private var mine: Bool { viewedID == selected.membership.franchiseId }
    private var shown: FranchiseDetail? { preview ? PreviewFixtures.franchiseDetail : detail }
    private var dynasty: Dynasty { currentDynasty ?? selected.dynasty }
    private var due: [LeagueRoster] { mine ? SeasonRosterRules.due(shown?.rosters ?? []) : [] }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 26) {
                HStack {
                    Spacer()
                    Menu {
                        ForEach(dynasty.franchises.sorted { $0.name < $1.name }) { franchise in
                            Button(franchise.name) { viewedFranchiseID = franchise.id }
                        }
                    } label: {
                        Label("Franchises", systemImage: "person.2")
                            .font(.subheadline.weight(.semibold))
                    }
                    .accessibilityLabel("Browse franchises")
                }
                if loading && shown == nil {
                    ProgressView("Loading teams").frame(maxWidth: .infinity, alignment: .leading)
                } else if let error, shown == nil {
                    StatePanel(symbol: "wifi.exclamationmark", title: "Could not load teams",
                               detail: error, actionTitle: "Try again", action: { Task { await load() } })
                } else if let shown {
                    if let error {
                        Label("Could not refresh: \(error)", systemImage: "exclamationmark.circle")
                            .font(.subheadline).foregroundStyle(MosaicPalette.bad)
                    }
                    identity(shown.franchise)
                    if !mine {
                        Label("Viewing another franchise · read only", systemImage: "eye")
                            .font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    }
                    if !due.isEmpty { seasonTasks(due) }
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Sport teams").font(.headline)
                        ForEach(shown.rosters) { roster in
                            NavigationLink {
                                SportTeamView(session: session, selected: selected,
                                              franchiseID: viewedID, initialRoster: roster,
                                              onChanged: { await load() })
                            } label: { sportCard(roster) }
                            .buttonStyle(.plain)
                        }
                    }
                    picks(shown.picks)
                }
            }
            .foregroundStyle(MosaicPalette.ink)
            .padding(.horizontal, 20)
            .padding(.top, 12)
            .padding(.bottom, 32)
        }
        .background(MosaicPalette.ground)
        .refreshable { await load() }
        .task(id: viewedID) { if !preview { await load() } }
        .sheet(item: $editingSeason) { roster in
            SeasonRosterEditor(session: session, selected: selected, roster: roster,
                               onSaved: { await load() })
        }
        .sheet(isPresented: $editingIdentity) {
            TeamIdentityEditor(session: session, selected: selected, leagueID: nil,
                               name: shown?.franchise.name ?? "",
                               imageURL: shown?.franchise.imageUrl ?? "",
                               onSaved: { await load() })
        }
    }

    private func identity(_ franchise: DynastyFranchise) -> some View {
        HStack(spacing: 14) {
            TeamCrest(name: franchise.name, imageURL: franchise.imageUrl, baseURL: session.api.baseURL)
            VStack(alignment: .leading, spacing: 4) {
                Text(franchise.name).font(.title3.bold())
                if let manager = franchise.managerName, !manager.isEmpty {
                    Text(manager).font(.subheadline).foregroundStyle(MosaicPalette.soft)
                }
            }
            Spacer(minLength: 4)
            if mine {
                Button { editingIdentity = true } label: { Image(systemName: "pencil") }
                    .buttonStyle(.bordered).accessibilityLabel("Edit franchise identity")
            }
        }
        .frame(minHeight: 64)
    }

    private func seasonTasks(_ rosters: [LeagueRoster]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(rosters.count == 1 ? "Season roster to set" : "Season rosters to set").font(.headline)
            Text("Choose your main roster and reserve list before each deadline.")
                .font(.subheadline).foregroundStyle(MosaicPalette.soft)
            ForEach(rosters) { roster in
                Button { editingSeason = roster } label: {
                    HStack(spacing: 10) {
                        SportTag(sport: roster.competition)
                        Text("Set \(roster.name) roster").font(.subheadline.weight(.semibold))
                        Spacer(minLength: 4)
                        Image(systemName: "chevron.right").font(.caption.weight(.semibold))
                    }
                    .frame(minHeight: 44)
                }
                .buttonStyle(.plain)
                if let deadline = roster.season?.deadline {
                    Text("Locks \(SeasonRosterEditor.deadlineLabel(deadline))")
                        .font(.caption).foregroundStyle(MosaicPalette.gold)
                }
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.goldSurface, in: RoundedRectangle(cornerRadius: 16))
    }

    private func sportCard(_ roster: LeagueRoster) -> some View {
        let main = roster.players.filter { $0.list == "main" }.count
        let reserve = roster.players.filter { $0.list == "reserve" }.count
        let rights = roster.players.filter { $0.list == "rights" }.count
        let table = standings.first { $0.leagueId == roster.leagueId }
        let place = table?.rows.firstIndex { $0.franchiseId == viewedID }
        let row = place.flatMap { table?.rows[$0] }
        let current = matchups[roster.leagueId]?.matchups.first {
            $0.homeFranchiseId == viewedID || $0.awayFranchiseId == viewedID
        }
        return VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 10) {
                SportTag(sport: roster.competition)
                Text(roster.teamName).font(.headline).lineLimit(2)
                Spacer(minLength: 0)
                Image(systemName: "chevron.right").font(.caption.weight(.semibold))
                    .foregroundStyle(MosaicPalette.soft)
            }
            if let place {
                HStack(spacing: 14) {
                    Text("#\(place + 1) place")
                    if let wins = row?.wins, let losses = row?.losses {
                        Text(StandingsOrder.record(wins, losses, row?.ties))
                    } else if let points = row?.points {
                        Text("\(StandingsOrder.points(points)) pts")
                    }
                }
                .font(.subheadline.weight(.medium))
            }
            if let current {
                let own = current.homeFranchiseId == viewedID
                let opponentID = own ? current.awayFranchiseId : current.homeFranchiseId
                let opponent = dynasty.franchises.first { $0.id == opponentID }?.name ?? "Opponent"
                Text("\(opponent) · \(StandingsOrder.points(own ? current.homePoints : current.awayPoints))–\(StandingsOrder.points(own ? current.awayPoints : current.homePoints))\(current.final ? " final" : "")")
                    .font(.caption).foregroundStyle(MosaicPalette.soft)
            }
            HStack(spacing: 12) {
                Text("Main \(main)/\(roster.limits.main)")
                Text("Reserve \(reserve)/\(roster.limits.reserve)")
                if rights > 0 { Text("\(rights) unsigned").foregroundStyle(MosaicPalette.gold) }
            }
            .font(.caption).foregroundStyle(MosaicPalette.soft)
            if let season = roster.season {
                Text("\(String(season.year)) season · \(season.needsSetting ? "roster needs setting" : season.setAt != nil ? "roster set" : "lists locked")")
                    .font(.caption).foregroundStyle(season.needsSetting ? MosaicPalette.gold : MosaicPalette.soft)
            }
            if let phase = table?.season?.status, !phase.isEmpty {
                Text("Season \(phase.capitalized)").font(.caption).foregroundStyle(MosaicPalette.soft)
            }
            if (roster.overage ?? 0) > 0 {
                Text("Over roster limit by \(roster.overage ?? 0)")
                    .font(.caption.weight(.semibold)).foregroundStyle(MosaicPalette.bad)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
        .accessibilityElement(children: .combine)
    }

    private func picks(_ picks: [HeldPick]) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Upcoming rookie draft picks").font(.headline)
            if picks.isEmpty {
                Text("No picks in upcoming rookie drafts.")
                    .font(.subheadline).foregroundStyle(MosaicPalette.soft)
            } else {
                ForEach(picks.sorted { ($0.year, $0.round, $0.position) < ($1.year, $1.round, $1.position) }) { pick in
                    HStack(alignment: .top, spacing: 12) {
                        Text(String(pick.year)).font(.subheadline.weight(.bold)).foregroundStyle(MosaicPalette.gold)
                        VStack(alignment: .leading, spacing: 3) {
                            Text("Round \(pick.round) · Pick \(pick.position)").font(.subheadline.weight(.medium))
                            Text("\(pick.draftName) · \(pick.competitions.map { $0.uppercased() }.joined(separator: ", "))")
                                .font(.caption).foregroundStyle(MosaicPalette.soft)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(.vertical, 5)
                }
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
    }

    @MainActor
    private func load() async {
        guard !preview, let token = session.token else { return }
        let requestedID = viewedID
        if loadedFranchiseID != requestedID {
            detail = nil
            standings = []
            matchups = [:]
        }
        loading = true
        error = nil
        defer { loading = false }
        do {
            let fresh = try await session.api.dynasty(token: token)
            guard fresh.id == selected.id, fresh.franchises.contains(where: { $0.id == requestedID }) else {
                detail = nil
                throw APIError(message: "This franchise is not in the selected league.", statusCode: nil)
            }
            async let fetched = session.api.franchise(for: selected, franchiseID: requestedID, token: token)
            async let tables = session.api.standings(for: selected, token: token)
            let (newDetail, newTables) = try await (fetched, tables)
            guard viewedID == requestedID else { return }
            currentDynasty = fresh
            detail = newDetail
            loadedFranchiseID = requestedID
            standings = newTables
            var scores: [String: MatchupsData] = [:]
            for roster in newDetail.rosters where fresh.leagues.first(where: { $0.id == roster.leagueId })?.settings?.format?.type == "head_to_head" {
                scores[roster.leagueId] = try? await session.api.matchups(for: selected, leagueID: roster.leagueId,
                                                                          period: nil, token: token)
            }
            if viewedID == requestedID { matchups = scores }
        } catch {
            guard viewedID == requestedID else { return }
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

#Preview("Teams overview") {
    NavigationStack {
        TeamsView(session: PreviewFixtures.signedInStore,
                  selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                  preview: true)
    }
    .preferredColorScheme(.dark)
}
