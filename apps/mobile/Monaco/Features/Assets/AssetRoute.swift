import SwiftUI

nonisolated struct AssetRoute: AppRoute {
    let symbol: String

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Asset")
    }
}
