import SwiftUI

nonisolated struct TransactionRoute: AppRoute {
    let cabalID: String
    let transactionID: String

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Transaction")
    }
}
