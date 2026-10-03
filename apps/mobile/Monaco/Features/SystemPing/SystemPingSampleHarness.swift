#if DEBUG
import MonacoCore
import SwiftUI

final class SystemPingSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-systemPingHarness") else { return nil }
        return AnyView(SystemPingRoute().destination())
    }
}

final class SystemPingFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow00Scenario.matching(arguments) else { return nil }
        let model = SystemPingModel.preview(answering: scenario)
        return AnyView(
            SystemPingRoute().destination(model: model)
                .task { await model.send(note: "Sample") }
        )
    }
}
#endif
