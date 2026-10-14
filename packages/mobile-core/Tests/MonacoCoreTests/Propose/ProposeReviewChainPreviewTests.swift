import Foundation
import MonacoAPI
import MonacoCore
import XCTest

@MainActor
final class ProposeReviewChainPreviewTests: XCTestCase {
    func testThePrefetchShowsEveryChainAtOnceWithTheBestChainChosen() throws {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service, bestChain: try Self.bestChain())

        XCTAssertTrue(model.isEstimate)
        XCTAssertEqual(model.chain, .best)
        XCTAssertEqual(model.bestPrice?.winner.network, .base)
        XCTAssertEqual(model.chainOptions.map(\.title), ["Best price", "Base", "Solana", "Monad"])
        XCTAssertEqual(
            model.chainOptions.map(\.figure),
            ["about 1.0030 shares", "about 1.0030 shares", "about 1.0000 shares", "No price yet"])
        XCTAssertEqual(model.rows.first, .init(label: "Cabal gets", value: "about 0.73 shares"))
        XCTAssertTrue(model.canSend)
    }

    func testPickingAnotherChainPreviewsOnlyThatChain() async throws {
        let service = ReviewService(chainQuoteOut: 99_000_000, holdsChain: true)
        let model = makeModel(kind: .stock, service: service, bestChain: try Self.bestChain())

        model.choose(.network(.solana))

        XCTAssertEqual(model.rows.first, .init(label: "Cabal gets", value: "Estimate: about 1.0000 shares"))
        XCTAssertFalse(model.canSend)
        let asked = await service.waitUntilChainAsked()
        XCTAssertTrue(asked, "picking Solana previews Solana")
        await service.releaseChain()
        await settle()
        let previews = await service.previewNetworks
        XCTAssertEqual(previews, [.solana])
        XCTAssertEqual(model.rows.first, .init(label: "Cabal gets", value: "about 0.99 shares"))
        XCTAssertTrue(model.canSend)
        await model.send()
        let networks = await service.networks
        XCTAssertEqual(networks, [.solana])
    }

    func testPickingTheBestChainAgainAsksForNothing() async throws {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service, bestChain: try Self.bestChain())

        model.choose(.network(.base))
        model.choose(.best)
        await settle()

        let previews = await service.previewNetworks
        XCTAssertEqual(previews, [])
        XCTAssertEqual(model.rows.first, .init(label: "Cabal gets", value: "about 0.73 shares"))
    }

    func testAChainWithNoPriceSaysSoAndCannotBeSent() async throws {
        let service = ReviewService(chainCode: "no_route")
        let model = makeModel(kind: .stock, service: service, bestChain: try Self.bestChain())

        model.choose(.network(.solana))
        await settle()

        XCTAssertEqual(model.chainProblem, "Can't buy GOOGL right now. Try a smaller amount or another stock.")
        XCTAssertFalse(model.canSend)
        await model.send()
        let networks = await service.networks
        XCTAssertEqual(networks, [])
    }

    private static func bestChain() throws -> AssetBestChain {
        try XCTUnwrap(AssetBestChain(.fixture(now: Date())))
    }

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }

    private func makeModel(kind: AssetKind, service: ReviewService, bestChain: AssetBestChain) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(quoteOutAmount: 73_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: ""), preview: preview,
            trade: .buy(symbol: "GOOGL", kind: kind, tokenDecimals: 8), buyName: "Alphabet", bestChain: bestChain)
    }
}
