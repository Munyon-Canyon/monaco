#if DEBUG
import MonacoCore
import SwiftUI

struct SystemPingRoute: Hashable, Sendable {
    @MainActor func destination() -> SystemPingView {
        SystemPingView(model: .preview())
    }
}
#endif
