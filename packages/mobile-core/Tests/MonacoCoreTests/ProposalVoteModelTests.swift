import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalVoteModelTests: XCTestCase {
    private let voted =
        #"{"proposal_id":"proposal-1","status":"open","tally":{"yes":1,"no":0,"voters":3,"needed":2},"my_ballot":"yes"}"#

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
        let model = model(StubTransport(.json(.ok, voted)))
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
}
