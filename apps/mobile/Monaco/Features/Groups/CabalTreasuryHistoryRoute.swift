import SwiftUI

nonisolated struct CabalTreasuryHistoryRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        CabalTreasuryHistoryView(cabalID: cabalID)
    }
}
