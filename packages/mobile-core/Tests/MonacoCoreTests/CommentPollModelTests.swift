import Foundation
import MonacoAPI
import MonacoTestClock
import XCTest

@testable import MonacoCore

private actor ScriptedVoting: PollVoting {
    typealias State = Components.Schemas.PollState
    private(set) var calls: [String] = []
    private var results: [Result<State, any Error>]

    init(_ results: [Result<State, any Error>]) {
        self.results = results
    }

    func vote(commentID: String, position: Int) async throws -> State {
        calls.append("vote \(commentID) \(position)")
        return try results.removeFirst().get()
    }

    func withdraw(commentID: String) async throws -> State {
        calls.append("withdraw \(commentID)")
        return try results.removeFirst().get()
    }
}

private actor GatedVoting: PollVoting {
    typealias State = Components.Schemas.PollState
    private let result: State
    private var waiter: CheckedContinuation<Void, Never>?
    private(set) var started = false

    init(_ result: State) {
        self.result = result
    }

    func vote(commentID: String, position: Int) async throws -> State {
        started = true
        await withCheckedContinuation { waiter = $0 }
        return result
    }

    func withdraw(commentID: String) async throws -> State {
        try await vote(commentID: commentID, position: -1)
    }

    func release() {
        waiter?.resume()
        waiter = nil
    }
}

