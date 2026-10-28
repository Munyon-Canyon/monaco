import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

extension ProposeAmountKeypadTests {
    @Test(.timeLimit(.minutes(1)))
    func theEstimateShowsAtOnceAndThePreviewIsPinnedToTheBestChain() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let service = HeldPreviewService()
        let store = Self.bestChains(answer: Self.bestChain)
        await store.refreshIfNeeded(symbol: "GOOGLx")
        let (window, _) = try Self.window(service: service, bestChains: store)
        defer { TestWindow.close(window) }
        func note() -> String? { Self.element("propose-amount-quantity", in: window)?.accessibilityLabel }
        try await ProposeAmountKeypadTests.typeOne(in: window)

        #expect(await Self.eventually(within: 10) { note() == "Estimate: about 0.004 shares on Base" })
        #expect(await service.answered == false)
        while await !service.isAsked() { await Task.yield() }
        #expect(note() == "Estimate: about 0.004 shares on Base")
        #expect(await service.networks == [.base])
        await service.answer()
        #expect(await Self.eventually(within: 10) { note() == nil })
    }

    @Test(.timeLimit(.minutes(1)))
    func withoutAPrefetchTheAmountScreenPreviewsEveryChainAsBefore() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let service = HeldPreviewService()
        let store = Self.bestChains(answer: nil)
        let (window, _) = try Self.window(service: service, bestChains: store)
        defer { TestWindow.close(window) }
        func helper() -> String? { Self.element("amount-entry-helper", in: window)?.accessibilityLabel }
        #expect(await Self.eventually(within: 10) { helper() == "The cabal can spend $500.00" })

        try await ProposeAmountKeypadTests.typeOne(in: window)

        #expect(await Self.eventually(within: 10) { helper() == "Finding the best price…" })
        while await !service.isAsked() { await Task.yield() }
        #expect(Self.element("propose-amount-quantity", in: window) == nil)
        #expect(await service.networks == [nil])
        await service.answer()
        #expect(await Self.eventually(within: 10) { helper() == "The cabal can spend $500.00" })
    }

    fileprivate static func bestChains(answer: MonacoCore.AssetBestChain?) -> MonacoCore.AssetBestChainStore {
        MonacoCore.AssetBestChainStore(
            now: { Date() },
            fetch: { _ in
                guard let answer else { throw CancellationError() }
                return answer
            })
    }

    fileprivate static var bestChain: MonacoCore.AssetBestChain? {
        let now = Date()
        let listing = "01928a6e-0000-7000-8000-000000000002"
        let quote = Components.Schemas.RouteQuote(
            listingId: listing, issuer: "xstocks", network: .base, venue: .uniswap, netShareNanos: "4000000",
            netMicros: "1000000", costs: [], quotedAt: now, expiresAt: now.addingTimeInterval(30))
        return MonacoCore.AssetBestChain(
            .init(
                symbol: "GOOGLx", amountMicros: "1000000", network: .base, venue: .uniswap, listingId: listing,
                netShareNanos: "4000000", quotedAt: now,
                priceComparison: .init(winnerListingId: listing, quotes: [quote], unavailable: [])))
    }
}
