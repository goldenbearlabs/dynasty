// Edits franchise and sport-team names and images with the website's fallback behavior.
import PhotosUI
import SwiftUI
import UIKit

struct TeamIdentityEditor: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var session: SessionStore
    let selected: SelectedLeague
    let leagueID: String?
    let name: String
    let imageURL: String
    let onSaved: () async -> Void

    @State private var editedName = ""
    @State private var editedImageURL = ""
    @State private var photo: PhotosPickerItem?
    @State private var saving = false
    @State private var error: String?

    private var valid: Bool {
        (leagueID != nil || !editedName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            && editedName.count <= 80 && editedImageURL.count <= 210_000
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField(leagueID == nil ? "Franchise name" : "Team name", text: $editedName)
                        .textInputAutocapitalization(.words)
                    if editedImageURL.hasPrefix("data:image/") {
                        HStack {
                            Label("Selected photo", systemImage: "photo")
                            Spacer()
                            Button("Remove") { editedImageURL = "" }
                        }
                    } else {
                        TextField("Image URL", text: $editedImageURL)
                            .keyboardType(.URL).textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                    }
                    PhotosPicker(selection: $photo, matching: .images) {
                        Label("Choose photo", systemImage: "photo")
                    }
                } footer: {
                    Text(leagueID == nil
                         ? "The franchise image and name are the default for every sport."
                         : "Leave either field blank to use your franchise identity for that sport.")
                }
                if let error {
                    Section {
                        Label(error, systemImage: "exclamationmark.circle")
                            .foregroundStyle(MosaicPalette.bad)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .background(MosaicPalette.ground)
            .navigationTitle(leagueID == nil ? "Franchise identity" : "Team identity")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }.disabled(saving)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(saving ? "Saving…" : "Save") { Task { await save() } }
                        .disabled(!valid || saving)
                }
            }
            .onAppear { editedName = name; editedImageURL = imageURL }
            .onChange(of: photo) { _, item in
                guard let item else { return }
                Task { await loadPhoto(item) }
            }
        }
        .preferredColorScheme(.dark)
        .interactiveDismissDisabled(saving)
    }

    @MainActor
    private func loadPhoto(_ item: PhotosPickerItem) async {
        do {
            guard let data = try await item.loadTransferable(type: Data.self),
                  let image = UIImage(data: data) else {
                throw APIError(message: "This photo could not be opened.", statusCode: nil)
            }
            let longest = max(image.size.width, image.size.height)
            let scale = min(1, 256 / longest)
            let size = CGSize(width: max(1, image.size.width * scale),
                              height: max(1, image.size.height * scale))
            let renderer = UIGraphicsImageRenderer(size: size)
            let resized = renderer.image { _ in image.draw(in: CGRect(origin: .zero, size: size)) }
            var quality: CGFloat = 0.82
            var jpeg = resized.jpegData(compressionQuality: quality)
            while let data = jpeg, data.count > 145_000 && quality > 0.35 {
                quality -= 0.12
                jpeg = resized.jpegData(compressionQuality: quality)
            }
            guard let jpeg, jpeg.count <= 150_000 else {
                throw APIError(message: "Choose a smaller photo.", statusCode: nil)
            }
            editedImageURL = "data:image/jpeg;base64," + jpeg.base64EncodedString()
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    @MainActor
    private func save() async {
        guard valid, !saving, let token = session.token else { return }
        saving = true
        error = nil
        defer { saving = false }
        do {
            try await session.api.updateIdentity(for: selected, leagueID: leagueID,
                                                 name: editedName.trimmingCharacters(in: .whitespacesAndNewlines),
                                                 imageURL: editedImageURL.trimmingCharacters(in: .whitespacesAndNewlines),
                                                 token: token)
            await onSaved()
            dismiss()
        } catch {
            self.error = error.localizedDescription
            if (error as? APIError)?.statusCode == 401 { await session.refresh() }
        }
    }
}

#Preview("Team identity") {
    TeamIdentityEditor(session: PreviewFixtures.signedInStore,
                       selected: SelectedLeague(membership: PreviewFixtures.leagues[0], dynasty: PreviewFixtures.dynasty),
                       leagueID: "nba-1", name: "Northern Lights", imageURL: "", onSaved: {})
}
