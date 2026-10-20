import MonacoAPI
import MonacoCore
import XCTest

final class FeedQueryTests: XCTestCase {
    func testEachGroupSendsItsKinds() {
        let expected: [FeedKindGroup: String] = [
            .proposals: "proposal",
            .completedTrades: "trade",
            .priceMoves: "price_move",
            .repliesAndVotes: "reply",
            .proposalOutcomes: "threshold",
            .cabalActivity: "cabal_created,member_joined",
        ]
        for group in FeedKindGroup.allCases {
            let params = FeedQuery(kinds: [group]).parameters(cursor: nil, limit: 30)
            XCTAssertEqual(params.kind, expected[group], group.title)
        }
        XCTAssertEqual(
            FeedKindGroup.allCases.map(\.title),
            [
                "Proposals", "Completed trades", "Price moves", "Replies & votes", "Proposal outcomes",
                "Cabal activity",
            ])
    }

    func testSeveralGroupsSendTheSortedUnionAndNoneSendsNoKind() {
        let union = FeedQuery(kinds: [.cabalActivity, .proposals, .repliesAndVotes])
        XCTAssertEqual(union.parameters(cursor: nil, limit: 30).kind, "cabal_created,member_joined,proposal,reply")
        XCTAssertNil(FeedQuery().parameters(cursor: nil, limit: 30).kind)
        XCTAssertNil(FeedQuery(kinds: []).parameters(cursor: nil, limit: 30).kind)
    }

    func testAllIsOnOnlyWhenNoGroupIsChosen() {
        XCTAssertTrue(FeedQuery().showsAll)
        XCTAssertTrue(FeedQuery(scope: .following).showsAll)
        XCTAssertFalse(FeedQuery(kinds: [.proposals]).showsAll)
        XCTAssertFalse(FeedQuery(kinds: Set(FeedKindGroup.allCases)).showsAll)
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
