import SwiftUI

nonisolated struct HomeLeaderboardStubRoute: AppRoute {
    @MainActor func destination() -> some View { NotMigratedView(screen: "Leaderboard") }
}

nonisolated struct HomeActivityStubRoute: AppRoute {
    @MainActor func destination() -> some View { NotMigratedView(screen: "Activity") }
}
