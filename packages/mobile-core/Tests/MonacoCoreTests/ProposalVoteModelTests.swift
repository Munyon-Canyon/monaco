import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalVoteModelTests: XCTestCase {
    private let voted =
        #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":1,"no":0,"voters":3,"needed":2},"my_ballot":"yes","my_vote":{"order":1,"of":3,"yes_order":1,"no_order":0,"decided":false}}"#

    private func model(_ transport: StubTransport) -> ProposalVoteModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return ProposalVoteModel(repository: ProposalsRepository(api: api))
    }

    private func summary(ballot: Components.Schemas.Proposal.MyBallotPayload? = nil) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample()
        proposal.myBallot = ballot
        return ProposalSummary(proposal, canVote: true)
    }

    func testAVoteIsCountedAtOnceAndPostedToTheProposal() async throws {
        let transport = StubTransport(.json(.ok, voted))
        let model = model(transport)
        let proposal = summary()
        let sent = await model.vote("yes", on: proposal)
        XCTAssertTrue(sent)
        let shown = model.applying(proposal)
        XCTAssertEqual(shown.myBallot, "yes")
        XCTAssertEqual(shown.tally.yes, 1)
        XCTAssertEqual(shown.tally.no, 0)
        let sentRequests = await transport.sent
        let request = try XCTUnwrap(sentRequests.first)
        XCTAssertEqual(request.path, "/v1/proposals/proposal-1/votes")
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(request.headerFields[key])
    }

    func testChangingABallotMovesTheCountAndOnlyOnce() async {
        let model = model(
            StubTransport(
                .json(
                    .ok,
                    #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":0,"no":1,"voters":3,"needed":2},"my_ballot":"no","my_vote":{"order":1,"of":3,"yes_order":1,"no_order":0,"decided":false}}"#
                )))
        let proposal = summary(ballot: .yes)
        _ = await model.vote("no", on: proposal)
        let shown = model.applying(
            ProposalSummary(
                proposal, ballot: "yes", tally: ProposalTally(yes: 1, no: 0, voters: 3, needed: 2)))
        XCTAssertEqual(shown.myBallot, "no")
        XCTAssertEqual(shown.tally.yes, 0)
        XCTAssertEqual(shown.tally.no, 1)
        let refreshed = ProposalSummary(
            proposal, ballot: "no", tally: ProposalTally(yes: 0, no: 1, voters: 3, needed: 2))
        XCTAssertEqual(model.applying(refreshed), refreshed)
    }

    func testAFailedVoteIsRolledBackWithTheServerMessage() async {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let model = model(transport)
        let proposal = summary()
        let sent = await model.vote("yes", on: proposal)
        XCTAssertFalse(sent)
        XCTAssertEqual(model.errorMessage, "You're offline. Try again.")
        XCTAssertEqual(model.applying(proposal), proposal)
    }

    func testAVoteAfterAnUndoThatNeverGotAnAnswerSendsTheNextUndoUnderAFreshKey() async throws {
        let retracted =
            #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":0,"no":0,"voters":3,"needed":2}}"#
        let transport = StubTransport(scripted: [
            .json(.internalServerError, #"{"message":"Try again"}"#), .json(.ok, voted), .json(.ok, retracted),
        ])
        let model = model(transport)
        let proposal = summary()
        let first = await model.undo(on: proposal)
        XCTAssertNil(first)
        _ = await model.vote("yes", on: proposal)
        let second = await model.undo(on: proposal)
        XCTAssertNotNil(second)
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let sent = await transport.sent.filter { $0.method == .delete }
        XCTAssertEqual(sent.count, 2)
        XCTAssertNotEqual(sent[0].headerFields[key], sent[1].headerFields[key])
    }

    func testARetryAfterAnErrorReplaysTheSameKey() async throws {
        let transport = StubTransport(scripted: [
            .json(.internalServerError, #"{"message":"Try again"}"#), .json(.ok, voted),
        ])
        let model = model(transport)
        let proposal = summary()
        _ = await model.vote("yes", on: proposal)
        let sent = await model.vote("yes", on: proposal)
        XCTAssertTrue(sent)
        XCTAssertNil(model.errorMessage)
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.compactMap { $0.headerFields[key] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys.first, keys.last)
    }

    private let retracted =
        #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":0,"no":0,"voters":3,"needed":2}}"#

    func testServerBallotWinsAfterAcknowledgement() async {
        let model = model(StubTransport(.json(.ok, voted)))
        let proposal = summary()
        _ = await model.vote("yes", on: proposal)
        let server = ProposalSummary(
            proposal, ballot: "no", tally: ProposalTally(yes: 0, no: 1, voters: 3, needed: 2))
        XCTAssertEqual(model.applying(server).myBallot, "yes")
        model.acknowledge(server)
        XCTAssertEqual(model.applying(server), server)
        XCTAssertTrue(model.votedIDs.contains(proposal.id))
    }

    func testAServerAnswerWithNoBallotLeavesTheOverlayAlone() async {
        let model = model(StubTransport(.json(.ok, voted)))
        let proposal = summary()
        _ = await model.vote("yes", on: proposal)
        model.acknowledge(proposal)
        XCTAssertEqual(model.applying(proposal).myBallot, "yes")
    }

    func testUndoClearsTheOverlay() async throws {
        let transport = StubTransport(scripted: [.json(.ok, voted), .json(.ok, retracted)])
        let model = model(transport)
        let proposal = summary()
        _ = await model.vote("yes", on: proposal)
        let result = await model.undo(on: proposal)
        XCTAssertEqual(result?.tally.yes, 0)
        XCTAssertEqual(model.applying(proposal), proposal)
        XCTAssertFalse(model.votedIDs.contains(proposal.id))
        let sent = await transport.sent
        let request = try XCTUnwrap(sent.last)
        XCTAssertEqual(request.method, .delete)
        XCTAssertEqual(request.path, "/v1/proposals/proposal-1/votes")
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(request.headerFields[key])
    }

    func testAnExpiredUndoKeepsTheVoteAndTheServerMessage() async {
        let problem = """
            {"type":"about:blank","title":"Conflict","status":409,"code":"undo_expired",\
            "message":"It's too late to undo this vote.","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
            """
        let model = model(
            StubTransport(scripted: [
                .json(.ok, voted),
                .response(status: .conflict, contentType: "application/problem+json", body: Data(problem.utf8)),
            ]))
        let proposal = summary()
        _ = await model.vote("yes", on: proposal)
        let result = await model.undo(on: proposal)
        XCTAssertNil(result)
        XCTAssertEqual(model.errorMessage, "It's too late to undo this vote.")
        XCTAssertEqual(model.undoRefusal, .expired(message: "It's too late to undo this vote."))
        XCTAssertEqual(model.applying(proposal).myBallot, "yes")
    }

    func testTheVoteResultKeepsTheUndoWindowAndThePlaceInTheVote() async throws {
        let until = Date().addingTimeInterval(15)
        let body =
            #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":1,"no":0,"voters":8,"needed":5},"my_ballot":"yes","undo_until":"\#(ISO8601DateFormatter().string(from: until))","my_vote":{"order":6,"of":8,"yes_order":1,"no_order":0,"decided":false}}"#
        let model = model(StubTransport(.json(.ok, body)))
        let outcome = await model.cast("yes", on: summary())
        let result = try XCTUnwrap(outcome)
        XCTAssertEqual(try XCTUnwrap(result.undoUntil).timeIntervalSince1970, until.timeIntervalSince1970, accuracy: 1)
        XCTAssertEqual(result.myVote, ProposalMyVote(order: 6, of: 8, yesOrder: 1, noOrder: 0, decided: false))
    }
}

@MainActor
final class ProposalDetailVoteTests: XCTestCase {
    private static let retracted =
        #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":4,"no":1,"voters":8,"needed":5}}"#
    private let detailPath = "/v1/proposals/proposal-1"
    private let votesPath = "/v1/proposals/proposal-1/votes"

    private func detail(
        status: Components.Schemas.ProposalStatus = .open, ballot: Components.Schemas.BallotChoice? = nil,
        canVote: Bool = true, yes: Int = 4, no: Int = 1
    ) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var value = Components.Schemas.ProposalDetail.sample(status: status, myBallot: ballot, canVote: canVote)
        value.tally = .init(yes: yes, no: no, voters: 8, needed: 5)
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func castBody(
        status: String = "open", yes: Int, no: Int, ballot: String = "yes", undoIn: TimeInterval? = 15, order: Int = 6,
        yesOrder: Int = 4, decided: Bool = false
    ) -> String {
        let until = undoIn.map { "\"\(ISO8601DateFormatter().string(from: Date().addingTimeInterval($0)))\"" } ?? "null"
        return
            #"{"proposal_id":"proposal-1","status":"\#(status)","tally":{"yes":\#(yes),"no":\#(no),"voters":8,"needed":5},"my_ballot":"\#(ballot)","undo_until":\#(until),"my_vote":{"order":\#(order),"of":8,"yes_order":\#(yesOrder),"no_order":0,"decided":\#(decided)}}"#
    }

    private func problem(_ code: String, _ message: String) -> StubTransport.Reply {
        let body = """
            {"type":"about:blank","title":"Conflict","status":409,"code":"\(code)","message":"\(message)",\
            "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
            """
        return .response(status: .conflict, contentType: "application/problem+json", body: Data(body.utf8))
    }

    private func model(_ transport: StubTransport) -> ProposalDetailModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: ProposalsRepository(api: api), hints: FakeHintStream())
    }

    func testAnOpenProposalWithNoBallotStartsIdleAndWithABallotStartsCommitted() async throws {
        let idle = model(StubTransport(routes: [detailPath: [try detail()]]))
        await idle.load()
        XCTAssertEqual(idle.voteSelection, .none)
        let voted = model(StubTransport(routes: [detailPath: [try detail(ballot: .no)]]))
        await voted.load()
        XCTAssertEqual(voted.voteSelection, .committed(.no))
    }

    func testAnOrdinaryVoteCommitsAndKeepsTheUndoWindow() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5)],
                votesPath: [.json(.ok, castBody(yes: 5, no: 1))],
            ])
        let model = model(transport)
        await model.load()
        let accepted = await model.vote("yes")
        XCTAssertTrue(accepted)
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        let outcome = try XCTUnwrap(model.lastVote)
        XCTAssertFalse(outcome.settled)
        XCTAssertEqual(outcome.myVote?.order, 6)
        XCTAssertTrue(model.canUndo(now: Date()))
        XCTAssertFalse(model.canUndo(now: Date().addingTimeInterval(16)))
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome), VoteToast(message: "Voted Yes. 6th of 8.", offersUndo: true))
        XCTAssertNil(model.settledLine)
    }

    func testTheVoteThatReachesQuorumSettlesWithNoUndo() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(status: .passed, ballot: .yes, canVote: false, yes: 5)],
                votesPath: [
                    .json(.ok, castBody(status: "passed", yes: 5, no: 1, undoIn: nil, yesOrder: 5, decided: true))
                ],
            ])
        let model = model(transport)
        await model.load()
        _ = await model.vote("yes")
        let outcome = try XCTUnwrap(model.lastVote)
        XCTAssertTrue(outcome.settled)
        XCTAssertFalse(model.canUndo(now: Date()))
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome), VoteToast(message: "Voted Yes. Passed 5–1.", offersUndo: false))
        XCTAssertEqual(model.settledLine, "Passed 5–1 · 6 of 8 voted")
        XCTAssertEqual(model.decidingLine, "Yours was the 5th yes · at 5 of 8 it passes")
    }

    func testChangingASideMovesTheSelectionAndToastsWithNoUndo() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(ballot: .yes), try detail(ballot: .no, yes: 3, no: 2)],
                votesPath: [.json(.ok, castBody(yes: 3, no: 2, ballot: "no", undoIn: nil, yesOrder: 0))],
            ])
        let model = model(transport)
        await model.load()
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        model.beginChange()
        XCTAssertEqual(model.voteSelection, .picked(.no))
        _ = await model.vote("no")
        XCTAssertEqual(model.voteSelection, .committed(.no))
        let outcome = try XCTUnwrap(model.lastVote)
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome), VoteToast(message: "Changed to No.", offersUndo: false))
        XCTAssertFalse(model.canUndo(now: Date()))
    }

    func testARefusedVoteReturnsToPickedAndAHoldAgainSendsANewRequest() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5)],
                votesPath: [
                    .json(.internalServerError, #"{"message":"Try again"}"#),
                    .json(.ok, castBody(yes: 5, no: 1)),
                ],
            ])
        let model = model(transport)
        await model.load()
        let first = await model.vote("yes")
        XCTAssertFalse(first)
        XCTAssertEqual(model.voteSelection, .picked(.yes))
        XCTAssertNotNil(model.errorMessage)
        XCTAssertNil(model.lastVote)
        let second = await model.vote("yes")
        XCTAssertTrue(second)
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        let sent = await transport.sent.filter { $0.path == votesPath }
        XCTAssertEqual(sent.count, 2)
    }

    func testAVoteWhileOneIsPendingSendsNothing() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5)],
                votesPath: [.gate],
            ])
        let model = model(transport)
        await model.load()
        let first = Task { await model.vote("yes") }
        await transport.waitForRequest(path: votesPath)
        XCTAssertEqual(model.voteSelection, .pending(.yes))
        let second = await model.vote("yes")
        XCTAssertFalse(second)
        await transport.releaseGate(.json(.ok, castBody(yes: 5, no: 1)))
        let accepted = await first.value
        XCTAssertTrue(accepted)
        let sent = await transport.sent.filter { $0.path == votesPath }
        XCTAssertEqual(sent.count, 1)
    }

    func testNotAVoterReloadsIntoTheReadOnlyCard() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(canVote: false)],
                votesPath: [problem("not_a_voter", "Only voters from when it opened can vote.")],
            ])
        let model = model(transport)
        await model.load()
        XCTAssertEqual(model.viewerRole(viewerID: "viewer"), .voter)
        let accepted = await model.vote("yes")
        XCTAssertFalse(accepted)
        XCTAssertEqual(model.voteRefusal, .notAVoter(message: "Only voters from when it opened can vote."))
        XCTAssertEqual(model.voteSelection, .none)
        XCTAssertEqual(model.viewerRole(viewerID: "viewer"), .nonMember)
    }

    func testAClosedAnswerReloadsIntoTheClosedState() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(status: .failed, canVote: false, yes: 2, no: 3)],
                votesPath: [problem("proposal_closed", "Voting has closed.")],
            ])
        let model = model(transport)
        await model.load()
        _ = await model.vote("yes")
        XCTAssertEqual(model.voteRefusal, .closed(message: "Voting has closed."))
        XCTAssertEqual(model.errorMessage, "Voting has closed.")
        XCTAssertEqual(model.summary?.status, .failed)
        XCTAssertEqual(model.voteSelection, .none)
    }

    func testUndoRestoresTheIdleCardAndTheTally() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5), try detail()],
                votesPath: [
                    .json(.ok, castBody(yes: 5, no: 1)),
                    .json(.ok, Self.retracted),
                ],
            ])
        let model = model(transport)
        await model.load()
        _ = await model.vote("yes")
        let undone = await model.undo()
        XCTAssertTrue(undone)
        XCTAssertNil(model.lastVote)
        XCTAssertEqual(model.voteSelection, .none)
        XCTAssertEqual(model.summary?.tally.yes, 4)
        XCTAssertNil(model.summary?.myBallot)
        let sent = await transport.sent.filter { $0.path == votesPath }
        XCTAssertEqual(sent.map(\.method), [.post, .delete])
    }

    func testAnExpiredUndoShowsTheServerMessageAndKeepsTheVote() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), try detail(ballot: .yes, yes: 5)],
                votesPath: [
                    .json(.ok, castBody(yes: 5, no: 1)), problem("undo_expired", "It's too late to undo this vote."),
                ],
            ])
        let model = model(transport)
        await model.load()
        _ = await model.vote("yes")
        let undone = await model.undo()
        XCTAssertFalse(undone)
        XCTAssertEqual(model.errorMessage, "It's too late to undo this vote.")
        XCTAssertEqual(model.undoRefusal, .expired(message: "It's too late to undo this vote."))
        XCTAssertNotNil(model.lastVote)
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        XCTAssertEqual(model.summary?.myBallot, "yes")
    }

    func testAMemberOutsideTheFrozenVoterSetIsNotANonMember() async throws {
        let cabal = Components.Schemas.Cabal.sampleWithMembers(role: "member")
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let cabalJSON = String(decoding: try encoder.encode(cabal), as: UTF8.self)
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(canVote: false)],
                "/v1/cabals/\(Components.Schemas.Cabal.sample(role: nil).id)": [.json(.ok, cabalJSON)],
                "/v1/assets/GOOGLx": [.json(.internalServerError, "{}")],
            ])
        let model = model(transport)
        await model.load()
        let member = try XCTUnwrap(cabal.members.first).userId
        XCTAssertEqual(model.viewerRole(viewerID: member), .memberNotVoter)
        XCTAssertEqual(model.viewerRole(viewerID: "someone-else"), .nonMember)
        XCTAssertEqual(model.viewerRole(viewerID: nil), .nonMember)
    }
}

