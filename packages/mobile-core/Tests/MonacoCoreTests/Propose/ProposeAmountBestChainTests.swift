import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class ProposeAmountBestChainTests: XCTestCase {
    func testTypingShowsTheEstimateAtOnceAndPinsThePreviewToTheBestChain() async {
        let clock = TestClock()
        let service = PreviewService(quoteOut: 10_030_000)
        let store = await Self.prefetched()
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: Self.buy, clock: clock, bestChains: store)

        model.setAmount(micros: 25_000_000)

        XCTAssertEqual(model.buyQuantityNote, "Estimate: about 0.1003 shares on Base")
        XCTAssertEqual(model.helperText, "Finding the best price…")
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        let networks = await service.recordedNetworks()
        XCTAssertEqual(networks, [.base])
        XCTAssertNil(model.estimate)
        XCTAssertEqual(model.buyQuantityNote, "About 0.1003 shares on Base")
        XCTAssertEqual(model.previewBestChain?.network, .base)
        XCTAssertTrue(model.reviewEnabled(assetName: "Alphabet"))
    }

    func testWithoutAPrefetchTheAmountScreenPreviewsEveryChainAsBefore() async {
        let clock = TestClock()
        let service = PreviewService()
        let store = AssetBestChainStore(now: { Date() }, fetch: { _ in throw PreviewService.Failure() })
        await store.refreshIfNeeded(symbol: "GOOGLx")
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: Self.buy, clock: clock, bestChains: store)

        model.setAmount(micros: 25_000_000)

        XCTAssertNil(model.estimate)
        XCTAssertNil(model.buyQuantityNote)
        XCTAssertEqual(model.helperText, "Finding the best price…")
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        let networks = await service.recordedNetworks()
        XCTAssertEqual(networks, [nil])
        XCTAssertNil(model.previewBestChain)
        XCTAssertTrue(model.reviewEnabled(assetName: "Alphabet"))
    }

    func testAPinnedChainWithNoRouteFallsBackToEveryChain() async {
        let clock = TestClock()
        let service = PreviewService(pinnedCode: "no_route")
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: Self.buy, clock: clock, bestChains: await Self.prefetched())

        model.setAmount(micros: 25_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        let networks = await service.recordedNetworks()
        XCTAssertEqual(networks, [.base, nil])
        XCTAssertNil(model.previewBestChain)
        XCTAssertNil(model.message(assetName: "Alphabet"))
        XCTAssertTrue(model.reviewEnabled(assetName: "Alphabet"))
    }

    func testASellNeverAsksForTheBestChain() async {
        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.0000", tokenAmount: 100_000_000)
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(
            service: service, cabalID: "cabal", trade: .sell(apple), clock: clock, bestChains: await Self.prefetched())

        model.setAmount(micros: 25_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertNil(model.estimate)
        let networks = await service.recordedNetworks()
        XCTAssertEqual(networks, [nil])
    }

    private static func prefetched() async -> AssetBestChainStore {
        let store = AssetBestChainStore(
            now: { Date() },
            fetch: { symbol in
                guard let bestChain = AssetBestChain(.fixture(symbol: symbol, now: Date())) else {
                    throw PreviewService.Failure()
                }
                return bestChain
            })
        await store.refreshIfNeeded(symbol: "GOOGLx")
        return store
    }

    private static let buy = ProposeTrade.buy(symbol: "GOOGLx", kind: .stock, tokenDecimals: 8)

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }
}
