import SwiftUI

nonisolated struct TransactionRoute: AppRoute {
    let cabalID: String
    let transactionID: String

    @MainActor func destination() -> some View {
        CabalTransactionView(cabalID: cabalID, transactionID: transactionID)
    }
}
