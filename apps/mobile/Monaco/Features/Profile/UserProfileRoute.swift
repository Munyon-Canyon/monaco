import SwiftUI

nonisolated struct UserProfileRoute: AppRoute {
    let userID: String
    var preview: UserPreview? = nil

    @MainActor func destination() -> UserProfileScreen {
        UserProfileScreen(userID: userID, preview: preview)
    }
}
