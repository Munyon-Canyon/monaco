import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

#if DEBUG
@MainActor
final class CommentAttachmentTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport

    private func make(_ script: [StubTransport.Reply]) async throws -> (CommentsModel, StubTransport) {
        let opening = [try Support.page(Thread.samples), try Support.detail(canComment: true)]
        let transport = StubTransport(scripted: opening + script)
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: "item-1", api: Support.api(transport)), hints: FakeHintStream(),
            clock: TestClock())
        await model.load()
        return (model, transport)
    }

    func testPickingFillsTheDraftWithNoToast() async throws {
        let (model, _) = try await make([])

        let toast = model.pick(.thesis(start: 0, end: 1))

        XCTAssertNil(toast)
        XCTAssertEqual(model.attachmentDraft, .thesis(start: 0, end: 1))
    }

    func testPickingASecondReplacesTheFirstWithAToast() async throws {
        let (model, _) = try await make([])
        _ = model.pick(.thesis(start: 0, end: 1))

        let toast = model.pick(.thesis(start: 2, end: 3))

        XCTAssertEqual(toast, "Replaced the quote")
        XCTAssertEqual(model.attachmentDraft, .thesis(start: 2, end: 3))
    }

    func testRemovingClearsTheDraft() async throws {
        let (model, _) = try await make([])
        _ = model.pick(.thesis(start: 0, end: 1))

        model.removeAttachment()

        XCTAssertNil(model.attachmentDraft)
    }

    func testAFailedSendKeepsTheTextAndTheDraft() async throws {
        let (model, _) = try await make([.json(.internalServerError, "{}")])
        model.edit("hello")
        _ = model.pick(.thesis(start: 0, end: 1))

        let ok = await model.post(CommentDraft(text: model.draft))

        XCTAssertFalse(ok)
        XCTAssertEqual(model.draft, "hello")
        XCTAssertEqual(model.attachmentDraft, .thesis(start: 0, end: 1))
    }

    func testASuccessfulSendSendsTheAttachmentAndClearsTheDraft() async throws {
        let posted = Components.Schemas.Comment.sample("c5", mine: true)
        let (model, transport) = try await make([
            .json(.created, try Support.encode(posted)), try Support.page(Thread.samples),
        ])
        _ = model.pick(.thesis(start: 0, end: 1))

        let ok = await model.post(CommentDraft(text: ""))

        XCTAssertTrue(ok)
        XCTAssertNil(model.attachmentDraft)
        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let index = try XCTUnwrap(sent.firstIndex { $0.method == .post })
        let json = try XCTUnwrap(
            JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[index])) as? [String: Any])
        let attachment = try XCTUnwrap(json["attachment"] as? [String: Any])
        XCTAssertEqual(attachment["kind"] as? String, "thesis_quote")
    }

    func testRuneOffsetsCountEmojiAsOneRune() {
        let text = "a👍🏽b é"
        let start = text.index(text.startIndex, offsetBy: 1)
        let end = text.index(text.startIndex, offsetBy: 2)

        XCTAssertEqual(ThesisQuote.runeCount(text), 6)
        XCTAssertEqual(ThesisQuote.selection(start..<end, in: text), .thesis(start: 1, end: 3))
        XCTAssertEqual(ThesisQuote.all(text), .thesis(start: 0, end: 6))
    }

    func testTheThesisDraftMapsToTheInput() async throws {
        let input = CommentAttachmentDraft.thesis(start: 3, end: 9).input

        guard case .thesisQuote(let quote) = input else { return XCTFail("not a thesis quote") }
        XCTAssertEqual(quote.start, 3)
        XCTAssertEqual(quote.end, 9)
        XCTAssertEqual(CommentAttachmentDraft.thesis(start: 0, end: 1).kindName, "quote")
    }

    func testOnlyANonBlankThesisIsQuotable() {
        XCTAssertFalse(ThesisQuote.isQuotable(nil))
        XCTAssertFalse(ThesisQuote.isQuotable("  \n"))
        XCTAssertTrue(ThesisQuote.isQuotable("Buy more"))
    }

    func testAnExplicitAttachmentWinsOverTheDraft() async throws {
        let (model, _) = try await make([.json(.internalServerError, "{}")])
        _ = model.pick(.thesis(start: 0, end: 1))

        let ok = await model.post(
            CommentDraft(text: ""), attachment: .thesisQuote(.init(kind: .thesisQuote, start: 1, end: 2)))

        XCTAssertFalse(ok)
    }
}
#endif
