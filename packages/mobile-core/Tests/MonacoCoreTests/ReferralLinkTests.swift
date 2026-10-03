import Foundation
import XCTest

@testable import MonacoCore

private final class MemoryStore: KeyValueStoring {
    var values: [String: Any] = [:]

    func data(forKey key: String) -> Data? { values[key] as? Data }
    func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
    func set(_ value: Any?, forKey key: String) { values[key] = value }
    func removeObject(forKey key: String) { values[key] = nil }
}

final class ReferralLinkTests: XCTestCase {
    private let day: TimeInterval = 24 * 60 * 60
    private let captured = Date(timeIntervalSince1970: 1_800_000_000)

    func testParseAccepts() {
        let cases: [(String, String)] = [
            ("https://monacolabs.xyz/r/k7m4qx2p", "k7m4qx2p"),
            ("https://monacolabs.xyz/r/K7M4QX2P", "k7m4qx2p"),
            ("https://www.monacolabs.xyz/r/kaicenat/", "kaicenat"),
            ("https://monacolabs.xyz/r/kaicenat?utm_source=x", "kaicenat"),
            ("https://monacolabs.xyz/r/kai_cenat#top", "kai_cenat"),
            ("  https://monacolabs.xyz/r/abc\n", "abc"),
            ("https://monacolabs.xyz/r/\(String(repeating: "a", count: 20))", String(repeating: "a", count: 20)),
        ]
        for (input, code) in cases {
            XCTAssertEqual(ReferralLink.parse(input)?.value, code, input)
            if let url = URL(string: input.trimmingCharacters(in: .whitespacesAndNewlines)) {
                XCTAssertEqual(ReferralLink.parse(url)?.value, code, input)
            }
        }
    }

    func testParseRejects() {
        let cases = [
            "http://monacolabs.xyz/r/abc",
            "https://monacolabs.xyz.evil.com/r/abc",
            "https://evil.com/r/abc",
            "evil.com/r/abc",
            "monacolabs.xyz/r/abc",
            "https://monacolabs.xyz/r/",
            "https://monacolabs.xyz/r/a",
            "https://monacolabs.xyz/r/ab",
            "https://monacolabs.xyz/r/\(String(repeating: "a", count: 21))",
            "https://monacolabs.xyz/r/abc/def",
            "https://monacolabs.xyz/r/abc//",
            "https://monacolabs.xyz/x/abc",
            "https://monacolabs.xyz/abc",
            "https://user@monacolabs.xyz/r/abc",
            "https://user:pw@monacolabs.xyz/r/abc",
            "https://monacolabs.xyz:8443/r/abc",
            "https://monacolabs.xyz/r/ab🦈c",
            "https://monacolabs.xyz/r/ab%63",
            "https://monacolabs.xyz/r/ab c",
            "https://monacolabs.xyz/r/ab-c",
            "Join my cabal on Monaco: https://monacolabs.xyz/r/k7m4qx2p see you there",
            "https://monacolabs.xyz/r/abc https://monacolabs.xyz/r/def",
            "",
        ]
        for input in cases {
            XCTAssertNil(ReferralLink.parse(input), input)
        }
    }

    func testParseRejectsURLs() {
        let cases = [
            URL(fileURLWithPath: "/r/abc"),
            URL(string: "/r/abc"),
            URL(string: "https:///r/abc"),
        ].compactMap { $0 }
        for url in cases {
            XCTAssertNil(ReferralLink.parse(url), url.absoluteString)
        }
    }

    func testReferralCode() {
        XCTAssertEqual(ReferralCode(" KaiCenat_9 ")?.value, "kaicenat_9")
        XCTAssertEqual(ReferralCode("23456789abcdefghjkmnpqrstuvwxyz"[...].prefix(8).description)?.value, "23456789")
        for raw in ["ab", "", "   ", "abc.def", "abçd", "İstanbul", String(repeating: "z", count: 21)] {
            XCTAssertNil(ReferralCode(raw), raw)
        }
    }

    func testURLForCodeRoundTrips() throws {
        let code = try XCTUnwrap(ReferralCode("K7M4QX2P"))
        let url = ReferralLink.url(for: code)
        XCTAssertEqual(url.absoluteString, "https://monacolabs.xyz/r/k7m4qx2p")
        XCTAssertEqual(ReferralLink.parse(url), code)
    }

