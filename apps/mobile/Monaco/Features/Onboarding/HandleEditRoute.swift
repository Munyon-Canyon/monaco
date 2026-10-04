import SwiftUI

nonisolated struct HandleEditRoute: AppRoute {
    @MainActor func destination() -> some View {
        HandleStepView(mode: .edit)
    }
}
