import MonacoCore
import XCTest

@MainActor
final class GifPlaybackBudgetTests: XCTestCase {
    func testReleasingASlotLetsAWaitingTileClaimIt() async {
        let budget = GifPlaybackBudget(limit: 1)
        XCTAssertTrue(budget.claim("a"))
        XCTAssertFalse(budget.claim("b"))
        let before = budget.revision

        budget.release("a")

        XCTAssertGreaterThan(budget.revision, before)
        XCTAssertTrue(budget.claim("b"))
    }

    func testClaimingAHeldIdTakesNoSecondSlot() async {
        let budget = GifPlaybackBudget(limit: 1)
        XCTAssertTrue(budget.claim("a"))
        XCTAssertTrue(budget.claim("a"))
        XCTAssertFalse(budget.claim("b"))
    }

    func testReleasingAnIdWithoutASlotLeavesRevisionUnchanged() async {
        let budget = GifPlaybackBudget(limit: 1)
        let before = budget.revision

        budget.release("a")

        XCTAssertEqual(budget.revision, before)
    }
}
