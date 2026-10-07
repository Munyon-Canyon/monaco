import Foundation
import MonacoCore
import XCTest

private final class MemoryStore: KeyValueStoring {
    var values: [String: Any] = [:]

    func data(forKey key: String) -> Data? { values[key] as? Data }
    func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
    func set(_ value: Any?, forKey key: String) { values[key] = value }
    func removeObject(forKey key: String) { values[key] = nil }
}

@MainActor
final class InvitePasteModelTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let store = MemoryStore()
    private var events: [ReferralAppEvent] = []

    func testItOffersOnTheFirstLaunchWithAURLOnTheClipboardAndTracksIt() async {
        let model = makeModel()

        let offer = await model.shouldOffer(isFirstLaunch: true, clipboardHasProbableURL: { true })

        XCTAssertTrue(offer)
        XCTAssertEqual(events, [.invitePasteShown])
    }

    func testItNeverLooksAtTheClipboardOffTheFirstLaunch() async {
        let model = makeModel()
        var looked = false

        let offer = await model.shouldOffer(
            isFirstLaunch: false,
            clipboardHasProbableURL: {
                looked = true
                return true
            })

        XCTAssertFalse(offer)
        XCTAssertFalse(looked)
        XCTAssertEqual(events, [])
    }

    func testItDoesNotOfferWithoutAURLOnTheClipboard() async {
        let offer = await makeModel().shouldOffer(isFirstLaunch: true, clipboardHasProbableURL: { false })

        XCTAssertFalse(offer)
        XCTAssertEqual(events, [])
    }

    func testItDoesNotOfferWhenAReferralIsAlreadyPending() async throws {
        let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))
        try PendingReferralStore(store: store).save(code, source: .universalLink, at: now)

        let offer = await makeModel().shouldOffer(isFirstLaunch: true, clipboardHasProbableURL: { true })

        XCTAssertFalse(offer)
    }

    func testAValidPasteSavesTheClipboardCodeAndMarksTheOfferDone() async throws {
        let model = makeModel()
        let url = try XCTUnwrap(URL(string: "https://monacolabs.xyz/r/k7m4qx2p"))

        let result = model.paste(url)

        XCTAssertEqual(result, .added)
        XCTAssertEqual(result.toast, "Invite added.")
        let pending = PendingReferralStore(store: store).load(now: now)
        XCTAssertEqual(pending?.code.value, "k7m4qx2p")
        XCTAssertEqual(pending?.source, .clipboard)
        XCTAssertEqual(events, [.invitePasted])
        let again = await model.shouldOffer(isFirstLaunch: true, clipboardHasProbableURL: { true })
        XCTAssertFalse(again)
    }

    func testAnInvalidPasteSavesNothingAndMarksTheOfferDone() async throws {
        let model = makeModel()
        let url = try XCTUnwrap(URL(string: "https://example.com/r/k7m4qx2p"))

        let result = model.paste(url)

        XCTAssertEqual(result, .notAnInvite)
        XCTAssertEqual(result.toast, "That link isn't a Monaco invite.")
        XCTAssertNil(PendingReferralStore(store: store).load(now: now))
        XCTAssertEqual(events, [])
        XCTAssertTrue(store.bool(forKey: InvitePasteGate.key))
    }

    func testSkipMarksTheOfferDoneAndTracksIt() async {
        let model = makeModel()

        model.skip()

        XCTAssertEqual(events, [.inviteSkipped])
        let offer = await model.shouldOffer(isFirstLaunch: true, clipboardHasProbableURL: { true })
        XCTAssertFalse(offer)
    }

    func testOnlyTheFirstLaunchIsFirst() {
        let marker = FirstLaunchMarker(store: store)

        XCTAssertTrue(marker.registerLaunch())
        XCTAssertFalse(marker.registerLaunch())
    }

    func testEventNamesMatchTheAnalyticsFunnel() {
        XCTAssertEqual(
            ReferralAppEvent.allCases.map(\.rawValue), ["invite_paste_shown", "invite_pasted", "invite_skipped"])
    }

    private func makeModel() -> InvitePasteModel {
        let at = now
        return InvitePasteModel(store: store, now: { at }, track: { [weak self] in self?.events.append($0) })
    }
}
