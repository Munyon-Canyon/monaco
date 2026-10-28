import SwiftUI

nonisolated struct FundRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        FundCabalScreen(cabalID: cabalID)
    }
}
