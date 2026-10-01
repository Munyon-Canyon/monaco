#if DEBUG
import SwiftUI

final class SystemPingSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-systemPingHarness") else { return nil }
        return AnyView(SystemPingRoute().destination())
    }
}
#endif
