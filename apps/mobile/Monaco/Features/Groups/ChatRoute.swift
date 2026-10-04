import SwiftUI

nonisolated struct ChatRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Chat")
    }
}
