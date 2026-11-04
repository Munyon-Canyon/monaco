import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import Synchronization
import XCTest

@testable import MonacoCore

@MainActor
final class ChatPollVotingTests: XCTestCase {
    private typealias Fixtures = ChatFixtures
    private typealias Schemas = Components.Schemas

    private static let closesAt = Fixtures.epoch.addingTimeInterval(86_400)

    private static func options(_ counts: [Int]) -> [Schemas.PollOption] {
        let labels = ["Yes", "Wait", "No", "Later"]
        return counts.indices.map { Schemas.PollOption(position: $0, label: labels[$0], count: counts[$0]) }
    }

    private static func poll(_ counts: [Int], mine: Int? = nil) -> Schemas.PollAttachment {
        Schemas.PollAttachment(
            kind: .poll, question: "Trim to 40%?", closesAt: closesAt, closed: false, options: options(counts),
            total: counts.reduce(0, +), myPosition: mine)
    }

    private static func state(_ counts: [Int], mine: Int?, closed: Bool = false) -> String {
        let rows = counts.indices.map { #"{"position":\#($0),"label":"L\#($0)","count":\#(counts[$0])}"# }
        let choice = mine.map { #","my_position":\#($0)"# } ?? ""
        return #"{"closes_at":"2026-10-04T08:00:00Z","closed":\#(closed),"options":[\#(rows.joined(separator: ","))],"#
            + #""total":\#(counts.reduce(0, +))\#(choice)}"#
    }

    private func voting(
        _ transport: StubTransport, settled: PollLog? = nil, gone: PollLog? = nil
    ) -> ChatPollVoting {
        ChatPollVoting(
            cabalID: Fixtures.cabalID, api: CommentsTestSupport.api(transport),
            settled: { id, state in settled?.add("\(id):\(state.myPosition.map(String.init) ?? "-")") },
            messageGone: { id in gone?.add(id) })
    }

    private func model(_ poll: Schemas.PollAttachment, _ voting: ChatPollVoting) -> CommentPollModel {
        CommentPollModel(
            commentID: "m1", poll: poll, now: Fixtures.epoch, voting: voting, clock: TestClock())
    }

    func testAVoteIsAPutOnTheMessagePollRouteWithThePosition() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.state([3, 2], mine: 1))])
        let settled = PollLog()

        let state = try await voting(transport, settled: settled).vote(commentID: "m1", position: 1)

        XCTAssertEqual(state.myPosition, 1)
        XCTAssertEqual(state.total, 5)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method.rawValue), ["PUT"])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Fixtures.cabalID)/messages/m1/poll/vote")
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: Int], ["position": 1])
        XCTAssertEqual(settled.values, ["m1:1"])
    }

    func testTappingAnotherOptionMovesTheVote() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.state([3, 1], mine: 0)), .json(.ok, Self.state([2, 2], mine: 1)),
        ])
        let poll = model(Self.poll([2, 1]), voting(transport))

        await poll.tap(0)
        XCTAssertEqual(poll.myPosition, 0)
        XCTAssertEqual(poll.poll.options.map(\.count), [3, 1])
        await poll.tap(1)

        XCTAssertEqual(poll.myPosition, 1)
        XCTAssertEqual(poll.poll.options.map(\.count), [2, 2])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method.rawValue), ["PUT", "PUT"])
    }

    func testTappingTheChosenOptionWithdrawsWithADelete() async throws {
        let transport = StubTransport(scripted: [.json(.ok, Self.state([2, 1], mine: nil))])
        let poll = model(Self.poll([3, 1], mine: 0), voting(transport))

        await poll.tap(0)

        XCTAssertNil(poll.myPosition)
        XCTAssertEqual(poll.poll.options.map(\.count), [2, 1])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method.rawValue), ["DELETE"])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Fixtures.cabalID)/messages/m1/poll/vote")
    }

    func testAClosedPollRefusesTheVoteAndKeepsTheOldCounts() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(409, "poll_closed", "This poll has closed.")
        ])
        let gone = PollLog()
        let poll = model(Self.poll([2, 1]), voting(transport, gone: gone))

        await poll.tap(1)

        XCTAssertNil(poll.myPosition)
        XCTAssertEqual(poll.poll.options.map(\.count), [2, 1])
        XCTAssertEqual(poll.noticeTick, 1)
        guard case .problem(let problem) = poll.failure else { return XCTFail("expected a problem") }
        XCTAssertEqual(problem.code, .known(.pollClosed))
        XCTAssertEqual(gone.values, [])
    }

    func testAVoteOnADeletedMessageReportsItGone() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(404, "chat_message_not_found", "That message is gone.")
        ])
        let gone = PollLog()

        do {
            _ = try await voting(transport, gone: gone).vote(commentID: "m1", position: 0)
            XCTFail("expected the vote to throw")
        } catch {
            XCTAssertTrue(ChatPollVoting.isGone(APIError(error)))
        }

        XCTAssertEqual(gone.values, ["m1"])
    }

    func testTheSessionKeepsTheViewersVoteWhenCountsArriveLive() async throws {
        var message = Fixtures.message("m1", body: "Trim to 40%?")
        message.attachment = .poll(Self.poll([2, 1], mine: 1))
        let transport = StubTransport(scripted: [try Fixtures.page([message])])
        let session = Fixtures.session(transport)
        await session.open()

        let counts: [ChatPollCount] = [.init(position: 0, votes: 4), .init(position: 1, votes: 2)]
        await session.apply(.pollUpdated(messageId: "m1", options: counts, total: 6))

        let state = await Fixtures.state(session)
        guard case .poll(let poll)? = state.timeline.message(id: "m1")?.attachment else {
            return XCTFail("expected a poll")
        }
        XCTAssertEqual(poll.options.map(\.count), [4, 2])
        XCTAssertEqual(poll.total, 6)
        XCTAssertEqual(poll.myPosition, 1)
        XCTAssertEqual(poll.options.map(\.label), ["Yes", "Wait"])
    }

    func testASettledVoteWritesTheViewersChoiceIntoTheLoadedMessage() async throws {
        var message = Fixtures.message("m1", body: "Trim to 40%?")
        message.attachment = .poll(Self.poll([2, 1]))
        let transport = StubTransport(scripted: [try Fixtures.page([message])])
        let session = Fixtures.session(transport)
        await session.open()
        let settled = Schemas.PollState(
            closesAt: Self.closesAt, closed: false, options: Self.options([3, 1]), total: 4, myPosition: 0)

        await session.settlePoll(id: "m1", state: settled)
        let counts: [ChatPollCount] = [.init(position: 0, votes: 4), .init(position: 1, votes: 1)]
        await session.apply(.pollUpdated(messageId: "m1", options: counts, total: 5))

        let state = await Fixtures.state(session)
        guard case .poll(let poll)? = state.timeline.message(id: "m1")?.attachment else {
            return XCTFail("expected a poll")
        }
        XCTAssertEqual(poll.myPosition, 0)
        XCTAssertEqual(poll.options.map(\.count), [4, 1])
    }

    func testAPollUpdateForAnUnknownMessageChangesNothing() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([Fixtures.message("m1")])])
        let session = Fixtures.session(transport)
        await session.open()
        let before = await Fixtures.state(session)

        await session.apply(.pollUpdated(messageId: "zz", options: [.init(position: 0, votes: 1)], total: 1))
        await session.apply(.pollUpdated(messageId: "m1", options: [.init(position: 0, votes: 1)], total: 1))

        let after = await Fixtures.state(session)
        XCTAssertEqual(after.timeline.rows, before.timeline.rows)
    }

    func testAGoneMessageIsDroppedFromTheThread() async throws {
        var message = Fixtures.message("m1", body: "Trim to 40%?")
        message.attachment = .poll(Self.poll([2, 1]))
        let transport = StubTransport(scripted: [try Fixtures.page([message, Fixtures.message("m2", minutes: 1)])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.dropMessage(id: "m1")

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m2"])
    }

    func testThePollEventDecodesItsCountsAndTotal() async {
        let data = Data(
            #"{"message_id":"m1","options":[{"position":0,"votes":3},{"position":1,"votes":2}],"total_votes":5}"#.utf8)

        XCTAssertEqual(
            ChatEventDecoder.decode(name: "poll.updated", data: data),
            .pollUpdated(
                messageId: "m1", options: [.init(position: 0, votes: 3), .init(position: 1, votes: 2)], total: 5))
        XCTAssertNil(ChatEventDecoder.decode(name: "poll.updated", data: Data("{}".utf8)))
    }

    func testAPollDraftSendsAsAPollAttachmentWithTheQuestionAsItsBody() async throws {
        let input = try XCTUnwrap(
            CommentPollDraft(question: "Trim to 40%?", options: ["Yes", "Wait"], closes: .day).input)

        let draft = try XCTUnwrap(ChatAttachmentDraft(input, proposalID: nil))

        XCTAssertEqual(draft.fallbackBody, "Trim to 40%?")
        guard case .poll(let sent) = draft.request() else { return XCTFail("expected a poll request") }
        XCTAssertEqual(sent.question, "Trim to 40%?")
        XCTAssertEqual(sent.options, ["Yes", "Wait"])
        XCTAssertEqual(sent.closesInHours, 24)
    }

    func testAPollMessageHidesItsQuestionAboveTheBubble() async throws {
        var message = Fixtures.message("m1", body: "Trim to 40%?")
        message.attachment = .poll(Self.poll([0, 0]))
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest([message], pageSize: 50, loadedBefore: [])

        XCTAssertEqual(timeline.rows.first?.displayText, "")
    }
}

private final class PollLog: Sendable {
    private let storage = Mutex<[String]>([])

    var values: [String] { storage.withLock { $0 } }

    func add(_ value: String) { storage.withLock { $0.append(value) } }
}
