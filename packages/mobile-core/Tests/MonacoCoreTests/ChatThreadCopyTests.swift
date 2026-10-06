import Foundation
import XCTest

@testable import MonacoCore

final class ChatThreadCopyTests: XCTestCase {
    private let now = ChatFixtures.epoch

    func testOneReplyReadsSingular() {
        let text = ChatThreadCopy.summary(
            replyCount: 1, lastReplyAt: now.addingTimeInterval(-120), now: now)
        XCTAssertEqual(text, "1 reply · last reply 2m ago")
    }

    func testManyRepliesReadPlural() {
        let text = ChatThreadCopy.summary(
            replyCount: 12, lastReplyAt: now.addingTimeInterval(-3 * 3600), now: now)
        XCTAssertEqual(text, "12 replies · last reply 3h ago")
    }

    func testAReplyUnderAMinuteAgoReadsJustNow() {
        let text = ChatThreadCopy.summary(replyCount: 1, lastReplyAt: now.addingTimeInterval(-20), now: now)
        XCTAssertEqual(text, "1 reply · last reply just now")
    }

    func testAReplyOlderThanADayNamesTheDate() {
        let calendar = Calendar(identifier: .gregorian)
        let text = ChatThreadCopy.summary(
            replyCount: 2, lastReplyAt: now.addingTimeInterval(-3 * 86_400), now: now, calendar: calendar)
        XCTAssertTrue(text.hasPrefix("2 replies · last reply on "), text)
        XCTAssertFalse(text.hasSuffix("ago"))
    }

    func testNoLastReplyShowsOnlyTheCount() {
        XCTAssertEqual(ChatThreadCopy.summary(replyCount: 3, lastReplyAt: nil, now: now), "3 replies")
    }

    func testTheHeaderQuotesTheFirst60CharactersOfTheParent() {
        let long = String(repeating: "a", count: 80)
        XCTAssertEqual(ChatThreadCopy.header(parentBody: "idea"), "replied to a thread: idea")
        XCTAssertEqual(
            ChatThreadCopy.header(parentBody: long), "replied to a thread: \(String(repeating: "a", count: 60))")
        XCTAssertEqual(ChatThreadCopy.header(parentBody: nil), "replied to a thread")
    }

    func testNewCopyNeverSaysGroup() {
        let strings = [
            ChatThreadCopy.deleteTitle, ChatThreadCopy.deleteMessage, ChatThreadCopy.composerPlaceholder,
            ChatThreadCopy.alsoInChannel, ChatThreadCopy.noReplies, ChatThreadCopy.loadFailure,
        ]
        XCTAssertTrue(strings.allSatisfy { !$0.lowercased().contains("group") })
        XCTAssertEqual(ChatThreadCopy.deleteMessage, "It's removed for everyone in the cabal.")
    }
}
