#if DEBUG
import MonacoAPI
import MonacoFlows
import MonacoRanking
import SwiftUI

enum LedgerSampleScenario: String {
    case week42, noBestCall, loss, notReady, loading

    static let launchArgument = "-MonacoLedgerSample"

    static func matching(_ arguments: [String]) -> LedgerSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return LedgerSampleScenario(rawValue: arguments[flag + 1])
    }

    var state: LoadState<LedgerScreen> {
        switch self {
        case .week42: .loaded(LedgerScreen(.week42))
        case .noBestCall: .loaded(LedgerScreen(.noBestCall))
        case .loss: .loaded(LedgerScreen(.loss))
        case .loading: .loading
        case .notReady:
            .failed(
                .problem(
                    ProblemError(
                        status: 404, code: .known(.ledgerNotFound), message: LedgerCopy.notReady, traceID: "sample",
                        retryable: false)))
        }
    }
}

final class LedgerSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = LedgerSampleScenario.matching(arguments) else { return nil }
        return AnyView(
            SampleAppFrame(
                auth: auth, tab: .home,
                sheet: {
                    AnyView(
                        LedgerSheet(week: "2026-W42", model: .preview(state: scenario.state))
                            .analyticsScreen("ledger"))
                }))
    }
}
final class LedgerFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard Flow37Scenario.matching(arguments) != nil else { return nil }
        return AnyView(
            SampleAppFrame(
                auth: auth, tab: .home,
                sheet: {
                    AnyView(
                        LedgerSheet(
                            week: "2026-W42",
                            model: .preview(
                                state: .failed(
                                    .transport(URLError(.notConnectedToInternet))))
                        )
                        .analyticsScreen("ledger"))
                }))
    }
}
#endif
