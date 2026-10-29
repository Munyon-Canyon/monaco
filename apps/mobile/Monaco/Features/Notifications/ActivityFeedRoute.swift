import MonacoAnalytics
import SwiftUI

nonisolated struct ActivityFeedRoute: AppRoute {
    @MainActor func destination() -> some View {
        CatchUpView()
            .analyticsScreen("catch_up")
    }
}
