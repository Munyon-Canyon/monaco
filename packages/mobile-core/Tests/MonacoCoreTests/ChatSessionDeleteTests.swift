import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatSessionDeleteTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    func testADeleteHidesTheRowAtOnceAndSendsTheMessageIDWithAKey() async throws {
        let mine = Fixtures.message("m1", author: Fixtures.viewerID, minutes: 1)
        let transport = StubTransport(scripted: [try Fixtures.page([mine]), .gate])
        let session = Fixtures.session(transport)
        await session.open()

        let deleting = Task { await session.delete(messageId: "m1") }
        await transport.waitForRequests(2)
        let hidden = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(hidden), [])
        await transport.releaseGate(Fixtures.noContent)
        let failure = await deleting.value

        XCTAssertNil(failure)
        let sent = await transport.sent
        XCTAssertEqual(sent[1].method, .delete)
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Fixtures.cabalID)/messages/m1")
        let keys = await Fixtures.idempotencyKeys(transport)
        XCTAssertEqual(keys, ["key-1"])
        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), [])
    }

    func testAFailedDeleteRestoresTheRowAndReturnsTheProblem() async throws {
        let mine = Fixtures.message("m1", author: Fixtures.viewerID, minutes: 1)
        let transport = StubTransport(scripted: [
            try Fixtures.page([mine]),
            Fixtures.problem(403, "chat_message_not_owned", "Only the author can delete it."),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        let failure = await session.delete(messageId: "m1")

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
        XCTAssertEqual(state.timeline.rows.first?.message.body, mine.body)
        guard case .problem(let problem) = failure else {
            return XCTFail("expected a problem, got \(String(describing: failure))")
        }
        XCTAssertEqual(problem.message, "Only the author can delete it.")
    }

    func testADeletedParentWithRepliesBecomesAPlaceholderThatKeepsItsReplyCount() async throws {
        let parent = Fixtures.message("m1", author: Fixtures.viewerID, minutes: 1, replyCount: 2)
        let transport = StubTransport(scripted: [try Fixtures.page([parent]), Fixtures.noContent])
        let session = Fixtures.session(transport)
        await session.open()

        let failure = await session.delete(messageId: "m1")

        XCTAssertNil(failure)
        let state = await Fixtures.state(session)
        let row = try XCTUnwrap(state.timeline.rows.first)
        XCTAssertTrue(row.isPlaceholder)
        XCTAssertEqual(row.message.replyCount, 2)
    }

    func testADeletedPlaceholderComesBackWhenTheDeleteFails() async throws {
        let parent = Fixtures.message("m1", author: Fixtures.viewerID, minutes: 1, replyCount: 2)
        let transport = StubTransport(scripted: [
            try Fixtures.page([parent]), .failure(URLError(.notConnectedToInternet)),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        let failure = await session.delete(messageId: "m1")

        XCTAssertNotNil(failure)
        let state = await Fixtures.state(session)
        let row = try XCTUnwrap(state.timeline.rows.first)
        XCTAssertFalse(row.isPlaceholder)
        XCTAssertEqual(row.message.body, parent.body)
    }

    func testADeleteRemovesTheReplyFromAnOpenThreadAndRestoresItOnFailure() async throws {
        let parent = Fixtures.message("p1", minutes: 1, replyCount: 1)
        let mine = Fixtures.message("r1", author: Fixtures.viewerID, minutes: 2, parentID: "p1")
        let transport = StubTransport(scripted: [
            try Fixtures.page([parent]),
            try Fixtures.thread(parent: parent, replies: [mine]),
            Fixtures.problem(500, "internal", "Something went wrong."),
            Fixtures.noContent,
        ])
        let session = Fixtures.session(transport)
        await session.open()
        let thread = await session.thread(parentId: "p1")
        await thread.open()

        _ = await session.delete(messageId: "r1")
        var current = await threadState(thread)
        XCTAssertEqual(current.timeline.rows.map(\.id), ["r1"])

        let failure = await session.delete(messageId: "r1")
        XCTAssertNil(failure)
        current = await threadState(thread)
        XCTAssertEqual(current.timeline.rows.map(\.id), [])
    }

    private func threadState(_ thread: ThreadSession) async -> ThreadSession.State {
        for await state in await thread.states() { return state }
        preconditionFailure("states ended")
    }
}
