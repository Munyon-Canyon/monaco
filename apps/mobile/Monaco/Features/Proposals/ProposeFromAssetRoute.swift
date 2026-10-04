import SwiftUI

nonisolated struct ProposeFromAssetRoute: AppRoute {
    let symbol: String
    let kind: ProposeKind

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Propose")
    }
}

nonisolated enum ProposeKind: Hashable, Sendable {
    case buy, sell
}
