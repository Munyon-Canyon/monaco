import SwiftUI

nonisolated struct HandleEditRoute: AppRoute {
    @MainActor func destination() -> some View {
        EditProfileView(focus: .username)
    }
}
