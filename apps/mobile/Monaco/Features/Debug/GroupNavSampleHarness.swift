#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum GroupNavSampleEntry: String, CaseIterable {
    case root
    case list
    case create
    case start

    static let launchArgument = "-MonacoGroupNavSample"

}

extension GroupNavSampleEntry {
    var tab: MainTab {
        self == .list ? .home : .cabals
    }

    var routes: [any AppRoute] {
        let cabal = CabalRoute(id: GroupDetailSampleData.cabalID)
        return switch self {
        case .root, .list: [cabal]
        case .create: [CreateCabalRoute(), cabal]
        case .start: [CreateCabalRoute()]
        }
    }
}

struct GroupNavSampleHarness: View {
    let entry: GroupNavSampleEntry
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(auth: auth, tab: entry.tab, routes: entry.routes)
    }
}

final class GroupNavSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        return scenarioRoot(
            GroupNavSampleEntry.self, flag: GroupNavSampleEntry.launchArgument, arguments: arguments
        ) { entry in
            SampleAPIProtocol.install(SampleAPIScript())
            return AnyView(GroupNavSampleHarness(entry: entry, auth: auth))
        }
    }
}
#endif
