import Foundation
import XCTest

@testable import MonacoCore

final class ChatThreadCopyTests: XCTestCase {
    private let now = ChatFixtures.epoch

    func testTheThreadLineReadsCountAgeAndNames() {
        let text = ChatThreadCopy.lineSpoken(
            replyCount: 6, lastReplyAt: now.addingTimeInterval(-60), names: ["Sam", "Kai", "Logan"], now: now)
        XCTAssertEqual(text, "6 replies, last 1 minute ago, from Sam, Kai and Logan. Opens the thread.")
    }

    func testTheThreadLineSkipsWhatItDoesNotKnow() {
        XCTAssertEqual(
            ChatThreadCopy.lineSpoken(replyCount: 1, lastReplyAt: nil, names: [], now: now),
            "1 reply. Opens the thread.")
        XCTAssertEqual(
            ChatThreadCopy.lineSpoken(
                replyCount: 2, lastReplyAt: now.addingTimeInterval(-20), names: ["Sam"], now: now),
            "2 replies, last just now, from Sam. Opens the thread.")
    }

    func testTheSpokenAgeUsesWords() {
        let calendar = Calendar(identifier: .gregorian)
        XCTAssertEqual(
            ChatThreadCopy.spokenAge(of: now.addingTimeInterval(-7_200), now: now, calendar: calendar), "2 hours ago")
        XCTAssertEqual(
            ChatThreadCopy.spokenAge(of: now.addingTimeInterval(-3_600), now: now, calendar: calendar), "1 hour ago")
        XCTAssertEqual(
            ChatThreadCopy.spokenAge(of: now.addingTimeInterval(-300), now: now, calendar: calendar), "5 minutes ago")
        XCTAssertTrue(
            ChatThreadCopy.spokenAge(of: now.addingTimeInterval(-3 * 86_400), now: now, calendar: calendar).hasPrefix(
                "on "))
    }

    func testTheLineAgeIsTheShortLabel() {
        XCTAssertEqual(ChatThreadCopy.lineAge(of: now.addingTimeInterval(-60), now: now), "1m")
        XCTAssertNil(ChatThreadCopy.lineAge(of: nil, now: now))
    }

    func testNamesJoinWithAnd() {
        XCTAssertEqual(ChatThreadCopy.spokenNames(["Sam", "Kai", "Logan"]), "Sam, Kai and Logan")
        XCTAssertEqual(ChatThreadCopy.spokenNames(["Sam", "Kai"]), "Sam and Kai")
        XCTAssertEqual(ChatThreadCopy.spokenNames(["Sam", ""]), "Sam")
        XCTAssertEqual(ChatThreadCopy.spokenNames([]), "")
    }

    func testAProposalReplyHeaderNamesTheCard() {
        XCTAssertEqual(
            ChatThreadCopy.proposalHeader(headline: "Buy NVDA \u{00B7} $400"), "replied to Buy NVDA \u{00B7} $400")
        XCTAssertEqual(ChatThreadCopy.proposalHeader(headline: nil), "replied to a proposal")
        XCTAssertEqual(ChatThreadCopy.proposalHeader(headline: "  "), "replied to a proposal")
    }

    func testTheChannelToggleReadsAlsoSendToChat() {
        XCTAssertEqual(ChatThreadCopy.alsoInChannel, "Also send to chat")
    }

    func testTheHeaderQuotesTheFirst60CharactersOfTheParent() {
        let long = String(repeating: "a", count: 80)
        XCTAssertEqual(ChatThreadCopy.header(parentBody: "idea"), "Replied to a thread: idea")
        XCTAssertEqual(
            ChatThreadCopy.header(parentBody: long), "Replied to a thread: \(String(repeating: "a", count: 60))…")
        XCTAssertEqual(
            ChatThreadCopy.header(parentBody: String(repeating: "a", count: 61)),
            "Replied to a thread: \(String(repeating: "a", count: 60))…")
        XCTAssertEqual(
            ChatThreadCopy.header(parentBody: String(repeating: "a", count: 60)),
            "Replied to a thread: \(String(repeating: "a", count: 60))")
        XCTAssertEqual(ChatThreadCopy.header(parentBody: nil), "Replied to a thread")
    }

    func testADeletedParentHasItsOwnLine() {
        XCTAssertEqual(ChatThreadCopy.parentDeleted, "This message was deleted.")
    }

    func testNewCopyNeverSaysGroup() {
        let strings = [
            ChatThreadCopy.deleteTitle, ChatThreadCopy.deleteMessage, ChatThreadCopy.composerPlaceholder,
            ChatThreadCopy.alsoInChannel, ChatThreadCopy.noReplies, ChatThreadCopy.loadFailure,
        ]
        XCTAssertTrue(strings.allSatisfy { !$0.lowercased().contains("group") })
        XCTAssertEqual(ChatThreadCopy.deleteMessage, "It's removed for everyone in the cabal.")
    }

    func testAProposalThreadRepliesToTheProposersFirstName() {
        XCTAssertEqual(ChatThreadCopy.composerPlaceholder(proposer: "Mia Chen"), "Reply to Mia")
        XCTAssertEqual(ChatThreadCopy.composerPlaceholder(proposer: "Mia"), "Reply to Mia")
        XCTAssertEqual(ChatThreadCopy.composerPlaceholder(proposer: ""), "Reply in thread")
        XCTAssertEqual(ChatThreadCopy.composerPlaceholder(proposer: nil), "Reply in thread")
    }

    func testTheQuoteAndProposalButtonCopy() {
        XCTAssertEqual(ChatThreadCopy.proposalButton, "Proposal")
        XCTAssertEqual(ChatThreadCopy.quoteDeleted, "Reply deleted")
        XCTAssertEqual(
            ChatThreadCopy.quoteSpoken(name: "Logan", excerpt: "44% NVDA"), "Replying to Logan: 44% NVDA")
    }
}
