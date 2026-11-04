import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ChatSessionReactionTests: XCTestCase {
    private typealias Fixtures = ChatFixtures
    private typealias Reaction = Components.Schemas.Reaction

    private let target = ReactionTarget.chatMessage(cabalID: Fixtures.cabalID, id: "m1")
    private let laugh = Reaction(emoji: "😂", count: 2, mine: true)

    func testTheTargetNamesTheMessageAndKeysItApartFromComments() {
        XCTAssertEqual(target.id, "m1")
        XCTAssertEqual(target.key, "chat-m1")
        XCTAssertNotEqual(target, .comment("m1"))
    }

    func testReactingSendsAPutToTheChatRouteAndEndsWithTheServerSummary() async throws {
        let transport = StubTransport(scripted: [try summary([("😂", 3, true)], total: 3)])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())
        let current = [Reaction(emoji: "😂", count: 2, mine: false)]

        let result = await model.toggle("😂", on: target, current: current)

        XCTAssertEqual(result, [Reaction(emoji: "😂", count: 3, mine: true)])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method.rawValue), ["PUT"])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Fixtures.cabalID)/messages/m1/reactions/%F0%9F%98%82")
        XCTAssertNil(model.optimistic[target])
    }

    func testUnreactingSendsADeleteAndShowsTheRemovalAtOnce() async throws {
        let transport = StubTransport(scripted: [try summary([("😂", 1, false)], total: 1)])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let toggled = Task { await model.toggle("😂", on: target, current: [laugh]) }
        let showed = await CommentsTestSupport.waitUntil {
            model.reactions(for: self.target, fallback: [self.laugh]).first?.mine == false
        }
        let result = await toggled.value

        XCTAssertTrue(showed)
        XCTAssertEqual(result, [Reaction(emoji: "😂", count: 1, mine: false)])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method.rawValue), ["DELETE"])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Fixtures.cabalID)/messages/m1/reactions/%F0%9F%98%82")
    }

    func testTheReactionLimitRollsBackAndShowsTheServerCopy() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(422, "invalid_input", "You can react with 10 emoji on a message.")
        ])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let result = await model.toggle("🎉", on: target, current: [laugh])

        XCTAssertEqual(result, [laugh])
        XCTAssertNil(model.optimistic[target])
        XCTAssertEqual(model.notice, "You can react with 10 emoji on a message.")
    }

    func testAPageThenALiveMessageThenAToggleResultFeedOneReactionsList() async throws {
        let loaded = Fixtures.message("m1", minutes: 1, reactions: [laugh])
        let transport = StubTransport(scripted: [try Fixtures.page([loaded])])
        let session = Fixtures.session(transport)
        await session.open()
        var state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.reactions(for: "m1"), [laugh])
        XCTAssertEqual(state.timeline.message(id: "m1")?.reactionCount, 2)

        let live = Fixtures.message("m2", minutes: 2, reactions: [.init(emoji: "🔥", count: 1, mine: false)])
        await session.apply(.messageCreated(live))
        await session.settleReactions(id: "m1", reactions: [.init(emoji: "😂", count: 1, mine: false)], total: 1)

        state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.reactions(for: "m2"), [.init(emoji: "🔥", count: 1, mine: false)])
        XCTAssertEqual(state.timeline.reactions(for: "m1"), [.init(emoji: "😂", count: 1, mine: false)])
        XCTAssertEqual(state.timeline.rows.first?.message.reactionCount, 1)
        XCTAssertEqual(state.timeline.reactions(for: "missing"), [])
    }

    func testASettledResultForAnUnloadedMessageChangesNothing() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([Fixtures.message("m1", minutes: 1)])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.settleReactions(id: "gone", reactions: [laugh], total: 2)

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.reactions(for: "m1"), [])
    }

    func testAReactionEventReplacesTheCountsAndKeepsTheViewersOwnFlags() async throws {
        let loaded = Fixtures.message(
            "m1", minutes: 1, reactions: [laugh, .init(emoji: "🔥", count: 1, mine: false)])
        let transport = StubTransport(scripted: [try Fixtures.page([loaded])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(
            .reactionUpdated(
                messageId: "m1",
                reactions: [.init(emoji: "😂", count: 3), .init(emoji: "🔥", count: 2), .init(emoji: "👍", count: 1)],
                total: 6))

        let state = await Fixtures.state(session)
        XCTAssertEqual(
            state.timeline.reactions(for: "m1"),
            [
                .init(emoji: "😂", count: 3, mine: true), .init(emoji: "🔥", count: 2, mine: false),
                .init(emoji: "👍", count: 1, mine: false),
            ])
        XCTAssertEqual(state.timeline.message(id: "m1")?.reactionCount, 6)
    }

    func testAReactionEventForAMessageOutsideTheWindowIsIgnored() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([Fixtures.message("m1", minutes: 1)])])
        let session = Fixtures.session(transport)
        await session.open()
        let before = await Fixtures.state(session)

        await session.apply(.reactionUpdated(messageId: "old", reactions: [.init(emoji: "😂", count: 1)], total: 1))

        let after = await Fixtures.state(session)
        XCTAssertEqual(after, before)
    }

    func testAnOpenThreadFollowsReactionsOnItsParentAndRepliesAndIgnoresOthers() async throws {
        let parent = Fixtures.message("p1", minutes: 1, replyCount: 1, reactions: [laugh])
        let reply = Fixtures.message("r1", minutes: 2, parentID: "p1")
        let transport = StubTransport(scripted: [
            try Fixtures.page([parent]), try Fixtures.thread(parent: parent, replies: [reply]),
        ])
        let session = Fixtures.session(transport)
        await session.open()
        let thread = await session.thread(parentId: "p1")
        await thread.open()

        await session.apply(.reactionUpdated(messageId: "p1", reactions: [.init(emoji: "😂", count: 5)], total: 5))
        await session.apply(.reactionUpdated(messageId: "r1", reactions: [.init(emoji: "🔥", count: 1)], total: 1))
        await session.apply(.reactionUpdated(messageId: "other", reactions: [.init(emoji: "🔥", count: 1)], total: 1))

        let state = await threadState(thread)
        XCTAssertEqual(state.parent?.reactions, [.init(emoji: "😂", count: 5, mine: true)])
        XCTAssertEqual(state.parent?.reactionCount, 5)
        XCTAssertEqual(state.timeline.reactions(for: "r1"), [.init(emoji: "🔥", count: 1, mine: false)])
    }

    private func threadState(_ thread: ThreadSession) async -> ThreadSession.State {
        for await state in await thread.states() { return state }
        preconditionFailure("states ended")
    }

    private func summary(_ rows: [(String, Int32, Bool)], total: Int32) throws -> StubTransport.Reply {
        let reactions = rows.map { Reaction(emoji: $0.0, count: $0.1, mine: $0.2) }
        return .json(
            .ok, try CommentsTestSupport.encode(Components.Schemas.ReactionSummary(reactions: reactions, total: total)))
    }
}
