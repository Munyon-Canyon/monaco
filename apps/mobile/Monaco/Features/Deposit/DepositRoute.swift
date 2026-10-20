import MonacoAnalytics
import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var depositNavigator: AppNavigator?
}

nonisolated struct DepositRequest: Identifiable, Hashable, Sendable {
    let id = UUID()
    let prefillMicros: Int64?
    let cabalID: String?
}

nonisolated struct DepositNetworksRoute: AppRoute {
    let networks: [DepositNetwork]

    @MainActor func destination() -> some View {
        DepositNetworksView(networks: networks)
            .analyticsScreen("deposit_networks")
    }
}

nonisolated struct DepositAddressRoute: AppRoute {
    var network: DepositNetwork?

    @MainActor func destination() -> some View {
        DepositAddressView(network: network)
            .analyticsScreen("deposit_address", step: .cryptoDeposit(.depositOpened))
    }
}

nonisolated struct CardFundingRoute: AppRoute {
    let prefillMicros: Int64?
    let cabalID: String?

    @MainActor func destination() -> some View {
        CardFundingView(prefillMicros: prefillMicros, cabalID: cabalID)
            .analyticsScreen("card_funding")
    }
}
