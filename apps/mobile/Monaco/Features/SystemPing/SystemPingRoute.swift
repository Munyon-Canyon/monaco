#if DEBUG
import MonacoCore
import SwiftUI

nonisolated struct SystemPingRoute: AppRoute {
    @MainActor func destination() -> SystemPingView {
        SystemPingView(model: .preview())
    }
}
#endif