@MainActor
final class CommentPollModelTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_000_000)

    private func poll(mine: Int?, closed: Bool = false, closesIn: Double = 3600) -> Components.Schemas.PollAttachment {
        let counts = [3, 2]
        let options = counts.enumerated().map { index, count in
            Components.Schemas.PollOption(position: index, label: "Option \(index)", count: count)
        }
        return .init(
            kind: .poll, question: "Trim?", closesAt: now.addingTimeInterval(closesIn), closed: closed,
            options: options, total: 5, myPosition: mine)
    }

    private func state(_ counts: [Int], mine: Int?, closed: Bool = false) -> Components.Schemas.PollState {
        .init(
            closesAt: now.addingTimeInterval(3600), closed: closed,
            options: counts.enumerated().map {
                .init(position: $0.offset, label: "Option \($0.offset)", count: $0.element)
            },
            total: counts.reduce(0, +), myPosition: mine)
    }

    private func model(
        _ poll: Components.Schemas.PollAttachment, _ voting: any PollVoting, clock: TestClock = TestClock()
    ) -> CommentPollModel {
        CommentPollModel(commentID: "c1", poll: poll, now: now, voting: voting, clock: clock)
    }

    func testVotingAddsYourCheckAndReconcilesWithTheServer() async {
        let voting = ScriptedVoting([.success(state([4, 2], mine: 0))])
        let model = model(poll(mine: nil), voting)
        await model.tap(0)
        XCTAssertEqual(model.myPosition, 0)
        XCTAssertEqual(model.poll.options.map(\.count), [4, 2])
        XCTAssertEqual(model.poll.total, 6)
        XCTAssertEqual(model.percent(model.poll.options[0]), 67)
        let calls = await voting.calls
        XCTAssertEqual(calls, ["vote c1 0"])
        XCTAssertNil(model.failure)
    }

    func testMovingTheVoteShiftsOneCount() async {
        let voting = ScriptedVoting([.success(state([2, 3], mine: 1))])
        let model = model(poll(mine: 0), voting)
        await model.tap(1)
        XCTAssertEqual(model.myPosition, 1)
        XCTAssertEqual(model.poll.options.map(\.count), [2, 3])
        let calls = await voting.calls
        XCTAssertEqual(calls, ["vote c1 1"])
    }

    func testTappingYourChoiceWithdraws() async {
        let voting = ScriptedVoting([.success(state([2, 2], mine: nil))])
        let model = model(poll(mine: 0), voting)
        await model.tap(0)
        XCTAssertNil(model.myPosition)
        XCTAssertEqual(model.poll.total, 4)
        let calls = await voting.calls
        XCTAssertEqual(calls, ["withdraw c1"])
    }

    func testAFailedVoteRollsBackAndSaysWhy() async {
        let voting = ScriptedVoting([.failure(URLError(.notConnectedToInternet))])
        let before = poll(mine: 1)
        let model = model(before, voting)
        await model.tap(0)
        XCTAssertEqual(model.poll, before)
        XCTAssertEqual(model.failure.map(ToastCopy.message(for:)), "You're offline. Try again.")
        XCTAssertEqual(model.noticeTick, 1)
        XCTAssertFalse(model.isSending)
    }

    func testAClosedPollIsReadOnly() async {
        let voting = ScriptedVoting([])
        let flagged = model(poll(mine: 0, closed: true), voting)
        await flagged.tap(1)
        XCTAssertTrue(flagged.isClosed)
        XCTAssertEqual(flagged.footer, "Closed")
        let expired = model(poll(mine: nil, closesIn: -5), voting)
        await expired.tap(1)
        XCTAssertTrue(expired.isClosed)
        let calls = await voting.calls
        XCTAssertEqual(calls, [])
        XCTAssertEqual(expired.myPosition, nil)
    }

    func testAnUnknownOptionIsIgnored() async {
        let voting = ScriptedVoting([])
        let model = model(poll(mine: nil), voting)
        await model.tap(3)
        let calls = await voting.calls
        XCTAssertEqual(calls, [])
    }

    func testAnEmptyPollShowsZeroPercent() {
        var empty = poll(mine: nil)
        empty.total = 0
        empty.options = empty.options.map { .init(position: $0.position, label: $0.label, count: 0) }
        let model = model(empty, ScriptedVoting([]))
        XCTAssertEqual(model.percent(empty.options[0]), 0)
        XCTAssertEqual(model.shareBps(empty.options[0]), 0)
        XCTAssertEqual(CommentsCopy.pollVotes(0), "0 votes")
        XCTAssertEqual(CommentsCopy.pollVotes(1), "1 vote")
    }

    func testTheShareIsTheCountOverTheTotal() {
        let model = model(poll(mine: nil), ScriptedVoting([]))
        XCTAssertEqual(model.shareBps(model.poll.options[0]), 6000)
    }

    func testTheFooterCountsDownOnTheInjectedClockAndCloses() async {
        let clock = TestClock()
        let model = model(poll(mine: nil, closesIn: 60), ScriptedVoting([]), clock: clock)
        XCTAssertEqual(model.footer, "5 votes · closes in 1m")
        let running = Task { await model.run() }
        for _ in 0..<2 {
            _ = await CommentsTestSupport.waitUntil { clock.state.current.pending == 1 }
            clock.advance(by: CommentPollModel.tick)
        }
        await running.value
        XCTAssertTrue(model.isClosed)
        XCTAssertEqual(model.footer, "Closed")
    }

    func testCancellingTheCountdownStopsIt() async {
        let clock = TestClock()
        let model = model(poll(mine: nil), ScriptedVoting([]), clock: clock)
        let running = Task { await model.run() }
        _ = await CommentsTestSupport.waitUntil { clock.state.current.pending == 1 }
        running.cancel()
        await running.value
        XCTAssertFalse(model.isClosed)
    }

    func testCountdownLabels() {
        XCTAssertEqual(CommentPollModel.countdown(.seconds(20 * 3600)), "20h")
        XCTAssertEqual(CommentPollModel.countdown(.seconds(30 * 60)), "30m")
        XCTAssertEqual(CommentPollModel.countdown(.seconds(10)), "1m")
        XCTAssertEqual(CommentPollModel.countdown(.seconds(72 * 3600)), "3d")
    }

    func testReconcileTakesTheRefreshedPoll() {
        let model = model(poll(mine: nil), ScriptedVoting([]))
        var next = poll(mine: 1, closed: true)
        next.total = 9
        model.reconcile(next, now: now)
        XCTAssertEqual(model.poll.total, 9)
        XCTAssertTrue(model.isClosed)
    }

    func testReconcileWaitsOutAVoteInFlight() async {
        let voting = GatedVoting(state([4, 2], mine: 0))
        let model = model(poll(mine: nil), voting)
        let tapping = Task { await model.tap(0) }
        _ = await CommentsTestSupport.waitUntil { await voting.started }
        model.reconcile(poll(mine: nil, closed: true), now: now)
        await voting.release()
        await tapping.value
        XCTAssertFalse(model.poll.closed)
    }
}
