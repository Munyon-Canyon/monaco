import SwiftUI

nonisolated struct AgentRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Trading bot")
    }
}
