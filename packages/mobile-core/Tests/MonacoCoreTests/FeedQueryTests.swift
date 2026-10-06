import MonacoAPI
import MonacoCore
import XCTest

final class FeedQueryTests: XCTestCase {
    func testEachChipSendsItsKinds() {
        let expected: [FeedChip: String?] = [
            .all: nil,
            .proposals: "proposal",
            .trades: "trade",
            .priceMoves: "price_move",
            .cabals: "cabal_created,member_joined",
        ]
        for chip in FeedChip.allCases {
            let params = FeedQuery(chip: chip).parameters(cursor: nil, limit: 30)
            XCTAssertEqual(params.kind, expected[chip] ?? nil, chip.title)
        }
        XCTAssertEqual(FeedChip.allCases.map(\.title), ["All", "Proposals", "Trades", "Price moves", "Cabals"])
    }

    func testEachScopeSendsItsWireValue() {
        XCTAssertEqual(FeedQuery(scope: .everyone).parameters(cursor: nil, limit: 30).scope, .all)
        XCTAssertEqual(FeedQuery(scope: .mine).parameters(cursor: nil, limit: 30).scope, .mine)
        XCTAssertEqual(FeedQuery(scope: .following).parameters(cursor: nil, limit: 30).scope, .following)
        XCTAssertEqual(FeedScope.allCases.map(\.title), ["Everyone", "My cabals", "Following"])
    }

    func testSearchIsTrimmedAndABlankOneSendsNoQ() {
        XCTAssertEqual(FeedQuery(search: "  earnings \n").parameters(cursor: nil, limit: 30).q, "earnings")
        XCTAssertNil(FeedQuery(search: "   ").parameters(cursor: nil, limit: 30).q)
        XCTAssertNil(FeedQuery().parameters(cursor: nil, limit: 30).q)
    }

    func testCursorAndLimitPassThrough() {
        let params = FeedQuery().parameters(cursor: "c2", limit: 30)
        XCTAssertEqual(params.cursor, "c2")
        XCTAssertEqual(params.limit, 30)
    }
}
