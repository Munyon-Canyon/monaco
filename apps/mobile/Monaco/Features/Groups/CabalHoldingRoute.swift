import MonacoAnalytics
import SwiftUI

nonisolated struct CabalHoldingRoute: AppRoute {
    let cabalID: String
    let symbol: String

    @MainActor func destination() -> some View {
        CabalHoldingView(cabalID: cabalID, symbol: symbol)
    }
}
