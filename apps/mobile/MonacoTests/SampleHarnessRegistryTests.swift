import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@Suite(.serialized)
@MainActor
struct SampleHarnessRegistryTests {
    private func auth() -> PrivyAuthService {
        guard let auth = PrivyAuthService.processInstance else {
            preconditionFailure("the app host did not create PrivyAuthService")
        }
        return auth
    }

    private func withConflictHook<T>(_ body: (_ tripped: () -> Bool) -> T) -> T {
        let previous = SampleHarnessRegistry.reportConflict
        var tripped = false
        SampleHarnessRegistry.reportConflict = { tripped = true }
        defer { SampleHarnessRegistry.reportConflict = previous }
        return body { tripped }
    }

    @Test func oneMatchingEntryReturnsItsRoot() {
        _ = RegistryOneEntry.self
        RegistryOneEntry.built = false
        let root = withConflictHook { tripped in
            let root = SampleHarnessRegistry.requestedRoot(arguments: ["-RegistryTestOne"], auth: auth())
            #expect(root != nil)
            #expect(!tripped())
            return root
        }
        #expect(root != nil)
        #expect(RegistryOneEntry.built)
    }

    @Test func twoMatchingEntriesTripTheAssertionHook() {
        _ = RegistryBothA.self
        _ = RegistryBothB.self
        withConflictHook { tripped in
            let root = SampleHarnessRegistry.requestedRoot(arguments: ["-RegistryTestBoth"], auth: auth())
            #expect(root != nil)
            #expect(tripped())
        }
    }

    @Test func noFlagReturnsNil() {
        let root = withConflictHook { tripped in
            let root = SampleHarnessRegistry.requestedRoot(arguments: ["-NotAHarness"], auth: auth())
            #expect(!tripped())
            return root
        }
        #expect(root == nil)
    }

    @Test(arguments: Flow00Scenario.allCases)
    func flowScenarioLaunchFlagReturnsTheFlowHarness(scenario: Flow00Scenario) {
        let root = withConflictHook { tripped in
            let root = SampleHarnessRegistry.requestedRoot(
                arguments: ["-MonacoFlow", "00", scenario.rawValue],
                auth: auth()
            )
            #expect(!tripped())
            return root
        }
        #expect(root != nil)
    }

    @Test func homeLaunchFlagReturnsTheHomeHarness() {
        let root = SampleHarnessRegistry.requestedRoot(
            arguments: ["-MonacoHomeSample", "populated"],
            auth: auth()
        )
        #expect(root != nil)
    }
}

private final class RegistryOneEntry: SampleHarnessEntry {
    static var built = false

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-RegistryTestOne") else { return nil }
        built = true
        return AnyView(EmptyView())
    }
}

private final class RegistryBothA: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        arguments.contains("-RegistryTestBoth") ? AnyView(EmptyView()) : nil
    }
}

private final class RegistryBothB: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        arguments.contains("-RegistryTestBoth") ? AnyView(EmptyView()) : nil
    }
}
