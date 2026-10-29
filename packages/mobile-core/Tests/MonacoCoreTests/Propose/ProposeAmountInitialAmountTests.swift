import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class ProposeAmountInitialAmountTests: XCTestCase {
    func testAnInitialAmountRunsThePreviewOnLoad() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: Self.buy, clock: clock, initialAmountMicros: 25_000_000)

        await model.load()
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertEqual(model.amountMicros, 25_000_000)
        XCTAssertNotNil(model.preview)
        let amounts = await service.recordedAmounts()
        XCTAssertEqual(amounts, [25_000_000])
    }

    func testAnInitialAmountAboveTheCabalsCashShowsTheErrorAndNoPreview() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: Self.buy, clock: clock, initialAmountMicros: 600_000_000)

        await model.load()
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertTrue(model.isOverLimit)
        XCTAssertEqual(model.message(assetName: "Alphabet"), ProposeAmountModel.cashShort)
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        let amounts = await service.recordedAmounts()
        XCTAssertTrue(amounts.isEmpty)
    }

    func testASellInitialAmountRunsThePreviewOnLoad() async {
        let clock = TestClock()
        let service = PreviewService()
        let apple = ProposeHoldingTests.holding(
            name: "Apple", units: "1.2034", tokenAmount: 120_345_678, valueMicros: 278_470_000)
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: .sell(apple), clock: clock, initialAmountMicros: 100_000_000)

        await model.load()
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertEqual(model.amountMicros, 100_000_000)
        XCTAssertNotNil(model.preview)
    }

    private static let buy = ProposeTrade.buy(symbol: "GOOGLx", kind: .stock, tokenDecimals: 8)

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }
}
