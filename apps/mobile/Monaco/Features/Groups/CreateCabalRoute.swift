import SwiftUI

nonisolated struct CreateCabalRoute: AppRoute {
    @MainActor func destination() -> some View {
        CreateGroupView()
    }
}
