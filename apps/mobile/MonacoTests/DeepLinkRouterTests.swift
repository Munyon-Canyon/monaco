import Foundation
import MonacoCore
import MonacoReferrals
import SwiftUI
import Synchronization
import Testing

@testable import Monaco

@MainActor
struct DeepLinkRouterTests {
    @Test func matchingURLOpensItsRouteInItsTab() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
            URL(string: "monaco://match")!,
            navigator: navigator,
            handlers: [ProbeDeepLink.self, NeverMatchesDeepLink.self]
        )

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.path(for: .cabals).count == 1)
        for tab in MainTab.allCases where tab != .cabals {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func theLinkIsReadOnlyAfterTheSessionRestores() async {
        let navigator = AppNavigator()
        RestoreGatedDeepLink.restored.withLock { $0 = false }

        await DeepLinkRouter.handle(
            URL(string: "monaco://gated")!,
            navigator: navigator,
            handlers: [RestoreGatedDeepLink.self],
            sessionRestored: { RestoreGatedDeepLink.restored.withLock { $0 = true } },
            dismissPresented: {}
        )

        #expect(navigator.path(for: .cabals).count == 1)
        #expect(navigator.signedOutLanding == nil)
    }

    @Test func matchingURLClosesOpenSheetsBeforeItsRouteOpens() async {
        let navigator = AppNavigator()
        var routeWasOpenWhenSheetsClosed: [Bool] = []

        await DeepLinkRouter.handle(
            URL(string: "monaco://match")!,
            navigator: navigator,
            handlers: [ProbeDeepLink.self],
            dismissPresented: { routeWasOpenWhenSheetsClosed.append(!navigator.path(for: .cabals).isEmpty) }
        )

        #expect(routeWasOpenWhenSheetsClosed == [false])
        #expect(navigator.path(for: .cabals).count == 1)
    }

    @Test func nonMatchingURLLeavesOpenSheetsAlone() async {
        let navigator = AppNavigator()
        var closings = 0

        await DeepLinkRouter.handle(
            URL(string: "monaco://nothing")!,
            navigator: navigator,
            handlers: [NeverMatchesDeepLink.self],
            dismissPresented: { closings += 1 }
        )

        #expect(closings == 0)
    }

    @Test func nonMatchingURLOpensNothing() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
            URL(string: "monaco://nothing")!,
            navigator: navigator,
            handlers: [NeverMatchesDeepLink.self]
        )

        #expect(navigator.selectedTab == .home)
        for tab in MainTab.allCases {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func twoMatchingHandlersOnlyOpenTheFirst() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
            URL(string: "monaco://either")!,
            navigator: navigator,
            handlers: [AlwaysMatchesInCabals.self, AlwaysMatchesInProfile.self]
        )

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.path(for: .cabals).count == 1)
        #expect(navigator.path(for: .profile).isEmpty)
    }
}

extension DeepLinkRouterTests {
    private final class MemoryStore: KeyValueStoring {
        var values: [String: Any] = [:]

        func data(forKey key: String) -> Data? { values[key] as? Data }
        func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
        func set(_ value: Any?, forKey key: String) { values[key] = value }
        func removeObject(forKey key: String) { values[key] = nil }
    }

    private static let clock = Date(timeIntervalSince1970: 1_800_000_000)

    private func pendingStore(
        kind: PendingLinkTarget.Kind?, code: String = "k7m4qx2p", proposalID: String? = "p1", savedAt: Date = clock
    ) -> PendingLinkTargetStore {
        let store = PendingLinkTargetStore(store: MemoryStore())
        if let kind {
            store.save(
                PendingLinkTarget(
                    kind: kind, code: code, cabalID: "c1", proposalID: proposalID, joinTapped: false, savedAt: savedAt))
        }
        return store
    }

