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
    func aPinnedBuyWhoseChainLostItsLiveQuoteShowsTheChainUnavailableAndNoPrice() async throws {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.priceComparison = BestPrice.sampleComparison(now: now)
        detail.network = .base
        let lost = #"""
            {"quote_out_amount": 0, "quoted_at": "\#(ISO8601DateFormatter().string(from: Date()))",
             "price_comparison": {"winner_listing_id": "b", "quotes": [], "unavailable": [
               {"listing_id": "b", "network": "base", "venue": "uniswap", "reason": "simulation_failed",
                "reason_detail": "not set up on this chain yet"}]}}
            """#
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let transport = StubTransport(routes: [
            "/v1/proposals/\(detail.id)": [.json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))],
            "/v1/cabals/\(detail.cabalId)/proposals/\(detail.id)/price-comparison": [.json(.ok, lost)],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = ProposalDetailModel(
            id: detail.id, cabalID: detail.cabalId, repository: ProposalsRepository(api: api), hints: FakeHintSource())
        await model.load()
        model.setVisible(true)
        defer { model.setVisible(false) }
        while model.livePrice.latest == nil { await Task.yield() }

        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(hosting: ProposalDetailSlotView(proposalID: detail.id, model: model), api: api)
        defer { Self.release(window) }
        let unavailable = "Unavailable: not set up on this chain yet"
        await ProposalTestWindow.until {
            Self.labels(in: window).contains { $0.contains(unavailable) }
        }
        for _ in 0..<200 { await Task.yield() }

        let labels = Self.labels(in: window)
        #expect(labels.contains { $0.contains(unavailable) }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("Price when proposed") }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("a share") }, "labels: \(labels)")
    }

    @Test(.timeLimit(.minutes(1)))
    func aPinnedBuyShowsOnlyItsChainsPriceWhenAnotherChainWins() async throws {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.priceComparison = BestPrice.sampleComparison(now: now)
        detail.network = .robinhood
        let quotedAt = ISO8601DateFormatter().string(from: Date())
        let expiresAt = ISO8601DateFormatter().string(from: Date().addingTimeInterval(30))
        func quote(_ id: String, _ network: String, _ venue: String, shares: String) -> String {
            """
            {"listing_id": "\(id)", "issuer": "xstocks", "network": "\(network)", "venue": "\(venue)",
             "net_share_nanos": "\(shares)", "net_micros": "10000000", "costs": [],
             "quoted_at": "\(quotedAt)", "expires_at": "\(expiresAt)"}
            """
        }
        let live = """
            {"quote_out_amount": 40500000, "quoted_at": "\(quotedAt)",
             "price_comparison": {"winner_listing_id": "sol", "unavailable": [], "quotes": [
               \(quote("sol", "solana", "jupiter", shares: "40500000")),
               \(quote("rh", "robinhood", "uniswap", shares: "25000000"))]}}
            """
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let transport = StubTransport(routes: [
            "/v1/proposals/\(detail.id)": [.json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))],
            "/v1/cabals/\(detail.cabalId)/proposals/\(detail.id)/price-comparison": [.json(.ok, live)],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = ProposalDetailModel(
            id: detail.id, cabalID: detail.cabalId, repository: ProposalsRepository(api: api), hints: FakeHintSource())
        await model.load()
        model.setVisible(true)
        defer { model.setVisible(false) }
        while model.livePrice.latest == nil { await Task.yield() }

        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(hosting: ProposalDetailSlotView(proposalID: detail.id, model: model), api: api)
        defer { Self.release(window) }
        let pinned = "$400.00 a share on Robinhood Chain"
        await ProposalTestWindow.until {
            Self.labels(in: window).contains { $0.contains(pinned) }
        }
        for _ in 0..<200 { await Task.yield() }

        let labels = Self.labels(in: window)
        #expect(labels.contains { $0.contains(pinned) }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("Best price across") }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("Solana") }, "labels: \(labels)")
    }

    @Test(.timeLimit(.minutes(1)))
    func aPinnedBuyShowsTheBestPriceLoadingAndNoStoredPriceUntilItsFirstLiveComparison() async throws {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.priceComparison = BestPrice.sampleComparison(now: now)
        detail.network = .base
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let comparisonPath = "/v1/cabals/\(detail.cabalId)/proposals/\(detail.id)/price-comparison"
        let transport = StubTransport(routes: [
            "/v1/proposals/\(detail.id)": [.json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))],
            comparisonPath: [.hang],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = ProposalDetailModel(
            id: detail.id, cabalID: detail.cabalId, repository: ProposalsRepository(api: api), hints: FakeHintSource())
        await model.load()
        model.setVisible(true)
        defer { model.setVisible(false) }
        await transport.waitForRequest(path: comparisonPath)

        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(hosting: ProposalDetailSlotView(proposalID: detail.id, model: model), api: api)
        defer { Self.release(window) }
        let loading = ProposeAmountModel.findingPrice
        await ProposalTestWindow.until {
            Self.labels(in: window).contains { $0.contains(loading) }
        }
        for _ in 0..<200 { await Task.yield() }

        let labels = Self.labels(in: window)
        #expect(model.livePrice.latest == nil)
        #expect(labels.contains { $0.contains(loading) }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("Price when proposed") }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("a share") }, "labels: \(labels)")
    }

    @Test(.timeLimit(.minutes(1)))
    func aPinnedBuyWhoseLastLiveComparisonIsStaleShowsTheBestPriceLoadingAndNoStoredPrice() async throws {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.priceComparison = BestPrice.sampleComparison(now: now)
        detail.network = .base
        let staleAt = Date().addingTimeInterval(-(LiveBestPriceModel.staleAfter + 60))
        let stale = Components.Schemas.ProposalPriceComparison(quoteOutAmount: 10_000_000, quotedAt: staleAt)
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let transport = StubTransport(routes: [
            "/v1/proposals/\(detail.id)": [.json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))],
            "/v1/cabals/\(detail.cabalId)/proposals/\(detail.id)/price-comparison": [
                .json(.ok, String(decoding: try encoder.encode(stale), as: UTF8.self))
            ],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = ProposalDetailModel(
            id: detail.id, cabalID: detail.cabalId, repository: ProposalsRepository(api: api), hints: FakeHintSource())
        await model.load()
        model.setVisible(true)
        defer { model.setVisible(false) }
        while model.livePrice.latest == nil { await Task.yield() }

        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(hosting: ProposalDetailSlotView(proposalID: detail.id, model: model), api: api)
        defer { Self.release(window) }
        let loading = ProposeAmountModel.findingPrice
        await ProposalTestWindow.until {
            Self.labels(in: window).contains { $0.contains(loading) }
        }
        for _ in 0..<200 { await Task.yield() }

        let labels = Self.labels(in: window)
        let age = try #require(model.livePrice.latest.map { Date().timeIntervalSince($0.quotedAt) })
        #expect(age > LiveBestPriceModel.staleAfter)
        #expect(labels.contains { $0.contains(loading) }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("Price when proposed") }, "labels: \(labels)")
        #expect(!labels.contains { $0.contains("a share") }, "labels: \(labels)")
    }

    private static func labels(in window: UIWindow) -> [String] {
        AccessibilityTree.labels(in: window.rootViewController?.view ?? window)
    }

    private static func release(_ window: UIWindow) {
        window.isHidden = true
        window.rootViewController = nil
    }

    private static func window(hosting view: some View, api: APIClient) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), api: api, hints: FakeHintSource(),
            isAuthenticated: { true }, endAuthSession: {})
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { view }
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return window
    }

    @Test(.timeLimit(.minutes(1)))
    func aDetailWithNoComparisonShowsNoRow() async throws {
        let model = try await model(for: .sample())
        #expect(model.summary != nil)
        #expect(model.summary?.bestPrice == nil)
    }
}
