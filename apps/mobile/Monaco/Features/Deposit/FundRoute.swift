import SwiftUI

nonisolated struct FundRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Fund this cabal")
    }
}
