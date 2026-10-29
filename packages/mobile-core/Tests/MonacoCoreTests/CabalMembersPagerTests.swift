import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

private func member(_ id: String) -> CabalMemberRow {
    CabalMemberRow(
        Components.Schemas.CabalMember(
            userId: id, handle: nil, displayName: "Member \(id)", photoUrl: nil, role: "member", canVote: true,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000)))
}

@MainActor
final class CabalMembersPagerTests: XCTestCase {

    func testTwoPagesAppendWithoutRepeatingAUserID() async {
        let pager = CabalMembersPager { cursor in
            if cursor == nil { return ((0..<50).map { member("u\($0)") }, "next") }
            return ((50..<70).map { member("u\($0)") }, nil)
        }

        await pager.loadFirst()
        XCTAssertEqual(pager.rows.count, 50)
        XCTAssertTrue(pager.hasMore)

        await pager.loadMore()

        let ids = pager.rows.map(\.id)
        XCTAssertEqual(ids.count, 70)
        XCTAssertEqual(Set(ids).count, 70)
        XCTAssertEqual(ids.first, "u0")
        XCTAssertEqual(ids.last, "u69")
        XCTAssertFalse(pager.hasMore)
    }

    func testAPageThatRepeatsAMemberAddsItOnce() async {
        let pager = CabalMembersPager { cursor in
            if cursor == nil { return ([member("a"), member("b")], "next") }
            return ([member("b"), member("c")], nil)
        }

        await pager.loadFirst()
        await pager.loadMore()

        XCTAssertEqual(pager.rows.map(\.id), ["a", "b", "c"])
    }

    func testLoadMoreAfterARowOtherThanTheLastFetchesNothing() async {
        let pager = CabalMembersPager { cursor in
            if cursor == nil { return ([member("a"), member("b")], "next") }
            return ([member("c")], nil)
        }
        await pager.loadFirst()

        await pager.loadMore(after: member("a"))
        XCTAssertEqual(pager.rows.count, 2)

        await pager.loadMore(after: member("b"))
        XCTAssertEqual(pager.rows.map(\.id), ["a", "b", "c"])
    }
}
