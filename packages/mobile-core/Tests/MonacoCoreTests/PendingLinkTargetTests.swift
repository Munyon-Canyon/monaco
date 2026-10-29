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

final class PendingLinkTargetTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func target(cabalID: String = "c1", joinTapped: Bool = true, savedAt: Date? = nil) -> PendingLinkTarget {
        PendingLinkTarget(
            kind: .join, code: "k7m4qx2p", cabalID: cabalID, proposalID: nil, joinTapped: joinTapped,
            savedAt: savedAt ?? now)
    }

    func testARoundTripReturnsTheTargetOnceThenNil() {
        let store = PendingLinkTargetStore(store: MemoryStore())
        store.save(target())
        XCTAssertEqual(store.peek(now: now), target())
        XCTAssertEqual(store.take(now: now), target())
        XCTAssertNil(store.take(now: now))
    }

    func testTheNewestSaveWins() {
        let store = PendingLinkTargetStore(store: MemoryStore())
        store.save(target(cabalID: "c1"))
        store.save(target(cabalID: "c2"))
        XCTAssertEqual(store.take(now: now)?.cabalID, "c2")
    }

    func testAnEightDayOldRecordIsNilAndCleared() {
        let memory = MemoryStore()
        let store = PendingLinkTargetStore(store: memory)
        store.save(target(savedAt: now.addingTimeInterval(-8 * 86_400)))
        XCTAssertNil(store.take(now: now))
        XCTAssertNil(memory.data(forKey: PendingLinkTargetStore.key))
    }

    func testASixDayOldRecordStillCounts() {
        let store = PendingLinkTargetStore(store: MemoryStore())
        store.save(target(savedAt: now.addingTimeInterval(-6 * 86_400)))
        XCTAssertNotNil(store.peek(now: now))
    }

    func testAnUnreadableRecordIsNilAndCleared() {
        let memory = MemoryStore()
        memory.set(Data("nope".utf8), forKey: PendingLinkTargetStore.key)
        XCTAssertNil(PendingLinkTargetStore(store: memory).peek(now: now))
        XCTAssertNil(memory.data(forKey: PendingLinkTargetStore.key))
    }

    func testNothingStoredIsNil() {
        XCTAssertNil(PendingLinkTargetStore(store: MemoryStore()).take(now: now))
    }

    func testTakeJoinIsTrueOnceForTheMatchingCabal() {
        let store = PendingLinkTargetStore(store: MemoryStore())
        store.save(target())
        XCTAssertTrue(store.takeJoin(cabalID: "c1", now: now))
        XCTAssertFalse(store.takeJoin(cabalID: "c1", now: now))
    }

    func testTakeJoinForAnotherCabalIsFalseAndLeavesTheRecord() {
        let store = PendingLinkTargetStore(store: MemoryStore())
        store.save(target())
        XCTAssertFalse(store.takeJoin(cabalID: "c2", now: now))
        XCTAssertNotNil(store.peek(now: now))
    }

    func testTakeJoinWithoutAJoinTapIsFalseAndLeavesTheRecord() {
        let store = PendingLinkTargetStore(store: MemoryStore())
        store.save(target(joinTapped: false))
        XCTAssertFalse(store.takeJoin(cabalID: "c1", now: now))
        XCTAssertNotNil(store.peek(now: now))
    }

    func testTheCodeSurvivesClearingThePendingReferral() throws {
        let memory = MemoryStore()
        let referrals = PendingReferralStore(store: memory)
        try referrals.save(try XCTUnwrap(ReferralCode("k7m4qx2p")), source: .universalLink, at: now)
        let store = PendingLinkTargetStore(store: memory)
        store.save(target())
        referrals.clear()
        XCTAssertEqual(store.take(now: now)?.code, "k7m4qx2p")
    }
}
