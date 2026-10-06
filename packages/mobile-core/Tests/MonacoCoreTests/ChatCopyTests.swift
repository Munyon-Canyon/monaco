import Foundation
import XCTest

@testable import MonacoCore

final class ChatCopyTests: XCTestCase {
    func testTitle_usesCabalNameWithFallback() {
        XCTAssertEqual(GroupChatCopy.title(groupName: "Weekend investors"), "Weekend investors")
        XCTAssertEqual(GroupChatCopy.title(groupName: "  "), "Cabal chat")
        XCTAssertEqual(GroupChatCopy.title(groupName: nil), "Cabal chat")
    }

    func testNewMessagesPill_singularAndPlural() {
        XCTAssertEqual(GroupChatCopy.newMessagesPill(count: 1), "1 new message")
        XCTAssertEqual(GroupChatCopy.newMessagesPill(count: 4), "4 new messages")
    }

    func testRepliesAreSingularAndPlural() {
        XCTAssertEqual(GroupChatCopy.replies(1), "1 reply")
        XCTAssertEqual(GroupChatCopy.replies(3), "3 replies")
    }

    func testTheMemberFacingSentencesAreExact() {
        XCTAssertEqual(GroupChatCopy.loadFailure, "Couldn't load messages.")
        XCTAssertEqual(GroupChatCopy.closed, "You're no longer in this cabal, so its chat is closed to you.")
        XCTAssertEqual(GroupChatCopy.notSent, "Not sent · Retry")
    }

    func testChatCopy_passesMainFlowAudit() {
        XCTAssertTrue(
            MainFlowCopyAudit.stringsAreClean([
                GroupChatCopy.title,
                GroupChatCopy.title(groupName: "Weekend investors"),
                GroupChatCopy.emptyState,
                GroupChatCopy.composerPlaceholder,
                GroupChatCopy.loadEarlier,
                GroupChatCopy.loadFailure,
                GroupChatCopy.closed,
                GroupChatCopy.notSent,
                GroupChatCopy.deleted,
                GroupChatCopy.newMessagesPill(count: 3),
                GroupChatCopy.replies(2),
            ]))
    }
}
