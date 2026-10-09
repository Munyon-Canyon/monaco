import MonacoCore
import PhotosUI
import SwiftUI

/// The cabal's mark, and — for its creator — the control that changes it.
///
/// A member who did not create the cabal gets the plain mark: no badge, no tap
/// target, nothing to discover that would only answer 403. The creator gets the
/// same mark with a camera badge, a photo picker on tap, and "Remove picture" in
/// a long-press menu once there is one to remove.
struct CabalPicturePicker: View {
    let groupId: String
    let name: String
    /// Only the creator is offered the controls. The server checks again anyway.
    let canEdit: Bool
    var size: CGFloat = 36
    var showsRemoveButton = true
    /// Reports results so the host screen can toast them.
    var onResult: (MonacoToast) -> Void = { _ in }

    @ObservedObject var editor: CabalPictureEditor

    @State private var selection: PhotosPickerItem?
    @State private var confirmingRemoval = false

    var body: some View {
        Group {
            if canEdit {
                VStack(spacing: MonacoTheme.Space.s) {
                    editableMark
                    if editor.pictureUrl != nil && showsRemoveButton {
                        Button {
                            confirmingRemoval = true
                        } label: {
                            Text("Remove picture")
                                .font(MonacoTheme.Typo.calloutStrong)
                                .foregroundStyle(MonacoTheme.loss)
                                .frame(minHeight: 44)
                        }
                        .disabled(editor.isWorking)
                        .accessibilityIdentifier("cabal-picture-remove-button")
                    }
                }
            } else {
                CabalMark(
                    groupId: groupId,
                    name: name,
                    size: size,
                    pictureUrl: editor.pictureUrl,
                    accessibilityLabel: readOnlyAccessibilityLabel
                )
            }
        }
        .onChange(of: selection) { _, item in
            guard let item else { return }
            Task { await upload(item) }
        }
        .confirmationDialog("Remove the cabal picture?", isPresented: $confirmingRemoval, titleVisibility: .visible) {
            Button("Remove picture", role: .destructive) {
                Task { await remove() }
            }
            .accessibilityIdentifier("cabal-picture-remove-confirm")
        }
    }

    private var editableMark: some View {
        // The picker's label closure is `@Sendable` and not main-actor isolated, so it cannot
        // capture the editor or a `View`. Copy the Sendable inputs and build a value label inside.
        let pictureUrl = editor.pictureUrl
        let isWorking = editor.isWorking
        let groupId = groupId
        let name = name
        let size = size
        return PhotosPicker(selection: $selection, matching: .images, photoLibrary: .shared()) {
            CabalPictureLabel(
                groupId: groupId,
                name: name,
                size: size,
                pictureUrl: pictureUrl,
                isWorking: isWorking
            )
        }
        .buttonStyle(.plain)
        .disabled(editor.isWorking)
        .accessibilityLabel(editAccessibilityLabel)
        .accessibilityHint("Opens your photo library")
        .accessibilityIdentifier("cabal-picture-picker")
        .contextMenu {
            if editor.pictureUrl != nil {
                Button(role: .destructive) {
                    confirmingRemoval = true
                } label: {
                    Label("Remove picture", systemImage: "trash")
                }
                .accessibilityIdentifier("cabal-picture-remove")
            }
        }
        .accessibilityActions {
            if editor.pictureUrl != nil {
                Button("Remove picture") { confirmingRemoval = true }
            }
        }
    }

    /// VoiceOver still needs to be told there is a picture, even where the mark
    /// is decoration for sighted members.
    private var readOnlyAccessibilityLabel: String {
        editor.pictureUrl == nil ? "\(name), no picture" : "\(name) picture"
    }

    private var editAccessibilityLabel: String {
        if editor.isWorking { return "Updating cabal picture" }
        return editor.pictureUrl == nil ? "Add cabal picture" : "Change cabal picture"
    }

    private func upload(_ item: PhotosPickerItem) async {
        defer { selection = nil }

        let data: Data?
        do {
            data = try await item.loadTransferable(type: Data.self)
        } catch {
            data = nil
        }
        guard let data else {
            onResult(MonacoToast(message: "Couldn't read that picture.", isSuccess: false))
            return
        }

        // The same preparer profile photos use: downsample and re-encode off the
        // main thread, so the cap is met before the bytes leave the phone.
        let prepared: ProfilePhotoUploadPreparer.Prepared
        switch await ProfilePhotoUploadPreparer.prepared(from: data) {
        case .success(let ready):
            prepared = ready
        case .failure(.unreadable):
            onResult(MonacoToast(message: "That picture could not be opened. Try another.", isSuccess: false))
            return
        case .failure(.tooLarge):
            onResult(MonacoToast(message: "That picture is too big to upload. Try another.", isSuccess: false))
            return
        }

        switch await editor.setPicture(imageData: prepared.data, mimeType: prepared.mimeType) {
        case .saved:
            onResult(MonacoToast(message: "Picture updated.", isSuccess: true))
        case .failed(let message):
            onResult(MonacoToast(message: message, isSuccess: false))
        }
    }

    private func remove() async {
        switch await editor.removePicture() {
        case .saved:
            onResult(MonacoToast(message: "Picture removed.", isSuccess: true))
        case .failed(let message):
            onResult(MonacoToast(message: message, isSuccess: false))
        }
    }
}

/// The picker's label. Its initializer is nonisolated so the `@Sendable` label closure can
/// build it from copied values; SwiftUI still evaluates `body` on the main actor.
private struct CabalPictureLabel: View {
    let groupId: String
    let name: String
    let size: CGFloat
    let pictureUrl: String?
    let isWorking: Bool

    nonisolated init(
        groupId: String,
        name: String,
        size: CGFloat,
        pictureUrl: String?,
        isWorking: Bool
    ) {
        self.groupId = groupId
        self.name = name
        self.size = size
        self.pictureUrl = pictureUrl
        self.isWorking = isWorking
    }

    var body: some View {
        ZStack(alignment: .bottomTrailing) {
            CabalMark(
                groupId: groupId,
                name: name,
                size: size,
                pictureUrl: pictureUrl
            )
            .overlay {
                if isWorking {
                    // The mark's own corner, so the veil covers the tile and nothing else.
                    RoundedRectangle(cornerRadius: size * MonacoTheme.Radius.tile / 40, style: .continuous)
                        .fill(MonacoTheme.canvas.opacity(0.6))
                    ProgressView()
                        .controlSize(size >= 64 ? .regular : .mini)
                        .tint(MonacoTheme.ink)
                }
            }
            cameraBadge
        }
    }

    /// The camera on the mark's corner, in the brand fill.
    private var cameraBadge: some View {
        let diameter = max(18, size * 0.44)
        return CameraBadgeMark(diameter: diameter, ring: 1.5)
            .offset(x: 5, y: 5)
    }
}
