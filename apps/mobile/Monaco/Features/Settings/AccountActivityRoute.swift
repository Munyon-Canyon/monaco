import SwiftUI

nonisolated struct AccountActivityRoute: AppRoute {
    @MainActor func destination() -> some View {
        AccountActivityView()
    }
}
