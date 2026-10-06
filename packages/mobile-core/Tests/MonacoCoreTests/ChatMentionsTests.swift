import MonacoAPI
import MonacoCore
import XCTest

final class ChatMentionsTests: XCTestCase {
    private func member(_ id: String, _ handle: String?) -> Components.Schemas.CabalMember {
        .init(
            userId: id, handle: handle, displayName: handle ?? "", photoUrl: nil, role: "member", canVote: true,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000))
    }

    private func active(_ text: String) -> String? {
        MentionQuery.active(in: text, cursor: text.count)
    }

    func testActiveAtStartAndAfterSpaceAndPunctuation() {
        XCTAssertEqual(active("@ma"), "ma")
        XCTAssertEqual(active("hi @ma"), "ma")
        XCTAssertEqual(active("hi (@ma"), "ma")
        XCTAssertEqual(active("hi,@ma"), "ma")
        XCTAssertEqual(active("hi\n@ma"), "ma")
        XCTAssertEqual(active("hi @"), "")
    }

    func testNoQueryInsideEmailUrlOrDoubleAt() {
        XCTAssertNil(active("a@b.com"))
        XCTAssertNil(active("a@b"))
        XCTAssertNil(active("https://user@host"))
        XCTAssertNil(active("@@x"))
        XCTAssertNil(active("hi @ma "))
        XCTAssertNil(active("hi"))
    }

    func testLengthLimitAndCase() {
        XCTAssertEqual(active("@" + String(repeating: "a", count: 20)), String(repeating: "a", count: 20))
        XCTAssertNil(active("@" + String(repeating: "a", count: 21)))
        XCTAssertEqual(active("hi @MaYa"), "MaYa")
    }

    func testCursorInTheMiddleUsesTextBeforeIt() {
        XCTAssertEqual(MentionQuery.active(in: "hi @maya there", cursor: 6), "ma")
        XCTAssertNil(MentionQuery.active(in: "hi @maya there", cursor: 12))
        XCTAssertNil(MentionQuery.active(in: "hi", cursor: 9))
    }

    func testMatchesFilterSortAndLeaveOutViewerAndNullHandles() {
        let members = [
            member("v", "mark"), member("1", "Maya"), member("2", "mabel"), member("3", nil), member("4", "zed"),
            member("5", "ma_1"), member("6", "ma_2"), member("7", "ma_3"), member("8", "ma_4"),
        ]
        let found = MentionQuery.matches("MA", members: members, viewerID: "v")
        XCTAssertEqual(found.map(\.userId), ["5", "6", "7", "8", "2"])
        XCTAssertEqual(MentionQuery.matches("z", members: members, viewerID: "v").map(\.userId), ["4"])
        XCTAssertEqual(MentionQuery.matches("", members: members, viewerID: "v").count, 5)
        XCTAssertTrue(MentionQuery.matches("mar", members: members, viewerID: "v").isEmpty)
    }

    func testInsertReplacesQueryAndPlacesCursorAfterSpace() {
        let result = MentionInsertion.insert(handle: "maya", into: "hi @ma", cursor: 6)
        XCTAssertEqual(result.text, "hi @maya ")
        XCTAssertEqual(result.cursor, 9)
        let middle = MentionInsertion.insert(handle: "maya", into: "hi @ma there", cursor: 6)
        XCTAssertEqual(middle.text, "hi @maya  there")
        XCTAssertEqual(middle.cursor, 9)
        let none = MentionInsertion.insert(handle: "maya", into: "hi", cursor: 2)
        XCTAssertEqual(none.text, "hi")
    }

    func testRangesCoverOnlyCurrentMembers() {
        let members = [member("1", "maya")]
        let body = "hey @Maya and @nobody"
        let found = MentionRanges.ranges(in: body, members: members)
        XCTAssertEqual(found.count, 1)
        XCTAssertEqual(found.first.map { String(body[$0.range]) }, "@Maya")
        XCTAssertEqual(found.first?.userID, "1")
    }

    func testRangesFollowTheServerBoundaries() {
        let members = [member("1", "maya")]
        XCTAssertTrue(MentionRanges.ranges(in: "mail a@maya.com", members: members).isEmpty)
        XCTAssertTrue(MentionRanges.ranges(in: "@@maya", members: members).isEmpty)
        XCTAssertEqual(MentionRanges.ranges(in: "@maya @maya", members: members).count, 2)
        XCTAssertTrue(MentionRanges.ranges(in: "@ma", members: [member("2", "ma")]).isEmpty)
    }
}
