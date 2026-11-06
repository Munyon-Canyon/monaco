import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ChatPinsModelTests: XCTestCase {
    private let cabalID = ChatFixtures.cabalID
    private let viewerID = ChatFixtures.viewerID
    private var pinsPath: String { "/v1/cabals/\(cabalID)/chat/pins" }

    private func model(_ transport: StubTransport) -> ChatPinsModel {
        ChatPinsModel(
            cabalID: cabalID, viewerID: viewerID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            makeKey: { "key-1" })
    }

    private func pins(_ messages: [ChatMessage]) throws -> StubTransport.Reply {
        .json(.ok, #"{"messages":[\#(try messages.map(ChatFixtures.json).joined(separator: ","))]}"#)
    }

    private func loaded(_ messages: [ChatMessage], then replies: [StubTransport.Reply] = []) async throws -> (
        ChatPinsModel, StubTransport
    ) {
        let transport = StubTransport(scripted: [try pins(messages)] + replies)
        let model = model(transport)
        await model.load()
        return (model, transport)
    }

    private func message(_ id: String, author: String = ChatFixtures.otherID, body: String? = nil) -> ChatMessage {
        ChatFixtures.message(id, author: author, body: body)
    }

    private func system(_ moment: Components.Schemas.ChatMoment?) -> ChatMessage {
        ChatMessage(
            id: "s1", kind: .system,
            author: .init(id: "00000000-0000-0000-0000-000000000000", handle: nil, displayName: "", photoUrl: nil),
            moment: moment, body: nil, createdAt: ChatFixtures.epoch,
            alsoInChannel: false, replyCount: 0, lastRepliers: [], deleted: false, reactions: [], reactionCount: 0)
    }

    private func problem(_ status: Int, _ code: String) -> StubTransport.Reply {
        ChatFixtures.problem(status, code, "server text")
    }

    func testLoadKeepsTheServerOrderAtMostThreeAndAnEmptyListReadsEmpty() async throws {
        let all = ["a", "b", "c", "d"].map { message($0) }
        let (model, transport) = try await loaded(all)

        XCTAssertEqual(model.pins.map(\.id), ["a", "b", "c"])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), [pinsPath])

        let (empty, _) = try await loaded([])
        XCTAssertEqual(empty.pins, [])
        XCTAssertEqual(empty.current, 0)
    }

    func testAFailedLoadKeepsTheCurrentPins() async throws {
        let (model, _) = try await loaded([message("a")])
        let failing = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let other = self.model(failing)

        await other.load()
        XCTAssertEqual(other.pins, [])
        XCTAssertNil(other.toast)
        XCTAssertEqual(model.pins.map(\.id), ["a"])
    }

    func testAReloadClampsTheCurrentIndex() async throws {
        let (model, transport) = try await loaded(["a", "b", "c"].map { message($0) })
        model.next()
        model.next()
        XCTAssertEqual(model.current, 2)

        let shorter = self.model(StubTransport(scripted: [try pins([message("a")])]))
        await shorter.load()
        XCTAssertEqual(shorter.current, 0)
        _ = transport
    }

    func testNextCyclesAndWrapsAndStaysPutWithOnePin() async throws {
        let (model, _) = try await loaded(["a", "b", "c"].map { message($0) })

        model.next()
        XCTAssertEqual(model.current, 1)
        model.next()
        model.next()
        XCTAssertEqual(model.current, 0)

        let (single, _) = try await loaded([message("a")])
        single.next()
        XCTAssertEqual(single.current, 0)
    }

    func testCounterReadsOneOfTwoAndNothingForOne() {
        XCTAssertEqual(ChatPinsCopy.counter(index: 0, count: 2), "1 of 2")
        XCTAssertEqual(ChatPinsCopy.counter(index: 2, count: 3), "3 of 3")
        XCTAssertEqual(ChatPinsCopy.counter(index: 0, count: 1), "")
        XCTAssertEqual(ChatPinsCopy.counter(index: 0, count: 0), "")
    }

    func testPinShowsFirstAndKeepsTheServersStoredMessage() async throws {
        let stored = message("n", body: "stored")
        let (model, transport) = try await loaded(
            [message("a")], then: [.json(.ok, try ChatFixtures.json(stored))])

        await model.pin(message("n", body: "local"))

        XCTAssertEqual(model.pins.map(\.id), ["n", "a"])
        XCTAssertEqual(model.pins.first?.body, "stored")
        XCTAssertEqual(model.current, 0)
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.path, "/v1/cabals/\(cabalID)/messages/n/pin")
        XCTAssertEqual(sent.last?.method, .put)
    }

    func testPinningAPinnedMessageSendsNothing() async throws {
        let (model, transport) = try await loaded([message("a")])

        await model.pin(message("a"))

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testUnpinRemovesAtOnceAndSendsDelete() async throws {
        let (model, transport) = try await loaded(
            [message("a"), message("b")], then: [.json(.ok, try ChatFixtures.json(message("a")))])

        await model.unpin(message("a"))

        XCTAssertEqual(model.pins.map(\.id), ["b"])
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.method, .delete)
        XCTAssertEqual(sent.last?.path, "/v1/cabals/\(cabalID)/messages/a/pin")
    }

    func testUnpinningAMessageThatIsNotPinnedSendsNothing() async throws {
        let (model, transport) = try await loaded([message("a")])

        await model.unpin(message("zzz"))

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(model.pins.map(\.id), ["a"])
    }

    func testTheFourthPinRollsBackWithTheLimitCopy() async throws {
        let (model, _) = try await loaded(
            ["a", "b", "c"].map { message($0) }, then: [problem(409, "chat_pin_limit")])

        await model.pin(message("n"))

        XCTAssertEqual(model.pins.map(\.id), ["a", "b", "c"])
        XCTAssertEqual(model.toast, ChatPinsToast(serial: 1, message: ChatPinsCopy.limit))
        XCTAssertEqual(ChatPinsCopy.limit, "Unpin one first. A cabal can pin three messages.")
    }

    func testAForbiddenPinRollsBackWithTheForbiddenCopy() async throws {
        let (model, _) = try await loaded([message("a")], then: [problem(403, "chat_pin_forbidden")])

        await model.pin(message("n"))

        XCTAssertEqual(model.pins.map(\.id), ["a"])
        XCTAssertEqual(model.toast?.message, ChatPinsCopy.forbidden)
        XCTAssertEqual(ChatPinsCopy.forbidden, "Only admins and the author can pin this.")
    }

    func testAForbiddenUnpinRestoresThePinAndItsPlace() async throws {
        let (model, _) = try await loaded(
            [message("a"), message("b")], then: [problem(403, "chat_pin_forbidden")])
        model.next()

        await model.unpin(message("b"))

        XCTAssertEqual(model.pins.map(\.id), ["a", "b"])
        XCTAssertEqual(model.current, 1)
        XCTAssertEqual(model.toast?.message, ChatPinsCopy.forbidden)
    }

    func testAnotherFailureRollsBackWithTheServersOwnWordsAndCountsToasts() async throws {
        let (model, _) = try await loaded(
            [message("a")], then: [problem(500, "internal"), .failure(URLError(.notConnectedToInternet))])

        await model.pin(message("n"))
        XCTAssertEqual(model.pins.map(\.id), ["a"])
        XCTAssertEqual(model.toast, ChatPinsToast(serial: 1, message: "server text"))

        await model.pin(message("n"))
        XCTAssertEqual(model.toast, ChatPinsToast(serial: 2, message: "You're offline. Try again."))
    }

    func testAMessageThatIsGoneLeavesThePinListQuietly() async throws {
        let (model, _) = try await loaded(
            [message("a"), message("b")],
            then: [problem(404, "chat_message_not_found"), problem(404, "chat_message_not_found")]
        )

        await model.pin(message("n"))
        XCTAssertEqual(model.pins.map(\.id), ["a", "b"])
        await model.unpin(message("b"))

        XCTAssertEqual(model.pins.map(\.id), ["a"])
        XCTAssertNil(model.toast)
    }

    func testCanPinMatrix() async throws {
        let (model, _) = try await loaded([])
        let mine = message("m", author: viewerID)
        let theirs = message("t")
        var reply = message("r", author: viewerID)
        reply.parentId = "t"
        var deleted = message("d", author: viewerID)
        deleted.deleted = true
        let row = system(.cabalCreated(.init(kind: .cabalCreated)))

        for role in ["admin", "creator"] {
            XCTAssertTrue(model.canPin(theirs, role: role, viewerID: viewerID))
            XCTAssertTrue(model.canPin(row, role: role, viewerID: viewerID))
            XCTAssertFalse(model.canPin(reply, role: role, viewerID: viewerID))
            XCTAssertFalse(model.canPin(deleted, role: role, viewerID: viewerID))
        }
        XCTAssertTrue(model.canPin(mine, role: "member", viewerID: viewerID))
        XCTAssertTrue(model.canPin(mine, role: nil, viewerID: viewerID))
        XCTAssertFalse(model.canPin(theirs, role: "member", viewerID: viewerID))
        XCTAssertFalse(model.canPin(row, role: "member", viewerID: viewerID))
        XCTAssertFalse(model.canPin(reply, role: "member", viewerID: viewerID))
        XCTAssertFalse(model.canPin(deleted, role: "member", viewerID: viewerID))
    }

    func testTheLineIsTheFirstNameAndTheTextOnOneLine() async throws {
        let (model, _) = try await loaded([])
        var text = message("a", body: "  5 of 8 to pass,\n6 hour votes,   max trade 20%  ")
        text.author = .init(id: "u", handle: nil, displayName: "Christelle Dupont", photoUrl: nil)

        XCTAssertEqual(model.line(for: text), "Christelle 5 of 8 to pass, 6 hour votes, max trade 20%")
    }

    func testAnAttachmentOnlyPinReadsItsDisplayTextAndAMissingNameReadsSomeone() async throws {
        let (model, _) = try await loaded([])
        var gif = message("g", body: "GIF")
        gif.author = .init(id: "u", handle: nil, displayName: "", photoUrl: nil)
        var blank = message("b", body: " ")
        blank.body = nil

        XCTAssertEqual(model.line(for: gif), "Someone GIF")
        XCTAssertEqual(model.line(for: blank), "Kai")
    }

    func testAMomentPinReadsItsSentence() async throws {
        let (model, _) = try await loaded([])
        let created = system(.cabalCreated(.init(kind: .cabalCreated)))
        let joined = system(
            .memberJoined(
                .init(
                    kind: .memberJoined,
                    member: .init(id: "u9", handle: nil, displayName: "Ada Lovelace", photoUrl: nil))))

        XCTAssertEqual(model.line(for: created, cabalName: "Alpha"), "Alpha was created")
        XCTAssertEqual(model.line(for: joined, cabalName: "Alpha"), "Ada joined Alpha")
        XCTAssertEqual(model.line(for: created), "this cabal was created")
    }

    func testPinUpdatedDecodesTheMessageAndTheFlag() {
        let data = Data(#"{"message_id":"m1","pinned":true}"#.utf8)

        XCTAssertEqual(
            ChatEventDecoder.decode(name: "pin.updated", data: data), .pinUpdated(messageId: "m1", pinned: true))
        XCTAssertNil(ChatEventDecoder.decode(name: "pin.updated", data: Data("{}".utf8)))
    }

    func testPinUpdatedRefreshesTheSessionsPins() async throws {
        let transport = StubTransport(routes: [pinsPath: [try pins([message("a")])]])
        let session = ChatFixtures.session(transport, answersPins: false)

        await session.apply(.pinUpdated(messageId: "a", pinned: true))

        let state = await ChatFixtures.state(session)
        XCTAssertEqual(state.pins.pins.map(\.id), ["a"])
    }

    func testEveryAttachRefreshesTheSessionsPins() async throws {
        let transport = StubTransport(routes: [pinsPath: [try pins([message("a")]), try pins([message("b")])]])
        let session = ChatFixtures.session(transport, answersPins: false)

        await session.apply(.attached(resumed: true))
        let first = await settledPins(session, until: ["a"])
        XCTAssertEqual(first, ["a"])

        await session.apply(.attached(resumed: false))
        let second = await settledPins(session, until: ["b"])
        XCTAssertEqual(second, ["b"])
    }

    private func settledPins(_ session: ChatSession, until expected: [String]) async -> [String] {
        let model = await ChatFixtures.state(session).pins
        for _ in 0..<20_000 where model.pins.map(\.id) != expected {
            await Task.yield()
        }
        return model.pins.map(\.id)
    }
}
