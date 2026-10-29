import Foundation
import MonacoCore
import MonacoReferrals
import Testing

@testable import Monaco

@MainActor
@Suite(.serialized)
struct ReferralDeepLinkTests {
    private final class MemoryStore: KeyValueStoring {
        var values: [String: Any] = [:]

        func data(forKey key: String) -> Data? { values[key] as? Data }
        func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
        func set(_ value: Any?, forKey key: String) { values[key] = value }
        func removeObject(forKey key: String) { values[key] = nil }
    }

    private static let cabal = "01890a5d-ac96-774b-bcce-b302099a8058"
    private static let proposal = "01890a5d-ac96-774b-bcce-b302099a8057"

    private func withSession(
        signedIn: Bool, tab: MainTab = .home, _ body: (PendingReferralStore, PendingLinkTargetStore) throws -> Void
    ) rethrows {
        let savedSession = ReferralDeepLink.session
        let savedPending = ReferralDeepLink.pending
        let savedTargets = ReferralDeepLink.targets
        defer {
            ReferralDeepLink.session = savedSession
            ReferralDeepLink.pending = savedPending
            ReferralDeepLink.targets = savedTargets
        }
        let store = PendingReferralStore(store: MemoryStore())
        let targets = PendingLinkTargetStore(store: MemoryStore())
        ReferralDeepLink.session = { .init(isSignedIn: signedIn, currentTab: tab) }
        ReferralDeepLink.pending = store
        ReferralDeepLink.targets = targets
        try body(store, targets)
    }

    @Test func aReferralLinkOpensTheReferrerOnTheCurrentTabWhenSignedIn() throws {
        let url = try #require(URL(string: "https://monacolabs.xyz/r/k7m4qx2p"))

        withSession(signedIn: true, tab: .feed) { store, _ in
            guard case .open(let route, let tab)? = ReferralDeepLink.action(for: url) else {
                Issue.record("expected .open")
                return
            }

            #expect(tab == .feed)
            #expect((route as? ReferralRoute)?.code.value == "k7m4qx2p")
            #expect(store.load(now: .now) == nil)
        }
    }

    @Test func aCabalLinkOpensTheLandingInTheCabalsTabWhenSignedIn() throws {
        let url = try #require(URL(string: "https://monacolabs.xyz/r/k7m4qx2p?c=\(Self.cabal)"))

        withSession(signedIn: true, tab: .feed) { _, _ in
            guard case .open(let route, let tab)? = ReferralDeepLink.action(for: url) else {
                Issue.record("expected .open")
                return
            }

            #expect(tab == .cabals)
            #expect((route as? InviteLandingRoute)?.cabalID == Self.cabal)
            #expect((route as? InviteLandingRoute)?.code.value == "k7m4qx2p")
        }
    }

    @Test func aVoteLinkPresentsTheHoldSheetWhenSignedIn() throws {
        let url = try #require(URL(string: "https://monacolabs.xyz/r/k7m4qx2p?c=\(Self.cabal)&p=\(Self.proposal)"))

        withSession(signedIn: true) { _, _ in
            guard case .voteHold(let proposalID)? = ReferralDeepLink.action(for: url) else {
                Issue.record("expected .voteHold")
                return
            }

            #expect(proposalID == Self.proposal)
        }
    }

    @Test func aCabalLinkSavesCodeAndTargetAndShowsTheLandingWhenSignedOut() throws {
        let url = try #require(URL(string: "https://www.monacolabs.xyz/r/k7m4qx2p?c=\(Self.cabal)"))

        withSession(signedIn: false) { store, targets in
            guard case .signedOutLanding(let route)? = ReferralDeepLink.action(for: url) else {
                Issue.record("expected .signedOutLanding")
                return
            }

            #expect(route.cabalID == Self.cabal)
            #expect(store.load(now: .now)?.code.value == "k7m4qx2p")
            #expect(store.load(now: .now)?.source == .universalLink)
            let target = targets.peek(now: .now)
            #expect(target?.cabalID == Self.cabal)
            #expect(target?.kind == .join)
        }
    }

    @Test func aReferralLinkIsStoredNotOpenedWhenSignedOutWithoutACabal() throws {
        let url = try #require(URL(string: "https://www.monacolabs.xyz/r/k7m4qx2p"))

        withSession(signedIn: false) { store, targets in
            guard case .handled? = ReferralDeepLink.action(for: url) else {
                Issue.record("expected .handled")
                return
            }

            #expect(store.load(now: .now)?.code.value == "k7m4qx2p")
            #expect(store.load(now: .now)?.source == .universalLink)
            #expect(targets.peek(now: .now) == nil)
        }
    }

    @Test func otherURLsOpenNothingAndStoreNothing() throws {
        let urls = ["monaco://deposit/complete", "https://monacolabs.xyz/about", "https://example.com/r/k7m4qx2p"]

        for string in urls {
            let url = try #require(URL(string: string))
            withSession(signedIn: false) { store, _ in
                #expect(ReferralDeepLink.action(for: url) == nil)
                #expect(store.load(now: .now) == nil)
            }
        }
    }
}
