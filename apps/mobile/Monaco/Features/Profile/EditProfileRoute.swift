import SwiftUI

nonisolated struct EditProfileRoute: AppRoute {
    @MainActor func destination() -> some View {
        EditProfileView()
    }
}
