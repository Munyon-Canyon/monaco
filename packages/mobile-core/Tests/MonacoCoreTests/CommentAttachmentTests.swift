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

        let toast = model.pick(.sample(title: "One"))

        XCTAssertNil(toast)
        XCTAssertEqual(model.attachmentDraft, .sample(title: "One"))
    }

    func testPickingASecondReplacesTheFirstWithAToast() async throws {
        let (model, _) = try await make([])
        _ = model.pick(.sample(title: "One"))

        let toast = model.pick(.sample(title: "Two"))

        XCTAssertEqual(toast, "Replaced the sample")
        XCTAssertEqual(model.attachmentDraft, .sample(title: "Two"))
    }

    func testRemovingClearsTheDraft() async throws {
        let (model, _) = try await make([])
        _ = model.pick(.sample(title: "One"))

        model.removeAttachment()

        XCTAssertNil(model.attachmentDraft)
    }

    func testAFailedSendKeepsTheTextAndTheDraft() async throws {
        let (model, _) = try await make([.json(.internalServerError, "{}")])
        model.edit("hello")
        _ = model.pick(.sample(title: "One"))

        let ok = await model.post(CommentDraft(text: model.draft))

        XCTAssertFalse(ok)
        XCTAssertEqual(model.draft, "hello")
        XCTAssertEqual(model.attachmentDraft, .sample(title: "One"))
    }

    func testASuccessfulSendSendsTheAttachmentAndClearsTheDraft() async throws {
        let posted = Components.Schemas.Comment.sample("c5", mine: true)
        let (model, transport) = try await make([
            .json(.created, try Support.encode(posted)), try Support.page(Thread.samples),
        ])
        _ = model.pick(.sample(title: "One"))

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

    func testAnExplicitAttachmentWinsOverTheDraft() async throws {
        let (model, _) = try await make([.json(.internalServerError, "{}")])
        _ = model.pick(.sample(title: "One"))

        let ok = await model.post(
            CommentDraft(text: ""), attachment: .thesisQuote(.init(kind: .thesisQuote, start: 1, end: 2)))

        XCTAssertFalse(ok)
    }
}
#endif
