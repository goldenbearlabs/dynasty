// Lists the league's additional destinations and the manager account in the More tab.
import SwiftUI

struct MoreView: View {
    @ObservedObject var session: SessionStore
    let onLeagues: () -> Void
    let currentLeagueName: String

    private let futureSections = ["Franchises", "Watchlist", "Research", "Waivers", "Drafts", "Trades", "Scores", "Rules", "Commissioner"]

    var body: some View {
        List {
                Section {
                    NavigationLink {
                        AccountView(session: session)
                    } label: {
                        HStack(spacing: 14) {
                            ProfileAvatar(session: session.session)
                            Text(session.session?.displayName ?? session.session?.email ?? "Account")
                                .font(.headline)
                                .lineLimit(2)
                        }
                        .padding(.vertical, 8)
                        .frame(minHeight: 56)
                    }
                    .accessibilityLabel("Account, \(session.session?.displayName ?? "manager")")
                }
                Section(currentLeagueName) {
                    ForEach(futureSections, id: \.self) { section in
                        HStack {
                            Text(section)
                            Spacer()
                            Text("Coming soon")
                                .font(.caption)
                                .foregroundStyle(MosaicPalette.faint)
                        }
                        .foregroundStyle(MosaicPalette.faint)
                        .accessibilityLabel("\(section), coming soon")
                    }
                }
                Section {
                    Button {
                        onLeagues()
                    } label: {
                        Label("Switch league", systemImage: "square.stack.3d.up")
                    }
                }
        }
        .scrollContentBackground(.hidden)
        .background(MosaicPalette.ground)
    }
}

struct ProfileAvatar: View {
    let session: SessionInfo?
    var size: CGFloat = 48

    var body: some View {
        Group {
            if let urlString = session?.imageUrl, let url = URL(string: urlString), !urlString.isEmpty {
                AsyncImage(url: url) { image in
                    image.resizable().scaledToFill()
                } placeholder: {
                    initials
                }
            } else {
                initials
            }
        }
        .frame(width: size, height: size)
        .clipShape(RoundedRectangle(cornerRadius: size * 0.29))
        .accessibilityHidden(true)
    }

    private var initials: some View {
        ZStack {
            MosaicPalette.raised
            Text(session?.initials ?? "MF")
                .font((size < 40 ? Font.caption : Font.headline).weight(.bold))
                .foregroundStyle(MosaicPalette.ink)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
        }
    }
}
