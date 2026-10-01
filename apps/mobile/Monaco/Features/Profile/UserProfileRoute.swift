import SwiftUI

nonisolated struct UserProfileRoute: AppRoute {
    let userID: String

    @MainActor func destination() -> some View {
        UserProfileScreen(userID: userID)
    }
}
