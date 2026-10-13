import Foundation
import MonacoAPI
import Observation

public struct LivePriceComparison: Equatable, Sendable {
    public let comparison: Components.Schemas.PriceComparison?
    public let quoteOutAmount: Int64
    public let quotedAt: Date

    public init(_ value: Components.Schemas.ProposalPriceComparison) {
        comparison = value.priceComparison
        quoteOutAmount = value.quoteOutAmount
        quotedAt = value.quotedAt
    }
}

public struct BestPriceRow: Equatable, Sendable {
    public let network: Components.Schemas.Network
    public let figure: String
    public let headline: String?
    public let caption: String?
    public let venues: BestPrice?

    public init(_ bestPrice: BestPrice, caption: String? = nil) {
        network = bestPrice.winner.network
        figure = bestPrice.price ?? bestPrice.figure(bestPrice.winner)
        headline = bestPrice.headline
        self.caption = caption
        venues = bestPrice
    }

    init?(_ bestPrice: BestPrice, pinned network: Components.Schemas.Network, caption: String?) {
        guard let venue = bestPrice.venue(on: network), let price = bestPrice.price(of: venue) else { return nil }
        self.network = network
        figure = "\(price) on \(BestPrice.chainName(network))"
        headline = nil
        self.caption = caption
        venues = nil
    }

    init?(solanaQuote quoteOut: Int64, usdcMicros: Int64?, decimals: Int, isPinned: Bool, caption: String) {
        guard let usdcMicros, quoteOut > 0, (0...18).contains(decimals) else { return nil }
        var unit: Int64 = 1
        for _ in 0..<decimals { unit *= 10 }
        let (priceMicros, _) = quoteOut.dividingFullWidth(usdcMicros.multipliedFullWidth(by: unit))
        let price = "\(UsdAmountFormatter.format(micros: priceMicros)) a share"
        let chain = BestPrice.chainName(.solana)
        network = .solana
        figure = isPinned ? "\(price) on \(chain)" : price
        headline = isPinned ? nil : chain
        self.caption = caption
        venues = nil
    }
}

@Observable
@MainActor
public final class LiveBestPriceModel {
    public static let interval: Duration = .seconds(15)
    public static let staleAfter: TimeInterval = 60
    static let justNow: TimeInterval = 5
    public static let whenProposed = "Price when proposed"

    public private(set) var latest: LivePriceComparison?
    private let fetch: @Sendable (String) async throws -> LivePriceComparison
    private let sleep: @Sendable (Duration) async throws -> Void
    private let gate = RefreshGate()
    @ObservationIgnored private var loop: Task<Void, Never>?
    @ObservationIgnored private var pollingCabalID: String?

    public init(
        sleep: @escaping @Sendable (Duration) async throws -> Void,
        fetch: @escaping @Sendable (_ cabalID: String) async throws -> LivePriceComparison
    ) {
        self.sleep = sleep
        self.fetch = fetch
    }

    public static func isLive(_ summary: ProposalSummary) -> Bool {
        summary.status == .open && summary.kind == "buy"
    }

    public var isPolling: Bool { loop != nil }

    public func poll(cabalID: String?) {
        guard cabalID != pollingCabalID else { return }
        loop?.cancel()
        loop = nil
        pollingCabalID = cabalID
        guard let cabalID else { return }
        loop = Task { [weak self, sleep] in
            await self?.tick(cabalID: cabalID)
            while (try? await sleep(Self.interval)) != nil {
                guard let self else { return }
                await self.tick(cabalID: cabalID)
            }
        }
    }

    public func refresh() async {
        guard let cabalID = pollingCabalID else { return }
        try? await gate.runNow { try await load(cabalID: cabalID) }
    }

    private func tick(cabalID: String) async {
        _ = try? await gate.run { try await load(cabalID: cabalID) }
    }

    private func load(cabalID: String) async throws {
        let fresh = try await fetch(cabalID)
        guard !Task.isCancelled, pollingCabalID == cabalID else { return }
        if let latest, fresh.quotedAt < latest.quotedAt { return }
        latest = fresh
    }

    public func unavailableReason(for summary: ProposalSummary) -> String? {
        guard Self.isLive(summary) else { return nil }
        return BestPrice.pinnedUnavailable(latest?.comparison, network: summary.network)
    }

    public func isFindingPrice(for summary: ProposalSummary, decimals: Int, now: Date) -> Bool {
        Self.isLive(summary) && summary.network != nil && unavailableReason(for: summary) == nil
            && liveRow(for: summary, decimals: decimals, now: now) == nil
    }

    public func row(for summary: ProposalSummary, decimals: Int, now: Date) -> BestPriceRow? {
        guard Self.isLive(summary) else { return Self.row(summary.bestPrice, pinned: summary.network, caption: nil) }
        guard unavailableReason(for: summary) == nil else { return nil }
        if let row = liveRow(for: summary, decimals: decimals, now: now) { return row }
        guard summary.network == nil else { return nil }
        return summary.bestPrice.map { BestPriceRow($0, caption: Self.whenProposed) }
    }

    private func liveRow(for summary: ProposalSummary, decimals: Int, now: Date) -> BestPriceRow? {
        guard let latest, now.timeIntervalSince(latest.quotedAt) <= Self.staleAfter else { return nil }
        let caption = Self.age(since: latest.quotedAt, now: now)
        if let bestPrice = BestPrice(latest.comparison, isSell: false) {
            return Self.row(bestPrice, pinned: summary.network, caption: caption)
        }
        guard summary.network == nil || summary.network == .solana else { return nil }
        return BestPriceRow(
            solanaQuote: latest.quoteOutAmount, usdcMicros: summary.usdcMicros, decimals: decimals,
            isPinned: summary.network != nil, caption: caption)
    }

    private static func row(
        _ bestPrice: BestPrice?, pinned network: Components.Schemas.Network?, caption: String?
    ) -> BestPriceRow? {
        guard let bestPrice else { return nil }
        guard let network else { return BestPriceRow(bestPrice, caption: caption) }
        return BestPriceRow(bestPrice, pinned: network, caption: caption)
    }

    static func age(since quotedAt: Date, now: Date) -> String {
        let seconds = now.timeIntervalSince(quotedAt)
        guard seconds >= justNow else { return "Updated just now" }
        return "Updated \(Int(seconds))s ago"
    }
}
