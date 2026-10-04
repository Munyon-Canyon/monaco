import SwiftUI

nonisolated struct ChatRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        CabalChatView(cabalID: cabalID)
    }
}
