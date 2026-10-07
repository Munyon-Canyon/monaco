import Foundation
import MonacoCore
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

    private func withSession(
        signedIn: Bool, tab: MainTab = .home, _ body: (PendingReferralStore) throws -> Void
    ) rethrows {
        let savedSession = ReferralDeepLink.session
        let savedPending = ReferralDeepLink.pending
        defer {
            ReferralDeepLink.session = savedSession
            ReferralDeepLink.pending = savedPending
        }
        let store = PendingReferralStore(store: MemoryStore())
        ReferralDeepLink.session = { .init(isSignedIn: signedIn, currentTab: tab) }
        ReferralDeepLink.pending = store
        try body(store)
    }

    @Test func aReferralLinkOpensTheReferrerOnTheCurrentTabWhenSignedIn() throws {
        let url = try #require(URL(string: "https://monacolabs.xyz/r/k7m4qx2p"))

        try withSession(signedIn: true, tab: .feed) { store in
            let (route, tab) = try #require(ReferralDeepLink.route(for: url))

            #expect(tab == .feed)
            #expect((route as? ReferralRoute)?.code.value == "k7m4qx2p")
            #expect(store.load(now: .now) == nil)
        }
    }

    @Test func aReferralLinkIsStoredNotOpenedWhenSignedOut() throws {
        let url = try #require(URL(string: "https://www.monacolabs.xyz/r/k7m4qx2p"))

        withSession(signedIn: false) { store in
            #expect(ReferralDeepLink.route(for: url) == nil)

            let pending = store.load(now: .now)
            #expect(pending?.code.value == "k7m4qx2p")
            #expect(pending?.source == .universalLink)
        }
    }

    @Test func otherURLsOpenNothingAndStoreNothing() throws {
        let urls = ["monaco://deposit/complete", "https://monacolabs.xyz/about", "https://example.com/r/k7m4qx2p"]

        for string in urls {
            let url = try #require(URL(string: string))
            withSession(signedIn: false) { store in
                #expect(ReferralDeepLink.route(for: url) == nil)
                #expect(store.load(now: .now) == nil)
            }
        }
    }
}
