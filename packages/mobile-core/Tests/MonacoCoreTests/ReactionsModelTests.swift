import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ReactionsModelTests: XCTestCase {
    private typealias Reaction = Components.Schemas.Reaction
    private let target = ReactionTarget.feedItem("item-1")
    private let base: [Reaction] = [
        .init(emoji: "💪", count: 3, mine: false), .init(emoji: "👀", count: 2, mine: false),
    ]

    func testAddingAnEmojiShowsItAtOnceAndSendsOnePut() async throws {
        let transport = StubTransport(scripted: [
            try summary([("💪", 3, false), ("👀", 2, false), ("🎉", 1, true)], total: 6)
        ])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let toggled = Task { await model.toggle("🎉", on: target, current: base) }
        let showed = await CommentsTestSupport.waitUntil {
            model.reactions(for: self.target, fallback: self.base).contains { $0.emoji == "🎉" }
        }
        let reconciled = await toggled.value

        XCTAssertTrue(showed)
        XCTAssertEqual(reconciled.map(\.emoji), ["💪", "👀", "🎉"])
        XCTAssertEqual(reconciled.last?.mine, true)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method.rawValue), ["PUT"])
        XCTAssertEqual(sent.first?.path, "/v1/feed/item-1/reactions/%F0%9F%8E%89")
        XCTAssertNil(model.optimistic[target])
        XCTAssertNil(model.notice)
    }

    func testRemovingYourEmojiSendsADeleteAndReturnsTheReconciledSummary() async throws {
        let current = [Reaction(emoji: "🎉", count: 5, mine: true), Reaction(emoji: "👏", count: 2, mine: false)]
        let transport = StubTransport(scripted: [try summary([("🎉", 4, false), ("👏", 2, false)], total: 6)])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let reconciled = await model.toggle("🎉", on: target, current: current)

        XCTAssertEqual(reconciled.map(\.count), [4, 2])
        XCTAssertEqual(reconciled.first?.mine, false)
        let methods = await transport.sent.map(\.method.rawValue)
        XCTAssertEqual(methods, ["DELETE"])
    }

    func testTheServerSummaryWinsOverTheGuess() async throws {
        let transport = StubTransport(scripted: [try summary([("🎉", 9, true), ("💪", 3, false)], total: 12)])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let reconciled = await model.toggle("🎉", on: target, current: base)

        XCTAssertEqual(reconciled, [.init(emoji: "🎉", count: 9, mine: true), .init(emoji: "💪", count: 3, mine: false)])
    }

    func testAFailureRollsBackAndSetsAToast() async throws {
        let transport = try StubTransport.problem(
            problem(422, code: .invalidInput, message: "You can react with 10 emoji on a post."))
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let result = await model.toggle("🎉", on: target, current: base)

        XCTAssertEqual(result, base)
        XCTAssertNil(model.optimistic[target])
        XCTAssertEqual(model.notice, "You can react with 10 emoji on a post.")
        XCTAssertEqual(model.noticeTick, 1)
        model.dismissNotice()
        XCTAssertNil(model.notice)
    }

    func testAnOfflineFailureRollsBackWithTheOfflineToast() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))])
        let model = ReactionsModel(api: CommentsTestSupport.api(transport), clock: TestClock())

        let result = await model.toggle("🎉", on: target, current: base)

        XCTAssertEqual(result, base)
        XCTAssertEqual(model.notice, "You're offline. Try again.")
    }

    func testTheGuessAddsRemovesAndOrdersByCount() {
        let mine = [Reaction(emoji: "💪", count: 3, mine: false), Reaction(emoji: "👀", count: 2, mine: true)]

        let tied = [mine[0], Reaction(emoji: "👀", count: 2, mine: false)]
        let bumped = ReactionsModel.applying("👀", adding: true, to: tied)
        XCTAssertEqual(bumped.map(\.emoji), ["💪", "👀"], "a tie keeps the earlier order")
        XCTAssertEqual(bumped.last, .init(emoji: "👀", count: 3, mine: true))

        let behind = [Reaction(emoji: "💪", count: 1, mine: false), mine[1]]
        let passed = ReactionsModel.applying("👀", adding: true, to: behind)
        XCTAssertEqual(passed.map(\.emoji), ["👀", "💪"])

        let removed = ReactionsModel.applying("👀", adding: false, to: mine)
        XCTAssertEqual(removed, [mine[0], .init(emoji: "👀", count: 1, mine: false)])

        let last = [mine[0], Reaction(emoji: "👀", count: 1, mine: true)]
        let gone = ReactionsModel.applying("👀", adding: false, to: last)
        XCTAssertEqual(gone, [mine[0]])

        let fresh = ReactionsModel.applying("🎉", adding: true, to: mine)
        XCTAssertEqual(fresh.map(\.emoji), ["💪", "👀", "🎉"])
    }

    private func summary(_ rows: [(String, Int32, Bool)], total: Int32) throws -> StubTransport.Reply {
        let reactions = rows.map { Reaction(emoji: $0.0, count: $0.1, mine: $0.2) }
        return .json(
            .ok, try CommentsTestSupport.encode(Components.Schemas.ReactionSummary(reactions: reactions, total: total)))
    }

    private func problem(
        _ status: Int, code: Components.Schemas.ErrorCode, message: String
    ) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Error", status: status, code: code, message: message,
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
    }
}
