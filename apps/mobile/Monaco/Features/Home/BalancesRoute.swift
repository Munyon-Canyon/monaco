import MonacoAnalytics
import SwiftUI

nonisolated struct BalancesRoute: AppRoute {
    @MainActor func destination() -> some View {
        BalancesView()
            .analyticsScreen("balances")
    }
}
