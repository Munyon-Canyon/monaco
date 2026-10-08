#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum GroupNavSampleEntry: String, CaseIterable {
    case root
    case list
    case create
    case start
    case newCabal

    static let launchArgument = "-MonacoGroupNavSample"

    static func matching(_ arguments: [String]) -> GroupNavSampleEntry? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return GroupNavSampleEntry(rawValue: arguments[flag + 1])
    }
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
        case .newCabal: []
        }
    }
}

struct GroupNavSampleHarness: View {
    let entry: GroupNavSampleEntry
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(
            auth: auth, tab: entry.tab, routes: entry.routes,
            sheet: entry == .newCabal ? { AnyView(NewCabalSheet(onCreate: {})) } : nil)
    }
}

final class GroupNavSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let entry = GroupNavSampleEntry.matching(arguments) else { return nil }
        SampleAPIProtocol.install(SampleAPIScript())
        return AnyView(GroupNavSampleHarness(entry: entry, auth: auth))
    }
}
#endif
