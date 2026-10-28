import MonacoCore
import SwiftUI

nonisolated struct ShareRoute: AppRoute {
    let subject: ShareSubject

    init(subject: ShareSubject) {
        self.subject = subject
    }

    init(_ subject: ShareSubject) {
        self.subject = subject
    }

    @MainActor func destination() -> some View {
        ShareScreen(subject: subject)
    }
}
