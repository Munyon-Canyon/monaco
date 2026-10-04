import SwiftUI

nonisolated struct CashOutRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        CashOutView(cabalID: cabalID)
    }
}