extension ProposalDetailVoteTests {
    func testChangeDuringThePostVoteReloadLeavesTheCommittedBallotAlone() async throws {
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), .gate],
                votesPath: [.json(.ok, castBody(yes: 5, no: 1))],
            ])
        let model = model(transport)
        await model.load()
        let voting = Task { await model.vote("yes") }
        for _ in 0..<50_000 {
            let reloads = await transport.sent.filter { $0.path == detailPath }.count
            if reloads >= 2 { break }
            await Task.yield()
        }
        for _ in 0..<200 { await Task.yield() }
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        model.beginChange()
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var reloaded = Components.Schemas.ProposalDetail.sample(status: .open, myBallot: .yes, canVote: true)
        reloaded.tally = .init(yes: 5, no: 1, voters: 8, needed: 5)
        await transport.releaseGate(.json(.ok, String(decoding: try encoder.encode(reloaded), as: UTF8.self)))
        _ = await voting.value
        XCTAssertEqual(model.voteSelection, .committed(.yes))
        model.beginChange()
        XCTAssertEqual(model.voteSelection, .picked(.no))
    }

    func testTheSettledLineOfAPassedBuyKeepsTheApproximateSign() async throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var passed = Components.Schemas.ProposalDetail.passedNVDA()
        passed.myBallot = .yes
        passed.canVote = false
        passed.tally = .init(yes: 5, no: 1, voters: 8, needed: 5)
        let transport = StubTransport(
            routes: [
                detailPath: [try detail(), .json(.ok, String(decoding: try encoder.encode(passed), as: UTF8.self))],
                votesPath: [
                    .json(.ok, castBody(status: "passed", yes: 5, no: 1, undoIn: nil, yesOrder: 5, decided: true))
                ],
            ])
        let model = model(transport)
        await model.load()
        _ = await model.vote("yes")
        XCTAssertEqual(model.settledLine, "Passed 5–1 · 6 of 8 voted · buying ≈ 1.82 sh")
    }
}