    func testSourceRawValuesMatchAPI() {
        XCTAssertEqual(ReferralSource.allCases.map(\.rawValue), ["universal_link", "clipboard", "manual"])
    }

    func testStoreRoundTrip() throws {
        let store = PendingReferralStore(store: MemoryStore())
        let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))
        XCTAssertNil(store.load(now: captured))
        try store.save(code, source: .universalLink, at: captured)
        XCTAssertEqual(
            store.load(now: captured),
            PendingReferral(code: code, source: .universalLink, capturedAt: captured)
        )
    }

    func testStoreOverwriteKeepsLatest() throws {
        let store = PendingReferralStore(store: MemoryStore())
        try store.save(XCTUnwrap(ReferralCode("first")), source: .clipboard, at: captured)
        try store.save(XCTUnwrap(ReferralCode("second")), source: .manual, at: captured.addingTimeInterval(60))
        let loaded = try XCTUnwrap(store.load(now: captured.addingTimeInterval(60)))
        XCTAssertEqual(loaded.code.value, "second")
        XCTAssertEqual(loaded.source, .manual)
    }

    func testStoreExpiresAfterSevenDays() throws {
        let backing = MemoryStore()
        let store = PendingReferralStore(store: backing)
        let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))
        try store.save(code, source: .manual, at: captured)

        XCTAssertEqual(store.load(now: captured.addingTimeInterval(7 * day))?.code, code)
        XCTAssertNil(store.load(now: captured.addingTimeInterval(7 * day + 1)))
        XCTAssertNil(backing.values[PendingReferralStore.key], "expired referral is cleared")
        XCTAssertNil(store.load(now: captured))
    }

    func testStoreClear() throws {
        let backing = MemoryStore()
        let store = PendingReferralStore(store: backing)
        try store.save(XCTUnwrap(ReferralCode("k7m4qx2p")), source: .manual, at: captured)
        store.clear()
        XCTAssertNil(backing.values[PendingReferralStore.key])
        XCTAssertNil(store.load(now: captured))
    }

    func testStoreDropsUnreadableRecords() throws {
        let backing = MemoryStore()
        let store = PendingReferralStore(store: backing)
        let invalidCode = #"{"code":"no","source":"manual","capturedAt":0}"#
        for payload in [Data("garbage".utf8), Data(invalidCode.utf8)] {
            backing.values[PendingReferralStore.key] = payload
            XCTAssertNil(store.load(now: captured))
            XCTAssertNil(backing.values[PendingReferralStore.key])
        }
    }

    func testStoreWorksOverUserDefaults() throws {
        let suite = "ReferralLinkTests.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))
        try PendingReferralStore(store: defaults).save(code, source: .clipboard, at: captured)
        XCTAssertEqual(PendingReferralStore(store: defaults).load(now: captured)?.code, code)
        InvitePasteGate(store: defaults).markOffered()
        XCTAssertTrue(defaults.bool(forKey: InvitePasteGate.key))
    }

    func testGateOffersOnlyWhenEveryConditionHolds() {
        let gate = InvitePasteGate(store: MemoryStore())
        XCTAssertTrue(
            gate.shouldOffer(
                isFirstLaunch: true, isSignedIn: false, clipboardHasProbableURL: true, hasPendingReferral: false))
        XCTAssertFalse(
            gate.shouldOffer(
                isFirstLaunch: false, isSignedIn: false, clipboardHasProbableURL: true, hasPendingReferral: false))
        XCTAssertFalse(
            gate.shouldOffer(
                isFirstLaunch: true, isSignedIn: true, clipboardHasProbableURL: true, hasPendingReferral: false))
        XCTAssertFalse(
            gate.shouldOffer(
                isFirstLaunch: true, isSignedIn: false, clipboardHasProbableURL: false, hasPendingReferral: false))
        XCTAssertFalse(
            gate.shouldOffer(
                isFirstLaunch: true, isSignedIn: false, clipboardHasProbableURL: true, hasPendingReferral: true))
    }

    func testGateShowsOncePerInstall() {
        let backing = MemoryStore()
        InvitePasteGate(store: backing).markOffered()
        XCTAssertEqual(backing.values[InvitePasteGate.key] as? Bool, true)
        XCTAssertFalse(
            InvitePasteGate(store: backing).shouldOffer(
                isFirstLaunch: true, isSignedIn: false, clipboardHasProbableURL: true, hasPendingReferral: false))
    }
}
