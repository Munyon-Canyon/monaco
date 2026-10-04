import SwiftUI

nonisolated struct UserProfileRoute: AppRoute {
    let userID: String
    var preview: UserPreview? = nil

    @MainActor func destination() -> UserProfileScreen {
        UserProfileScreen(userID: userID, preview: preview)
    }
}

extension UserProfileScreen {
    init(userID: String, preview: UserPreview?) {
        self.context = UserProfileContext(userID: userID, preview: preview)
        self.sections = Self.sections
    }
}