    @Test func voteHoldActionPresentsTheSheetAfterDismissingAndLeavesPathsAlone() async {
        let navigator = AppNavigator()
        var holdWhenDismissed: [Bool] = []

        await DeepLinkRouter.handle(
            URL(string: "monaco://hold")!,
            navigator: navigator,
            handlers: [VoteHoldDeepLink.self],
            dismissPresented: { holdWhenDismissed.append(navigator.voteHold != nil) }
        )

        #expect(holdWhenDismissed == [false])
        #expect(navigator.voteHold?.proposalID == "p1")
        #expect(navigator.voteHold?.pick == nil)
        for tab in MainTab.allCases {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func signedOutLandingActionSetsTheLandingSlot() async throws {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
            URL(string: "monaco://landing")!,
            navigator: navigator,
            handlers: [SignedOutLandingDeepLink.self],
            dismissPresented: {}
        )

        #expect(navigator.signedOutLanding?.cabalID == "c1")
        navigator.signedOutLanding?.onSignIn.run()
        #expect(navigator.signedOutLanding == nil)
    }

    @Test func handledActionDismissesNothingAndOpensNothing() async {
        let navigator = AppNavigator()
        var closings = 0

        await DeepLinkRouter.handle(
            URL(string: "monaco://saved")!,
            navigator: navigator,
            handlers: [HandledDeepLink.self],
            dismissPresented: { closings += 1 }
        )

        #expect(closings == 0)
        #expect(navigator.voteHold == nil)
        #expect(navigator.signedOutLanding == nil)
    }

    @Test func resumePendingOpensASavedJoinTargetOnce() async {
        let navigator = AppNavigator()
        let store = pendingStore(kind: .join, proposalID: nil)

        await DeepLinkRouter.resumePending(
            navigator: navigator, store: store, now: { Self.clock }, dismissPresented: {})
        await DeepLinkRouter.resumePending(
            navigator: navigator, store: store, now: { Self.clock }, dismissPresented: {})

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.path(for: .cabals).count == 1)
        #expect(navigator.voteHold == nil)
    }

    @Test func resumePendingPresentsASavedVoteTargetOnce() async {
        let navigator = AppNavigator()
        let store = pendingStore(kind: .vote)
        var closings = 0

        await DeepLinkRouter.resumePending(
            navigator: navigator, store: store, now: { Self.clock }, dismissPresented: { closings += 1 })
        let first = navigator.voteHold
        navigator.dismissVoteHold()
        await DeepLinkRouter.resumePending(
            navigator: navigator, store: store, now: { Self.clock }, dismissPresented: { closings += 1 })

        #expect(first?.proposalID == "p1")
        #expect(first?.pick == nil)
        #expect(navigator.voteHold == nil)
        #expect(closings == 1)
        for tab in MainTab.allCases {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func resumePendingDoesNothingWithoutATarget() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.resumePending(
            navigator: navigator, store: pendingStore(kind: nil), now: { Self.clock }, dismissPresented: {})

        #expect(navigator.voteHold == nil)
        #expect(navigator.path(for: .cabals).isEmpty)
    }

    @Test func resumePendingDoesNothingForAnExpiredTarget() async {
        let navigator = AppNavigator()
        let store = pendingStore(kind: .vote)

        await DeepLinkRouter.resumePending(
            navigator: navigator, store: store, now: { Self.clock.addingTimeInterval(8 * 24 * 60 * 60) },
            dismissPresented: {})

        #expect(navigator.voteHold == nil)
    }

    @Test func resumePendingDropsAJoinTargetWithAnUnreadableCode() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.resumePending(
            navigator: navigator, store: pendingStore(kind: .join, code: "x"), now: { Self.clock },
            dismissPresented: {})

        #expect(navigator.path(for: .cabals).isEmpty)
    }

    @Test func resumePendingDropsAVoteTargetWithoutAProposal() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.resumePending(
            navigator: navigator, store: pendingStore(kind: .vote, proposalID: nil), now: { Self.clock },
            dismissPresented: {})

        #expect(navigator.voteHold == nil)
    }
}

private enum VoteHoldDeepLink: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? { .voteHold(proposalID: "p1") }
}

private enum SignedOutLandingDeepLink: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? {
        guard let code = ReferralCode("k7m4qx2p") else { return nil }
        return .signedOutLanding(InviteLandingRoute(code: code, cabalID: "c1"))
    }
}

private enum HandledDeepLink: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? { .handled }
}

nonisolated private struct ProbeRoute: AppRoute {
    let marker: String

    func destination() -> some View {
        Text(marker)
    }
}

private enum ProbeDeepLink: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? {
        guard url.host == "match" else { return nil }
        return .open(ProbeRoute(marker: "matched"), in: .cabals)
    }
}

private enum RestoreGatedDeepLink: DeepLinkHandler {
    static let restored = Mutex(false)

    static func action(for url: URL) -> DeepLinkAction? {
        guard restored.withLock({ $0 }) else {
            guard let code = ReferralCode("k7m4qx2p") else { return nil }
            return .signedOutLanding(InviteLandingRoute(code: code, cabalID: "c1"))
        }
        return .open(ProbeRoute(marker: "restored"), in: .cabals)
    }
}

private enum NeverMatchesDeepLink: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? {
        nil
    }
}

private enum AlwaysMatchesInCabals: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? {
        .open(ProbeRoute(marker: "cabals"), in: .cabals)
    }
}

private enum AlwaysMatchesInProfile: DeepLinkHandler {
    static func action(for url: URL) -> DeepLinkAction? {
        .open(ProbeRoute(marker: "profile"), in: .profile)
    }
}
