import SwiftUI

nonisolated struct DepositRoute: AppRoute {
    let prefillMicros: Int64?
    let cabalID: String?

    init(prefillMicros: Int64? = nil, cabalID: String? = nil) {
        self.prefillMicros = prefillMicros
        self.cabalID = cabalID
    }

    @MainActor func destination() -> some View {
        DepositView()
    }
}
