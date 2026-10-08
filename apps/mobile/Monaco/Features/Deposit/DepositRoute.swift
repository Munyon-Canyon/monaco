import MonacoCore
import SwiftUI

nonisolated struct DepositRoute: AppRoute {
    let prefillMicros: Int64?
    let cabalID: String?

    init(prefillMicros: Int64? = nil, cabalID: String? = nil) {
        self.prefillMicros = prefillMicros
        self.cabalID = cabalID
    }

    @MainActor func destination() -> some View {
        DepositView(prefillMicros: prefillMicros, cabalID: cabalID)
    }
}

nonisolated struct DepositAddressRoute: AppRoute {
    @MainActor func destination() -> some View {
        DepositAddressView()
    }
}

nonisolated struct DepositCompleteRoute: AppRoute {
    let sessionID: String

    @MainActor func destination() -> some View {
        DepositCompleteView(sessionID: sessionID)
    }
}

struct DepositCompleteView: View {
    let sessionID: String

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        AmountEntrySkeleton()
            .padding(.top, MonacoTheme.Space.xl)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            .task {
                environment.navigator.homePath.removeAll()
                await environment.cardDeposit.redirected(sessionID: sessionID)
            }
    }
}
