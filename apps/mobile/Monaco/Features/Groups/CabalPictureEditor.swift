import Combine
import Foundation
import MonacoAPI
import MonacoCore
import PhotosUI
import SwiftUI

/// How a cabal picture write ended.
enum CabalPictureOutcome: Equatable {
    /// The picture the cabal now has; nil after a removal.
    case saved(String?)
    /// Copy to show the member. Already phrased for them.
    case failed(String)
}

/// What the cabal picture needs the network to do. A protocol so the editor can
/// be driven in tests without a server, and so the view does not have to know
/// how a client is built.
@MainActor
protocol CabalPictureWriting {
    func uploadPicture(groupId: String, imageData: Data, mimeType: String) async throws -> String?
    func removePicture(groupId: String) async throws -> String?
}

/// Drives set, replace and remove for one cabal's picture.
///
/// It owns exactly two things: which picture is current, and whether a write is
/// in flight. Both matter to the view — the second is what stops a member
/// firing a second upload into a rate limit while the first is still going.
@MainActor
final class CabalPictureEditor: ObservableObject {
    /// The picture the cabal has right now, as far as this screen knows.
    @Published private(set) var pictureUrl: String?
    /// True while a write is in flight.
    @Published private(set) var isWorking = false
    /// The last failure, for the screen to toast. Cleared when a write starts.
    @Published private(set) var lastFailure: String?

    private let writer: CabalPictureWriting
    private let groupId: String

    init(groupId: String, pictureUrl: String?, writer: CabalPictureWriting) {
        self.groupId = groupId
        self.pictureUrl = pictureUrl
        self.writer = writer
    }

    /// Adopts a picture that arrived from a refresh, unless a write is in flight.
    ///
    /// Without the guard a refresh that started before an upload can land after
    /// it and put the old picture back, which reads to the member as the upload
    /// silently undoing itself.
    func adoptFromRefresh(_ refreshed: String?) {
        guard !isWorking else { return }
        pictureUrl = refreshed
    }

    func setPicture(imageData: Data, mimeType: String) async -> CabalPictureOutcome {
        await write(fallback: "Couldn't update the cabal picture.") { [groupId, writer] in
            try await writer.uploadPicture(groupId: groupId, imageData: imageData, mimeType: mimeType)
        }
    }

    func removePicture() async -> CabalPictureOutcome {
        await write(fallback: "Couldn't remove the cabal picture.") { [groupId, writer] in
            try await writer.removePicture(groupId: groupId)
        }
    }

    /// One in-flight write at a time. A second call while one is running is
    /// refused rather than queued: two uploads racing would leave the screen
    /// showing whichever finished last, not whichever the member picked last.
    private func write(
        fallback: String,
        _ work: () async throws -> String?
    ) async -> CabalPictureOutcome {
        guard !isWorking else {
            return .failed("Still working on the last change.")
        }
        isWorking = true
        lastFailure = nil
        defer { isWorking = false }

        do {
            let saved = try await work()
            pictureUrl = CabalPictureEditor.normalised(saved)
            return .saved(pictureUrl)
        } catch {
            // The picture on screen is deliberately left alone: the write failed,
            // so what the cabal has has not changed.
            let message = CabalPictureEditor.failureMessage(for: error, fallback: fallback)
            lastFailure = message
            return .failed(message)
        }
    }

    /// A blank URL is no picture. The server sends null, but a blank string
    /// would otherwise be loaded and fail forever.
    static func normalised(_ raw: String?) -> String? {
        guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }

    static func failureMessage(for error: Error, fallback: String) -> String {
        guard let apiError = error as? APIError else { return fallback }
        return ToastCopy.message(for: apiError)
    }
}

@MainActor
struct LiveCabalPictureWriter: CabalPictureWriting {
    private let uploads: CabalPictureUploads

    init(api: APIClient) {
        uploads = CabalPictureUploads(api: api)
    }

    func uploadPicture(groupId: String, imageData: Data, mimeType: String) async throws -> String? {
        try await uploads.setPicture(cabalID: groupId, imageData: imageData, mimeType: mimeType)
    }

    func removePicture(groupId: String) async throws -> String? {
        try await uploads.removePicture(cabalID: groupId)
    }
}

extension CabalPictureEditor {
    func upload(_ item: PhotosPickerItem) async -> MonacoToast {
        let data: Data?
        do {
            data = try await item.loadTransferable(type: Data.self)
        } catch {
            data = nil
        }
        guard let data else { return MonacoToast(message: "Couldn't read that picture.", isSuccess: false) }
        let prepared: ProfilePhotoUploadPreparer.Prepared
        switch await ProfilePhotoUploadPreparer.prepared(from: data) {
        case .success(let ready):
            prepared = ready
        case .failure(.unreadable):
            return MonacoToast(message: "That picture could not be opened. Try another.", isSuccess: false)
        case .failure(.tooLarge):
            return MonacoToast(message: "That picture is too big to upload. Try another.", isSuccess: false)
        }
        switch await setPicture(imageData: prepared.data, mimeType: prepared.mimeType) {
        case .saved: return MonacoToast(message: "Picture updated.", isSuccess: true)
        case .failed(let message): return MonacoToast(message: message, isSuccess: false)
        }
    }

    func remove() async -> MonacoToast {
        switch await removePicture() {
        case .saved: MonacoToast(message: "Picture removed.", isSuccess: true)
        case .failed(let message): MonacoToast(message: message, isSuccess: false)
        }
    }
}
