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
        defer { TestWindow.close(window) }

        await Self.ready(model, hints)
        hints.send(.changed(.cabal("cabal-1"), what: "proposal_created", id: "1"))
        await ProposalTestWindow.until { model.votes.map(\.id) == ["p"] }

        #expect(model.votes.map(\.id) == ["p"])
    }

    @Test(.timeLimit(.minutes(1)))
    func seeAllShowsAProposalAnotherMemberOpensWhileItIsOnScreen() async throws {
        let (model, hints) = Self.model()
        let window = try ProposalTestWindow.hosting(PendingVotesScreen(makeModel: { _ in model }))
        defer { TestWindow.close(window) }

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
        #"[{"proposal_id":"p","cabal_id":"cabal-1","cabal_name":"Cabal","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"asset_name":"Asset","asset_logo_url":null,"proposer":{"user_id":"u","handle":null,"display_name":"","photo_url":null},"tally":{"yes":0,"no":0,"voters":1,"needed":1},"voted_count":0,"reply_count":0,"created_at":"2025-01-01T00:00:00Z","expires_at":"2099-01-01T00:00:00Z"}]"#

    private static let detail =
        #"{"id":"p","cabal_id":"cabal-1","cabal_name":"Cabal","proposer_id":"u","proposer_return_1m_bps":null,"proposer_return_private":false,"kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[],"can_vote":true,"stats":{"treasury_share_bps":null,"closes_at":"2026-10-04T15:00:00Z","closed_at":null},"my_vote":null,"reply_count":0,"can_withdraw":false,"swap":null}"#
}
