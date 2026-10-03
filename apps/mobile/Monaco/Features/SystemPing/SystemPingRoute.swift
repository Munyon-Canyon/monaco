#if DEBUG
import MonacoCore
import SwiftUI

nonisolated struct SystemPingRoute: AppRoute {
    @MainActor func destination() -> SystemPingView {
        destination(model: .preview())
    }

    @MainActor func destination(model: SystemPingModel) -> SystemPingView {
        SystemPingView(model: model)
    }
}
#endif
