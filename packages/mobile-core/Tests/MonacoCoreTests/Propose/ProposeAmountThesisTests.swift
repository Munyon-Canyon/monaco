import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class ProposeAmountThesisTests: XCTestCase {
    func testReviewNeedsAThesisAndSaysSoOnce() async {
        let clock = TestClock()
        let apple = ProposeHoldingTests.holding(
            name: "Apple", units: "1.0000", tokenAmount: 100_000_000, valueMicros: 100_000_000)
        let model = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: .sell(apple), clock: clock)
        model.setAmount(micros: 25_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        model.setThesis("   \n ")
        XCTAssertFalse(model.hasThesis)
        XCTAssertFalse(model.requestReview(assetName: "Apple"))
        XCTAssertTrue(model.showsThesisRequired)

        model.setThesis("Earnings next week.")
        XCTAssertTrue(model.hasThesis)
        XCTAssertFalse(model.showsThesisRequired)
        XCTAssertTrue(model.requestReview(assetName: "Apple"))
    }

    func testAnEmptyThesisDoesNotRaiseTheNoticeWhileReviewIsDisabled() {
        let apple = ProposeHoldingTests.holding(
            name: "Apple", units: "1.0000", tokenAmount: 100_000_000, valueMicros: 100_000_000)
        let model = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: .sell(apple), clock: TestClock())

        XCTAssertFalse(model.requestReview(assetName: "Apple"))
        XCTAssertFalse(model.showsThesisRequired)
    }

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }
}
