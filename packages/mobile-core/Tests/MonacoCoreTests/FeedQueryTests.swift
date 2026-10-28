import MonacoAPI
import MonacoCore
import XCTest

final class FeedQueryTests: XCTestCase {
    func testTheDefaultScopeIsTheNetwork() {
        XCTAssertEqual(FeedQuery().scope, .network)
        XCTAssertEqual(FeedQuery().parameters(cursor: nil, limit: 30).scope, .network)
    }

    func testEachScopeSendsItsWireValue() {
        let expected: [FeedScope: Operations.GetFeed.Input.Query.ScopePayload] = [
            .network: .network, .mine: .mine, .following: .following, .everyone: .all, .others: .others,
        ]
        XCTAssertEqual(Set(FeedScope.allCases), Set(expected.keys))
        for (scope, wire) in expected {
            XCTAssertEqual(FeedQuery(scope: scope).parameters(cursor: nil, limit: 30).scope, wire, "\(scope)")
        }
    }

    func testTheMenuListsTheFourScopesInOrder() {
        XCTAssertEqual(FeedScope.menu, [.network, .mine, .following, .everyone])
        XCTAssertEqual(
            FeedScope.menu.map(\.title), ["Your cabals and friends", "Your cabals", "Friends", "Everyone"])
    }

    func testTheKindListNeverContainsPriceMoveInAnyScope() {
        for scope in FeedScope.allCases {
            let kinds = FeedQuery(scope: scope).parameters(cursor: nil, limit: 30).kind?.split(separator: ",")
            XCTAssertEqual(
                kinds.map(Set.init),
                ["proposal", "result", "reply", "member_joined", "cabal_created", "trade", "threshold"], "\(scope)")
            XCTAssertFalse(kinds?.contains("price_move") ?? true)
        }
    }

    func testCursorAndLimitPassThrough() {
        let params = FeedQuery().parameters(cursor: "c2", limit: 30)
        XCTAssertEqual(params.cursor, "c2")
        XCTAssertEqual(params.limit, 30)
        XCTAssertNil(params.q)
    }
}
