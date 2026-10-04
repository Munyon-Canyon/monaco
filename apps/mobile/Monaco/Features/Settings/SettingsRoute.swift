import SwiftUI

nonisolated struct SettingsRoute: AppRoute {
    @MainActor func destination() -> some View {
        SettingsView()
    }
}
