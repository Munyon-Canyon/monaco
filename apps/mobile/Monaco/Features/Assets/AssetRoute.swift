import MonacoCore
import SwiftUI

nonisolated struct AssetRoute: AppRoute {
    let symbol: String
    var seed: AssetSeed?

    @MainActor func destination() -> some View {
        AssetDetailClientView(symbol: symbol, seed: seed)
    }

    static func == (lhs: Self, rhs: Self) -> Bool { lhs.symbol == rhs.symbol }

    func hash(into hasher: inout Hasher) { hasher.combine(symbol) }
}
