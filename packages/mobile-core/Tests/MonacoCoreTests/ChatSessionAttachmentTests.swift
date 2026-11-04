import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatSessionAttachmentTests: XCTestCase {
    private typealias Fixtures = ChatFixtures
    private typealias Schemas = Components.Schemas

    private static let proposalID = "01890a5d-ac96-774b-bcce-b302099a8057"
    private static let assetID = "01890a5d-ac96-774b-bcce-b302099a8070"

    private static func media(_ name: String) -> Schemas.GifMedia {
        .init(url: "https://media.giphy.com/media/abc123/\(name).gif", width: 200, height: 150)
    }

    private static let gif = Schemas.GifInput(
        id: "abc123", title: "happy dance", still: media("still"), preview: media("preview"),
        original: media("giphy"))

    private static func gifAttachment() -> Schemas.CommentAttachment {
        .gif(
            .init(
                kind: .gif, id: "abc123", title: "happy dance", still: media("still"), preview: media("preview"),
                original: media("giphy")))
    }

    private func sentBody(_ transport: StubTransport, at index: Int) async throws -> [String: Any] {
        let bodies = await transport.sentBodies
        let data = try XCTUnwrap(bodies[index])
        return try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }

    private func opened(_ replies: [StubTransport.Reply], keys: [String] = ["key-1", "key-2"]) async throws
        -> (ChatSession, StubTransport)
    {
        let transport = StubTransport(scripted: [try Fixtures.page([])] + replies)
        let session = Fixtures.session(transport, keys: keys)
        await session.open()
        return (session, transport)
    }

    private func stored(_ id: String, body: String, attachment: Schemas.CommentAttachment? = nil) -> ChatMessage {
        var message = Fixtures.message(id, author: Fixtures.viewerID, body: body, minutes: 1)
        message.attachment = attachment
        return message
    }

    func testTextOnlySendsNoAttachment() async throws {
        let (session, transport) = try await opened([try Fixtures.created(stored("m1", body: "gm"))])

        let problem = await session.send(body: "gm")

        XCTAssertNil(problem)
        let body = try await sentBody(transport, at: 1)
        XCTAssertEqual(body["body"] as? String, "gm")
        XCTAssertNil(body["attachment"])
    }

    func testTextWithAGifSendsBoth() async throws {
        let (session, transport) = try await opened([
            try Fixtures.created(stored("m1", body: "look", attachment: Self.gifAttachment()))
        ])

        let problem = await session.send(body: "  look ", attachment: .gif(Self.gif))

        XCTAssertNil(problem)
        let body = try await sentBody(transport, at: 1)
        XCTAssertEqual(body["body"] as? String, "look")
        let attachment = try XCTUnwrap(body["attachment"] as? [String: Any])
        XCTAssertEqual(attachment["kind"] as? String, "gif")
        XCTAssertEqual((attachment["gif"] as? [String: Any])?["id"] as? String, "abc123")
        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.sent])
        XCTAssertNotNil(state.timeline.rows.first?.attachment)
    }

    func testAGifAloneSendsNoBody() async throws {
        let (session, transport) = try await opened([
            try Fixtures.created(stored("m1", body: "GIF", attachment: Self.gifAttachment()))
        ])

        let problem = await session.send(body: nil, attachment: .gif(Self.gif))

        XCTAssertNil(problem)
        let body = try await sentBody(transport, at: 1)
        XCTAssertNil(body["body"])
        XCTAssertEqual((body["attachment"] as? [String: Any])?["kind"] as? String, "gif")
    }

    func testBlankTextWithAnAttachmentSendsNoBody() async throws {
        let (session, transport) = try await opened([try Fixtures.created(stored("m1", body: "GIF"))])

        await session.send(body: "   ", attachment: .gif(Self.gif))

        let body = try await sentBody(transport, at: 1)
        XCTAssertNil(body["body"])
    }

    func testAStockSendsItsAssetID() async throws {
        let (session, transport) = try await opened([try Fixtures.created(stored("m1", body: "Shared a stock"))])

        await session.send(body: nil, attachment: .stock(assetID: Self.assetID))

        let sentStock = try await sentBody(transport, at: 1)
        let attachment = try XCTUnwrap(sentStock["attachment"] as? [String: Any])
        XCTAssertEqual(attachment["kind"] as? String, "stock")
        XCTAssertEqual(attachment["asset_id"] as? String, Self.assetID)
    }

    func testAThesisQuoteSendsTheProposalAndRange() async throws {
        let (session, transport) = try await opened([try Fixtures.created(stored("m1", body: "Quoted the thesis"))])

        await session.send(body: nil, attachment: .thesisQuote(proposalID: Self.proposalID, start: 2, end: 9))

        let sentQuote = try await sentBody(transport, at: 1)
        let attachment = try XCTUnwrap(sentQuote["attachment"] as? [String: Any])
        XCTAssertEqual(attachment["kind"] as? String, "thesis_quote")
        XCTAssertEqual(attachment["proposal_id"] as? String, Self.proposalID)
        XCTAssertEqual(attachment["start"] as? Int, 2)
        XCTAssertEqual(attachment["end"] as? Int, 9)
    }

    func testNeitherTextNorAttachmentIsRefused() async throws {
        let (session, transport) = try await opened([])

        let nothing = await session.send(body: nil)
        let blank = await session.send(body: "  ", attachment: nil)
        let long = await session.send(body: String(repeating: "a", count: 2001), attachment: .gif(Self.gif))

        XCTAssertEqual(nothing, .empty)
        XCTAssertEqual(blank, .empty)
        XCTAssertEqual(long, .tooLong(count: 2001))
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testRetryOfAFailedAttachmentResendsTheSameRequestAndKey() async throws {
        let (session, transport) = try await opened([
            .failure(URLError(.notConnectedToInternet)),
            try Fixtures.created(stored("m1", body: "GIF", attachment: Self.gifAttachment())),
        ])

        await session.send(body: nil, attachment: .gif(Self.gif))
        let failed = await Fixtures.state(session)
        XCTAssertEqual(failed.timeline.rows.map(\.delivery), [.failed])
        XCTAssertEqual(failed.timeline.rows.first?.pendingAttachment, .gif(Self.gif))
        XCTAssertEqual(failed.timeline.rows.first?.displayText, "")

        await session.retry(key: "key-1")

        let first = try await sentBody(transport, at: 1)
        let second = try await sentBody(transport, at: 2)
        XCTAssertEqual(NSDictionary(dictionary: first), NSDictionary(dictionary: second))
        let keys = await Fixtures.idempotencyKeys(transport)
        XCTAssertEqual(Array(keys.suffix(2)), ["key-1", "key-1"])
        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.sent])
    }

    func testAPendingAttachmentRowIsNotAnEmptyBubble() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([]), .gate])
        let session = Fixtures.session(transport)
        await session.open()

        let assetID = Self.assetID
        let sending = Task { await session.send(body: nil, attachment: .stock(assetID: assetID)) }
        await transport.waitForRequests(2)
        let pending = await Fixtures.state(session)

        XCTAssertEqual(pending.timeline.rows.map(\.delivery), [.pending])
        XCTAssertEqual(pending.timeline.rows.first?.message.body, "Shared a stock")
        XCTAssertEqual(pending.timeline.rows.first?.displayText, "")
        await transport.releaseGate(try Fixtures.created(stored("m1", body: "Shared a stock")))
        _ = await sending.value
    }

    func testAnAttachmentEchoReplacesItsUnsentRow() async throws {
        let echo = stored("m1", body: "GIF", attachment: Self.gifAttachment())
        let transport = StubTransport(scripted: [try Fixtures.page([]), .failure(URLError(.timedOut))])
        let session = Fixtures.session(transport)
        await session.open()
        await session.send(body: nil, attachment: .gif(Self.gif))

        await session.apply(.messageCreated(echo))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
    }

    func testAReplyCanCarryAnAttachment() async throws {
        let reply = Fixtures.message(
            "r1", author: Fixtures.viewerID, body: "Shared a stock", minutes: 2, parentID: "p1")
        let transport = StubTransport(scripted: [try Fixtures.created(reply)])
        let session = Fixtures.session(transport)

        let problem = await session.send(
            body: nil, attachment: .stock(assetID: Self.assetID), parentId: "p1", alsoInChannel: false)

        XCTAssertNil(problem)
        let body = try await sentBody(transport, at: 0)
        XCTAssertEqual(body["parent_id"] as? String, "p1")
        XCTAssertEqual((body["attachment"] as? [String: Any])?["asset_id"] as? String, Self.assetID)
        let empty = await session.send(body: nil, parentId: "p1")
        XCTAssertEqual(empty, .empty)
    }

    func testARejectedThesisQuoteReadsAsNoThesis() async throws {
        let (session, _) = try await opened([
            Fixtures.problem(400, "invalid_input", "The request is not valid.")
        ])

        await session.send(body: nil, attachment: .thesisQuote(proposalID: Self.proposalID, start: nil, end: nil))

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.failed])
        let notice = try XCTUnwrap(state.notice)
        guard case .problem(let problem) = notice.error else { return XCTFail("want a problem") }
        XCTAssertEqual(problem.message, "That proposal has no thesis to quote.")
        XCTAssertEqual(problem.status, 400)
    }

    func testARejectedThesisQuoteInAReplyReadsAsNoThesis() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(400, "invalid_input", "The request is not valid.")])
        let session = Fixtures.session(transport)

        let thread = await session.thread(parentId: "p1")
        await session.send(
            body: nil, attachment: .thesisQuote(proposalID: Self.proposalID, start: nil, end: nil), parentId: "p1")

        var notice: ChatSession.Notice?
        for await state in await thread.states() {
            notice = state.notice
            break
        }
        guard case .problem(let problem) = try XCTUnwrap(notice).error else { return XCTFail("want a problem") }
        XCTAssertEqual(problem.message, GroupChatCopy.noThesis)
    }

    func testOtherFailuresKeepTheirOwnCopy() async throws {
        let (gifSession, _) = try await opened([Fixtures.problem(400, "invalid_input", "Bad GIF.")])
        await gifSession.send(body: nil, attachment: .gif(Self.gif))
        let gifState = await Fixtures.state(gifSession)
        guard case .problem(let problem) = try XCTUnwrap(gifState.notice).error else { return XCTFail("problem") }
        XCTAssertEqual(problem.message, "Bad GIF.")

        let (thesisSession, _) = try await opened([.failure(URLError(.timedOut))])
        await thesisSession.send(
            body: nil, attachment: .thesisQuote(proposalID: Self.proposalID, start: nil, end: nil))
        let thesisState = await Fixtures.state(thesisSession)
        guard case .transport = try XCTUnwrap(thesisState.notice).error else { return XCTFail("transport") }
    }

    func testTheCopyIsTheSameWordAsForText() {
        XCTAssertEqual(GroupChatCopy.attachmentNotSent, "Not sent")
        XCTAssertEqual(GroupChatCopy.attachmentNotSent, GroupChatCopy.notSent)
    }

    func testRowsExposeTheAttachmentAndTheTextPartOnly() {
        let stock = Schemas.CommentAttachment.stock(
            .init(
                kind: .stock, assetId: Self.assetID, symbol: "AAPL", name: "Apple",
                logoUrl: "https://example.com/a.png",
                isPreIpo: false, priceMicros: nil, changeBps: nil))
        let quote = Schemas.CommentAttachment.thesisQuote(
            .init(kind: .thesisQuote, proposalId: Self.proposalID, text: "Small add.", proposerName: "Ada"))
        var deleted = Fixtures.message("d", minutes: 4)
        deleted.body = nil
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest(
            [
                Fixtures.message("a", body: "gm", minutes: 0),
                stored("b", body: "GIF", attachment: Self.gifAttachment()),
                stored("c", body: "look at this", attachment: Self.gifAttachment()),
                deleted,
                stored("e", body: "Shared a stock", attachment: stock),
                stored("f", body: "Quoted the thesis", attachment: quote),
                Fixtures.message("g", body: "GIF", minutes: 6),
            ], pageSize: 50)

        let rows = Dictionary(uniqueKeysWithValues: timeline.rows.map { ($0.id, $0) })
        XCTAssertEqual(rows["a"]?.displayText, "gm")
        XCTAssertNil(rows["a"]?.attachment)
        XCTAssertEqual(rows["b"]?.displayText, "")
        XCTAssertEqual(rows["b"]?.attachment, Self.gifAttachment())
        XCTAssertEqual(rows["c"]?.displayText, "look at this")
        XCTAssertEqual(rows["d"]?.displayText, "")
        XCTAssertEqual(rows["e"]?.displayText, "")
        XCTAssertEqual(rows["f"]?.displayText, "")
        XCTAssertEqual(rows["g"]?.displayText, "GIF")
    }

    func testAPollHasNoStandInText() {
        let poll = Schemas.CommentAttachment.poll(
            .init(
                kind: .poll, question: "Trim?", closesAt: Fixtures.epoch, closed: false,
                options: [.init(position: 0, label: "Yes", count: 1)], total: 1))
        XCTAssertNil(poll.fallbackBody)
    }
}

