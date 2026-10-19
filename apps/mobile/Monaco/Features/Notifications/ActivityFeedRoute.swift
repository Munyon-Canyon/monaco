import MonacoAnalytics
import SwiftUI

nonisolated struct ActivityFeedRoute: AppRoute {
    @MainActor func destination() -> some View {
        ActivityView()
            .analyticsScreen("activity_feed")
    }
}
