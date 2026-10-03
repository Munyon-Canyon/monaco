import Foundation
import MonacoCore

nonisolated struct Viewer: Sendable, Equatable {
    let userID: String
    var handle: String?
    var displayName: String
    var photoURL: URL?

    init(userID: String, handle: String?, displayName: String = "", photoURL: URL? = nil) {
        self.userID = userID
        self.handle = handle
        self.displayName = displayName
        self.photoURL = photoURL
    }

    init(_ profile: SessionProfile) {
        self.init(
            userID: profile.userID, handle: profile.handle,
            displayName: profile.displayName, photoURL: profile.photoURL
        )
    }
}
