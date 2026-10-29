import MonacoAnalytics
import MonacoCore
import SwiftUI

nonisolated struct CabalRoute: AppRoute {
    let id: String
    let tab: CabalTab

    init(id: String, tab: CabalTab = .default) {
        self.id = id
        self.tab = tab
    }

    @MainActor func destination() -> some View {
        CabalScreen(cabalID: id, tab: tab)
            .analyticsScreen("cabal", step: .joinCabal(.cabalViewed))
    }
}
