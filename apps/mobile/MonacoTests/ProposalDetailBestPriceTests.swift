import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct ProposalDetailBestPriceTests {
    private let now = Date(timeIntervalSince1970: 1_791_100_800)

    private func model(for detail: Components.Schemas.ProposalDetail) async throws -> ProposalDetailModel {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = String(decoding: try encoder.encode(detail), as: UTF8.self)
        let transport = StubTransport(scripted: [.json(.ok, body)])
        let model = ProposalDetailModel(
            id: detail.id, cabalID: detail.cabalId,
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)),
            hints: FakeHintSource())
        await model.load()
        return model
    }

    @Test(.timeLimit(.minutes(1)))
    func aProposalLoadedOnlyThroughItsDetailShowsTheBestPriceRowAndSheet() async throws {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.priceComparison = BestPrice.sampleComparison(now: now)

        let bestPrice = try #require(try await model(for: detail).summary?.bestPrice)

        #expect(BestPriceSection.figure(bestPrice) == "$249.25 a share")
        #expect(bestPrice.headline == "Best price across 3 venues")
        #expect(
            BestPriceSheet.rows(bestPrice).map(\.name) == [
                "Uniswap on Base", "Jupiter on Solana", "Uniswap on Ethereum",
            ])
        #expect(BestPriceSheet.rows(bestPrice).map(\.isWinner) == [true, false, false])
    }

    @Test(.timeLimit(.minutes(1)))
    func aDetailWithNoComparisonShowsNoRow() async throws {
        let model = try await model(for: .sample())
        #expect(model.summary != nil)
        #expect(model.summary?.bestPrice == nil)
    }
}
