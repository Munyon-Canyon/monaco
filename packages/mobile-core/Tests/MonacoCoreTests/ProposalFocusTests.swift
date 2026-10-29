import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProposalFocusTests: XCTestCase {
    func testNoneAndRepliesNeedNoRows() {
        XCTAssertEqual(ProposalFocus.none.resolve(in: [], hasMore: false), .nothing)
        XCTAssertEqual(ProposalFocus.replies.resolve(in: [], hasMore: false), .replySheet)
    }

    func testAMissingCommentPagesForwardThenIsNotFound() {
        XCTAssertEqual(ProposalFocus.anchor(commentID: "x").resolve(in: [], hasMore: true), .needsMore)
        XCTAssertEqual(ProposalFocus.reply(commentID: "x").resolve(in: [], hasMore: false), .notFound)
    }
}
