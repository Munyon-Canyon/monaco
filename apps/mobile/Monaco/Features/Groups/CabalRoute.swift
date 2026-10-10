import MonacoAnalytics
import SwiftUI

nonisolated struct CabalRoute: AppRoute {
    let id: String

    @MainActor func destination() -> some View {
        CabalScreen(cabalID: id)
            .analyticsScreen("cabal", step: .joinCabal(.cabalViewed))
    }
}
