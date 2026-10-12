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
    public let figure: String
    public let headline: String
    public let caption: String?
    public let venues: BestPrice?

    public init(_ bestPrice: BestPrice, caption: String? = nil) {
        figure = bestPrice.price ?? bestPrice.figure(bestPrice.winner)
        headline = bestPrice.headline
        self.caption = caption
        venues = bestPrice
    }

    init?(solanaQuote quoteOut: Int64, usdcMicros: Int64?, decimals: Int, caption: String) {
        guard let usdcMicros, quoteOut > 0, (0...18).contains(decimals) else { return nil }
        var unit: Int64 = 1
        for _ in 0..<decimals { unit *= 10 }
        let (priceMicros, _) = quoteOut.dividingFullWidth(usdcMicros.multipliedFullWidth(by: unit))
        figure = "\(UsdAmountFormatter.format(micros: priceMicros)) a share"
        headline = BestPrice.chainName(.solana)
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

    public func isAwaitingFirstQuote(for summary: ProposalSummary) -> Bool {
        Self.isLive(summary) && summary.network != nil && latest == nil
    }

    public func row(for summary: ProposalSummary, decimals: Int, now: Date) -> BestPriceRow? {
        guard Self.isLive(summary) else { return summary.bestPrice.map { BestPriceRow($0) } }
        guard unavailableReason(for: summary) == nil, !isAwaitingFirstQuote(for: summary) else { return nil }
        if let latest, now.timeIntervalSince(latest.quotedAt) <= Self.staleAfter {
            let caption = Self.age(since: latest.quotedAt, now: now)
            if let bestPrice = BestPrice(latest.comparison, isSell: false) {
                return BestPriceRow(bestPrice, caption: caption)
            }
            if let row = BestPriceRow(
                solanaQuote: latest.quoteOutAmount, usdcMicros: summary.usdcMicros, decimals: decimals,
                caption: caption)
            {
                return row
            }
        }
        return summary.bestPrice.map { BestPriceRow($0, caption: Self.whenProposed) }
    }

    static func age(since quotedAt: Date, now: Date) -> String {
        let seconds = now.timeIntervalSince(quotedAt)
        guard seconds >= justNow else { return "Updated just now" }
        return "Updated \(Int(seconds))s ago"
    }
}
