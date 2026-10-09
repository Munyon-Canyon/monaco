import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct PassedProposalBuyingTests {
    @Test
    func theCabalPageShowsAPassedBuyAsTrading() async throws {
        let page = #"{"proposals":[\#(Self.proposal(status: "passed"))],"next_cursor":null}"#
        let transport = StubTransport(.json(.ok, page))
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository(transport), hints: FakeHintSource())

        await model.load()

        #expect(model.trading.map(\.id) == ["p"])
        #expect(model.needsVote(votedThisSession: ["p"]).isEmpty)
        #expect(ProposalChip.label(status: model.trading[0].status, isSell: false) == "Buying")
        let query = await transport.sent.first?.path ?? ""
        #expect(query.contains("filter=all"))
    }

    @Test
    func theCabalPageListsAnOpenProposalUnderNeedsYourVoteAndNothingInProgress() async throws {
        let page =
            #"{"proposals":[\#(Self.proposal(status: "open", ballot: "null", canVote: true))],"next_cursor":null}"#
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository(StubTransport(.json(.ok, page))),
            hints: FakeHintSource())

        await model.load()

        #expect(model.needsVote(votedThisSession: []).map(\.id) == ["p"])
        #expect(model.trading.isEmpty)
    }

    @Test
    func homeKeepsAListedBuyThatPassedAndShowsItBuying() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.pending),
            .json(.ok, Self.detail(status: "open")),
            .json(.ok, #"{"paused":false}"#),
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, "[]"),
            .json(.ok, Self.detail(status: "passed")),
            .json(.ok, #"{"paused":false}"#),
        ])
        let model = PendingVotesModel(repository: Self.repository(transport), hints: FakeHintSource())
        await model.load()
        #expect(model.needsVote.map(\.id) == ["p"])
        #expect(model.inProgress.isEmpty)

        await model.load()

        #expect(model.votes.map(\.id) == ["p"])
        #expect(model.needsVote.isEmpty)
        #expect(model.inProgress.map(\.id) == ["p"])
        let summary = try #require(model.details["p"]?.summary)
        #expect(ProposalChip.label(status: summary.status, isSell: false) == "Buying")
    }

    @Test
    func homeKeepsTheOpenBuyItsMemberJustVotedOn() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.pending),
            .json(.ok, Self.detail(status: "open")),
            .json(.ok, #"{"paused":false}"#),
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, "[]"),
            .json(.ok, Self.detail(status: "open")),
        ])
        let model = PendingVotesModel(repository: Self.repository(transport), hints: FakeHintSource())
        await model.load()

        await model.load(keeping: ["p"])

        #expect(model.votes.map(\.id) == ["p"])
    }

    @Test
    func homeDropsAListedBuyOnceItIsNoLongerTrading() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.pending),
            .json(.ok, Self.detail(status: "open")),
            .json(.ok, #"{"paused":false}"#),
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, "[]"),
            .json(.ok, Self.detail(status: "executed")),
        ])
        let model = PendingVotesModel(repository: Self.repository(transport), hints: FakeHintSource())
        await model.load()

        await model.load(keeping: ["p"])

        #expect(model.votes.isEmpty)
        #expect(model.details.isEmpty)
        #expect(model.needsVote.isEmpty)
        #expect(model.inProgress.isEmpty)
    }

    @Test
    func aPassedBuyWhoseTradeFailedLeavesInProgress() async throws {
        let failed = Self.detail(status: "passed").replacingOccurrences(
            of: #""swap":null"#,
            with:
                #""swap":{"swap_id":"01890a5d-ac96-774b-bcce-b302099a8060","status":"failed","failure_code":"x","failure_message":"no","tx_signature":null,"retryable":true}"#
        )
        let transport = StubTransport(scripted: [
            .json(.ok, Self.pending),
            .json(.ok, Self.detail(status: "open")),
            .json(.ok, #"{"paused":false}"#),
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, "[]"),
            .json(.ok, failed),
        ])
        let model = PendingVotesModel(repository: Self.repository(transport), hints: FakeHintSource())
        await model.load()

        await model.load(keeping: ["p"])

        #expect(model.inProgress.isEmpty)
        #expect(model.votes.isEmpty)
    }

    @Test
    func theStatusTrackerReadsBuyingAtStepTwo() {
        let stepper = ProposalStepper.make(status: .passed, isSell: false, expiresAt: .now)
        #expect(stepper.steps.map(\.mark) == [.done, .active, .pending])
        #expect(stepper.accessibilityLabel.contains("Buying, step 2 of 3, in progress"))
    }

    private static let pending =
        #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"GOOGLx","expires_at":"2099-01-01T00:00:00Z"}]"#

    private static func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(api: api(transport))
    }

    private static func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private static func proposal(status: String, ballot: String = #""yes""#, canVote: Bool = false) -> String {
        #"{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"GOOGLx","usdc_micros":1000000,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":2,"no":0,"voters":2,"needed":2},"my_ballot":\#(ballot),"can_vote":\#(canVote)}"#
    }

    private static func detail(status: String) -> String {
        #"{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"GOOGLx","usdc_micros":1000000,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":2,"no":0,"voters":2,"needed":2},"my_ballot":"yes","voters":[],"can_vote":false,"can_withdraw":false,"swap":null}"#
    }
}
