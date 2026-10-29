import MonacoAPI
import XCTest

final class CommentSampleFixtureTests: XCTestCase {
    func testTheGifSampleCarriesAGifAttachmentAndNoBody() {
        let comment = Components.Schemas.Comment.gifSample("g1", author: "Christelle")
        guard case .gif(let gif) = comment.attachment else { return XCTFail("no gif attachment") }
        XCTAssertEqual(gif.kind, .gif)
        XCTAssertNil(comment.body)
        XCTAssertEqual(comment.bodyDisplay, "GIF")
    }

    func testTheThesisQuoteSampleCarriesItsText() {
        let comment = Components.Schemas.Comment.thesisQuoteSample("t1", author: "Nora", text: "Cheap on cash flow.")
        guard case .thesisQuote(let quote) = comment.attachment else { return XCTFail("no thesis quote") }
        XCTAssertEqual(quote.text, "Cheap on cash flow.")
        XCTAssertNil(comment.body)
    }

    func testTheStockSampleCarriesAStockAttachment() {
        let comment = Components.Schemas.Comment.stockSample("s1", author: "Sam")
        guard case .stock(let stock) = comment.attachment else { return XCTFail("no stock attachment") }
        XCTAssertEqual(stock.symbol, "AAPL")
        XCTAssertNil(comment.body)
    }

    func testTheDeletedSampleHasNoBody() {
        let comment = Components.Schemas.Comment.deletedSample("d1", parent: "c1")
        XCTAssertNil(comment.body)
        XCTAssertEqual(comment.bodyDisplay, "Comment deleted")
        XCTAssertEqual(comment.parentCommentId, "c1")
    }

    func testTheQuoteAndPollSamplesCarryTheirArguments() {
        let quote = Components.Schemas.CommentQuote.sample(
            "c2", root: "c1", author: "Alex", excerpt: "Agreed", folded: true, deleted: true, hasAttachment: true)
        XCTAssertEqual(quote.rootCommentId, "c1")
        XCTAssertTrue(quote.isFolded && quote.isDeleted && quote.hasAttachment)
        let poll = Components.Schemas.PollAttachment.sample(question: "Trim?", closesInHours: 2, myPosition: nil)
        XCTAssertEqual(poll.question, "Trim?")
        XCTAssertEqual(poll.total, 5)
        XCTAssertNil(poll.myPosition)
    }
}
