import SwiftUI

nonisolated struct SettingsRoute: AppRoute {
    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Settings")
    }
}
