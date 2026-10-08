import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalTradingTests: XCTestCase {
    @MainActor
    func testListModelSeparatesVotesStillNeededFromBuysTrading() async throws {
        let items = [Self.proposal("open", id: "p"), Self.proposal("passed", id: "q")].joined(separator: ",")
        let page = #"{"proposals":[\#(items)],"next_cursor":null}"#
        let transport = StubTransport(.json(.ok, page))
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository(transport), hints: FakeHintStream())

        await model.load()

        XCTAssertEqual(model.needsVote(votedThisSession: []).map(\.id), ["p"])
        XCTAssertEqual(model.trading.map(\.id), ["q"])
    }

    @MainActor
    func testPendingModelKeepsABuyThatPassedAndDropsOneThatSettled() async throws {
        for (status, kept) in [("passed", true), ("executed", false)] {
            let model = try await Self.reloaded(settledAs: status, keeping: [])
            XCTAssertEqual(model.votes.map(\.id), kept ? ["p"] : [], status)
            XCTAssertEqual(model.details.keys.sorted(), kept ? ["p"] : [], status)
        }
    }

    @MainActor
    func testPendingModelKeepsAnOpenBuyTheMemberJustVotedOn() async throws {
        let model = try await Self.reloaded(settledAs: "open", keeping: ["p"])
        XCTAssertEqual(model.votes.map(\.id), ["p"])
    }

    @MainActor
    func testPendingPhaseGoesFromLoadingToLoadedAndSurvivesAFailedRefresh() async throws {
        let model = PendingVotesModel(
            repository: Self.repository(
                StubTransport(scripted: [.json(.ok, "[]"), .failure(URLError(.notConnectedToInternet))])),
            hints: FakeHintStream())
        XCTAssertEqual(model.phase, .loading)
        await model.load()
        XCTAssertEqual(model.phase, .loaded)
        await model.load()
        XCTAssertEqual(model.phase, .loaded)
    }

    @MainActor
    func testPendingPhaseFailsWhenTheFirstLoadFails() async throws {
        let model = PendingVotesModel(
            repository: Self.repository(StubTransport(.failure(URLError(.notConnectedToInternet)))),
            hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.phase, .failed)
    }

    func testTheTrackerReadsTheStepForAnAssistiveTechnology() {
        func label(_ status: ProposalStatus, isSell: Bool, swapFailed: Bool = false) -> String {
            ProposalStepper.make(
                status: status, isSell: isSell, swapFailed: swapFailed, expiresAt: Date(timeIntervalSince1970: 0),
                failureMessage: "Price moved too far"
            ).accessibilityLabel
        }
        XCTAssertEqual(
            label(.passed, isSell: false),
            "Voting, step 1 of 3, done. Buying, step 2 of 3, in progress. Bought, step 3 of 3, to do")
        XCTAssertTrue(label(.open, isSell: true).hasPrefix("Voting, step 1 of 3, in progress, Closes "))
        XCTAssertEqual(
            label(.executed, isSell: true),
            "Voting, step 1 of 3, done. Selling, step 2 of 3, done. Sold, step 3 of 3, done")
        XCTAssertEqual(
            label(.passed, isSell: false, swapFailed: true),
            "Voting, step 1 of 2, done. Buying, step 2 of 2, failed, Price moved too far")
    }

    @MainActor
    private static func reloaded(settledAs status: String, keeping: Set<String>) async throws -> PendingVotesModel {
        let pending =
            #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2099-01-01T00:00:00Z"}]"#
        let transport = StubTransport(scripted: [
            .json(.ok, pending), .json(.ok, detail("open")), .json(.ok, "{}"),
            .json(.ok, "[]"), .json(.ok, detail(status)),
        ])
        let model = PendingVotesModel(repository: repository(transport), hints: FakeHintStream())
        await model.load()
        await model.load(keeping: keeping)
        return model
    }

    private static func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    private static func proposal(_ status: String, id: String) -> String {
        #"{"id":"\#(id)","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"can_vote":true}"#
    }

    private static func detail(_ status: String) -> String {
        #"{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[{"user_id":"u","choice":null,"cast_at":null}],"can_vote":false,"can_withdraw":false,"swap":null}"#
    }
}
