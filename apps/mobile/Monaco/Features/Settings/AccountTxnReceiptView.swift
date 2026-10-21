import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct AccountTxnReceiptRoute: AppRoute {
    let row: AccountActivityRow

    @MainActor func destination() -> some View {
        AccountTxnReceiptView(row: row)
    }
}

struct AccountTxnReceiptView: View {
    let row: AccountActivityRow

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ReceiptView(ReceiptContent(row), openAccountActivity: openAccountActivity)
    }

    private func openAccountActivity() {
        let navigator = environment.navigator
        if navigator.path(for: navigator.selectedTab).contains(AnyAppRoute(AccountActivityRoute())) {
            dismiss()
        } else {
            navigator.open(AccountActivityRoute(), in: navigator.selectedTab)
        }
    }
}

#if DEBUG
final class ReceiptSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-receiptHarness") else { return nil }
        let mode = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "completed"
        return AnyView(ReceiptHarnessScreen(mode: mode))
    }
}

private struct ReceiptHarnessScreen: View {
    let mode: String

    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            screen
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }

    @ViewBuilder private var screen: some View {
        switch mode {
        case "error":
            ReceiptView(phase: .failed, reload: {})
        case "loading":
            ReceiptView(phase: .loading, reload: {})
        case "trade":
            ReceiptView(.sampleTrade(status: .completed))
        case "trade-pending":
            ReceiptView(.sampleTrade(status: .pending))
        case "pending":
            ReceiptView(sample(.pending))
        case "failed":
            ReceiptView(sample(.failed))
        case "unknown":
            ReceiptView(sample(.settled, shown: .unknown))
        default:
            ReceiptView(sample(.settled))
        }
    }

    private func sample(
        _ status: Components.Schemas.UserTxn.StatusPayload, shown: ReceiptContent.Status? = nil
    ) -> ReceiptContent {
        let created = Date(timeIntervalSince1970: 1_790_996_400)
        let txn = Components.Schemas.UserTxn(
            id: "01890a5d-ac96-774b-bcce-b302099a8001", kind: .deposit, status: status, usdcMicros: "100000000",
            cabal: nil, txSignature: status == .pending ? nil : "sample-signature-1", createdAt: created)
        guard let row = try? AccountActivityRow(txn, now: created) else {
            return ReceiptContent.sampleTrade(status: .unknown)
        }
        return shown.map { ReceiptContent(row, status: $0) } ?? ReceiptContent(row)
    }
}
#endif
