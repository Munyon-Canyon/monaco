import SwiftUI

nonisolated struct HandleEditRoute: AppRoute {
    @MainActor func destination() -> some View {
        NotMigratedView(screen: "Edit handle")
    }
}
