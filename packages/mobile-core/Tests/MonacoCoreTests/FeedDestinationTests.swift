import MonacoAPI
import MonacoCore
import XCTest

final class FeedDestinationTests: XCTestCase {
    private let cabal = "00000000-0000-7000-8000-0000000fc001"
    private let ref = "00000000-0000-7000-8000-0000000fa001"

    func testEachKindOpensItsScreen() {
        XCTAssertEqual(FeedDestination(item("proposal")), .proposal(id: ref))
        XCTAssertEqual(FeedDestination(item("trade")), .transaction(cabalID: cabal, transactionID: ref))
        XCTAssertEqual(FeedDestination(item("price_move", symbol: "TSLAx")), .asset(symbol: "TSLAx"))
        XCTAssertEqual(FeedDestination(item("cabal_created")), .cabal(id: cabal))
        XCTAssertEqual(FeedDestination(item("member_joined")), .cabal(id: cabal))
    }

    func testAnItemMissingWhatItNeedsHasNoDestination() {
        XCTAssertNil(FeedDestination(item("trade", cabal: nil)))
        XCTAssertNil(FeedDestination(item("price_move", cabal: nil, symbol: nil)))
        XCTAssertNil(FeedDestination(item("cabal_created", cabal: nil)))
        XCTAssertNil(FeedDestination(item("member_joined", cabal: nil)))
        XCTAssertNil(FeedDestination(item("something_new")))
    }

    func testEverySampleOpensSomewhere() {
        XCTAssertTrue(Components.Schemas.FeedItem.samples.allSatisfy { FeedDestination($0) != nil })
    }

    private func item(_ kind: String, cabal: String? = "00000000-0000-7000-8000-0000000fc001", symbol: String? = nil)
        -> Components.Schemas.FeedItem
    {
        Components.Schemas.FeedItem(
            id: "00000000-0000-7000-8000-0000000fe001", kind: kind, refType: "x", refId: ref,
            cabalId: cabal, cabalName: nil, actorId: nil, actorName: nil, assetId: nil, symbol: symbol, title: "t",
            detail: nil, body: nil,
            status: nil, tone: "neutral", commentCount: 0, createdAt: Date(), updatedAt: Date())
    }
}
