#if DEBUG
import MonacoCore
import SwiftUI

enum ShareCardSampleScenario: String, CaseIterable {
    case cabalGain
    case cabalLoss
    case cabalShort
    case meGain
    case meNoCode

    static let launchArgument = "-MonacoShareCardSample"

    static func matching(_ arguments: [String]) -> ShareCardSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ShareCardSampleScenario(rawValue: arguments[flag + 1])
    }

    var content: ShareCardContent {
        switch self {
        case .cabalGain: ShareCardSamples.cabal(.gain)
        case .cabalLoss: ShareCardSamples.cabal(.loss)
        case .cabalShort: ShareCardSamples.cabalShort
        case .meGain: ShareCardSamples.me(.gain)
        case .meNoCode: ShareCardSamples.me(.gain, links: nil)
        }
    }
}

private struct ShareCardSampleHarness: View {
    let scenario: ShareCardSampleScenario
    @State private var model: ShareCardModel

    init(scenario: ShareCardSampleScenario) {
        self.scenario = scenario
        _model = State(initialValue: ShareCardModel.preview(scenario.content))
    }

    var body: some View {
        MonacoTheme.canvas
            .ignoresSafeArea()
            .sheet(isPresented: .constant(true)) {
                ShareCardSheet(model: model) {}
            }
    }
}

final class ShareCardSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ShareCardSampleScenario.matching(arguments) else { return nil }
        return AnyView(ShareCardSampleHarness(scenario: scenario))
    }
}
#endif
