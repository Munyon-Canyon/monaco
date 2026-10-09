import MonacoAPI
import MonacoCore
import PhotosUI
import SwiftUI

/// Avatar that opens the face sheet on tap: the eight pixel animals, or the photo library.
/// Either way the pick is uploaded as the signed-in member's profile photo. The one place
/// faces are changed; Profile and the first-run screen embed it.
struct ProfilePhotoPicker: View {
    @ObservedObject var auth: PrivyAuthService
    @Environment(AppSessionStore.self) private var session

    var size: CGFloat = 96
    /// Overridden by the first-run screen, which QA drives as `onboarding-photo`.
    var accessibilityID: String = "profile-photo-picker"
    /// Debug sample harness only: open the face sheet once the screen is up.
    var initiallyOpen = false
    /// Reports upload results so the host screen can toast them.
    var onResult: (MonacoToast) -> Void

    @State private var showFaces = false
    @State private var uploads = ProfilePhotoUploadGate()
    @State private var submission = IdempotentSubmission()

    var body: some View {
        Button {
            showFaces = true
        } label: {
            ZStack(alignment: .bottomTrailing) {
                MonacoAvatar(
                    photoURL: session.profile?.photoURL?.absoluteString,
                    displayName: session.profile?.displayName ?? "",
                    size: size,
                    seed: session.profile?.userID
                )
                .overlay {
                    if uploads.isUploading {
                        Circle()
                            .fill(MonacoTheme.canvas.opacity(0.6))
                        ProgressView()
                            .tint(MonacoTheme.ink)
                    }
                }

                CameraBadgeMark(diameter: max(24, size * 0.3), ring: 2)
            }
        }
        .buttonStyle(.plain)
        .disabled(uploads.isUploading || (auth.accessToken == nil && !initiallyOpen))
        .accessibilityLabel(session.profile?.photoURL == nil ? "Choose your face" : "Change your face")
        .accessibilityIdentifier(accessibilityID)
        .sheet(isPresented: $showFaces) {
            FacePickerSheet(
                currentAnimal: nil,
                onPickAnimal: { animal in
                    showFaces = false
                    Task { await wear(animal) }
                },
                onPickPhoto: { item in
                    showFaces = false
                    Task { await upload(item) }
                }
            )
        }
        .onAppear {
            if initiallyOpen { showFaces = true }
        }
    }

    /// An animal is uploaded as a PNG straight from the catalog: the preparer's JPEG
    /// pass is for photos, and would only soften the pixels.
    private func wear(_ animal: PixelAnimal) async {
        await uploads.run {
            guard let data = UIImage(named: animal.imageName)?.pngData() else {
                onResult(MonacoToast(message: "That face is missing. Try another.", isSuccess: false))
                return
            }
            await save(data, success: "You're \(animal.withArticle) now.")
        }
    }

    private func upload(_ item: PhotosPickerItem) async {
        await uploads.run {
            let data: Data?
            do {
                data = try await item.loadTransferable(type: Data.self)
            } catch {
                data = nil
            }
            guard let data else {
                onResult(MonacoToast(message: "Couldn't read that photo.", isSuccess: false))
                return
            }
            let prepared: ProfilePhotoUploadPreparer.Prepared
            switch await ProfilePhotoUploadPreparer.prepared(from: data) {
            case .success(let ready):
                prepared = ready
            case .failure(let failure):
                onResult(MonacoToast(message: failure.memberMessage, isSuccess: false))
                return
            }

            await save(prepared.data, success: "Profile photo updated.")
        }
    }

    private func save(_ data: Data, success: String) async {
        switch await session.saveProfilePhoto(data, auth: auth, submission: submission) {
        case .saved, .unchanged:
            onResult(MonacoToast(message: success, isSuccess: true))
        case .failed(let message):
            onResult(MonacoToast(message: message, isSuccess: false))
        }
    }
}
