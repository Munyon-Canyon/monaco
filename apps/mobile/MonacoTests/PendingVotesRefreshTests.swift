import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct PendingVotesRefreshTests {
    @Test(.timeLimit(.minutes(1)))
    func homeShowsAProposalAnotherMemberOpensWhileItIsOnScreen() async throws {
        let (model, hints) = Self.model()
        let window = try ProposalTestWindow.hosting(HomePendingVotes(makeModel: { _ in model }))
        defer { window.isHidden = true }

        await Self.ready(model, hints)
        hints.send(.changed(.cabal("cabal-1"), what: "proposal_created", id: "1"))
        await ProposalTestWindow.until { model.votes.map(\.id) == ["p"] }

        #expect(model.votes.map(\.id) == ["p"])
    }

    @Test(.timeLimit(.minutes(1)))
    func seeAllShowsAProposalAnotherMemberOpensWhileItIsOnScreen() async throws {
        let (model, hints) = Self.model()
        let window = try ProposalTestWindow.hosting(PendingVotesScreen(makeModel: { _ in model }))
        defer { window.isHidden = true }

        await Self.ready(model, hints)
        hints.send(.changed(.cabal("cabal-1"), what: "proposal_created", id: "1"))
        await ProposalTestWindow.until { model.votes.map(\.id) == ["p"] }

        #expect(model.votes.map(\.id) == ["p"])
    }

    private static func model() -> (PendingVotesModel, EmittingHintSource) {
        let transport = StubTransport(routes: [
            "/v1/me/pending-votes": [.json(.ok, "[]"), .json(.ok, pending)],
            "/v1/proposals/p": [.json(.ok, detail)],
        ])
        let hints = EmittingHintSource()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (PendingVotesModel(repository: ProposalsRepository(api: api), hints: hints), hints)
    }

    private static func ready(_ model: PendingVotesModel, _ hints: EmittingHintSource) async {
        await ProposalTestWindow.until { model.phase == .loaded && hints.subscribers == 3 }
        model.setVisible(true)
    }

    private static let pending =
        #"[{"proposal_id":"p","cabal_id":"cabal-1","kind":"buy","symbol":"AAPLx","expires_at":"2099-01-01T00:00:00Z"}]"#

    private static let detail =
        #"{"id":"p","cabal_id":"cabal-1","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[],"can_vote":true,"can_withdraw":false,"swap":null}"#
}
