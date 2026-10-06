import SwiftUI

nonisolated struct ChatThreadRoute: AppRoute {
    let cabalID: String
    let parentID: String

    @MainActor func destination() -> some View {
        ChatThreadView(cabalID: cabalID, parentID: parentID)
    }
}
