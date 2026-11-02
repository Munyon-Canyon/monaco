import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class HomeVoteCardModelTests: XCTestCase {
    private let proposalID = "proposal-1"
    private let votesPath = "/v1/proposals/proposal-1/votes"
    private let detailPath = "/v1/proposals/proposal-1"
    private let clock = Date(timeIntervalSince1970: 1_999_000_000)

    private func row(kind: String = "buy", rule: String? = nil) -> HomePendingVote {
        HomePendingVote(
            id: proposalID, cabalID: "cabal-1", cabalName: "AC Capital", kind: kind, rule: rule,
            symbol: kind == "rule" ? nil : "NVDAx", assetName: "NVIDIA", usdcMicros: 400_000_000,
            tally: ProposalTally(yes: 4, no: 1, voters: 8, needed: 5), replyCount: 6,
            expiresAt: clock.addingTimeInterval(14_400))
    }

    private func model(
        _ transport: StubTransport, hints: FakeHintStream = FakeHintStream(), row: HomePendingVote? = nil
    ) -> HomeVoteCardModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let at: Date = clock
        return HomeVoteCardModel(api: api, hints: hints, row: row ?? self.row(), now: { at })
    }

    private func castBody(
        status: String = "open", yes: Int, no: Int, undoIn: TimeInterval? = 15, order: Int = 5, yesOrder: Int = 5,
        decided: Bool = false
    ) -> String {
        let formatter = ISO8601DateFormatter()
        let until: String = undoIn.map { "\"\(formatter.string(from: clock.addingTimeInterval($0)))\"" } ?? "null"
        return
            #"{"proposal_id":"proposal-1","status":"\#(status)","tally":{"yes":\#(yes),"no":\#(no),"voters":8,"needed":5},"my_ballot":"yes","undo_until":\#(until),"my_vote":{"order":\#(order),"of":8,"yes_order":\#(yesOrder),"no_order":0,"decided":\#(decided)}}"#
    }

    private let retracted =
        #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":4,"no":1,"voters":8,"needed":5}}"#

    private func detail(yes: Int = 4, replies: Int = 6) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var value = Components.Schemas.ProposalDetail.openNVDA(now: clock)
        value.tally = .init(yes: yes, no: 1, voters: 8, needed: 5)
        value.replyCount = replies
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func problem(_ code: String, _ message: String) -> StubTransport.Reply {
        let body = """
            {"type":"about:blank","title":"Conflict","status":409,"code":"\(code)","message":"\(message)",\
            "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
            """
        return .response(status: .conflict, contentType: "application/problem+json", body: Data(body.utf8))
    }

    func testPickHoldCommitPostsOneBallotWithAnIdempotencyKey() async throws {
        let transport = StubTransport(routes: [votesPath: [.json(.ok, castBody(yes: 5, no: 1))]])
        let model = model(transport)
        model.select(.yes)
        XCTAssertEqual(model.phase, .picked(.yes))
        model.hold(.yes, progress: 0.4)
        XCTAssertEqual(model.phase, .holding(.yes, progress: 0.4))
        model.beginCommit(.yes)
        let outcome = await model.vote(.yes)
        XCTAssertEqual(outcome?.choice, .yes)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), [votesPath])
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(sent.first?.headerFields[key])
        XCTAssertEqual(model.tally.yes, 5)
    }

    func testLiftingBeforeTheHoldCompletesPostsNothing() async {
        let transport = StubTransport(routes: [:])
        let model = model(transport)
        model.select(.yes)
        model.hold(.yes, progress: 0.4)
        model.lift()
        XCTAssertEqual(model.phase, .picked(.yes))
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testAnOrdinaryResultExposesUndoAndThePlaceInTheVote() async throws {
        let transport = StubTransport(routes: [votesPath: [.json(.ok, castBody(yes: 5, no: 1, order: 6))]])
        let model = model(transport)
        let voted = await model.vote(.yes)
        let outcome = try XCTUnwrap(voted)
        XCTAssertNotNil(outcome.undoUntil)
        XCTAssertEqual(outcome.myVote?.order, 6)
        XCTAssertTrue(model.canUndo)
        XCTAssertEqual(model.hint(holding: false), "Yours was the 6th vote \u{00B7} 1 more yes passes it")
        XCTAssertTrue(ProposalVoteCopy.toast(outcome: outcome).offersUndo)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, [votesPath])
    }

    func testASettlingResultExposesNoUndoAndReadsTheProposalOnce() async throws {
        let transport = StubTransport(
            routes: [
                votesPath: [.json(.ok, castBody(status: "passed", yes: 5, no: 1, undoIn: nil, decided: true))],
                detailPath: [try detail(yes: 5)],
            ])
        let model = model(transport)
        let voted = await model.vote(.yes)
        let outcome = try XCTUnwrap(voted)
        XCTAssertTrue(outcome.settled)
        XCTAssertFalse(model.canUndo)
        XCTAssertFalse(ProposalVoteCopy.toast(outcome: outcome).offersUndo)
        let reads = await transport.sent.filter { $0.path == detailPath }
        XCTAssertEqual(reads.count, 1)
        XCTAssertEqual(model.hint(holding: false), "Yours was the 5th yes \u{00B7} at 5 of 8 it passes")
        XCTAssertEqual(model.subhead(now: clock), "AC Capital \u{00B7} passed 5\u{2013}1 \u{00B7} buying ≈ 1.82 sh")
    }

    func testUndoCallsTheRetractAndReturnsToIdle() async throws {
        let transport = StubTransport(
            routes: [votesPath: [.json(.ok, castBody(yes: 5, no: 1)), .json(.ok, retracted)]])
        let model = model(transport)
        _ = await model.vote(.yes)
        let undone = await model.undo()
        XCTAssertTrue(undone)
        XCTAssertEqual(model.phase, .idle)
        XCTAssertEqual(model.tally.yes, 4)
        let sent = await transport.sent.filter { $0.path == votesPath }
        XCTAssertEqual(sent.map(\.method), [.post, .delete])
    }

    func testAHintForTheProposalReloadsItAndAHintForAnotherDoesNot() async throws {
        let transport = StubTransport(routes: [detailPath: [try detail(yes: 6, replies: 9)]])
        let hints = FakeHintStream()
        let model = model(transport, hints: hints)
        let observing = Task { await model.observe() }
        await hints.waitForSubscribers(1)
        await hints.send(.changed(.cabal("cabal-1"), what: "proposal_updated", id: "proposal-2"))
        await hints.send(.changed(.cabal("cabal-1"), what: "proposal_updated", id: proposalID))
        await transport.waitForRequests(1)
        for _ in 0..<5_000 where model.tally.yes != 6 { await Task.yield() }
        observing.cancel()
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), [detailPath])
        XCTAssertEqual(model.tally.yes, 6)
        XCTAssertEqual(model.replyCount, 9)
    }

    func testACommittedCardKeepsFollowingTheTallyAndTheReplies() async throws {
        let transport = StubTransport(
            routes: [
                votesPath: [.json(.ok, castBody(yes: 5, no: 1))],
                detailPath: [try detail(yes: 6, replies: 9)],
            ])
        let model = model(transport)
        _ = await model.vote(.yes)
        XCTAssertTrue(model.isCommitted)
        await model.reload()
        XCTAssertEqual(model.tally.yes, 6)
        XCTAssertEqual(model.replyCount, 9)
        XCTAssertTrue(model.isCommitted)
        var next = row()
        next = HomePendingVote(
            id: next.id, cabalID: next.cabalID, cabalName: next.cabalName, kind: next.kind, rule: next.rule,
            symbol: next.symbol, assetName: next.assetName, usdcMicros: next.usdcMicros,
            tally: ProposalTally(yes: 7, no: 1, voters: 8, needed: 5), replyCount: 11, expiresAt: next.expiresAt)
        model.update(next)
        XCTAssertEqual(model.tally.yes, 7)
        XCTAssertEqual(model.replyCount, 11)
    }

    func testAClosedProposalShowsTheServersMessage() async throws {
        let transport = StubTransport(
            routes: [
                votesPath: [problem("proposal_closed", "Voting has closed.")],
                detailPath: [try detail()],
            ])
        let model = model(transport)
        let outcome = await model.vote(.yes)
        XCTAssertNil(outcome)
        XCTAssertEqual(model.phase, .failed("Voting has closed."))
        XCTAssertEqual(model.errorMessage, "Voting has closed.")
        model.dismissFailure()
        XCTAssertEqual(model.phase, .idle)
    }

    func testARobinhoodSellRowScalesItsSharesByTheListingDecimals() {
        let robinhood = HomePendingVote(
            id: proposalID, cabalID: "cabal-1", cabalName: "AC Capital", kind: "sell", symbol: "AAPLx",
            assetName: "Apple", tokenAmount: 500_000_000_000_000_000, tokenDecimals: 18,
            tally: ProposalTally(yes: 0, no: 0, voters: 3, needed: 2), replyCount: 0,
            expiresAt: clock.addingTimeInterval(14_400))
        let solana = HomePendingVote(
            id: proposalID, cabalID: "cabal-1", cabalName: "AC Capital", kind: "sell", symbol: "AAPLx",
            assetName: "Apple", tokenAmount: 50_000_000, tally: robinhood.tally, replyCount: 0,
            expiresAt: clock.addingTimeInterval(14_400))
        let transport = StubTransport(routes: [:])

        XCTAssertEqual(model(transport, row: robinhood).headline, "Sell AAPL \u{00B7} 0.5 shares")
        XCTAssertEqual(model(transport, row: solana).headline, "Sell AAPL \u{00B7} 0.5 shares")
    }

    func testARuleRowBuildsTheRuleHeadlineAndABuyRowTheTradeHeadline() {
        let transport = StubTransport(routes: [:])
        XCTAssertEqual(
            model(transport, row: row(kind: "rule", rule: "show_sides")).headline,
            "Rule change \u{00B7} Show who voted what")
        XCTAssertEqual(model(transport).headline, "Buy NVDA \u{00B7} $400")
        XCTAssertEqual(
            model(transport).subhead(now: clock), "AC Capital \u{00B7} 5 of 8 voted \u{00B7} 6 replies \u{00B7} 4h")
    }
}
