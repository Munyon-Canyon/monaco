import SwiftUI

nonisolated struct WithdrawRoute: AppRoute {
    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Withdraw")
    }
}
