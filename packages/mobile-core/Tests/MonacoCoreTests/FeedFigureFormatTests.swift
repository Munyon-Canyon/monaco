import MonacoAPI
import MonacoCore
import XCTest

final class FeedFigureFormatTests: XCTestCase {
    private func item(_ kind: String, _ change: (inout Components.Schemas.FeedItem) -> Void = { _ in })
        -> Components.Schemas.FeedItem
    {
        var item = Components.Schemas.FeedItem.samples[0]
        item.kind = kind
        item.cabalName = "Big Ballers"
        change(&item)
        return item
    }

    func testTheSideFollowsTheAction() {
        XCTAssertEqual(FeedFigureFormat.side(item("trade") { $0.action = "sell" }), .sell)
        XCTAssertEqual(FeedFigureFormat.side(item("trade") { $0.action = "buy" }), .buy)
        XCTAssertEqual(FeedFigureFormat.side(item("proposal")), .buy)
    }

    func testTheAmountIsTheUsdcFigureAndAbsentWithoutOne() {
        XCTAssertEqual(FeedFigureFormat.amount(item("proposal") { $0.usdcMicros = 400_000_000 }), "$400.00")
        XCTAssertNil(FeedFigureFormat.amount(item("proposal") { $0.usdcMicros = nil }))
    }

    func testAProposalDetailIsItsShareOfThePot() {
        let proposal = item("proposal") { $0.shareOfPotBps = 200 }
        XCTAssertEqual(FeedFigureFormat.detail(proposal), "2.00% of Big Ballers’s pot")
        XCTAssertNil(FeedFigureFormat.detail(item("proposal") { $0.shareOfPotBps = nil }))
        XCTAssertEqual(
            FeedFigureFormat.detail(item("threshold") { $0.shareOfPotBps = 140 }), "1.40% of Big Ballers’s pot")
    }

    func testATradeDetailIsTheUnitsAndTheAveragePrice() {
        let sold = item("trade") {
            $0.action = "sell"
            $0.tokenAmount = 200_000_000
            $0.tokenDecimals = 8
            $0.priceMicros = 240_000_000
        }
        XCTAssertEqual(FeedFigureFormat.detail(sold), "Sold 2 units · Avg $240.00")
        let bought = item("trade") {
            $0.action = "buy"
            $0.tokenAmount = 2_444_449_999
            $0.tokenDecimals = 8
            $0.priceMicros = 200_000_000
        }
        XCTAssertEqual(FeedFigureFormat.detail(bought), "Bought 24.4445 units · Avg $200.00")
        let single = item("trade") {
            $0.tokenAmount = 100_000_000
            $0.tokenDecimals = 8
        }
        XCTAssertEqual(FeedFigureFormat.detail(single), "Bought 1 unit")
        XCTAssertNil(FeedFigureFormat.detail(item("trade")))
    }

    func testOtherKindsHaveNoDetailFigure() {
        XCTAssertNil(FeedFigureFormat.detail(item("price_move") { $0.shareOfPotBps = 200 }))
        XCTAssertNil(FeedFigureFormat.detail(item("reply") { $0.shareOfPotBps = 200 }))
    }

    func testThePriceChangeIsSignedBasisPoints() {
        XCTAssertEqual(FeedFigureFormat.change(item("price_move") { $0.changeBps = 418 }), "+4.18%")
        XCTAssertEqual(FeedFigureFormat.change(item("price_move") { $0.changeBps = -620 }), "\u{2212}6.20%")
        XCTAssertEqual(FeedFigureFormat.change(item("price_move") { $0.changeBps = 0 }), "0.00%")
        XCTAssertNil(FeedFigureFormat.change(item("price_move") { $0.changeBps = nil }))
    }

    func testTheAuthorReturnIsAPercentHiddenOrAbsent() {
        XCTAssertEqual(FeedFigureFormat.authorReturn(item("trade") { $0.return1mBps = 820 }), .percent("+8.2%"))
        XCTAssertEqual(FeedFigureFormat.authorReturn(item("trade") { $0.return1mBps = -240 }), .percent("\u{2212}2.4%"))
        XCTAssertEqual(
            FeedFigureFormat.authorReturn(
                item("trade") {
                    $0.return1mBps = nil
                    $0.returnPrivate = true
                }), .hidden)
        XCTAssertNil(FeedFigureFormat.authorReturn(item("price_move")))
    }

    func testACommentAuthorReturnMapsTheSameWay() {
        var author = Components.Schemas.Comment.sample("c1").author
        author.return1mBps = 820
        XCTAssertEqual(FeedFigureFormat.authorReturn(author), .percent("+8.2%"))
        author.return1mBps = nil
        author.returnPrivate = true
        XCTAssertEqual(FeedFigureFormat.authorReturn(author), .hidden)
        author.returnPrivate = false
        XCTAssertNil(FeedFigureFormat.authorReturn(author))
    }
}
