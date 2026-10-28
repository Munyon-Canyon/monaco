import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@Suite(.serialized)
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
        let rows = BestPriceSheet.rows(bestPrice)
        #expect(rows.map(\.name) == ["Base", "Solana", "Ethereum", "Monad"])
        #expect(rows.map(\.isWinner) == [true, false, false, false])
        #expect(rows.last?.figure == "No price yet")
        #expect(rows.last?.isUnavailable == true)
    }

    @Test(.timeLimit(.minutes(1)))
    func anOpenBuyShowsTheLiveComparisonWithItsAgeInPlaceOfTheStoredOne() async throws {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.priceComparison = BestPrice.sampleComparison(now: now)
        let quotedAt = Date()
        let live = Components.Schemas.ProposalPriceComparison(quoteOutAmount: 10_000_000, quotedAt: quotedAt)
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let transport = StubTransport(routes: [
            "/v1/proposals/\(detail.id)": [.json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))],
            "/v1/cabals/\(detail.cabalId)/proposals/\(detail.id)/price-comparison": [
                .json(.ok, String(decoding: try encoder.encode(live), as: UTF8.self))
            ],
        ])
        let model = ProposalDetailModel(
            id: detail.id, cabalID: detail.cabalId,
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)),
            hints: FakeHintSource())
        await model.load()
        let summary = try #require(model.summary)
        #expect(model.livePrice.row(for: summary, decimals: 8, now: quotedAt)?.caption == "Price when proposed")

        model.setVisible(true)
        while model.livePrice.latest == nil { await Task.yield() }
        model.setVisible(false)

        let row = try #require(model.livePrice.row(for: summary, decimals: 8, now: quotedAt.addingTimeInterval(12)))
        #expect(BestPriceSection(row: row).row.figure == "$250.00 a share")
        #expect(row.headline == "Solana")
        #expect(row.caption == "Updated 12s ago")
        #expect(row.venues == nil)
    }

    @Test(.timeLimit(.minutes(1)))
    func aDetailWithNoComparisonShowsNoRow() async throws {
        let model = try await model(for: .sample())
        #expect(model.summary != nil)
        #expect(model.summary?.bestPrice == nil)
    }
}
