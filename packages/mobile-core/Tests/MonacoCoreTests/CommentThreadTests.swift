import MonacoAPI
import XCTest

@testable import MonacoCore

final class CommentThreadTests: XCTestCase {
    private typealias Comment = Components.Schemas.Comment
    private typealias Thread = Components.Schemas.CommentThread

    func testRowsKeepTheServersOrderEachThreadFollowedByItsReplies() {
        let threads = [
            Thread(comment: .sample("c1"), replies: [.sample("c3", parent: "c1"), .sample("c4", parent: "c1")]),
            Thread(comment: .sample("c2"), replies: []),
        ]

        let rows = CommentThreadRow.rows(from: threads)

        XCTAssertEqual(rows.map(\.id), ["c1", "c3", "c4", "c2"])
        XCTAssertEqual(rows.map(\.indentLevel), [0, 1, 1, 0])
        XCTAssertEqual(rows.map(\.isReply), [false, true, true, false])
    }

    func testMonacosOutcomeRowsLeadTheirThreadsAsTheSystemAuthor() {
        let threads = [Thread.nvdaPassedMonacoReply, Thread.tslaFailedMonacoReply]

        let rows = CommentThreadRow.rows(from: threads)

        XCTAssertEqual(rows.map(\.id), ["nvda-monaco", "tsla-monaco"])
        XCTAssertEqual(threads.map(\.comment.author.kind), [.system, .system])
        XCTAssertEqual(threads.map(\.comment.author.displayName), ["Monaco", "Monaco"])
        XCTAssertEqual(
            threads.map(\.comment.body),
            ["Passed and filled. 38 → 39.82 sh NVDA in the treasury.", "Closed without passing. Nothing was sold."])
    }

    func testTheSampleThreadsFlattenIntoOneRowPerComment() {
        let sets = [Thread.samples, Thread.loganAAPLThread, Thread.nvdaThread, Thread.pilatesThread]

        for threads in sets {
            let rows = CommentThreadRow.rows(from: threads)
            XCTAssertEqual(rows.count, threads.reduce(0) { $0 + 1 + $1.replies.count })
            XCTAssertEqual(Set(rows.map(\.id)).count, rows.count)
        }
    }

    func testAReplyToAReplyStaysOneLevelDeepAndNamesWhoItAnswers() {
        let threads = [
            Thread(
                comment: .sample("c1", author: "Ben"),
                replies: [
                    .sample("c2", author: "Ada", parent: "c1"),
                    .sample("c3", author: "Ben", parent: "c1", replyTo: "ada"),
                ])
        ]

        let rows = CommentThreadRow.rows(from: threads)

        XCTAssertEqual(rows.map(\.indentLevel), [0, 1, 1])
        XCTAssertEqual(rows.map(\.replyToHandle), [nil, nil, "ada"])
    }

    func testADeletedRowKeepsItsPlaceShowsBodyDisplayAndOpensNoProfile() {
        let threads = [Thread(comment: .deletedSample("c1"), replies: [.sample("c2", parent: "c1")])]

        let rows = CommentThreadRow.rows(from: threads)

        XCTAssertEqual(rows.map(\.id), ["c1", "c2"])
        XCTAssertTrue(rows[0].isDeleted)
        XCTAssertEqual(rows[0].text, "Comment deleted")
        XCTAssertFalse(rows[0].opensAuthorProfile)
        XCTAssertFalse(rows[1].isDeleted)
        XCTAssertTrue(rows[1].opensAuthorProfile)
        XCTAssertEqual(rows[1].authorID, "author-maya")
    }

    func testAGifReplyWithNoTextIsNotADeletedRow() {
        let row = CommentThreadRow(comment: .gifSample("g1", author: "Christelle", parent: "c1"), isReply: true)

        XCTAssertNil(row.comment.body)
        XCTAssertFalse(row.isDeleted)
        XCTAssertTrue(row.opensAuthorProfile)
    }

    func testAnAuthorWithNoNameShowsTheHandleThenAPlaceholder() {
        var comment = Comment.sample("c1")
        comment.author.displayName = ""
        XCTAssertEqual(CommentThreadRow(comment: comment, isReply: false).authorName, "maya")
        comment.author.handle = nil
        XCTAssertEqual(CommentThreadRow(comment: comment, isReply: false).authorName, CommentsCopy.unknownAuthor)
    }

    func testNoThreadsMakeNoRows() {
        XCTAssertEqual(CommentThreadRow.rows(from: []), [])
    }

    func testDraftWhitespaceOnlyIsEmpty() {
        XCTAssertEqual(CommentDraft(text: "  \n\t "), .empty)
        XCTAssertNil(CommentDraft(text: " ").body)
    }

    func testDraftTrimsSurroundingWhitespace() {
        XCTAssertEqual(CommentDraft(text: "  In on Apple.\n").body, "In on Apple.")
    }

    func testDraftCountsScalarsLikeTheBackend() {
        let atLimit = String(repeating: "é", count: CommentDraft.maxScalars)

        XCTAssertNotNil(CommentDraft(text: atLimit).body)
        XCTAssertEqual(CommentDraft(text: atLimit + "é"), .tooLong(overBy: 1))
    }
}
