import SwiftUI

nonisolated struct ProposeRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Propose")
    }
}
