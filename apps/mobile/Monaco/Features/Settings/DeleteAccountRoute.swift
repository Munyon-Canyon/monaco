import SwiftUI

nonisolated struct DeleteAccountRoute: AppRoute {
    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Delete account")
    }
}
