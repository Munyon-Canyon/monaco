import Foundation
import MonacoCore

nonisolated struct Viewer: Sendable, Equatable {
    let userID: String
    var handle: String?

    init(userID: String, handle: String?) {
        self.userID = userID
        self.handle = handle
    }

    init(_ profile: SessionProfile) {
        self.init(userID: profile.userID, handle: profile.handle)
    }
}
