import SwiftUI

nonisolated struct FullLedgerRoute: AppRoute {
    let week: String

    @MainActor func destination() -> FullLedgerView {
        FullLedgerView(week: week)
    }
}