final class ChatAttachmentDraftTests: XCTestCase {
    private typealias Schemas = Components.Schemas

    private static let proposalID = "01890a5d-ac96-774b-bcce-b302099a8057"
    private static let assetID = "01890a5d-ac96-774b-bcce-b302099a8070"

    private static func media(_ name: String) -> Schemas.GifMedia {
        .init(url: "https://media.giphy.com/media/abc123/\(name).gif", width: 200, height: 150)
    }

    private static let gif = Schemas.GifInput(
        id: "abc123", title: "happy dance", still: media("still"), preview: media("preview"),
        original: media("giphy"))

    func testTheDraftMapsWhatTheCommentSheetsReturn() {
        let gif = Schemas.CommentAttachmentInput.gif(.init(kind: .gif, gif: Self.gif))
        let stock = Schemas.CommentAttachmentInput.stock(.init(kind: .stock, assetId: Self.assetID))
        let quote = Schemas.CommentAttachmentInput.thesisQuote(.init(kind: .thesisQuote, start: 1, end: 5))
        let poll = Schemas.CommentAttachmentInput.poll(
            .init(kind: .poll, question: "Trim?", options: ["Yes", "No"], closesInHours: nil))

        XCTAssertEqual(ChatAttachmentDraft(gif, proposalID: nil), .gif(Self.gif))
        XCTAssertEqual(ChatAttachmentDraft(stock, proposalID: nil), .stock(assetID: Self.assetID))
        XCTAssertEqual(
            ChatAttachmentDraft(quote, proposalID: Self.proposalID),
            .thesisQuote(proposalID: Self.proposalID, start: 1, end: 5))
        XCTAssertNil(ChatAttachmentDraft(quote, proposalID: nil))
        XCTAssertNil(ChatAttachmentDraft(poll, proposalID: Self.proposalID))
    }

    func testTheDraftBuildsTheChatRequest() {
        XCTAssertEqual(ChatAttachmentDraft.gif(Self.gif).request(), .gif(.init(kind: .gif, gif: Self.gif)))
        XCTAssertEqual(
            ChatAttachmentDraft.stock(assetID: Self.assetID).request(),
            .stock(.init(kind: .stock, assetId: Self.assetID)))
        XCTAssertEqual(
            ChatAttachmentDraft.thesisQuote(proposalID: Self.proposalID, start: nil, end: nil).request(),
            .thesisQuote(.init(kind: .thesisQuote, proposalId: Self.proposalID)))
    }
}
