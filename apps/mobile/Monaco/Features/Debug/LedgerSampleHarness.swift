#if DEBUG
import MonacoAPI
import MonacoCore
import MonacoFlows
import MonacoRanking
import SwiftUI

enum LedgerSampleScenario: String {
    case week42, noBestCall, loss, notReady, loading, story, storyNoCalls

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
        case .loading, .story, .storyNoCalls: .loading
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
        if scenario == .story || scenario == .storyNoCalls {
            return AnyView(LedgerStorySample(eyes: ShareEyes(buySell: scenario == .story)))
        }
        SampleAPIProtocol.install(HomeSampleScenario.populated.script)
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
private struct LedgerStorySample: View {
    @State private var eyes: ShareEyes
    @State private var format = ShareFormat.story

    init(eyes: ShareEyes) {
        _eyes = State(initialValue: eyes)
    }

    private var content: LedgerShareContent {
        var ledger = Components.Schemas.Ledger.week42
        let start = ledger.windowStart
        ledger.series = (0..<42).map { (step: Int) -> Components.Schemas.LedgerPoint in
            let bps: Int64 = Int64(step * 3) + Int64(step % 5) * 4 - 20
            let at: Date = start.addingTimeInterval(Double(step) * 14_400)
            return Components.Schemas.LedgerPoint(at: at, returnBps: bps)
        }
        let callAt: Date = start.addingTimeInterval(5 * 86_400 + 54_000)
        let call = Components.Schemas.LedgerMarker(at: callAt, kind: .call, symbol: "AMZN", label: "Called AMZN")
        ledger.markers = [call]
        let cover = SampleImage.flat(.indigo, name: "ledger-cover")?.absoluteString
        ledger.cabals = ledger.cabals.map { cabal in
            Components.Schemas.LedgerCabal(id: cabal.id, name: cabal.name, pictureUrl: cover)
        }
        return LedgerShareContent(
            ledger, name: "Andres", photoURL: nil, code: "ANDRES", timeZone: TimeZone(identifier: "UTC") ?? .gmt)
    }

    var body: some View {
        NavigationStack {
            ShareScreenContent(
                state: .loaded(ShareLedgerBuilder.build(content: content, eyes: eyes)), eyes: $eyes, format: $format,
                share: { _, _ in }, copyImage: { _ in }, retry: {}
            )
            .monacoCanvas()
            .navigationTitle(content.weekTitle)
            .navigationBarTitleDisplayMode(.inline)
            .navigationBarBackButtonHidden()
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    MonacoGlassButton(
                        systemImage: "chevron.left", accessibilityLabel: "Back", placement: .toolbar, action: {})
                }
            }
        }
        .environment(\.colorScheme, .dark)
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
