import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class VoteHoldModelTests: XCTestCase {
    private func json(_ detail: Components.Schemas.ProposalDetail) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(detail), as: UTF8.self)
    }

    private func model(
        _ detail: Components.Schemas.ProposalDetail, pick: ProposalBallot? = .yes
    ) throws -> VoteHoldModel {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"),
            transport: StubTransport(.json(.ok, try json(detail))))
        return VoteHoldModel(
            proposalID: "proposal-1", pick: pick, repository: ProposalsRepository(api: api), hints: FakeHintStream())
    }

    func testItIsLoadingBeforeTheFirstLoadWithThePickAlreadyOnTheSelection() async throws {
        let model = try model(.sample(canVote: true))
        XCTAssertEqual(model.phase, .loading)
        XCTAssertEqual(model.detail.voteSelection, .picked(.yes))
    }

    func testAnOpenVotableProposalIsReadyAndKeepsThePickThroughALoadAndAReload() async throws {
        let model = try model(.sample(canVote: true))
        await model.load()
        XCTAssertEqual(model.phase, .ready(picked: .yes))
        await model.load()
        XCTAssertEqual(model.phase, .ready(picked: .yes))
        XCTAssertEqual(model.detail.voteSelection, .picked(.yes))
    }

    func testWithoutAPickItIsReadyWithNothingPicked() async throws {
        let model = try model(.sample(canVote: true), pick: nil)
        await model.load()
        XCTAssertEqual(model.phase, .ready(picked: nil))
        XCTAssertEqual(model.detail.voteSelection, .none)
    }

    func testAProposalYouAlreadyVotedOnDropsThePick() async throws {
        let model = try model(.sample(myBallot: .no, canVote: true), pick: .yes)
        await model.load()
        XCTAssertEqual(model.phase, .alreadyVoted(.no))
        XCTAssertEqual(model.detail.voteSelection, .committed(.no))
    }

    func testAClosedProposalIgnoresThePick() async throws {
        let model = try model(.sample(status: .passed), pick: .yes)
        await model.load()
        XCTAssertEqual(model.phase, .closed)
        XCTAssertEqual(model.detail.voteSelection, .none)
    }

    func testANonVoterIsViewOnlyAndLosesThePick() async throws {
        let model = try model(.sample(), pick: .yes)
        await model.load()
        XCTAssertEqual(model.phase, .viewOnly)
        XCTAssertEqual(model.detail.voteSelection, .none)
    }

    func testALoadThatFailsIsFailed() async throws {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"),
            transport: StubTransport(.failure(URLError(.notConnectedToInternet))))
        let model = VoteHoldModel(
            proposalID: "proposal-1", pick: .yes, repository: ProposalsRepository(api: api), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.phase, .failed)
    }

    func testAFailedCommitReturnsThePickSoAHoldRetries() async throws {
        let transport = StubTransport(
            scripted: [
                .json(.ok, try json(.sample(canVote: true))), .json(.ok, "{}"), .json(.ok, "{}"),
                .failure(URLError(.notConnectedToInternet)),
            ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = VoteHoldModel(
            proposalID: "proposal-1", pick: .yes, repository: ProposalsRepository(api: api), hints: FakeHintStream())
        await model.load()
        let sent = await model.commit(.yes)
        XCTAssertFalse(sent)
        XCTAssertEqual(model.detail.voteSelection, .picked(.yes))
        XCTAssertEqual(model.phase, .ready(picked: .yes))
    }
}
