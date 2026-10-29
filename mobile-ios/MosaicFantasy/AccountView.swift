// Shows a manager's account status and provides secure sign-out in the first native chunk.
import SwiftUI

struct AccountView: View {
    @ObservedObject var session: SessionStore
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        List {
            Section {
                HStack(spacing: 14) {
                    ProfileAvatar(session: session.session)
                    VStack(alignment: .leading, spacing: 4) {
                        Text(session.session?.displayName ?? "Account").font(.headline)
                        Text(session.session?.email ?? "").font(.subheadline).foregroundStyle(MosaicPalette.soft)
                    }
                }
                .padding(.vertical, 8)
            }
            Section("Email") {
                Label(session.session?.verified == true ? "Verified" : "Verification needed",
                      systemImage: session.session?.verified == true ? "checkmark.seal" : "envelope.badge")
                if session.session?.verified == false {
                    Text("Verify your email before claiming a commissioner invite.")
                        .font(.subheadline)
                        .foregroundStyle(MosaicPalette.soft)
                }
            }
            Section {
                Button(role: .destructive) {
                    Task { await signOut() }
                } label: {
                    if busy { ProgressView() }
                    else { Label("Sign out", systemImage: "rectangle.portrait.and.arrow.right") }
                }
                .disabled(busy)
            }
            if let error {
                Section {
                    Text(error).foregroundStyle(MosaicPalette.bad)
                }
            }
        }
        .scrollContentBackground(.hidden)
        .background(MosaicPalette.ground)
        .navigationTitle("Account")
    }

    private func signOut() async {
        busy = true
        defer { busy = false }
        do { try await session.signOut() }
        catch { self.error = error.localizedDescription }
    }
}

#Preview("Account") {
    NavigationStack { AccountView(session: PreviewFixtures.signedInStore) }
        .preferredColorScheme(.dark)
}
