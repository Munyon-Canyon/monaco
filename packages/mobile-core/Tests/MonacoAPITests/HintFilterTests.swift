import MonacoAPI
import XCTest

final class HintFilterTests: XCTestCase {
    private func hint(_ key: HintKey, _ what: String) -> Hint { .changed(key, what: what, id: "1") }

    func testCabalFilterMatchesOnlyItsCabal() {
        let filter = HintFilter.cabal(id: "42", what: nil)
        XCTAssertTrue(filter.matches(hint(.cabal("42"), "updated")))
        XCTAssertTrue(filter.matches(hint(.cabal("42"), "member_joined")))
        XCTAssertFalse(filter.matches(hint(.cabal("7"), "updated")))
        XCTAssertFalse(filter.matches(hint(.global, "updated")))
    }

    func testWhatNarrowsToOneToken() {
        let filter = HintFilter.global(what: "feed")
        XCTAssertTrue(filter.matches(hint(.global, "feed")))
        XCTAssertFalse(filter.matches(hint(.global, "prices")))
        XCTAssertFalse(filter.matches(hint(.user("9f2c"), "feed")))
        XCTAssertFalse(filter.matches(hint(.cabal("42"), "feed")))
        XCTAssertFalse(HintFilter.cabal(id: "42", what: "updated").matches(hint(.cabal("42"), "voted")))
    }

    func testUserFilterNeedsNoID() {
        XCTAssertTrue(HintFilter.user(what: nil).matches(hint(.user("9f2c"), "balance")))
        XCTAssertTrue(HintFilter.user(what: "balance").matches(hint(.user("9f2c"), "balance")))
        XCTAssertFalse(HintFilter.user(what: nil).matches(hint(.cabal("42"), "balance")))
    }

    func testResyncMatchesEveryFilter() {
        for filter in [HintFilter.user(what: nil), .cabal(id: "42", what: "updated"), .global(what: "feed")] {
            XCTAssertTrue(filter.matches(.resync), "\(filter)")
        }
    }
}
