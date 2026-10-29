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

    func testAnAnswerIsFoldedUnderItsRootAndTheRootCountsIt() {
        let threads = [
            Thread(
                comment: .sample("c1", author: "Ben"),
                replies: [.sample("c2", author: "Ada", parent: "c1"), .sample("c3", author: "Ben", parent: "c1")]),
            Thread(comment: .sample("c4"), replies: []),
        ]

        let rows = CommentThreadRow.rows(from: threads)

        XCTAssertEqual(rows.map(\.foldedCount), [2, 0, 0, 0])
        XCTAssertEqual(rows.map(\.isFolded), [false, true, true, false])
        XCTAssertEqual(rows.map(\.indentLevel), [0, 1, 1, 0])
    }

    func testAQuoteNamesWhoItAnswersAndWhereToJump() throws {
        let quote = Components.Schemas.CommentQuote.sample(
            "aapl-logan", root: "aapl-nora", author: "Logan", excerpt: "Took the gain.", folded: true)
        let row = CommentThreadRow(
            comment: .sample("c9", author: "Christelle", quote: quote), isReply: false)

        let shown = try XCTUnwrap(row.quote)

        XCTAssertEqual(shown.name, "Logan")
        XCTAssertEqual(shown.excerpt, "Took the gain.")
        XCTAssertEqual(shown.targetID, "aapl-logan")
        XCTAssertEqual(shown.targetRootID, "aapl-nora")
        XCTAssertTrue(shown.targetIsFolded)
        XCTAssertTrue(shown.canJump)
    }

    func testAQuoteOfADeletedReplyReadsDeletedReplyAndCannotJump() throws {
        let quote = Components.Schemas.CommentQuote.sample("c1", author: "Ada", excerpt: nil, deleted: true)
        let row = CommentThreadRow(comment: .sample("c2", quote: quote), isReply: false)

        let shown = try XCTUnwrap(row.quote)

        XCTAssertEqual(shown.excerpt, CommentsCopy.quoteDeleted)
        XCTAssertEqual(shown.excerpt, "Deleted reply")
        XCTAssertFalse(shown.canJump)
    }

    func testAQuoteOfAGifOnlyReplyReadsGif() throws {
        let quote = Components.Schemas.CommentQuote.sample(
            "c1", author: "Christelle", excerpt: nil, hasAttachment: true)
        let row = CommentThreadRow(comment: .sample("c2", quote: quote), isReply: false)

        XCTAssertEqual(row.quote?.excerpt, "GIF")
    }

    func testAGifOnlyReplyIsNotDeletedAndKeepsItsAttachment() {
        let row = CommentThreadRow(comment: .gifSample("g1", author: "Christelle"), isReply: false)

        XCTAssertFalse(row.isDeleted)
        XCTAssertTrue(row.opensAuthorProfile)
        guard case .gif = row.attachment else { return XCTFail("no gif attachment") }
    }

    func testASystemRowMapsIsSystemAndTakesNoActions() {
        let row = CommentThreadRow.rows(from: [Thread.nvdaPassedMonacoReply])[0]

        XCTAssertTrue(row.isSystem)
        XCTAssertFalse(row.canReact)
        XCTAssertFalse(row.opensAuthorProfile)
        XCTAssertNil(row.quote)
        XCTAssertEqual(row.authorName, "Monaco")
    }

    func testARowReadsNameAgeQuotedNameBodyAndReactionCount() {
        let quote = Components.Schemas.CommentQuote.sample("c1", author: "Logan", excerpt: "Took the gain.")
        let reactions = [Components.Schemas.Reaction(emoji: "😂", count: 3, mine: false)]
        let row = CommentThreadRow(
            comment: .sample("c2", author: "Sam", body: "Same.", minutesAgo: 2, quote: quote, reactions: reactions),
            isReply: false)

        XCTAssertEqual(row.spoken(age: "2m"), "Sam, 2m, replying to Logan: Same.. 3 reactions")
    }

    func testTheCopyReadsRepliesAndViewNReplies() {
        XCTAssertEqual(CommentsCopy.title, "Replies")
        XCTAssertEqual(CommentsCopy.viewReplies(1), "View 1 reply")
        XCTAssertEqual(CommentsCopy.viewReplies(3), "View 3 replies")
        XCTAssertEqual(CommentsCopy.hideReplies, "Hide replies")
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
